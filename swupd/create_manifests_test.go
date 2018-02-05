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

func TestInitBuildDirs(t *testing.T) {
	var c config
	var err error
	bundles := []string{"os-core", "os-core-update", "test-bundle"}
	c, _ = getConfig("./testdata/state_builddirs")
	if c.imageBase, err = ioutil.TempDir("testdata", "image"); err != nil {
		t.Fatalf("Could not initialize image dir for testing: %v", err)
	}

	defer removeAllIgnoreErr(c.imageBase)

	if err = initBuildDirs(10, bundles, c.imageBase); err != nil {
		t.Errorf("initBuildDirs raised unexpected error: %v", err)
	}

	mustDirExistsWithPerm(t, filepath.Join(c.imageBase, "10"), 0755)

	for _, dir := range bundles {
		mustDirExistsWithPerm(t, filepath.Join(c.imageBase, "10", dir), 0755)
	}
}

func TestCreateManifestsBadMinVersion(t *testing.T) {
	if _, err := CreateManifests(10, 20, 1, "testdir"); err == nil {
		t.Error("No error raised with invalid minVersion (20) for version 10")
	}
}

func TestCreateManifests(t *testing.T) {
	testDir := mustSetupTestDir(t, "basic")
	defer removeIfNoErrors(t, testDir)
	mustInitStandardTest(t, testDir, "0", "10", []string{"test-bundle"})
	mustGenFile(t, testDir, "10", "test-bundle", "foo", "content")
	mustCreateManifestsStandard(t, 10, testDir)

	expSubs := []string{
		"MANIFEST\t1",
		"version:\t10",
		"previous:\t0",
		"filecount:\t5",
		"timestamp:\t",
		"contentsize:\t",
		"includes:\tos-core",
		"10\t/foo",
		"10\t/usr/share",
	}
	checkManifestContains(t, testDir, "10", "test-bundle", expSubs...)

	nExpSubs := []string{
		"\t0\t/foo",
		".d..\t",
	}
	checkManifestNotContains(t, testDir, "10", "test-bundle", nExpSubs...)
	checkManifestNotContains(t, testDir, "10", "MoM", "10\tManifest.full")

	mustInitStandardTest(t, testDir, "10", "20", []string{"test-bundle"})
	mustGenFile(t, testDir, "20", "test-bundle", "foo", "new content")
	mustCreateManifestsStandard(t, 20, testDir)

	expSubs = []string{
		"MANIFEST\t1",
		"version:\t20",
		"previous:\t10",
		"filecount:\t5",
		"includes:\tos-core",
		"20\t/foo",
	}
	checkManifestContains(t, testDir, "20", "test-bundle", expSubs...)
	checkManifestNotContains(t, testDir, "20", "test-bundle", "10\t/foo")
	checkManifestNotContains(t, testDir, "20", "MoM", "20\tManifest.full")
}

func TestCreateManifestsDeleteNoVerBump(t *testing.T) {
	testDir := mustSetupTestDir(t, "deletenoverbump")
	defer removeIfNoErrors(t, testDir)
	mustInitStandardTest(t, testDir, "0", "10", []string{"test-bundle1", "test-bundle2"})
	mustGenFile(t, testDir, "10", "test-bundle1", "foo", "content")
	mustGenFile(t, testDir, "10", "test-bundle2", "foo", "content")
	mustCreateManifestsStandard(t, 10, testDir)

	checkManifestContains(t, testDir, "10", "full", "10\t/foo")

	mustInitStandardTest(t, testDir, "10", "20", []string{"test-bundle1", "test-bundle2"})
	mustGenFile(t, testDir, "20", "test-bundle1", "foo", "content")
	mustCreateManifestsStandard(t, 20, testDir)

	checkManifestContains(t, testDir, "20", "full", "10\t/foo")
	checkManifestNotContains(t, testDir, "20", "full", "20\t/foo")
}

func TestCreateManifestIllegalChar(t *testing.T) {
	testDir := mustSetupTestDir(t, "illegalfname")
	defer removeIfNoErrors(t, testDir)
	mustInitStandardTest(t, testDir, "0", "10", []string{})
	mustGenFile(t, testDir, "10", "os-core", "semicolon;", "")
	mustCreateManifestsStandard(t, 10, testDir)

	checkManifestNotContains(t, testDir, "10", "os-core", "semicolon;")
}

