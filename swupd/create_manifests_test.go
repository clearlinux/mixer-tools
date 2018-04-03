package swupd

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"regexp"
	"testing"
)

func TestInitBuildEnv(t *testing.T) {
	var err error
	var sdir string
	if sdir, err = ioutil.TempDir("testdata", "state"); err != nil {
		t.Fatalf("Could not initialize state dir for testing: %v", err)
	}

	defer removeAllIgnoreErr(sdir)

	if err = initBuildEnv(config{stateDir: sdir}); err != nil {
		t.Errorf("initBuildEnv raised unexpected error: %v", err)
	}

	if !exists(filepath.Join(sdir, "temp")) {
		t.Error("initBuildEnv failed to set up temporary directory")
	}
}

func TestCreateManifestsBadMinVersion(t *testing.T) {
	if _, err := CreateManifests(10, 20, 1, "testdir"); err == nil {
		t.Error("No error raised with invalid minVersion (20) for version 10")
	}
}

func TestCreateManifestsBasic(t *testing.T) {
	ts := newTestSwupd(t, "basic")
	defer ts.cleanup()

	ts.Bundles = []string{"test-bundle"}

	ts.addFile(10, "test-bundle", "/foo", "content")
	ts.createManifests(10)

	expSubs := []string{
		"MANIFEST\t1",
		"version:\t10",
		"previous:\t0",
		"filecount:\t2",
		"timestamp:\t",
		"contentsize:\t",
		"includes:\tos-core",
		"10\t/foo",
		"10\t/usr/share",
	}
	checkManifestContains(t, ts.Dir, "10", "test-bundle", expSubs...)

	nExpSubs := []string{
		"\t0\t/foo",
		".d..\t",
	}
	checkManifestNotContains(t, ts.Dir, "10", "test-bundle", nExpSubs...)
	checkManifestNotContains(t, ts.Dir, "10", "MoM", "10\tManifest.full")

	ts.addFile(20, "test-bundle", "/foo", "new content")
	ts.createManifests(20)

	expSubs = []string{
		"MANIFEST\t1",
		"version:\t20",
		"previous:\t10",
		"filecount:\t2",
		"includes:\tos-core",
		"20\t/foo",
	}
	checkManifestContains(t, ts.Dir, "20", "test-bundle", expSubs...)
	checkManifestNotContains(t, ts.Dir, "20", "test-bundle", "10\t/foo")
	checkManifestNotContains(t, ts.Dir, "20", "MoM", "20\tManifest.full")
}

func TestCreateManifestsDeleteNoVerBump(t *testing.T) {
	ts := newTestSwupd(t, "delete-no-version-bump")
	defer ts.cleanup()
	ts.Bundles = []string{"test-bundle1", "test-bundle2"}
	ts.addFile(10, "test-bundle1", "/foo", "content")
	ts.addFile(10, "test-bundle2", "/foo", "content")
	ts.createManifests(10)

	checkManifestContains(t, ts.Dir, "10", "full", "10\t/foo")

	ts.addFile(20, "test-bundle1", "/foo", "content")
	ts.createManifests(20)

	fileInManifest(t, ts.parseManifest(20, "full"), 10, "/foo")
}

func TestCreateManifestIllegalChar(t *testing.T) {
	ts := newTestSwupd(t, "illegal-file-name")
	defer ts.cleanup()
	ts.addFile(10, "os-core", "semicolon;", "")
	ts.createManifests(10)
	fileNotInManifest(t, ts.parseManifest(10, "full"), "/semicolon;")
	fileNotInManifest(t, ts.parseManifest(10, "os-core"), "/semicolon;")
}

func TestCreateManifestDebuginfo(t *testing.T) {
	ts := newTestSwupd(t, "debuginfo-banned")
	defer ts.cleanup()
	ts.Bundles = []string{"test-bundle"}
	files := []string{"/usr/bin/foobar", "/usr/lib/debug/foo", "/usr/src/debug/bar"}
	for _, f := range files {
		ts.addFile(10, "test-bundle", f, "content")
	}

	ts.createManifests(10)

	m := ts.parseManifest(10, "test-bundle")
	fileInManifest(t, m, 10, "/usr/bin/foobar")
	fileNotInManifest(t, m, "/usr/lib/debug/foo")
	fileNotInManifest(t, m, "/usr/src/debug/bar")
}

