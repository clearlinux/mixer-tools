package main

import (
	"flag"
	"fmt"
	"io/ioutil"
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
	outputDir := flag.String("o", "", "output directory, creates temporary if not specified")
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
		chrootDir = filepath.Join(stateDir, "image", toVersion, "full")
		if _, err := os.Stat(chrootDir); err != nil {
			log.Fatalf("couldn't access the full chroot: %s", err)
		}
	}

	if *outputDir == "" {
		tempDir, err := ioutil.TempDir(".", "packs-")
		if err != nil {
			log.Fatalf("couldn't create output directory: %s", err)
		}
		*outputDir = tempDir
	}
	log.Printf("Output directory: %s", *outputDir)

	// If we are handling a single bundle, its name is taken directly from the command line.
	if !*allBundles {
		name := flag.Arg(3)
		if name == "full" || name == "MoM" || name == "" {
			log.Fatalf("invalid bundle name %q", name)
		}
		bundle := &bundleToPack{
			name: name,
			from: fromVersionUint,
			to:   toVersionUint,
		}
		pack(stateDir, chrootDir, *outputDir, bundle, *force)
		return
	}

	//
	// Collect the corresponding bundle versions. Note that the code below will create packs on
	// different directories, based on the version that each bundle is in the Manifest.MoM for
	// the specified toVersion.
	//
	toDir := filepath.Join(stateDir, "www", toVersion)
	toMoM, err := swupd.ParseManifestFile(filepath.Join(toDir, "Manifest.MoM"))
	if err != nil {
		log.Fatalf("couldn't read MoM of TO_VERSION (%s): %s", toVersion, err)
	}

	bundles := make(map[string]*bundleToPack, len(toMoM.Files))
	for _, f := range toMoM.Files {
		bundles[f.Name] = &bundleToPack{f.Name, 0, f.Version}
	}

	// If this is not a zero pack, we might be able to skip some bundles.
	if fromVersionUint > 0 {
		fromDir := filepath.Join(stateDir, "www", fromVersion)
		fromMoM, err := swupd.ParseManifestFile(filepath.Join(fromDir, "Manifest.MoM"))
		if err != nil {
			log.Fatalf("couldn't read MoM of FROM_VERSION (%s): %s", fromVersion, err)
		}

		for _, f := range fromMoM.Files {
			e, ok := bundles[f.Name]
			if !ok {
				// Bundle doesn't exist in new version, no pack needed.
				continue
			}
			if e.to == f.Version {
				// Versions match, so no pack required.
				delete(bundles, f.Name)
				continue
			}
			if e.to < f.Version {
				log.Fatalf("invalid bundle versions for bundle %s, check the MoMs", f.Name)
			}
			e.from = f.Version
		}
	}

	// TODO: Use goroutines.
	for _, b := range bundles {
		pack(stateDir, chrootDir, *outputDir, b, *force)
	}
}

type bundleToPack struct {
	name string
	from uint32
	to   uint32
}

func pack(stateDir, chrootDir string, outputDir string, bundle *bundleToPack, force bool) {
	toDir := filepath.Join(outputDir, "www", fmt.Sprint(bundle.to))
	err := os.MkdirAll(toDir, 0755)
	if err != nil {
		log.Fatal(err)
	}
	outputFilename := filepath.Join(toDir, fmt.Sprintf("pack-%s-from-%d.tar", bundle.name, bundle.from))

	// If we are not forcing, skip the packs already on disk.
	if !force {
		_, err = os.Stat(outputFilename)
		if err == nil {
			fmt.Printf("Pack already exists for %s from %d to %d\n", bundle.name, bundle.from, bundle.to)
			return
		}
		if !os.IsNotExist(err) {
			log.Fatal(err)
		}
	}

	fmt.Printf("Packing %s from %d to %d...\n", bundle.name, bundle.from, bundle.to)

	m, err := swupd.ParseManifestFile(filepath.Join(stateDir, "www", fmt.Sprint(bundle.to), "Manifest."+bundle.name))
	if err != nil {
		log.Fatal(err)
	}
	m.Name = bundle.name

	//
	// Create the file and write the pack to it.
	//
	output, err := os.Create(outputFilename)
	if err != nil {
		log.Fatal(err)
	}
	info, err := swupd.WritePack(output, m, bundle.from, filepath.Join(stateDir, "www"), chrootDir)
	if err != nil {
		_ = os.RemoveAll(outputFilename)
		log.Fatal(err)
	}
	err = output.Close()
	if err != nil {
		_ = os.RemoveAll(outputFilename)
		log.Fatal(err)
	}

	//
	// Output information about the pack produced.
	//
	if len(info.Warnings) > 0 {
		fmt.Println("Warnings during pack:")
		for _, w := range info.Warnings {
			fmt.Printf("  %s\n", w)
		}
		fmt.Println()
	}
	// TODO: Move this to the info structure itself?
	fullfiles := 0
	deltas := 0
	for _, e := range info.Entries {
		switch e.State {
		case swupd.PackedFullfile:
			fullfiles++
		case swupd.PackedDelta:
			deltas++
		}
	}
	fmt.Printf("  Fullfiles in pack: %d\n", fullfiles)
	fmt.Printf("  Deltas in pack: %d\n", deltas)
}

func parseUint32(s string) uint32 {
	parsed, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		log.Fatalf("error parsing value %q: %s", s, err)
	}
	return uint32(parsed)
}