func TestCreateManifestDebuginfo(t *testing.T) {
	testDir := mustSetupTestDir(t, "debuginfo")
	defer removeIfNoErrors(t, testDir)
	mustInitStandardTest(t, testDir, "0", "10", []string{"test-bundle"})
	files := []string{"usr/bin/foobar", "usr/lib/debug/foo", "usr/src/debug/bar"}
	for _, f := range files {
		mustGenFile(t, testDir, "10", "test-bundle", f, "content")
	}

	mustCreateManifestsStandard(t, 10, testDir)

	checkManifestContains(t, testDir, "10", "test-bundle", "/usr/bin/foobar")

	subs := []string{"/usr/lib/debug/foo", "/usr/src/debug/bar"}
	checkManifestNotContains(t, testDir, "10", "test-bundle", subs...)
}

func TestCreateManifestFormatNoDecrement(t *testing.T) {
	ts := newTestSwupd(t, "format-no-decrement-")
	defer ts.cleanup()

	ts.Bundles = []string{"os-core"}

	ts.write("image/10/os-core/foo", "foo")
	ts.write("image/10/os-core/bar", "bar")
	ts.Format = 3
	ts.createManifests(10)

	ts.copyChroots(10, 20)

	// Using a decremented format results in failure.
	_, err := CreateManifests(20, 0, ts.Format-1, ts.Dir)
	if err == nil {
		t.Fatal("unexpected success calling create manifests with decremented format")
	}

	_, err = CreateManifests(20, 0, ts.Format, ts.Dir)
	if err != nil {
		t.Fatalf("create manifests with same format as before failed: %s", err)
	}
}

func TestCreateManifestFormat(t *testing.T) {
	testDir := mustSetupTestDir(t, "format")
	defer removeIfNoErrors(t, testDir)
	mustInitStandardTest(t, testDir, "0", "10", []string{})
	mustGenFile(t, testDir, "10", "os-core", "baz", "bazcontent")
	mustGenFile(t, testDir, "10", "os-core", "foo", "foocontent")
	mustCreateManifestsStandard(t, 10, testDir)

	mustInitStandardTest(t, testDir, "10", "20", []string{})
	mustCreateManifests(t, 20, 20, 2, testDir)
}

func TestCreateManifestGhosted(t *testing.T) {
	testDir := mustSetupTestDir(t, "ghosted")
	defer removeIfNoErrors(t, testDir)
	mustInitStandardTest(t, testDir, "0", "10", []string{"test-bundle"})
	mustGenFile(t, testDir, "10", "test-bundle", "usr/lib/kernel/bar", "bar")
	mustCreateManifestsStandard(t, 10, testDir)

	re := regexp.MustCompile("F\\.b\\.\t.*\t10\t/usr/lib/kernel/bar")
	checkManifestMatches(t, testDir, "10", "full", re)

	mustInitStandardTest(t, testDir, "10", "20", []string{"test-bundle"})
	mustGenFile(t, testDir, "20", "test-bundle", "usr/lib/kernel/baz", "baz")
	mustCreateManifestsStandard(t, 20, testDir)

	res := []*regexp.Regexp{
		regexp.MustCompile("\\.gb\\.\t.*\t20\t/usr/lib/kernel/bar"),
		regexp.MustCompile("F\\.b\\.\t.*\t20\t/usr/lib/kernel/baz"),
	}
	checkManifestMatches(t, testDir, "20", "full", res...)

	mustInitStandardTest(t, testDir, "20", "30", []string{"test-bundle"})
	mustCreateManifestsStandard(t, 30, testDir)

	checkManifestNotContains(t, testDir, "30", "full", "/usr/lib/kernel/bar")

	re = regexp.MustCompile("\\.gb\\.\t.*\t30\t/usr/lib/kernel/baz")
	checkManifestMatches(t, testDir, "30", "full", re)
}

func TestCreateManifestIncludesDeduplicate(t *testing.T) {
	testDir := mustSetupTestDir(t, "includes-dedup")
	defer removeIfNoErrors(t, testDir)
	mustInitStandardTest(t, testDir, "0", "10", []string{"test-bundle1", "test-bundle2"})
	mustInitIncludesFile(t, testDir, "10", "test-bundle2", []string{"test-bundle1", "test-bundle1"})
	mustGenFile(t, testDir, "10", "test-bundle1", "test1", "test1")
	mustGenFile(t, testDir, "10", "test-bundle2", "test2", "test2")
	mustCreateManifestsStandard(t, 10, testDir)

	dualIncludes := "includes:\ttest-bundle1\nincludes:\ttest-bundle1"
	checkManifestNotContains(t, testDir, "10", "test-bundle2", dualIncludes)

	mustInitStandardTest(t, testDir, "10", "20", []string{"test-bundle1", "test-bundle2"})
	mustInitIncludesFile(t, testDir, "20", "test-bundle2", []string{"test-bundle1", "test-bundle1"})
	mustCreateManifestsStandard(t, 20, testDir)

	checkManifestNotContains(t, testDir, "20", "test-bundle2", dualIncludes)
}

