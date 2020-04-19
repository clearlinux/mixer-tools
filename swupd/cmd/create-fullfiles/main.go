package main

// Creates the fullfiles for a specific version given a state
// directory. Used to test the logic in the library functions.

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime/pprof"

	"github.com/clearlinux/mixer-tools/log"
	"github.com/clearlinux/mixer-tools/swupd"
)

func usage() {
	log.Info(log.Mixer, `Usage: create-fullfiles [FLAGS] STATEDIR VERSION
Flags:
`)
	flag.PrintDefaults()
	os.Exit(2)
}

func main() {
	cpuProfile := flag.String("cpuprofile", "", "write CPU profile to a file")
	outputDir := flag.String("o", "", "output directory, creates temporary if not specified")
	logFile := flag.String("log", "", "write logs to a file")
	logLevel := flag.Int("log-level", 4, "set the log level between 1-5")
	flag.Usage = usage

	flag.Parse()
	if flag.NArg() != 2 {
		usage()
	}

	if *cpuProfile != "" {
		f, err := os.Create(*cpuProfile)
		if err != nil {
			log.Error(log.Mixer, "couldn't create file for CPU profile: %s", err)
			os.Exit(1)
		}
		_ = pprof.StartCPUProfile(f)
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
	version := flag.Arg(1)

	chrootDir := filepath.Join(stateDir, "image", version, "full")
	if _, err := os.Stat(chrootDir); err != nil {
		log.Error(log.Mixer, "couldn't access the full chroot: %s", err)
		os.Exit(1)
	}

	manifestFile := filepath.Join(stateDir, "www", version, "Manifest.full")
	if _, err := os.Stat(manifestFile); err != nil {
		log.Error(log.Mixer, "couldn't access the full manifest: %s", err)
		os.Exit(1)
	}

	m, err := swupd.ParseManifestFile(manifestFile)
	if err != nil {
		log.Error(log.Mixer, "couldn't read full manifest %s: %s", manifestFile, err)
		os.Exit(1)
	}

	if *outputDir == "" {
		tempDir, terr := ioutil.TempDir(".", "fullfiles-")
		if terr != nil {
			log.Error(log.Mixer, "couldn't create output directory: %s", terr)
			os.Exit(1)
		}
		*outputDir = tempDir
	} else {
		if _, err = os.Stat(*outputDir); err == nil {
			log.Error(log.Mixer, "output dir already exists, exiting to not overwrite files")
			os.Exit(1)
		}
		err = os.MkdirAll(*outputDir, 0755)
		if err != nil {
			log.Error(log.Mixer, "couldn't create output directory: %s", err)
			os.Exit(1)
		}
	}

	log.Info(log.Mixer, "Output directory: %s", *outputDir)
	_, err = swupd.CreateFullfiles(m, chrootDir, *outputDir, 0, []string{"external-xz"})
	if err != nil {
		log.Error(log.Mixer, err.Error())
		os.Exit(1)
	}
}