func TestCreateManifestFormatNoDecrement(t *testing.T) {
	ts := newTestSwupd(t, "format-no-decrement-")
	defer ts.cleanup()

	ts.Bundles = []string{"os-core"}

	ts.addFile(10, "os-core", "/foo", "foo")
	ts.addFile(10, "os-core", "/bar", "bar")
	ts.Format = 3
	ts.createManifests(10)

	ts.copyChroots(10, 20)

	// Using a decremented format results in failure.
	_, err := CreateManifests(20, 0, ts.Format-1, ts.Dir)
	if err == nil {
		t.Fatal("unexpected success calling create manifests with decremented format")
	}

	ts.addFile(20, "os-core", "/bar", "bar")
	_, err = CreateManifests(20, 0, ts.Format, ts.Dir)
	if err != nil {
		t.Fatalf("create manifests with same format as before failed: %s", err)
	}
}

func TestCreateManifestFormat(t *testing.T) {
	ts := newTestSwupd(t, "format-basic")
	defer ts.cleanup()
	ts.addFile(10, "os-core", "/baz", "bazcontent")
	ts.addFile(10, "os-core", "/foo", "foocontent")
	ts.createManifests(10)
	ts.Format = 2
	ts.createManifests(20)
}

func TestCreateManifestGhosted(t *testing.T) {
	ts := newTestSwupd(t, "ghosted")
	defer ts.cleanup()
	ts.Bundles = []string{"test-bundle"}
	ts.addFile(10, "test-bundle", "/usr/lib/kernel/bar", "bar")
	ts.createManifests(10)

	f := fileInManifest(t, ts.parseManifest(10, "full"), 10, "/usr/lib/kernel/bar")
	if f.Modifier != ModifierBoot {
		t.Errorf("%s not marked as boot", f.Name)
	}

	ts.addFile(20, "test-bundle", "/usr/lib/kernel/baz", "baz")
	ts.createManifests(20)

	m20 := ts.parseManifest(20, "full")
	f1 := fileInManifest(t, m20, 20, "/usr/lib/kernel/bar")
	if f1.Status != StatusGhosted {
		t.Errorf("%s present in 20 full but expected to be ghosted", f1.Name)
	}
	if f1.Modifier != ModifierBoot {
		t.Errorf("%s not marked as boot", f1.Name)
	}
	f2 := fileInManifest(t, m20, 20, "/usr/lib/kernel/baz")
	if f2.Status != StatusUnset {
		t.Errorf("%s not present in 20 full but expected to be", f2.Name)
	}
	if f2.Modifier != ModifierBoot {
		t.Errorf("%s not marked as boot", f2.Name)
	}

	ts.createManifests(30)
	m30 := ts.parseManifest(30, "full")
	fileNotInManifest(t, m30, "/usr/lib/kernel/bar")
	f3 := fileInManifest(t, m30, 30, "/usr/lib/kernel/baz")
	if f3.Status != StatusGhosted {
		t.Errorf("%s present in 20 full but expected to be ghosted", f3.Name)
	}
}

func TestCreateManifestIncludesDeduplicate(t *testing.T) {
	ts := newTestSwupd(t, "includes-dedup")
	defer ts.cleanup()
	ts.Bundles = []string{"test-bundle1", "test-bundle2"}
	ts.addIncludes(10, "test-bundle2", []string{"test-bundle1", "test-bundle1"})
	ts.addFile(10, "test-bundle1", "/test1", "/test1")
	ts.addFile(10, "test-bundle2", "/test2", "/test2")
	ts.createManifests(10)

	dualIncludes := "includes:\ttest-bundle1\nincludes:\ttest-bundle1"
	checkManifestNotContains(t, ts.Dir, "10", "test-bundle2", dualIncludes)
	checkManifestContains(t, ts.Dir, "10", "test-bundle2", "includes:\ttest-bundle1\n")

	ts.addIncludes(20, "test-bundle2", []string{"test-bundle1", "test-bundle1"})
	ts.createManifests(20)

	checkManifestNotContains(t, ts.Dir, "20", "test-bundle2", dualIncludes)
}

