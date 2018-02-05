package swupd

import (
	"io/ioutil"
	"path/filepath"
	"regexp"
	"strings"
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
	testDir := mustSetupTestDir(t, "format-no-decrement")
	defer removeIfNoErrors(t, testDir)
	mustInitStandardTest(t, testDir, "0", "10", []string{})
	mustGenFile(t, testDir, "10", "os-core", "baz", "bazcontent")
	mustGenFile(t, testDir, "10", "os-core", "foo", "foocontent")
	mustCreateManifestsStandard(t, 10, testDir)

	mustInitStandardTest(t, testDir, "10", "20", []string{})
	mustCreateManifestsStandard(t, 20, testDir)
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

func TestCreateManifestIncludeVersionBump(t *testing.T) {
	testDir := mustSetupTestDir(t, "includeverbump")
	defer removeIfNoErrors(t, testDir)
	bundles := []string{"test-bundle", "included", "included2", "included-nested"}
	mustInitStandardTest(t, testDir, "0", "10", bundles)
	mustGenFile(t, testDir, "10", "test-bundle", "foo", "foo")
	// no includes file for first update
	mustCreateManifestsStandard(t, 10, testDir)

	expected := []string{"includes:\tos-core\n", "10\t/foo\n"}
	checkManifestContains(t, testDir, "10", "test-bundle", expected...)

	mustInitStandardTest(t, testDir, "10", "20", bundles)
	// same file as last update, now manifest creation must be triggered by new includes
	mustGenFile(t, testDir, "20", "test-bundle", "foo", "foo")
	mustGenFile(t, testDir, "20", "included", "bar", "bar")
	mustGenFile(t, testDir, "20", "included2", "baz", "baz")
	mustInitIncludesFile(t, testDir, "20", "test-bundle", []string{"included", "included2"})
	mustCreateManifestsStandard(t, 20, testDir)

	cases := []struct {
		exp   []string
		bname string
	}{
		{[]string{
			"includes:\tos-core\n",
			"includes:\tincluded\n",
			"includes:\tincluded2\n",
		},
			"test-bundle"},
		{[]string{"20\t/bar\n"}, "included"},
		{[]string{"20\t/baz\n"}, "included2"},
	}
	for _, tc := range cases {
		checkManifestContains(t, testDir, "20", tc.bname, tc.exp...)
	}

	mustInitStandardTest(t, testDir, "20", "30", bundles)
	// again, same files
	mustGenFile(t, testDir, "30", "test-bundle", "foo", "foo")
	mustGenFile(t, testDir, "30", "included", "bar", "bar")
	mustGenFile(t, testDir, "30", "included2", "baz", "baz")
	mustGenFile(t, testDir, "30", "included-nested", "foobarbaz", "foobarbaz")
	mustInitIncludesFile(t, testDir, "30", "test-bundle", []string{"included", "included2"})
	mustInitIncludesFile(t, testDir, "30", "included", []string{"included-nested"})
	mustCreateManifestsStandard(t, 30, testDir)

	cases = []struct {
		exp   []string
		bname string
	}{
		{[]string{"includes:\tincluded-nested\n", "20\t/bar\n"}, "included"},
		{[]string{"30\t/foobarbaz\n"}, "included-nested"},
	}
	for _, tc := range cases {
		checkManifestContains(t, testDir, "30", tc.bname, tc.exp...)
	}

	mustNotExist(t, filepath.Join(testDir, "www/30/Manifest.test-bundle"))
	mustNotExist(t, filepath.Join(testDir, "www/30/Manifest.included2"))
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

	checkManifestNotContains(t, testDir, "30", "test-bundle", AllZeroHash+"\t30\t/foo1\n")
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
	testDir := mustSetupTestDir(t, "minVersion")
	defer removeIfNoErrors(t, testDir)
	mustInitStandardTest(t, testDir, "0", "10", []string{"test-bundle"})
	mustGenFile(t, testDir, "10", "test-bundle", "foo", "foo")
	mustCreateManifestsStandard(t, 10, testDir)

	checkManifestContains(t, testDir, "10", "test-bundle", "10\t/foo\n")
	checkManifestContains(t, testDir, "10", "full", "10\t/foo\n")

	mustInitStandardTest(t, testDir, "10", "20", []string{"test-bundle"})
	// same file and same contents
	mustGenFile(t, testDir, "20", "test-bundle", "foo", "foo")
	mustCreateManifests(t, 20, 20, 1, testDir)

	// since the minVersion was set to this version the file version should
	// be updated despite there being no change to the file.
	checkManifestContains(t, testDir, "20", "test-bundle", "20\t/foo\n")
	checkManifestContains(t, testDir, "20", "full", "20\t/foo\n")
	checkManifestNotContains(t, testDir, "20", "test-bundle", "10\t/foo\n")
	checkManifestNotContains(t, testDir, "20", "full", "10\t/foo\n")
	// we can even check that there are NO files left at version 10
	checkManifestNotContains(t, testDir, "20", "full", "\t10\t")
}

func TestCreateManifestsDirectRenames(t *testing.T) {
	testDir := mustSetupTestDir(t, "directRenames")
	defer removeIfNoErrors(t, testDir)

	mustInitStandardTest(t, testDir, "0", "10", []string{"test-bundle"})
	mustGenFile(t, testDir, "10", "test-bundle", "foo", strings.Repeat("foo", 100))
	mustCreateManifestsStandard(t, 10, testDir)

	mustInitStandardTest(t, testDir, "10", "20", []string{"test-bundle"})
	mustGenFile(t, testDir, "20", "test-bundle", "bar", strings.Repeat("foo", 100))
	mustCreateManifestsStandard(t, 20, testDir)

	re := regexp.MustCompile("F\\.\\.r\t.*\t20\t/bar\n")
	checkManifestMatches(t, testDir, "20", "test-bundle", re)
	re = regexp.MustCompile("\\.d\\.r\t.*\t20\t/foo\n")
	checkManifestMatches(t, testDir, "20", "test-bundle", re)
}

func TestCreateManifestsRenamesNewHash(t *testing.T) {
	testDir := mustSetupTestDir(t, "renamesNewHash")
	defer removeIfNoErrors(t, testDir)

	mustInitStandardTest(t, testDir, "0", "10", []string{"test-bundle"})
	mustGenFile(t, testDir, "10", "test-bundle", "foo", strings.Repeat("foo", 100))
	mustCreateManifestsStandard(t, 10, testDir)

	mustInitStandardTest(t, testDir, "10", "20", []string{"test-bundle"})
	mustGenFile(t, testDir, "20", "test-bundle", "foo33", strings.Repeat("foo", 100)+"ch-ch-ch-changes")
	mustCreateManifestsStandard(t, 20, testDir)

	re := regexp.MustCompile("F\\.\\.r\t.*\t20\t/foo33\n")
	checkManifestMatches(t, testDir, "20", "test-bundle", re)
	re = regexp.MustCompile("\\.d\\.r\t.*\t20\t/foo\n")
	checkManifestMatches(t, testDir, "20", "test-bundle", re)
}

func TestCreateManifestsRenamesOrphanedDeletes(t *testing.T) {
	testDir := mustSetupTestDir(t, "renamesOrphanedDeletes")
	defer removeIfNoErrors(t, testDir)
	mustInitStandardTest(t, testDir, "0", "10", []string{"test-bundle"})
	mustGenFile(t, testDir, "10", "test-bundle", "direct", strings.Repeat("foo", 100))
	mustGenFile(t, testDir, "10", "test-bundle", "hashchange", strings.Repeat("boo", 100))
	mustCreateManifestsStandard(t, 10, testDir)

	mustInitStandardTest(t, testDir, "10", "20", []string{"test-bundle"})
	mustGenFile(t, testDir, "20", "test-bundle", "direct1", strings.Repeat("foo", 100))
	mustGenFile(t, testDir, "20", "test-bundle", "hashchange1", strings.Repeat("roo", 100))
	mustCreateManifestsStandard(t, 20, testDir)

	mustInitStandardTest(t, testDir, "20", "30", []string{"test-bundle"})
	mustCreateManifestsStandard(t, 30, testDir)

	res := []*regexp.Regexp{
		regexp.MustCompile("\\.d\\.\\.\t.*\t30\t/direct\n"),
		regexp.MustCompile("\\.d\\.\\.\t.*\t30\t/direct1\n"),
		regexp.MustCompile("\\.d\\.\\.\t.*\t30\t/hashchange\n"),
		regexp.MustCompile("\\.d\\.\\.\t.*\t30\t/hashchange1\n"),
	}
	checkManifestMatches(t, testDir, "30", "test-bundle", res...)
}

func TestCreateRenamesOrphanedChain(t *testing.T) {
	testDir := mustSetupTestDir(t, "renamesOrphanedChain")
	defer removeIfNoErrors(t, testDir)
	mustInitStandardTest(t, testDir, "0", "10", []string{"test-bundle"})
	mustGenFile(t, testDir, "10", "test-bundle", "direct", strings.Repeat("foo", 100))
	mustGenFile(t, testDir, "10", "test-bundle", "hashchange", strings.Repeat("boo", 100))
	mustCreateManifestsStandard(t, 10, testDir)

	mustInitStandardTest(t, testDir, "10", "20", []string{"test-bundle"})
	mustGenFile(t, testDir, "20", "test-bundle", "direct1", strings.Repeat("foo", 100))
	mustGenFile(t, testDir, "20", "test-bundle", "hashchange1", strings.Repeat("roo", 100))
	mustCreateManifestsStandard(t, 20, testDir)

	mustInitStandardTest(t, testDir, "20", "30", []string{"test-bundle"})
	mustGenFile(t, testDir, "30", "test-bundle", "direct2", strings.Repeat("foo", 100))
	mustGenFile(t, testDir, "30", "test-bundle", "hashchange2", strings.Repeat("goo", 100))
	mustCreateManifestsStandard(t, 30, testDir)

	res := []*regexp.Regexp{
		// direct deleted
		regexp.MustCompile("\\.d\\.\\.\t.*\t30\tdirect\n"),
		// direct1 is now rename-from
		regexp.MustCompile("\\.d\\.r\t.*\t30\tdirect1\n"),
		// direct2 is now rename-to
		regexp.MustCompile("F\\.\\.r\t.*\t30\tdirect2\n"),

		// hashchange deleted
		regexp.MustCompile("\\.d\\.\\.\t.*\t30\thashchange\n"),
		// hashchange1 now rename-from
		regexp.MustCompile("\\.d\\.r\t.*\t30\thashchange1\n"),
		// hashchange2 now rename-to
		regexp.MustCompile("F\\.\\.r\t.*\t30\thashchange2\n"),
	}
	checkManifestMatches(t, testDir, "30", "test-bundle", res...)
}