func TestCreateManifestDeletes(t *testing.T) {
	testDir := mustSetupTestDir(t, "deletes")
	defer removeIfNoErrors(t, testDir)
	mustInitStandardTest(t, testDir, "0", "10", []string{"test-bundle"})
	mustGenFile(t, testDir, "10", "test-bundle", "test", "test")
	mustCreateManifestsStandard(t, 10, testDir)

	mustInitStandardTest(t, testDir, "10", "20", []string{"test-bundle"})
	mustCreateManifestsStandard(t, 20, testDir)

	deletedLine := ".d..\t" + AllZeroHash + "\t20\t/test"
	checkManifestContains(t, testDir, "20", "test-bundle", deletedLine)
}

func TestCreateManifestsState(t *testing.T) {
	testDir := mustSetupTestDir(t, "state")
	defer removeIfNoErrors(t, testDir)
	mustInitStandardTest(t, testDir, "0", "10", []string{})
	mustGenFile(t, testDir, "10", "os-core", "var/lib/test", "test")
	mustCreateManifestsStandard(t, 10, testDir)

	res := []*regexp.Regexp{
		regexp.MustCompile("D\\.s\\.\t.*\t10\t/var/lib\n"),
		regexp.MustCompile("F\\.s\\.\t.*\t10\t/var/lib/test\n"),
	}
	checkManifestMatches(t, testDir, "10", "os-core", res...)
}

func TestCreateManifestsEmptyDir(t *testing.T) {
	testDir := mustSetupTestDir(t, "emptydir")
	defer removeIfNoErrors(t, testDir)
	mustInitStandardTest(t, testDir, "0", "10", []string{})
	mustGenBundleDir(t, testDir, "10", "os-core", "emptydir")
	mustCreateManifestsStandard(t, 10, testDir)

	re := regexp.MustCompile("D\\.\\.\\.\t.*\t10\t/emptydir\n")
	checkManifestMatches(t, testDir, "10", "os-core", re)
}

func TestCreateManifestsMoM(t *testing.T) {
	testDir := mustSetupTestDir(t, "MoM")
	defer removeIfNoErrors(t, testDir)
	bundles := []string{"test-bundle1", "test-bundle2", "test-bundle3", "test-bundle4"}
	mustInitStandardTest(t, testDir, "0", "10", bundles)
	mustCreateManifestsStandard(t, 10, testDir)

	// initial update, all manifests should be present at this version
	subs := []string{
		"10\ttest-bundle1",
		"10\ttest-bundle2",
		"10\ttest-bundle3",
		"10\ttest-bundle4",
	}
	checkManifestContains(t, testDir, "10", "MoM", subs...)

	mustInitStandardTest(t, testDir, "10", "20", bundles)
	mustGenFile(t, testDir, "20", "test-bundle1", "foo", "foo")
	mustGenFile(t, testDir, "20", "test-bundle2", "bar", "bar")
	mustGenFile(t, testDir, "20", "test-bundle3", "baz", "baz")
	mustCreateManifestsStandard(t, 20, testDir)

	// no update to test-bundle4
	subs = []string{
		"20\ttest-bundle1",
		"20\ttest-bundle2",
		"20\ttest-bundle3",
		"10\ttest-bundle4",
	}
	checkManifestContains(t, testDir, "20", "MoM", subs...)

	mustInitStandardTest(t, testDir, "20", "30", bundles)
	mustGenFile(t, testDir, "30", "test-bundle1", "foo", "foo20")
	mustGenFile(t, testDir, "30", "test-bundle2", "bar", "bar20")
	mustGenFile(t, testDir, "30", "test-bundle3", "foobar", "foobar")
	mustCreateManifestsStandard(t, 30, testDir)

	// again no update to test-bundle4
	subs = []string{
		"30\ttest-bundle1",
		"30\ttest-bundle2",
		"30\ttest-bundle3",
		"10\ttest-bundle4",
	}
	checkManifestContains(t, testDir, "30", "MoM", subs...)

	mustInitStandardTest(t, testDir, "30", "40", bundles)
	mustGenFile(t, testDir, "40", "test-bundle1", "foo", "foo30")
	mustGenFile(t, testDir, "40", "test-bundle2", "bar", "bar20")
	mustCreateManifestsStandard(t, 40, testDir)

	// update only to test-bundle1, test-bundle3 has another deleted file now too
	subs = []string{
		"40\ttest-bundle1",
		"40\ttest-bundle3",
		"30\ttest-bundle2",
		"10\ttest-bundle4",
	}
	checkManifestContains(t, testDir, "40", "MoM", subs...)
}