func TestCreateManifestDeletes(t *testing.T) {
	ts := newTestSwupd(t, "deletes")
	defer ts.cleanup()

	ts.Bundles = []string{"test-bundle"}
	ts.addFile(10, "test-bundle", "/test", "test")
	ts.createManifests(10)
	ts.createManifests(20)

	deletedLine := ".d..\t" + AllZeroHash + "\t20\t/test"
	checkManifestContains(t, ts.Dir, "20", "test-bundle", deletedLine)
}

func TestCreateManifestsState(t *testing.T) {
	ts := newTestSwupd(t, "state")
	defer ts.cleanup()
	ts.addDir(10, "os-core", "/var/lib")
	ts.addFile(10, "os-core", "/var/lib/test", "test")
	ts.createManifests(10)

	res := []*regexp.Regexp{
		regexp.MustCompile("D\\.s\\.\t.*\t10\t/var/lib\n"),
		regexp.MustCompile("F\\.s\\.\t.*\t10\t/var/lib/test\n"),
	}
	checkManifestMatches(t, ts.Dir, "10", "os-core", res...)
}

func TestCreateManifestsEmptyDir(t *testing.T) {
	ts := newTestSwupd(t, "emptydir")
	defer ts.cleanup()
	ts.addDir(10, "os-core", "/emptydir")
	ts.createManifests(10)

	re := regexp.MustCompile("D\\.\\.\\.\t.*\t10\t/emptydir\n")
	checkManifestMatches(t, ts.Dir, "10", "os-core", re)
}

func TestCreateManifestsMoM(t *testing.T) {
	ts := newTestSwupd(t, "MoM")
	defer ts.cleanup()
	ts.Bundles = []string{"test-bundle1", "test-bundle2", "test-bundle3", "test-bundle4"}
	ts.createManifests(10)

	// initial update, all manifests should be present at this version
	subs := []string{
		"10\ttest-bundle1",
		"10\ttest-bundle2",
		"10\ttest-bundle3",
		"10\ttest-bundle4",
	}
	checkManifestContains(t, ts.Dir, "10", "MoM", subs...)

	ts.addFile(20, "test-bundle1", "/foo", "foo")
	ts.addFile(20, "test-bundle2", "/bar", "bar")
	ts.addFile(20, "test-bundle3", "/baz", "baz")
	ts.createManifests(20)

	// no update to test-bundle4
	subs = []string{
		"20\ttest-bundle1",
		"20\ttest-bundle2",
		"20\ttest-bundle3",
		"10\ttest-bundle4",
	}
	checkManifestContains(t, ts.Dir, "20", "MoM", subs...)

	ts.addFile(30, "test-bundle1", "/foo", "foo20")
	ts.addFile(30, "test-bundle2", "/bar", "bar20")
	ts.addFile(30, "test-bundle3", "/foobar", "foobar")
	ts.createManifests(30)

	// again no update to test-bundle4
	subs = []string{
		"30\ttest-bundle1",
		"30\ttest-bundle2",
		"30\ttest-bundle3",
		"10\ttest-bundle4",
	}
	checkManifestContains(t, ts.Dir, "30", "MoM", subs...)

	ts.addFile(40, "test-bundle1", "/foo", "foo30")
	ts.addFile(40, "test-bundle2", "/bar", "bar20")
	ts.createManifests(40)

	// update only to test-bundle1, test-bundle3 has another deleted file now too
	subs = []string{
		"40\ttest-bundle1",
		"40\ttest-bundle3",
		"30\ttest-bundle2",
		"10\ttest-bundle4",
	}
	checkManifestContains(t, ts.Dir, "40", "MoM", subs...)
}

