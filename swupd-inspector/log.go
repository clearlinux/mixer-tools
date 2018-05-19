package main

import (
	"fmt"
	"log"
	"path/filepath"

	"github.com/clearlinux/mixer-tools/internal/client"
	"github.com/clearlinux/mixer-tools/swupd"
)

// TODO: Consider a --unique flag that would not print consecutive versions that have the same hash.

func runLog(cacheDir, url, filename string) {
	base, version := parseURL(url)
	stateDir := filepath.Join(cacheDir, convertContentBaseToDirname(base))
	state, err := client.NewState(stateDir, base)
	if err != nil {
		log.Fatalf("ERROR: %s", err)
	}

	if filename == "" || filename[0] != '/' {
		// TODO: Support Manifest.* files too, but showing the diff of the files?
		log.Fatalf("Second argument to 'log' must be an absolute path")
	}

	var lastVersion uint32
	lastBundle := ""

	for version != "0" {
		mom, err := state.GetMoM(version)
		if err != nil {
			// TODO: check for the "aborted" file in the version directory?
			log.Fatalf("ERROR: %s", err)
		}

		var found *swupd.File

		visit := func(bundle, file *swupd.File) bool {
			if file.Name == filename && file.Present() {
				found = file
				lastBundle = bundle.Name
				return true
			}
			return false
		}

		// Look first in the bundle that had the file before, since it is more likely to
		// have the file again. This reduces the amount of manifest files downloaded.
		if lastBundle != "" {
			var bundleF *swupd.File
			for _, b := range mom.Files {
				if b.Name == lastBundle {
					bundleF = b
				}
			}

			if bundleF != nil {
				err = visitFilesInBundle(state, bundleF, visit)
				if err != nil {
					log.Fatalf("ERROR: %s", err)
				}
			}
		}

		if found == nil {
			err = visitAllFiles(state, mom, visit)
			if err != nil {
				log.Fatalf("ERROR: %s", err)
			}
		}

		if found == nil {
			break
		}
		if lastVersion != found.Version {
			fmt.Printf("%s/%d/files/%s.tar\n", base, found.Version, found.Hash)
			lastVersion = found.Version
		}

		// Look at the immediate previous OS version, unless we already know the file is
		// from an earlier version, so we can skip to it.
		v := mom.Header.Previous
		if v > found.Version {
			v = found.Version
		}
		version = fmt.Sprint(v)
	}

	if lastBundle == "" {
		log.Fatalf("ERROR: file %s not found in version %s", filename, version)
	}

}

func visitFilesInBundle(state *client.State, bundleFile *swupd.File, visitFunc func(bundle, file *swupd.File) bool) error {
	bundle, err := state.GetBundleManifest(fmt.Sprint(bundleFile.Version), bundleFile.Name, "")
	if err != nil {
		return err
	}
	for _, f := range bundle.Files {
		if visitFunc(bundleFile, f) {
			break
		}
	}
	return nil
}
