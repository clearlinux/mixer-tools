package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime/pprof"
	"strconv"

	"github.com/clearlinux/mixer-tools/swupd"
)

func usage() {
	fmt.Fprintf(os.Stderr, `Create pack files using a swupd state directory

Usage:
  create-pack [FLAGS] STATEDIR FROM_VERSION TO_VERSION BUNDLE

  create-pack -all [FLAGS] STATEDIR FROM_VERSION TO_VERSION

Flags:
`)
	flag.PrintDefaults()
	os.Exit(2)
}

func main() {
	log.SetFlags(0)

	cpuProfile := flag.String("cpuprofile", "", "write CPU profile to a file")
	useChroot := flag.Bool("chroot", false, "use chroot to speed up pack generation")
	allBundles := flag.Bool("all", false, "create packs for all bundles new in TO version")
	force := flag.Bool("f", false, "rewrite packs that already exist")
	flag.Usage = usage

	flag.Parse()
	if (*allBundles && flag.NArg() != 3) || (!*allBundles && flag.NArg() != 4) {
		usage()
	}

	if *cpuProfile != "" {
		f, err := os.Create(*cpuProfile)
		if err != nil {
			log.Fatalf("couldn't create file for CPU profile: %s", err)
		}
		err = pprof.StartCPUProfile(f)
		if err != nil {
			log.Fatal(err)
		}
		defer pprof.StopCPUProfile()
	}

	stateDir := flag.Arg(0)

	fromVersion := flag.Arg(1)
	toVersion := flag.Arg(2)

	fromVersionUint := parseUint32(fromVersion)
	toVersionUint := parseUint32(toVersion)

	if fromVersionUint >= toVersionUint {
		log.Fatalf("couldn't create pack: FROM_VERSION (%d) must be smaller than TO_VERSION (%d)", fromVersionUint, toVersionUint)
	}

	chrootDir := ""
	if *useChroot {
		chrootDir = filepath.Join(stateDir, "image")
		if _, err := os.Stat(chrootDir); err != nil {
			log.Fatalf("couldn't access the full chroot: %s", err)
		}
	}

	var bundles map[string]*swupd.BundleToPack

	if *allBundles {
		toDir := filepath.Join(stateDir, "www", toVersion)
		toMoM, err := swupd.ParseManifestFile(filepath.Join(toDir, "Manifest.MoM"))
		if err != nil {
			log.Fatalf("couldn't read MoM of TO_VERSION (%s): %s", toVersion, err)
		}

		var fromMoM *swupd.Manifest
		if fromVersionUint > 0 {
			fromDir := filepath.Join(stateDir, "www", fromVersion)
			fromMoM, err = swupd.ParseManifestFile(filepath.Join(fromDir, "Manifest.MoM"))
			if err != nil {
				log.Fatalf("couldn't read MoM of FROM_VERSION (%s): %s", fromVersion, err)
			}
		}

		bundles, err = swupd.FindBundlesToPack(fromMoM, toMoM)
		if err != nil {
			log.Fatalf("couldn't find the bundles to pack: %s", err)
		}
	} else {
		// If we are handling a single bundle, its name is taken directly from the command line.
		name := flag.Arg(3)
		if name == "full" || name == "MoM" || name == "" {
			log.Fatalf("invalid bundle name %q", name)
		}
		bundle := &swupd.BundleToPack{
			Name:        name,
			FromVersion: fromVersionUint,
			ToVersion:   toVersionUint,
		}
		bundles["name"] = bundle
	}

	// TODO: Use goroutines.
	for _, b := range bundles {
		// Unless we are forcing, skip the packs already on disk.
		if !*force {
			_, err := os.Lstat(filepath.Join(stateDir, "www", fmt.Sprint(b.ToVersion), swupd.GetPackFilename(b.Name, b.FromVersion)))
			if err == nil {
				fmt.Printf("Pack already exists for %s from %d to %d\n", b.Name, b.FromVersion, b.ToVersion)
				continue
			}
			if !os.IsNotExist(err) {
				log.Fatal(err)
			}
		}

		fmt.Printf("Packing %s from %d to %d...\n", b.Name, b.FromVersion, b.ToVersion)

		info, err := swupd.CreatePack(b.Name, b.FromVersion, b.ToVersion, filepath.Join(stateDir, "www"), chrootDir, 0)
		if err != nil {
			log.Fatal(err)
		}

		if len(info.Warnings) > 0 {
			fmt.Println("Warnings during pack:")
			for _, w := range info.Warnings {
				fmt.Printf("  %s\n", w)
			}
			fmt.Println()
		}
		fmt.Printf("  Fullfiles in pack: %d\n", info.FullfileCount)
		fmt.Printf("  Deltas in pack: %d\n", info.DeltaCount)
	}
}

func parseUint32(s string) uint32 {
	parsed, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		log.Fatalf("error parsing value %q: %s", s, err)
	}
	return uint32(parsed)
}