func TestCreateManifestMaximizeFull(t *testing.T) {
	ts := newTestSwupd(t, "max-full")
	defer ts.cleanup()
	ts.Bundles = []string{"test-bundle1", "test-bundle2"}
	ts.addFile(10, "test-bundle1", "/foo", "foo")
	ts.createManifests(10)

	fileInManifest(t, ts.parseManifest(10, "full"), 10, "/foo")

	ts.addFile(20, "test-bundle1", "/foo", "foo")
	ts.addFile(20, "test-bundle2", "/foo", "foo")
	ts.createManifests(20)

	fileInManifest(t, ts.parseManifest(20, "full"), 20, "/foo")
	checkManifestNotContains(t, ts.Dir, "20", "full", "10\t/foo\n")
}

func TestCreateManifestResurrect(t *testing.T) {
	ts := newTestSwupd(t, "resurrect-file")
	defer ts.cleanup()
	ts.Bundles = []string{"test-bundle"}
	ts.addFile(10, "test-bundle", "/foo", "foo")
	ts.addFile(10, "test-bundle", "/foo1", "foo1")
	ts.createManifests(10)

	ts.addFile(20, "test-bundle", "/foo1", "foo1")
	ts.createManifests(20)

	ts.addFile(30, "test-bundle", "/foo", "foo1")
	ts.createManifests(30)

	checkManifestContains(t, ts.Dir, "30", "test-bundle", AllZeroHash+"\t30\t/foo1\n")
	fileInManifest(t, ts.parseManifest(30, "test-bundle"), 30, "/foo")
}

func TestCreateManifestsManifestVersion(t *testing.T) {
	ts := newTestSwupd(t, "manifest-version")
	defer ts.cleanup()
	ts.Bundles = []string{"test-bundle"}
	ts.addFile(10, "test-bundle", "/foo", "foo")
	ts.createManifests(10)

	// same file so no manifest for test-bundle
	ts.addFile(20, "test-bundle", "/foo", "foo")
	ts.createManifests(20)

	mustNotExist(t, filepath.Join(ts.Dir, "www/20/Manifest.test-bundle"))

	// file changed so should have a manifest for this version
	ts.addFile(30, "test-bundle", "/foo", "bar")
	ts.createManifests(30)

	mustExist(t, filepath.Join(ts.Dir, "www/30/Manifest.test-bundle"))
	// previous version should be 10, not 20, since there was no manifest
	// generated for version 20
	ts.checkContains("www/30/Manifest.test-bundle", "previous:\t10\n")
}

func TestCreateManifestsMinVersion(t *testing.T) {
	ts := newTestSwupd(t, "minVersion")
	defer ts.cleanup()

	ts.Bundles = []string{"test-bundle"}
	ts.addFile(10, "test-bundle", "/foo", "foo")
	ts.createManifests(10)

	ts.checkContains("www/10/Manifest.test-bundle", "10\t/foo\n")
	ts.checkContains("www/10/Manifest.full", "10\t/foo\n")

	// Update minVersion, but keep same file and contents.
	ts.MinVersion = 20
	ts.addFile(20, "test-bundle", "/foo", "foo")
	ts.createManifests(20)

	// since the minVersion was set to this version the file version should
	// be updated despite there being no change to the file.
	ts.checkContains("www/20/Manifest.test-bundle", "20\t/foo\n")
	ts.checkContains("www/20/Manifest.full", "20\t/foo\n")
	ts.checkNotContains("www/20/Manifest.test-bundle", "10\t/foo\n")
	ts.checkNotContains("www/20/Manifest.full", "10\t/foo\n")
	// we can even check that there are NO files left at version 10
	ts.checkNotContains("www/20/Manifest.full", "\t10\t")
}

