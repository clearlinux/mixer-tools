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

// TODO: Support reading text files.

func runCat(cacheDir, url, arg string) {
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
		err = copyFileToStdout(path)
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
		err = copyFileToStdout(path)
		if err != nil {
			log.Fatalf("ERROR: %s", err)
		}

	default:
		log.Fatalf("Second argument to 'cat' must be a Manifest name")
	}
}

func copyFileToStdout(src string) error {
	srcF, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() {
		_ = srcF.Close()
	}()
	_, err = io.Copy(os.Stdout, srcF)
	return err
}