func TestCreateManifestMaximizeFull(t *testing.T) {
	testDir := mustSetupTestDir(t, "max-full")
	defer removeIfNoErrors(t, testDir)
	bundles := []string{"test-bundle1", "test-bundle2"}
	mustInitStandardTest(t, testDir, "0", "10", bundles)
	mustGenFile(t, testDir, "10", "test-bundle1", "foo", "foo")
	mustCreateManifestsStandard(t, 10, testDir)

	checkManifestContains(t, testDir, "10", "full", "10\t/foo\n")

	mustInitStandardTest(t, testDir, "10", "20", bundles)
	mustGenFile(t, testDir, "20", "test-bundle1", "foo", "foo")
	mustGenFile(t, testDir, "20", "test-bundle2", "foo", "foo")
	mustCreateManifestsStandard(t, 20, testDir)

	checkManifestContains(t, testDir, "20", "full", "20\t/foo\n")
	checkManifestNotContains(t, testDir, "20", "full", "10\t/foo\n")
}

func TestCreateManifestResurrect(t *testing.T) {
	testDir := mustSetupTestDir(t, "resurrect-file")
	defer removeIfNoErrors(t, testDir)
	mustInitStandardTest(t, testDir, "0", "10", []string{"test-bundle"})
	mustGenFile(t, testDir, "10", "test-bundle", "foo", "foo")
	mustGenFile(t, testDir, "10", "test-bundle", "foo1", "foo1")
	mustCreateManifestsStandard(t, 10, testDir)

	mustInitStandardTest(t, testDir, "10", "20", []string{"test-bundle"})
	mustGenFile(t, testDir, "20", "test-bundle", "foo1", "foo1")
	mustCreateManifestsStandard(t, 20, testDir)

	mustInitStandardTest(t, testDir, "20", "30", []string{"test-bundle"})
	mustGenFile(t, testDir, "30", "test-bundle", "foo", "foo1")
	mustCreateManifestsStandard(t, 30, testDir)

	checkManifestContains(t, testDir, "30", "test-bundle", AllZeroHash+"\t30\t/foo1\n")
	checkManifestContains(t, testDir, "30", "test-bundle", "\t30\t/foo\n")
}

func TestCreateManifestsManifestVersion(t *testing.T) {
	testDir := mustSetupTestDir(t, "manifest-version")
	defer removeIfNoErrors(t, testDir)
	mustInitStandardTest(t, testDir, "0", "10", []string{"test-bundle"})
	mustGenFile(t, testDir, "10", "test-bundle", "foo", "foo")
	mustCreateManifestsStandard(t, 10, testDir)

	mustInitStandardTest(t, testDir, "10", "20", []string{"test-bundle"})
	// same file so no manifest for test-bundle
	mustGenFile(t, testDir, "20", "test-bundle", "foo", "foo")
	mustCreateManifestsStandard(t, 20, testDir)

	mustNotExist(t, filepath.Join(testDir, "www/20/Manifest.test-bundle"))

	mustInitStandardTest(t, testDir, "20", "30", []string{"test-bundle"})
	// file changed so should have a manifest for this version
	mustGenFile(t, testDir, "30", "test-bundle", "foo", "bar")
	mustCreateManifestsStandard(t, 30, testDir)

	mustExist(t, filepath.Join(testDir, "www/30/Manifest.test-bundle"))
	// previous version should be 10, not 20, since there was no manifest
	// generated for version 20
	checkManifestContains(t, testDir, "30", "test-bundle", "previous:\t10\n")
}

