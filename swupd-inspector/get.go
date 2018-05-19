package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/clearlinux/mixer-tools/internal/client"
	"github.com/clearlinux/mixer-tools/swupd"
)

func runGet(cacheDir, url, arg string) {
	base, version := parseURL(url)
	stateDir := filepath.Join(cacheDir, convertContentBaseToDirname(base))
	state, err := client.NewState(stateDir, base)
	if err != nil {
		log.Fatalf("ERROR: %s", err)
	}

	mom, err := state.GetMoM(version)
	if err != nil {
		log.Fatalf("ERROR: %s", err)
	}

	switch {

	case arg == "Manifest.MoM", arg == "Manifest.full":
		path, err := state.GetFile(version, arg)
		if err != nil {
			log.Fatalf("ERROR: %s", err)
		}
		err = cp(path, arg)
		if err != nil {
			log.Fatalf("ERROR: %s", err)
		}

	case strings.HasPrefix(arg, "Manifest."):
		// Look at MoM first to find the version of the bundle.
		name := arg[9:]
		var found *swupd.File
		for _, f := range mom.Files {
			if f.Name == name {
				found = f
				break
			}
		}
		if found == nil {
			log.Fatalf("ERROR: Manifest.MoM for version %s doesn't have a bundle named %s", version, name)
		}
		path, err := state.GetFile(fmt.Sprint(found.Version), arg)
		if err != nil {
			log.Fatalf("ERROR: %s", err)
		}
		err = cp(path, arg)
		if err != nil {
			log.Fatalf("ERROR: %s", err)
		}

	case len(arg) > 0 && arg[0] == '/':
		var found *swupd.File
		err := visitAllFiles(state, mom, func(bundle, file *swupd.File) bool {
			if file.Name == arg {
				found = file
				return true
			}
			return false
		})
		if err != nil {
			log.Fatalf("ERROR: %s", err)
		}
		if found == nil {
			log.Fatalf("ERROR: file %s not found in version %s", arg, version)
		}
		log.Printf("%s => %s", arg, found.Hash)
		err = downloadFullfile(state, found)
		if err != nil {
			log.Fatalf("ERROR: %s", err)
		}

	case len(arg) == len(swupd.AllZeroHash):
		// TODO: Support shortened hashes.
		var found *swupd.File
		err := visitAllFiles(state, mom, func(bundle, file *swupd.File) bool {
			if file.Hash.String() == arg {
				found = file
				return true
			}
			return false
		})
		if err != nil {
			log.Fatalf("ERROR: %s", err)
		}
		if found == nil {
			log.Fatalf("ERROR: file with hash %s not found in version %s", arg, version)
		}
		err = downloadFullfile(state, found)
		if err != nil {
			log.Fatalf("ERROR: %s", err)
		}

	default:
		log.Fatalf("Second argument to 'get' must be an absolute path, a hash or be a Manifest name")
	}
}

func visitAllFiles(state *client.State, mom *swupd.Manifest, visitFunc func(bundle, file *swupd.File) bool) error {
	var stop bool
	for _, bundleF := range mom.Files {
		bundle, err := state.GetBundleManifest(fmt.Sprint(bundleF.Version), bundleF.Name, "")
		if err != nil {
			return err
		}
		for _, f := range bundle.Files {
			if visitFunc(bundleF, f) {
				stop = true
				break
			}
		}
		if stop {
			break
		}
	}
	return nil
}

func downloadFullfile(state *client.State, file *swupd.File) error {
	fullfile := file.Hash.String() + ".tar"
	f, err := state.GetFile(fmt.Sprint(file.Version), "files", fullfile)
	if err != nil {
		return err
	}
	return cp(f, fullfile)
}

func cp(src, dst string) error {
	srcF, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() {
		_ = srcF.Close()
	}()

	dstF, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		_ = dstF.Close()
	}()

	_, err = io.Copy(dstF, srcF)
	return err
}
