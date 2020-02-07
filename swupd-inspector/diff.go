package main

import (
	"fmt"
	"log"
	"path/filepath"
	"sort"

	"github.com/clearlinux/mixer-tools/internal/client"
	"github.com/clearlinux/mixer-tools/swupd"
)

// TODO: MoM signature verification.

// Terminal colors.
var (
	RED   = "\x1b[31m"
	GREEN = "\x1b[32m"
	RESET = "\x1b[0m"
)

type diffFlags struct {
	noColor bool
	strict  bool
}

func runDiff(cacheDir string, flags *diffFlags, urlA, urlB string) {
	if flags.noColor {
		RED = ""
		GREEN = ""
		RESET = ""
	}

	baseA, versionA := parseURL(urlA)
	baseB, versionB := parseURL(urlB)

	var stateA, stateB *client.State
	stateDirA := filepath.Join(cacheDir, convertContentBaseToDirname(baseA))
	stateA, err := client.NewState(stateDirA, baseA)
	if err != nil {
		log.Fatalf("ERROR: %s", err)
	}

	stateDirB := filepath.Join(cacheDir, convertContentBaseToDirname(baseB))
	if stateDirB == stateDirA {
		if baseB == baseA {
			stateB = stateA
		} else {
			// This should be a rare case, if we hit this we should improve
			// our normalization function.
			stateDirB = stateDirB + "_other"
			stateB, err = client.NewState(stateDirB, baseB)
			if err != nil {
				log.Fatalf("ERROR: %s", err)
			}
		}
	} else {
		stateB, err = client.NewState(stateDirB, baseB)
		if err != nil {
			log.Fatalf("ERROR: %s", err)
		}
	}

	momA, err := stateA.GetMoM(versionA)
	if err != nil {
		log.Fatalf("ERROR: %s", err)
	}
	momB, err := stateB.GetMoM(versionB)
	if err != nil {
		log.Fatalf("ERROR: %s", err)
	}

	fmt.Printf(`=== Differences from A to B

  A Base:            %s
  A Version:         %s
  A State directory: %s

  B Base:            %s
  B Version:         %s
  B State directory: %s

`, baseA, versionA, stateDirA, baseB, versionB, stateDirB)

	sortFiles(momA)
	sortFiles(momB)

	fmt.Println("=== Manifest.MoM")

	type bundlePair struct {
		Name string
		A, B *swupd.File
	}
	var bundles []*bundlePair

	// TODO: Check all them are manifests...

	walkFiles(momA.Files, momB.Files, func(a, b *swupd.File) {
		switch {
		case a == nil:
			fmt.Printf("%s+%s%s %s%s\n", GREEN, b.Type, b.Status, b.Name, RESET)
		case b == nil:
			fmt.Printf("%s-%s%s %s%s\n", RED, a.Type, a.Status, a.Name, RESET)
		default:
			if a.Type != b.Type || a.Status != b.Status {
				fmt.Printf("%s-%s%s %s%s\n", RED, a.Type, a.Status, a.Name, RESET)
				fmt.Printf("%s+%s%s %s%s\n", RED, a.Type, a.Status, a.Name, RESET)
			} else {
				fmt.Printf(" %s%s %s", a.Type, a.Status, a.Name)
				if flags.strict && a.Version != b.Version {
					fmt.Printf(" (VERSION: %s-%d%s / %s+%d%s)", RED, a.Version, RESET, GREEN, b.Version, RESET)
				}
				if a.Hash != b.Hash {
					pair := &bundlePair{
						Name: a.Name,
						A:    a,
						B:    b,
					}
					bundles = append(bundles, pair)
					fmt.Printf(" (HASH: %s-%s%s / %s+%s%s)", RED, a.Hash.String()[:7], RESET, GREEN, b.Hash.String()[:7], RESET)
				}
				fmt.Println()
			}
		}
	})
	fmt.Println()

	flagString := func(f *swupd.File) string {
		result, err := f.GetFlagString()
		if err != nil {
			result = "...."
		}
		return result[:3]
	}

	for _, pair := range bundles {
		mA, err := stateA.GetBundleManifest(fmt.Sprint(pair.A.Version), pair.Name, "")
		if err != nil {
			log.Fatalf("ERROR: %s", err)
		}

		mB, err := stateB.GetBundleManifest(fmt.Sprint(pair.B.Version), pair.Name, "")
		if err != nil {
			log.Fatalf("ERROR: %s", err)
		}

		fmt.Printf("=== Manifest.%s A=%d B=%d\n", pair.Name, mA.Header.Version, mB.Header.Version)

		var extraIncludesA, extraIncludesB []string
		{
			includesA := map[string]bool{}
			for _, inc := range mA.Header.Includes {
				includesA[inc.Name] = true
			}
			includesB := map[string]bool{}
			for _, inc := range mB.Header.Includes {
				includesB[inc.Name] = true
			}
			for name := range includesA {
				if !includesB[name] {
					extraIncludesA = append(extraIncludesA, name)
				}
			}
			for name := range includesB {
				if !includesA[name] {
					extraIncludesB = append(extraIncludesB, name)
				}
			}
		}

		if mA.Header.FileCount != mB.Header.FileCount {
			fmt.Printf("%s-filecount: %d%s\n", RED, mA.Header.FileCount, RESET)
			fmt.Printf("%s+filecount: %d%s\n", GREEN, mB.Header.FileCount, RESET)
		}
		// TODO: Compare ContentSize?

		for _, name := range extraIncludesA {
			fmt.Printf("%s-includes: %s%s\n", RED, name, RESET)
		}
		for _, name := range extraIncludesB {
			fmt.Printf("%s+includes: %s%s\n", GREEN, name, RESET)
		}

		// TODO: Move sort files to walker?
		sortFiles(mA)
		sortFiles(mB)
		walkFiles(mA.Files, mB.Files, func(a, b *swupd.File) {
			switch {
			case a == nil:
				fmt.Printf("%s+%s %s%s\n", GREEN, flagString(b), b.Name, RESET)
			case b == nil:
				fmt.Printf("%s-%s %s%s\n", RED, flagString(a), a.Name, RESET)
			default:
				if flagString(a) != flagString(b) {
					fmt.Printf("%s-%s %s%s\n", RED, flagString(a), a.Name, RESET)
					fmt.Printf("%s+%s %s%s\n", GREEN, flagString(b), b.Name, RESET)
				} else if a.Misc != b.Misc || a.Hash != b.Hash || (flags.strict && a.Version != b.Version) {
					fmt.Printf(" %s %s", flagString(a), a.Name)
					if flags.strict && a.Version != b.Version {
						fmt.Printf(" (VERSION: %s-%d%s / %s+%d%s)", RED, a.Version, RESET, GREEN, b.Version, RESET)
					}
					if a.Misc != b.Misc {
						fmt.Printf(" (RENAME: %s-%d%s / %s+%d%s)", RED, a.Misc, RESET, GREEN, b.Misc, RESET)
					}
					if a.Hash != b.Hash {
						fmt.Printf(" (HASH: %s-%s%s / %s+%s%s)", RED, a.Hash.String()[:7], RESET, GREEN, b.Hash.String()[:7], RESET)
					}
					fmt.Println()
				}
			}
		})
		fmt.Println()
	}

	// TODO: Print number of mismatches, new files in B and missing files in A.
}

func walkFiles(filesA, filesB []*swupd.File, fn func(a, b *swupd.File)) {
	indexA := 0
	indexB := 0
	for indexA < len(filesA) && indexB < len(filesB) {
		fileA := filesA[indexA]
		fileB := filesB[indexB]
		switch {
		case fileA.Name < fileB.Name:
			indexA++
			fn(fileA, nil)
		case fileA.Name > fileB.Name:
			indexB++
			fn(nil, fileB)
		case fileA.Name == fileB.Name:
			indexA++
			indexB++
			fn(fileA, fileB)
		}
	}
	for indexA < len(filesA) {
		fileA := filesA[indexA]
		indexA++
		fn(fileA, nil)
	}
	for indexB < len(filesB) {
		fileB := filesB[indexB]
		indexB++
		fn(nil, fileB)
	}
}

func sortFiles(m *swupd.Manifest) {
	sort.Slice(m.Files, func(i, j int) bool {
		return m.Files[i].Name < m.Files[j].Name
	})
}
