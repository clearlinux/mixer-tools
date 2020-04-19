package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime/pprof"
	"strconv"

	"github.com/clearlinux/mixer-tools/log"
	"github.com/clearlinux/mixer-tools/swupd"
)

func usage() {
	log.Info(log.Mixer, `Create pack files using a swupd state directory

Usage:
  create-pack [FLAGS] STATEDIR FROM_VERSION TO_VERSION BUNDLE

  create-pack -all [FLAGS] STATEDIR FROM_VERSION TO_VERSION

Flags:
`)
	flag.PrintDefaults()
	os.Exit(2)
}

func main() {
	cpuProfile := flag.String("cpuprofile", "", "write CPU profile to a file")
	useChroot := flag.Bool("chroot", false, "use chroot to speed up pack generation")
	allBundles := flag.Bool("all", false, "create packs for all bundles new in TO version")
	force := flag.Bool("f", false, "rewrite packs that already exist")
	logFile := flag.String("log", "", "write logs to a file")
	logLevel := flag.Int("log-level", 4, "set the log level between 1-5")
	flag.Usage = usage

	flag.Parse()
	if (*allBundles && flag.NArg() != 3) || (!*allBundles && flag.NArg() != 4) {
		usage()
	}

	if *cpuProfile != "" {
		f, err := os.Create(*cpuProfile)
		if err != nil {
			log.Error(log.Mixer, "couldn't create file for CPU profile: %s", err)
			os.Exit(1)
		}
		err = pprof.StartCPUProfile(f)
		if err != nil {
			log.Error(log.Mixer, err.Error())
			os.Exit(1)
		}
		defer pprof.StopCPUProfile()
	}
	if *logFile != "" {
		// Configure logger
		_, err := log.SetOutputFilename(*logFile)
		if err != nil {
			fmt.Printf("WARNING: couldn't create file for log: %s", err)
		} else {
			log.SetLogLevel(*logLevel)
		}
		defer log.CloseLogHandler()
	}
	stateDir := flag.Arg(0)

	fromVersion := flag.Arg(1)
	toVersion := flag.Arg(2)

	fromVersionUint := parseUint32(fromVersion)
	toVersionUint := parseUint32(toVersion)

	if fromVersionUint >= toVersionUint {
		log.Error(log.Mixer, "couldn't create pack: FROM_VERSION (%d) must be smaller than TO_VERSION (%d)", fromVersionUint, toVersionUint)
		os.Exit(1)
	}

	chrootDir := ""
	if *useChroot {
		chrootDir = filepath.Join(stateDir, "image")
		if _, err := os.Stat(chrootDir); err != nil {
			log.Error(log.Mixer, "couldn't access the full chroot: %s", err)
			os.Exit(1)
		}
	}

	bundles := make(map[string]*swupd.BundleToPack)

	if *allBundles {
		toDir := filepath.Join(stateDir, "www", toVersion)
		toMoM, err := swupd.ParseManifestFile(filepath.Join(toDir, "Manifest.MoM"))
		if err != nil {
			log.Error(log.Mixer, "couldn't read MoM of TO_VERSION (%s): %s", toVersion, err)
			os.Exit(1)
		}

		var fromMoM *swupd.Manifest
		if fromVersionUint > 0 {
			fromDir := filepath.Join(stateDir, "www", fromVersion)
			fromMoM, err = swupd.ParseManifestFile(filepath.Join(fromDir, "Manifest.MoM"))
			if err != nil {
				log.Error(log.Mixer, "couldn't read MoM of FROM_VERSION (%s): %s", fromVersion, err)
				os.Exit(1)
			}
		}

		bundles, err = swupd.FindBundlesToPack(fromMoM, toMoM)
		if err != nil {
			log.Error(log.Mixer, "couldn't find the bundles to pack: %s", err)
			os.Exit(1)
		}
	} else {
		// If we are handling a single bundle, its name is taken directly from the command line.
		name := flag.Arg(3)
		if name == "full" || name == "MoM" || name == "" {
			log.Error(log.Mixer, "invalid bundle name %q", name)
			os.Exit(1)
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
				log.Info(log.Mixer, "Pack already exists for %s from %d to %d", b.Name, b.FromVersion, b.ToVersion)
				continue
			}
			if !os.IsNotExist(err) {
				log.Error(log.Mixer, err.Error())
				os.Exit(1)
			}
		}

		log.Info(log.Mixer, "Packing %s from %d to %d...", b.Name, b.FromVersion, b.ToVersion)

		info, err := swupd.CreatePack(b.Name, b.FromVersion, b.ToVersion, filepath.Join(stateDir, "www"), chrootDir)
		if err != nil {
			log.Error(log.Mixer, err.Error())
			os.Exit(1)
		}

		if len(info.Warnings) > 0 {
			log.Warning(log.Mixer, "Warnings during pack:")
			for _, w := range info.Warnings {
				log.Info(log.Mixer, "  %s", w)
			}
		}
		log.Info(log.Mixer, "  Fullfiles in pack: %d", info.FullfileCount)
		log.Info(log.Mixer, "  Deltas in pack: %d", info.DeltaCount)
	}
}

func parseUint32(s string) uint32 {
	parsed, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		log.Error(log.Mixer, "error parsing value %q: %s", s, err)
		os.Exit(1)
	}
	return uint32(parsed)
}