func TestCreateManifestsPersistDeletes(t *testing.T) {
	ts := newTestSwupd(t, "persistDeletes")
	defer ts.cleanup()
	ts.Bundles = []string{"test-bundle"}
	ts.addFile(10, "test-bundle", "/foo", "foo")
	ts.createManifests(10)

	// foo is deleted
	ts.createManifests(20)

	// foo is still deleted
	// create new file to force manifest creation
	ts.addFile(30, "test-bundle", "/bar", "bar")
	ts.createManifests(30)

	// the old deleted file is still there
	re := regexp.MustCompile("\\.d\\.\\.\t.*\t20\t/foo")
	checkManifestMatches(t, ts.Dir, "30", "test-bundle", re)
}

// Imported from swupd-server/test/functional/contentsize-across-versions-includes.
func TestContentSizeAcrossVersionsIncludes(t *testing.T) {
	ts := newTestSwupd(t, "content-size-across")
	defer ts.cleanup()

	checkSize := func(m *Manifest, expectedSize uint64) {
		t.Helper()
		if m == nil {
			t.Error("couldn't check size, manifest not found")
			return
		}
		size := m.Header.ContentSize
		if size != expectedSize {
			t.Errorf("bundle %s has contentsize %d but expected %d", m.Name, size, expectedSize)
		}
	}

	// Create a couple updates to both check that contentsize does not add included
	// bundles and to verify that files changed in previous updates are counted.

	ts.Bundles = []string{"test-bundle0", "test-bundle1", "test-bundle2"}

	// Check that contentsize does not add included bundle.
	ts.addFile(10, "test-bundle1", "/foo", "foo\n")       // 4 bytes
	ts.addFile(10, "test-bundle1", "/foobar", "foobar\n") // 7 bytes
	ts.addFile(10, "test-bundle2", "/foo2", "foo2\n")     // 5 bytes
	ts.addIncludes(10, "test-bundle2", []string{"test-bundle1"})
	ts.createManifests(10)

	manifests := mustParseAllManifests(t, 10, ts.path("www"))
	emptySize := manifests["test-bundle0"].Header.ContentSize
	osCoreSize := manifests["os-core"].Header.ContentSize
	fullSize := manifests["full"].Header.ContentSize

	checkSize(manifests["test-bundle1"], 4+7+emptySize)
	checkSize(manifests["test-bundle2"], 5) // emptySize subtracted out

	// Check that content size does add files from previous updates.
	ts.addFile(20, "test-bundle1", "/foo", "foo\n")
	ts.addFile(20, "test-bundle1", "/foobar", "foobar\n")
	ts.addFile(20, "test-bundle2", "/foo2", "foo2\n")
	ts.addFile(20, "test-bundle1", "/foobarbaz", "foobarbaz\n") // 10 bytes
	ts.addFile(20, "test-bundle2", "/foo2bar", "foo2bar\n")     // 8 bytes
	ts.addIncludes(20, "test-bundle2", []string{"test-bundle1"})
	ts.createManifests(20)

	manifests = mustParseAllManifests(t, 20, ts.path("www"))

	checkSize(manifests["test-bundle1"], 4+7+10+emptySize)
	checkSize(manifests["test-bundle2"], 5+8) // emptySize subtracted out

	// os-core should have the same size as before.
	checkSize(manifests["os-core"], osCoreSize)

	// full should have increased with all new files.
	checkSize(manifests["full"], fullSize+10+8)
}

func mustParseAllManifests(t *testing.T, version uint32, outputDir string) map[string]*Manifest {
	t.Helper()

	mom, err := ParseManifestFile(filepath.Join(outputDir, fmt.Sprint(version), "Manifest.MoM"))
	if err != nil {
		t.Fatalf("couldn't parse Manifest.MoM for version %d: %s", version, err)
	}

	full, err := ParseManifestFile(filepath.Join(outputDir, fmt.Sprint(version), "Manifest.full"))
	if err != nil {
		t.Fatalf("couldn't parse Manifest.full for version %d: %s", version, err)
	}

	// Result contains all bundles manifests plus "full" and "MoM".
	result := make(map[string]*Manifest, len(mom.Files)+2)
	result["MoM"] = mom
	result["full"] = full

	for _, f := range mom.Files {
		var m *Manifest
		m, err = ParseManifestFile(filepath.Join(outputDir, fmt.Sprint(f.Version), "Manifest."+f.Name))
		if err != nil {
			t.Fatalf("could't parse Manifest.%s of version %d: %s", f.Name, f.Version, err)
		}
		result[f.Name] = m
	}

	return result
}