func TestCreateManifestsMinVersion(t *testing.T) {
	ts := newTestSwupd(t, "minVersion-")
	defer ts.cleanup()

	ts.Bundles = []string{"test-bundle"}
	ts.write("image/10/test-bundle/foo", "foo")
	ts.createManifests(10)

	ts.checkContains("www/10/Manifest.test-bundle", "10\t/foo\n")
	ts.checkContains("www/10/Manifest.full", "10\t/foo\n")

	// Update minVersion, but keep same file and contents.
	ts.MinVersion = 20
	ts.write("image/20/test-bundle/foo", "foo")
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
	testDir := mustSetupTestDir(t, "persistDeletes")
	defer removeIfNoErrors(t, testDir)
	mustInitStandardTest(t, testDir, "0", "10", []string{"test-bundle"})
	mustGenFile(t, testDir, "10", "test-bundle", "foo", "foo")
	mustCreateManifestsStandard(t, 10, testDir)

	mustInitStandardTest(t, testDir, "10", "20", []string{"test-bundle"})
	// foo is deleted
	mustCreateManifestsStandard(t, 20, testDir)

	mustInitStandardTest(t, testDir, "20", "30", []string{"test-bundle"})
	// foo is still deleted
	// create new file to force manifest creation
	mustGenFile(t, testDir, "30", "test-bundle", "bar", "bar")
	mustCreateManifestsStandard(t, 30, testDir)

	// the old deleted file is still there
	re := regexp.MustCompile("\\.d\\.\\.\t.*\t20\t/foo")
	checkManifestMatches(t, testDir, "30", "test-bundle", re)
}

// Imported from swupd-server/test/functional/contentsize-across-versions-includes.
func TestContentSizeAcrossVersionsIncludes(t *testing.T) {
	ts := newTestSwupd(t, "content-size-across-")
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
	ts.mkdir("image/10/test-bundle0")                    // Empty, used as reference.
	ts.write("image/10/test-bundle1/foo", "foo\n")       // 4 bytes.
	ts.write("image/10/test-bundle1/foobar", "foobar\n") // 7 bytes.
	ts.write("image/10/test-bundle2/foo2", "foo2\n")     // 5 bytes.
	ts.write("image/10/noship/test-bundle2-includes", "test-bundle1")
	ts.createManifests(10)

	manifests := mustParseAllManifests(t, 10, ts.path("www"))
	emptySize := manifests["test-bundle0"].Header.ContentSize
	osCoreSize := manifests["os-core"].Header.ContentSize
	fullSize := manifests["full"].Header.ContentSize

	checkSize(manifests["test-bundle1"], 4+7+emptySize)
	checkSize(manifests["test-bundle2"], 5+emptySize)

	// Check that content size does add files from previous updates.
	ts.copyChroots(10, 20)
	ts.write("image/20/test-bundle1/foobarbaz", "foobarbaz\n") // 10 bytes.
	ts.write("image/20/test-bundle2/foo2bar", "foo2bar\n")     // 8 bytes.
	ts.write("image/20/noship/test-bundle2-includes", "test-bundle1")
	ts.createManifests(20)

	manifests = mustParseAllManifests(t, 20, ts.path("www"))

	checkSize(manifests["test-bundle1"], 4+7+10+emptySize)
	checkSize(manifests["test-bundle2"], 5+8+emptySize)

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

	fileDeletedInManifest := func(m *Manifest, version uint32, name string) {
		f := fileInManifest(t, m, version, name)
		if f.Status != StatusDeleted {
			t.Fatalf("manifest %s version %d has file %s marked as %q, but expected \"d\" (deleted)", m.Name, m.Header.Version, name, f.Status)
		}
	}

	// Version 10. Start with both bundles containing foo.
	ts.write("image/10/os-core/foo", "foo\n")
	ts.write("image/10/test-bundle/foo", "foo\n")
	ts.createManifests(10)

	fileInManifest(t, ts.parseManifest(10, "os-core"), 10, "/foo")
	fileNotInManifest(t, ts.parseManifest(10, "test-bundle"), "/foo")

	// Version 20. Delete foo from os-core (the included bundle).
	ts.copyChroots(10, 20)
	ts.rm("image/20/os-core/foo")
	ts.createManifests(20)

	fileDeletedInManifest(ts.parseManifest(20, "os-core"), 20, "/foo")
	fileInManifest(t, ts.parseManifest(20, "test-bundle"), 20, "/foo")

	//
	// Version 30. Delete foo from test-bundle.
	//
	ts.copyChroots(20, 30)
	ts.rm("image/30/test-bundle/foo")
	ts.createManifests(30)

	fileDeletedInManifest(ts.parseManifest(30, "os-core"), 20, "/foo")
	fileDeletedInManifest(ts.parseManifest(30, "test-bundle"), 30, "/foo")

	//
	// Version 40. Make modification (add new file) to test-bundle.
	//
	ts.copyChroots(30, 40)
	ts.write("image/40/test-bundle/foobar", "foobar\n")
	ts.createManifests(40)

	fileDeletedInManifest(ts.parseManifest(40, "os-core"), 20, "/foo")
	fileDeletedInManifest(ts.parseManifest(40, "test-bundle"), 30, "/foo")
}
