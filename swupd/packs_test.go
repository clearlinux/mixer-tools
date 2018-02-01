package swupd

import (
	"fmt"
	"io"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestFindBundlesToPack(t *testing.T) {
	type M map[string]uint32
	tests := []struct {
		Name       string
		FromV      uint32
		From       M
		ToV        uint32
		To         M
		Expected   []BundleToPack
		ShouldFail bool
	}{
		{
			Name: "simple case",

			FromV: 10,
			From:  M{"os-core": 10},

			ToV: 20,
			To:  M{"os-core": 20},

			Expected: []BundleToPack{{"os-core", 10, 20}},
		},

		{
			Name: "from 0 keeps all bundles",

			ToV: 10,
			To:  M{"os-core": 10, "editors": 10},

			Expected: []BundleToPack{{"os-core", 0, 10}, {"editors", 0, 10}},
		},

		{
			Name: "new manifest has bundle not present in the old",

			FromV: 10,
			From:  M{"os-core": 10},

			ToV: 20,
			To:  M{"os-core": 20, "manuals": 20},

			Expected: []BundleToPack{{"os-core", 10, 20}, {"manuals", 0, 20}},
		},

		{
			Name: "new manifest has bundles from old",

			FromV: 10,
			From:  M{"os-core": 10, "editors": 10, "c-basic": 10, "R-basic": 10},

			ToV: 20,
			To:  M{"os-core": 20, "editors": 10, "c-basic": 20, "R-basic": 10},

			Expected: []BundleToPack{{"os-core", 10, 20}, {"c-basic", 10, 20}},
		},

		{
			Name: "both using an older bundle",

			FromV: 100,
			From:  M{"os-core": 100, "editors": 20},

			ToV: 200,
			To:  M{"os-core": 200, "editors": 20},

			Expected: []BundleToPack{{"os-core", 100, 200}},
		},

		{
			Name: "bundles with versions that dont match the manifest",

			FromV: 100,
			From:  M{"os-core": 100, "c-basic": 80},

			ToV: 200,
			To:  M{"os-core": 200, "c-basic": 150},

			Expected: []BundleToPack{{"os-core", 100, 200}, {"c-basic", 80, 150}},
		},

		{
			Name: "old bundle not around anymore",

			FromV: 100,
			From:  M{"os-core": 100, "b-basic": 100},

			ToV: 200,
			To:  M{"os-core": 200, "c-basic": 200},

			Expected: []BundleToPack{{"os-core", 100, 200}, {"c-basic", 0, 200}},
		},
	}

	addBundle := func(m *Manifest, name string, version uint32) {
		bundle := &File{
			Name:    name,
			Type:    TypeManifest,
			Version: version,
		}
		m.Files = append(m.Files, bundle)
	}

	sortBundles := func(bundles []BundleToPack) {
		sort.Slice(bundles, func(i, j int) bool {
			return bundles[i].Name < bundles[j].Name
		})
	}

	printBundles := func(bundles []BundleToPack) {
		for _, b := range bundles {
			fmt.Printf("  %s %d -> %d\n", b.Name, b.FromVersion, b.ToVersion)
		}
		fmt.Println()
	}

	for _, tt := range tests {
		var fromM *Manifest
		if tt.FromV != 0 {
			fromM = &Manifest{}
			fromM.Header.Version = tt.FromV
			for name, v := range tt.From {
				addBundle(fromM, name, v)
			}
		}
		toM := &Manifest{}
		toM.Header.Version = tt.ToV
		for name, v := range tt.To {
			addBundle(toM, name, v)
		}

		bundleMap, err := FindBundlesToPack(fromM, toM)
		failed := err != nil

		if failed != tt.ShouldFail {
			if tt.ShouldFail {
				t.Fatalf("unexpectedly succeeded when calculating bundles to pack in case %q", tt.Name)
			} else {
				t.Fatalf("failed to calculate bundles to pack in case %q: %s", tt.Name, err)
			}
			continue
		}
		if tt.ShouldFail {
			continue
		}

		bundles := make([]BundleToPack, 0, len(bundleMap))
		for _, b := range bundleMap {
			bundles = append(bundles, *b)
		}

		sortBundles(bundles)
		sortBundles(tt.Expected)

		if !reflect.DeepEqual(bundles, tt.Expected) {
			fmt.Printf("== CASE: %s\n", tt.Name)
			fmt.Printf("ACTUAL OUTPUT (%d bundles):\n", len(bundles))
			printBundles(bundles)

			fmt.Printf("EXPECTED OUTPUT (%d bundles):\n", len(tt.Expected))
			printBundles(tt.Expected)

			t.Fatalf("mismatch between returned bundles to pack and expected bundles to pack in case %q", tt.Name)
			continue
		}
	}
}

func TestCreatePackZeroPacks(t *testing.T) {
	fs := newTestFileSystem(t, "create-pack-")
	defer fs.cleanup()

	const (
		format = 1
		minVer = 0

		// Used when counting fullfiles.
		emptyDirAndEmptyFile = 2
	)

	//
	// In version 10, create two bundles and pass the chrootDir to pack creation.
	//
	mustInitStandardTest(t, fs.Dir, "0", "10", []string{"editors", "shells"})

	fs.write("image/10/editors/emacs", "emacs contents")
	fs.write("image/10/editors/joe", "joe contents")
	fs.write("image/10/editors/vim", "vim contents")

	fs.write("image/10/shells/bash", "bash contents")
	fs.write("image/10/shells/csh", "csh contents")
	fs.write("image/10/shells/fish", "fish contents")
	fs.write("image/10/shells/zsh", "zsh contents")

	mom10 := mustCreateManifests(t, 10, minVer, format, fs.Dir)

	// Let's create zero packs for version 10.
	info := mustCreatePack(t, "editors", 0, 10, fs.path("www"), fs.path("image"))
	mustHaveNoWarnings(t, info)
	mustHaveFullfileCount(t, info, 3+emptyDirAndEmptyFile)
	mustHaveDeltaCount(t, info, 0)
	mustValidateZeroPack(t, fs.path("www/10/Manifest.editors"), fs.path("www/10/pack-editors-from-0.tar"))

	info = mustCreatePack(t, "shells", 0, 10, fs.path("www"), fs.path("image"))
	mustHaveNoWarnings(t, info)
	mustHaveFullfileCount(t, info, 4+emptyDirAndEmptyFile)
	mustHaveDeltaCount(t, info, 0)
	mustValidateZeroPack(t, fs.path("www/10/Manifest.shells"), fs.path("www/10/pack-shells-from-0.tar"))

	//
	// In version 20, packs will use the fullfiles. Also check if errors happen when
	// the fullfiles are missing.
	//
	mustInitStandardTest(t, fs.Dir, "10", "20", []string{"editors", "shells"})
	fs.cp("image/10/editors", "image/20")
	fs.cp("image/10/shells", "image/20")
	fs.rm("image/20/editors/vim")
	fs.rm("image/20/editors/emacs")
	fs.write("image/20/shells/ksh", "ksh contents")
	mom20 := mustCreateManifests(t, 20, minVer, format, fs.Dir)

	// Expect failure when creating packs without the fullfiles.
	_, err := CreatePack("editors", 0, 20, fs.path("www"), "")
	if err == nil {
		t.Fatalf("unexpected success creating pack without chrootDir nor fullfiles available")
	}
	mustCreateFullfiles(t, mom10.FullManifest, fs.path("image/10/full"), fs.path("www/10/files"))

	// All the files in bundle editors are available, so it will pass work.
	info = mustCreatePack(t, "editors", 0, 20, fs.path("www"), "")
	mustHaveNoWarnings(t, info)
	mustHaveFullfileCount(t, info, 1+emptyDirAndEmptyFile)
	mustHaveDeltaCount(t, info, 0)
	mustValidateZeroPack(t, fs.path("www/20/Manifest.editors"), fs.path("www/20/pack-editors-from-0.tar"))

	// Expect failure when creating packs for bundle shells, it won't find the new
	// shell added in version 20.
	_, err = CreatePack("shells", 0, 20, fs.path("www"), "")
	if err == nil {
		t.Fatalf("unexpected success creating pack without all fullfiles available")
	}
	mustCreateFullfiles(t, mom20.FullManifest, fs.path("image/20/full"), fs.path("www/20/files"))

	// Now we have all fullfiles for both versions.
	info = mustCreatePack(t, "shells", 0, 20, fs.path("www"), "")
	mustHaveNoWarnings(t, info)
	mustHaveFullfileCount(t, info, 5+emptyDirAndEmptyFile)
	mustHaveDeltaCount(t, info, 0)
	mustValidateZeroPack(t, fs.path("www/20/Manifest.shells"), fs.path("www/20/pack-shells-from-0.tar"))
}

func TestCreatePackWithDelta(t *testing.T) {
	fs := newTestFileSystem(t, "create-pack-")
	defer fs.cleanup()

	const (
		format = 1
		minVer = 0
	)

	//
	// In version 10, create a bundle with files of different sizes.
	//
	emptyContents := ""
	smallContents := "small"
	largeContents := strings.Repeat("large", 1000)
	if len(emptyContents) >= minimumSizeToMakeDeltaInBytes || len(smallContents) >= minimumSizeToMakeDeltaInBytes || len(largeContents) < minimumSizeToMakeDeltaInBytes {
		t.Fatal("test contents sizes are invalid")
	}

	mustInitStandardTest(t, fs.Dir, "0", "10", []string{"contents"})
	fs.write("image/10/contents/small1", emptyContents)
	fs.write("image/10/contents/small2", smallContents)
	fs.write("image/10/contents/large1", largeContents)
	fs.write("image/10/contents/large2", largeContents)
	mustCreateManifests(t, 10, minVer, format, fs.Dir)

	//
	// In version 20, swap the content of small files, and modify the large files
	// changing one byte or all bytes.
	//
	mustInitStandardTest(t, fs.Dir, "10", "20", []string{"contents"})
	fs.write("image/20/contents/small1", smallContents)
	fs.write("image/20/contents/small2", smallContents)
	fs.write("image/20/contents/large1", strings.ToUpper(largeContents[:1])+largeContents[1:])
	fs.write("image/20/contents/large2", largeContents[:1]+strings.ToUpper(largeContents[1:]))
	mustCreateManifests(t, 20, minVer, format, fs.Dir)

	info := mustCreatePack(t, "contents", 10, 20, fs.path("www"), fs.path("image"))
	mustHaveDeltaCount(t, info, 2)

	//
	// In version 30, make a change to one large files from 20.
	//
	mustInitStandardTest(t, fs.Dir, "20", "30", []string{"contents"})
	fs.cp("image/20/contents", "image/30")
	fs.write("image/30/contents/large1", strings.ToUpper(largeContents[:2])+largeContents[2:])
	mustCreateManifests(t, 30, minVer, format, fs.Dir)

	// Pack between 20 and 30 has only a delta for large1.
	info = mustCreatePack(t, "contents", 20, 30, fs.path("www"), fs.path("image"))
	mustHaveDeltaCount(t, info, 1)

	// Pack between 10 and 30 has both deltas.
	info = mustCreatePack(t, "contents", 10, 30, fs.path("www"), fs.path("image"))
	mustHaveDeltaCount(t, info, 2)
}

func TestCreatePackWithIncompleteChrootDir(t *testing.T) {
	fs := newTestFileSystem(t, "create-pack-")
	defer fs.cleanup()

	mustInitStandardTest(t, fs.Dir, "0", "10", []string{"editors"})
	fs.write("image/10/editors/emacs", "emacs contents")
	fs.write("image/10/editors/joe", "joe contents")
	fs.write("image/10/editors/vim", "vim contents")
	fs.write("image/10/editors/vi", "vim contents") // Same content as vim!
	mom := mustCreateManifestsStandard(t, 10, fs.Dir)

	// Make the chrootDir incomplete.
	fs.rm("image/10/full/emacs")

	// Creating a pack should fail, no way to get emacs contents from neither chroot
	// or fullfile.
	info, err := CreatePack("editors", 0, 10, fs.path("www"), fs.path("image"))
	if err == nil {
		t.Fatalf("unexpected success when creating pack with incomplete chroot")
	}

	// Create the fullfiles, need to recover emacs and then delete it after.
	fs.write("image/10/full/emacs", "emacs contents")
	mustCreateFullfiles(t, mom.FullManifest, fs.path("image/10/full"), fs.path("www/10/files"))
	fs.rm("image/10/full/emacs")

	// Now create pack.
	info = mustCreatePack(t, "editors", 0, 10, fs.path("www"), fs.path("image"))
	mustValidateZeroPack(t, fs.path("www/10/Manifest.editors"), fs.path("www/10/pack-editors-from-0.tar"))

	// And note that we have a warning.
	if len(info.Warnings) != 1 {
		if len(info.Warnings) == 0 {
			t.Fatalf("got no warnings but expected a warning about emacs file")
		}
		t.Fatalf("got %d warnings but expected just one about emacs\nWARNINGS:\n%s", len(info.Warnings), strings.Join(info.Warnings, "\n"))
	}
}

// mustValidateZeroPack will open a zero pack and check that all the hashes not
// deleted/ghosted in the manifest are present in the pack, and their content does match
// the hash.
func mustValidateZeroPack(t *testing.T, manifestPath, packPath string) {
	t.Helper()

	m, err := ParseManifestFile(manifestPath)
	if err != nil {
		t.Fatalf("couldn't parse manifest to validate pack: %s", err)
	}

	uniqueHashes := make(map[Hashval]bool)
	for _, f := range m.Files {
		if f.Status == StatusDeleted || f.Status == StatusGhosted {
			continue
		}
		uniqueHashes[f.Hash] = true
	}

	_, err = os.Stat(packPath)
	if err != nil {
		t.Fatalf("couldn't open pack file: %s", err)
	}

	pack, err := os.Open(packPath)
	if err != nil {
		t.Fatalf("error opening pack: %s", err)
	}
	defer func() {
		_ = pack.Close()
	}()

	tr, err := newCompressedTarReader(pack)
	if err != nil {
		t.Fatalf("error uncompressing pack: %s", err)
	}
	defer func() {
		_ = tr.Close()
	}()

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("error reading pack: %s", err)
		}
		if hdr.Name == "staged/" || !strings.HasPrefix(hdr.Name, "staged/") {
			continue
		}
		h, err := newHashFromTarHeader(hdr)
		if err != nil {
			t.Fatalf("error calculating hash: %s", err)
		}
		_, err = io.Copy(h, tr)
		if err != nil {
			t.Fatalf("error calculating hash from file in pack: %s", err)
		}
		hash := internHash(h.Sum())
		if _, ok := uniqueHashes[hash]; !ok {
			t.Errorf("found %s that not has a corresponding hash in manifest", hdr.Name)
		}
		delete(uniqueHashes, hash)
	}

	for h := range uniqueHashes {
		t.Errorf("missing staged/%s from the pack", h.String())
	}
}