// Imported from swupd-server/test/functional/subtract-delete.
func TestSubtractDelete(t *testing.T) {
	ts := newTestSwupd(t, "subtract-delete-")
	defer ts.cleanup()

	ts.Bundles = []string{"os-core", "test-bundle"}

	// Version 10. Start with both bundles containing foo.
	ts.addFile(10, "os-core", "/foo", "foo\n")
	ts.addFile(10, "test-bundle", "/foo", "foo\n")
	ts.createManifests(10)

	fileInManifest(t, ts.parseManifest(10, "os-core"), 10, "/foo")
	fileNotInManifest(t, ts.parseManifest(10, "test-bundle"), "/foo")

	// Version 20. Delete foo from os-core (the included bundle).
	ts.addFile(20, "test-bundle", "/foo", "foo\n")
	ts.createManifests(20)

	fileDeletedInManifest(t, ts.parseManifest(20, "os-core"), 20, "/foo")
	fileInManifest(t, ts.parseManifest(20, "test-bundle"), 20, "/foo")

	// Version 30. Delete foo from test-bundle.
	ts.createManifests(30)

	fileDeletedInManifest(t, ts.parseManifest(30, "os-core"), 20, "/foo")
	fileDeletedInManifest(t, ts.parseManifest(30, "test-bundle"), 30, "/foo")

	// Version 40. Make modification (add new file) to test-bundle.
	ts.addFile(40, "test-bundle", "/foobar", "foobar\n")
	ts.createManifests(40)

	fileDeletedInManifest(t, ts.parseManifest(40, "os-core"), 20, "/foo")
	fileDeletedInManifest(t, ts.parseManifest(40, "test-bundle"), 30, "/foo")
}

func TestSubtractManifestsNested(t *testing.T) {
	ts := newTestSwupd(t, "subtract-nested-")
	defer ts.cleanup()

	ts.Bundles = []string{"os-core", "test-bundle", "included", "included-nested"}

	// os-core file
	ts.addFile(10, "os-core", "/os-core-file", "os-core-file")
	// included-nested has the os-core file to be subtracted
	ts.addFile(10, "included-nested", "/os-core-file", "os-core-file")
	ts.addFile(10, "included-nested", "/included-nested-file", "included-nested-file")
	ts.addFile(10, "included-nested", "/some-random-file", "some-random-file")
	// included has all the above files
	ts.addIncludes(10, "included", []string{"included-nested"})
	ts.addFile(10, "included", "/os-core-file", "os-core-file")
	ts.addFile(10, "included", "/included-nested-file", "included-nested-file")
	ts.addFile(10, "included", "/included-file", "included-file")
	// test-bundle has all above files plus its own
	ts.addIncludes(10, "test-bundle", []string{"included"})
	ts.addFile(10, "test-bundle", "/os-core-file", "os-core-file")
	ts.addFile(10, "test-bundle", "/included-nested-file", "included-nested-file")
	ts.addFile(10, "test-bundle", "/included-file", "included-file")
	ts.addFile(10, "test-bundle", "/test-bundle-file", "test-bundle-file")
	ts.addFile(10, "test-bundle", "/some-random-file", "some-random-file")

	ts.createManifests(10)

	osCore := ts.parseManifest(10, "os-core")
	fileInManifest(t, osCore, 10, "/os-core-file")

	includedNested := ts.parseManifest(10, "included-nested")
	fileInManifest(t, includedNested, 10, "/included-nested-file")
	fileNotInManifest(t, includedNested, "/os-core-file")

	included := ts.parseManifest(10, "included")
	fileInManifest(t, included, 10, "/included-file")
	fileNotInManifest(t, included, "/included-nested-file")
	fileNotInManifest(t, included, "/os-core-file")

	testBundle := ts.parseManifest(10, "test-bundle")
	fileInManifest(t, testBundle, 10, "/test-bundle-file")
	fileNotInManifest(t, testBundle, "/included-file")
	fileNotInManifest(t, testBundle, "/included-nested-file")
	fileNotInManifest(t, testBundle, "/os-core-file")
	fileNotInManifest(t, testBundle, "/some-random-file")
}

