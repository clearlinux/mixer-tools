package main

// Creates the fullfiles for a specific version given a state
// directory. Used to test the logic in the library functions.

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"runtime/pprof"

	"github.com/clearlinux/mixer-tools/swupd"
)

func usage() {
	fmt.Fprintf(os.Stderr, `Usage: create-fullfiles [FLAGS] STATEDIR VERSION
Flags:
`)
	flag.PrintDefaults()
	os.Exit(2)
}

func main() {
	log.SetFlags(0)

	cpuProfile := flag.String("cpuprofile", "", "write CPU profile to a file")
	outputDir := flag.String("o", "", "output directory, creates temporary if not specified")
	flag.Usage = usage

	flag.Parse()
	if flag.NArg() != 2 {
		usage()
	}

	if *cpuProfile != "" {
		f, err := os.Create(*cpuProfile)
		if err != nil {
			log.Fatalf("couldn't create file for CPU profile: %s", err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	stateDir := flag.Arg(0)
	version := flag.Arg(1)

	chrootDir := filepath.Join(stateDir, "image", version, "full")
	if _, err := os.Stat(chrootDir); err != nil {
		log.Fatalf("couldn't access the full chroot: %s", err)
	}

	manifestFile := filepath.Join(stateDir, "www", version, "Manifest.full")
	if _, err := os.Stat(manifestFile); err != nil {
		log.Fatalf("couldn't access the full manifest: %s", err)
	}

	m, err := swupd.ParseManifestFile(manifestFile)
	if err != nil {
		log.Fatalf("couldn't read full manifest %s: %s", manifestFile, err)
	}

	if *outputDir == "" {
		tempDir, err := ioutil.TempDir(".", "fullfiles-")
		if err != nil {
			log.Fatalf("couldn't create output directory: %s", err)
		}
		*outputDir = tempDir
	} else {
		if _, err := os.Stat(*outputDir); err == nil {
			log.Fatalf("output dir already exists, exiting to not overwrite files")
		}
		err := os.MkdirAll(*outputDir, 0755)
		if err != nil {
			log.Fatalf("couldn't create output directory: %s", err)
		}
	}

	log.Printf("Output directory: %s", *outputDir)
	err = swupd.CreateFullfiles(m, chrootDir, *outputDir)
	if err != nil {
		log.Fatal(err)
	}
}