func mustCreatePack(t *testing.T, name string, fromVersion, toVersion uint32, outputDir, chrootDir string) *PackInfo {
	t.Helper()
	info, err := CreatePack(name, fromVersion, toVersion, outputDir, chrootDir)
	if err != nil {
		t.Fatalf("error creating pack for bundle %s: %s", name, err)
	}
	return info
}

func mustHaveFullfileCount(t *testing.T, info *PackInfo, expected uint64) {
	t.Helper()
	if info.FullfileCount != expected {
		printPackInfo(info)
		t.Fatalf("pack has %d fullfiles but expected %d", info.FullfileCount, expected)
	}
}

func mustHaveDeltaCount(t *testing.T, info *PackInfo, expected uint64) {
	t.Helper()
	if info.DeltaCount != expected {
		printPackInfo(info)
		t.Fatalf("pack has %d deltas but expected %d", info.DeltaCount, expected)
	}
}

func mustHaveNoWarnings(t *testing.T, info *PackInfo) {
	t.Helper()
	if len(info.Warnings) > 0 {
		printPackInfo(info)
		t.Fatalf("unexpected warnings in pack: %s", strings.Join(info.Warnings, "\n"))
	}
}

func mustCreateFullfiles(t *testing.T, m *Manifest, chrootDir, outputDir string) {
	t.Helper()
	err := CreateFullfiles(m, chrootDir, outputDir)
	if err != nil {
		t.Fatalf("couldn't create fullfiles: %s", err)
	}
}

func printPackInfo(info *PackInfo) {
	fmt.Printf("WARNINGS (%d)\n", len(info.Warnings))
	for _, w := range info.Warnings {
		fmt.Println(w)
	}
	fmt.Println()
	fmt.Printf("ENTRIES (%d)\n", len(info.Entries))
	for _, e := range info.Entries {
		fmt.Printf("  %-40s %s (%s)\n", e.File.Name, e.State, e.Reason)
	}
	fmt.Println()
}