func TestCreateManifestsIndexContents(t *testing.T) {
	ts := newTestSwupd(t, "index")
	defer ts.cleanup()
	ts.Bundles = []string{"test-bundle1", "test-bundle2"}
	ts.addFile(10, "test-bundle1", "/bar", "bar")
	ts.addFile(10, "test-bundle2", "/foo", "foo")
	ts.createManifests(10)

	ts.checkContains("image/10/full/usr/share/clear/os-core-update-index", "/bar\ttest-bundle1\n")
	ts.checkContains("image/10/full/usr/share/clear/os-core-update-index", "/foo\ttest-bundle2\n")
	fileInManifest(t, ts.parseManifest(10, "MoM"), 10, "os-core-update-index")
	fileInManifest(t, ts.parseManifest(10, "full"), 10, "/usr/share/clear/os-core-update-index")

	ts.addFile(20, "test-bundle1", "/foo", "foo")
	ts.addFile(20, "test-bundle1", "/bar", "bar")
	ts.createManifests(20)

	ts.checkContains("image/20/full/usr/share/clear/os-core-update-index", "/foo\ttest-bundle1\n")
	ts.checkNotContains("image/20/full/usr/share/clear/os-core-update-index", "/foo\ttest-bundle2\n")
	fileInManifest(t, ts.parseManifest(20, "MoM"), 20, "os-core-update-index")
	// must exist at correct version
	fileInManifest(t, ts.parseManifest(20, "full"), 20, "/usr/share/clear/os-core-update-index")
	// no update to this dir
	fileInManifest(t, ts.parseManifest(20, "os-core-update-index"), 10, "/usr/share")

	ts.createManifests(30)
	// expect only the current version to show up in the MoM
	// this is an issue we ran into where the old index manifest was copied over
	// as well as generated.
	fileInManifest(t, ts.parseManifest(30, "full"), 30, "/usr/share/clear/os-core-update-index")
	fileInManifest(t, ts.parseManifest(30, "MoM"), 30, "os-core-update-index")
	checkManifestNotContains(t, ts.Dir, "30", "MoM", "10\tos-core-update-index")
}

func TestCreateManifestsIndexInclude(t *testing.T) {
	ts := newTestSwupd(t, "index-include")
	defer ts.cleanup()
	ts.Bundles = []string{"test-bundle1", "test-bundle2"}
	ts.addIncludes(10, "test-bundle2", []string{"os-core-update-index"})
	ts.addFile(10, "test-bundle1", "/foo", "foo")
	ts.createManifests(10)

	ts.checkContains("www/10/Manifest.test-bundle2", "includes:\tos-core-update-index")
}

func TestNoUpdateStateFiles(t *testing.T) {
	ts := newTestSwupd(t, "no-update-boot-files")
	defer ts.cleanup()

	ts.Bundles = []string{"os-core", "test-bundle"}

	// Version 10
	ts.addFile(10, "test-bundle", "/usr/lib/kernel/file", "content")
	ts.createManifests(10)

	// Version 20 has same content for /usr/lib/kernel/file and a new
	// file to force manifest creation
	ts.addFile(20, "test-bundle", "/usr/lib/kernel/file", "content")
	ts.addFile(20, "test-bundle", "/new", "new")
	ts.createManifests(20)

	m20 := ts.parseManifest(20, "test-bundle")
	fileInManifest(t, m20, 10, "/usr/lib/kernel/file")
}
