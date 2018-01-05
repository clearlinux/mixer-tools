package swupd

import (
	"bytes"
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

func TestCreateManifests(t *testing.T) {
	var err error
	testDir := mustSetupTestDir(t, "basic")
	defer removeIfNoErrors(t, testDir)
	mustInitStandardTest(t, testDir, "0", "10", []string{"test-bundle"})
	mustGenFile(t, testDir, "10", "test-bundle", "foo", "content")
	mustCreateManifestsStandard(t, 10, testDir)

	// set last version to 10
	mustInitStandardTest(t, testDir, "10", "20", []string{"test-bundle"})
	mustGenFile(t, testDir, "20", "test-bundle", "foo", "new content")
	mustCreateManifestsStandard(t, 20, testDir)

	lines := []struct {
		sub      []byte
		expected bool
	}{
		{[]byte("MANIFEST\t1"), true},
		{[]byte("version:\t10"), true},
		{[]byte("previous:\t0"), true},
		{[]byte("filecount:\t5"), true},
		{[]byte("timestamp:\t"), true},
		{[]byte("contentsize:\t"), true},
		{[]byte("includes:\tos-core"), true},
		{[]byte("10\t/foo"), true},
		{[]byte("\t0\t/foo"), false},
		{[]byte("10\t/usr/share"), true},
		{[]byte(".d..\t"), false},
	}

	// do not use the fileContains helper for this because we are checking a lot
	// of strings in the same file
	b, err := ioutil.ReadFile(filepath.Join(testDir, "www/10/Manifest.test-bundle"))
	if err != nil {
		t.Fatal("could not read test file for checking")
	}

	for _, l := range lines {
		if bytes.Contains(b, l.sub) != l.expected {
			if l.expected {
				t.Errorf("'%s' not found in 10/Manifest.test-bundle", l.sub)
			} else {
				t.Errorf("invalid '%s' found in 10/Manifest.test-bundle", l.sub)
			}
		}
	}

	lines20 := []struct {
		sub      []byte
		expected bool
	}{
		{[]byte("MANIFEST\t1"), true},
		{[]byte("version:\t20"), true},
		{[]byte("previous:\t10"), true},
		{[]byte("filecount:\t5"), true},
		{[]byte("includes:\tos-core"), true},
		{[]byte("20\t/foo"), true},
		{[]byte("10\t/foo"), false},
	}

	// do not use the fileContains helper for this because we are checking a lot
	// of strings in the same file
	b, err = ioutil.ReadFile(filepath.Join(testDir, "www/20/Manifest.test-bundle"))
	if err != nil {
		t.Fatal("could not read test file for checking")
	}

	for _, l := range lines20 {
		if bytes.Contains(b, l.sub) != l.expected {
			if l.expected {
				t.Errorf("'%s' not found in 20/Manifest.test-bundle", l.sub)
			} else {
				t.Errorf("invalid '%s' found in 20/Manifest.test-bundle", l.sub)
			}
		}
	}
}

func TestCreateManifestsDeleteNoVerBump(t *testing.T) {
	testDir := mustSetupTestDir(t, "deletenoverbump")
	defer removeIfNoErrors(t, testDir)
	mustInitStandardTest(t, testDir, "0", "10", []string{"test-bundle1", "test-bundle2"})
	mustGenFile(t, testDir, "10", "test-bundle1", "foo", "content")
	mustGenFile(t, testDir, "10", "test-bundle2", "foo", "content")
	mustCreateManifestsStandard(t, 10, testDir)

	mustInitStandardTest(t, testDir, "10", "20", []string{"test-bundle1", "test-bundle2"})
	mustGenFile(t, testDir, "20", "test-bundle1", "foo", "content")
	mustCreateManifestsStandard(t, 20, testDir)

	if !fileContains(filepath.Join(testDir, "www/10/Manifest.full"), []byte("10\t/foo")) {
		t.Error("'10\t/foo' not found in 10/Manifest.full")
	}

	if !fileContains(filepath.Join(testDir, "www/20/Manifest.full"), []byte("10\t/foo")) {
		t.Error("'10\t/foo' not found in 20/Manifest.full")
	}

	if fileContains(filepath.Join(testDir, "www/20/Manifest.full"), []byte("20\t/foo")) {
		t.Error("invalid '20\t/foo' found in 20/Manifest.full")
	}
}

func TestCreateManifestIllegalChar(t *testing.T) {
	testDir := mustSetupTestDir(t, "illegalfname")
	defer removeIfNoErrors(t, testDir)
	mustInitStandardTest(t, testDir, "0", "10", []string{})
	mustGenFile(t, testDir, "10", "os-core", "semicolon;", "")
	mustCreateManifestsStandard(t, 10, testDir)

	if fileContains(filepath.Join(testDir, "www/10/Manifest.os-core"), []byte("semicolon;")) {
		t.Error("illegal filename 'semicolon;' not blacklisted")
	}
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

	if !fileContains(filepath.Join(testDir, "www/10/Manifest.test-bundle"), []byte("/usr/bin/foobar")) {
		t.Error("non-debuginfo file '/usr/bin/foobar' banned")
	}

	if fileContains(filepath.Join(testDir, "www/10/Manifest.test-bundle"), []byte("/usr/lib/debug/foo")) {
		t.Error("debuginfo file '/usr/lib/debug/foo' not banned")
	}

	if fileContains(filepath.Join(testDir, "www/10/Manifest.test-bundle"), []byte("/usr/src/debug/bar")) {
		t.Error("debuginfo file '/usr/src/debug/bar' not banned")
	}
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
	mustCreateManifests(t, 20, true, 2, testDir)
}

func TestCreateManifestGhosted(t *testing.T) {
	testDir := mustSetupTestDir(t, "format")
	defer removeIfNoErrors(t, testDir)
	mustInitStandardTest(t, testDir, "0", "10", []string{"test-bundle"})
	mustGenFile(t, testDir, "10", "test-bundle", "usr/lib/kernel/bar", "bar")
	mustCreateManifestsStandard(t, 10, testDir)

	mustInitStandardTest(t, testDir, "10", "20", []string{"test-bundle"})
	mustGenFile(t, testDir, "20", "test-bundle", "usr/lib/kernel/baz", "baz")
	mustCreateManifestsStandard(t, 20, testDir)

	mustInitStandardTest(t, testDir, "20", "30", []string{"test-bundle"})
	mustCreateManifestsStandard(t, 30, testDir)

	re := regexp.MustCompile("F\\.b\\.\t.*\t10\t/usr/lib/kernel/bar")
	if fileContainsRe(filepath.Join(testDir, "www/10/Manifest.full"), re) == "" {
		t.Errorf("%v not found in 10/Manifest.full", re.String())
	}

	re = regexp.MustCompile("\\.gb\\.\t.*\t20\t/usr/lib/kernel/bar")
	if fileContainsRe(filepath.Join(testDir, "www/20/Manifest.full"), re) == "" {
		t.Errorf("%v not found in 20/Manifest.full", re.String())
	}

	re = regexp.MustCompile("F\\.b\\.\t.*\t20\t/usr/lib/kernel/baz")
	if fileContainsRe(filepath.Join(testDir, "www/20/Manifest.full"), re) == "" {
		t.Errorf("%v not found in 20/Manifest.full", re.String())
	}

	if fileContains(filepath.Join(testDir, "www/30/Manifest.full"), []byte("/usr/lib/kernel/bar")) {
		t.Error("/usr/lib/kernel/bar not cleaned up in 30/Manifest.full")
	}

	re = regexp.MustCompile("\\.gb\\.\t.*\t30\t/usr/lib/kernel/baz")
	if fileContainsRe(filepath.Join(testDir, "www/30/Manifest.full"), re) == "" {
		t.Errorf("%v not found in 30/Manifest.full", re.String())
	}
}

func TestCreateManifestIncludesDeduplicate(t *testing.T) {
	testDir := mustSetupTestDir(t, "includes-dedup")
	defer removeIfNoErrors(t, testDir)
	mustInitStandardTest(t, testDir, "0", "10", []string{"test-bundle1", "test-bundle2"})
	mustInitIncludesFile(t, testDir, "10", "test-bundle2", []string{"test-bundle1", "test-bundle1"})
	mustGenFile(t, testDir, "10", "test-bundle1", "test1", "test1")
	mustGenFile(t, testDir, "10", "test-bundle2", "test2", "test2")
	mustCreateManifestsStandard(t, 10, testDir)

	dualIncludes := []byte("includes:\ttest-bundle1\nincludes:\ttest-bundle1")
	if fileContains(filepath.Join(testDir, "www/10/Manifest.test-bundle2"), dualIncludes) {
		t.Error("includes not deduplicated for version 10")
	}

	mustInitStandardTest(t, testDir, "10", "20", []string{"test-bundle1", "test-bundle2"})
	mustInitIncludesFile(t, testDir, "20", "test-bundle2", []string{"test-bundle1", "test-bundle1"})
	mustCreateManifestsStandard(t, 20, testDir)

	if fileContains(filepath.Join(testDir, "www/20/Manifest.test-bundle2"), dualIncludes) {
		t.Error("includes not deduplicated for version 20")
	}
}

func TestCreateManifestDeletes(t *testing.T) {
	testDir := mustSetupTestDir(t, "deletes")
	defer removeIfNoErrors(t, testDir)
	mustInitStandardTest(t, testDir, "0", "10", []string{"test-bundle"})
	mustGenFile(t, testDir, "10", "test-bundle", "test", "test")
	mustCreateManifestsStandard(t, 10, testDir)

	mustInitStandardTest(t, testDir, "10", "20", []string{"test-bundle"})
	mustCreateManifestsStandard(t, 20, testDir)

	deletedLine := []byte(".d..\t" + AllZeroHash + "\t20\t/test")
	if !fileContains(filepath.Join(testDir, "www/20/Manifest.test-bundle"), deletedLine) {
		t.Error("file not properly deleted")
	}
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
	for _, s := range expected {
		if !fileContains(filepath.Join(testDir, "www/10/Manifest.test-bundle"), []byte(s)) {
			t.Errorf("Manifest.test-bundle did not include expected string '%s'", s)
		}
	}

	mustInitStandardTest(t, testDir, "10", "20", bundles)
	// same file as last update, now manifest creation must be triggered by new includes
	mustGenFile(t, testDir, "20", "test-bundle", "foo", "foo")
	mustGenFile(t, testDir, "20", "included", "bar", "bar")
	mustGenFile(t, testDir, "20", "included2", "baz", "baz")
	mustInitIncludesFile(t, testDir, "20", "test-bundle", []string{"included", "included2"})
	mustCreateManifestsStandard(t, 20, testDir)

	cases := []struct {
		exp   string
		bname string
	}{
		{"includes:\tos-core\n", "test-bundle"},
		{"includes:\tincluded\n", "test-bundle"},
		{"includes:\tincluded2\n", "test-bundle"},
		{"20\t/bar\n", "included"},
		{"20\t/baz\n", "included2"},
	}
	for _, tc := range cases {
		if !fileContains(filepath.Join(testDir, "www/20/Manifest."+tc.bname), []byte(tc.exp)) {
			t.Errorf("Manifest.%s did not include expected string '%s'", tc.bname, tc.exp)
		}
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
		exp   string
		bname string
	}{
		{"includes:\tincluded-nested\n", "included"},
		{"30\t/foobarbaz\n", "included-nested"},
		{"20\t/bar\n", "included"},
	}
	for _, tc := range cases {
		if !fileContains(filepath.Join(testDir, "www/30/Manifest."+tc.bname), []byte(tc.exp)) {
			t.Errorf("Manifest.%s did not include expected string '%s'", tc.bname, tc.exp)
		}
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

	re := regexp.MustCompile("D\\.s\\.\t.*\t10\t/var/lib\n")
	if fileContainsRe(filepath.Join(testDir, "www/10/Manifest.os-core"), re) == "" {
		t.Errorf("%v not found in 10/Manifest.os-core", re.String())
	}

	re = regexp.MustCompile("F\\.s\\.\t.*\t10\t/var/lib/test\n")
	if fileContainsRe(filepath.Join(testDir, "www/10/Manifest.os-core"), re) == "" {
		t.Errorf("%v not found in 10/Manifest.os-core", re.String())
	}
}

func TestCreateManifestsEmptyDir(t *testing.T) {
	testDir := mustSetupTestDir(t, "emptydir")
	defer removeIfNoErrors(t, testDir)
	mustInitStandardTest(t, testDir, "0", "10", []string{})
	mustGenBundleDir(t, testDir, "10", "os-core", "emptydir")
	mustCreateManifestsStandard(t, 10, testDir)

	re := regexp.MustCompile("D\\.\\.\\.\t.*\t10\t/emptydir\n")
	if fileContainsRe(filepath.Join(testDir, "www/10/Manifest.os-core"), re) == "" {
		t.Errorf("%v not found in 10/Manifest.os-core", re.String())
	}
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
	for _, s := range subs {
		if !fileContains(filepath.Join(testDir, "www/10/Manifest.MoM"), []byte(s)) {
			t.Errorf("10/Manifest.MoM did not contain expected '%s'", s)
		}
	}

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
	for _, s := range subs {
		if !fileContains(filepath.Join(testDir, "www/20/Manifest.MoM"), []byte(s)) {
			t.Errorf("20/Manifest.MoM did not contain expected '%s'", s)
		}
	}

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
	for _, s := range subs {
		if !fileContains(filepath.Join(testDir, "www/30/Manifest.MoM"), []byte(s)) {
			t.Errorf("30/Manifest.MoM did not contain expected '%s'", s)
		}
	}

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
	for _, s := range subs {
		if !fileContains(filepath.Join(testDir, "www/40/Manifest.MoM"), []byte(s)) {
			t.Errorf("40/Manifest.MoM did not contain expected '%s'", s)
		}
	}
}
