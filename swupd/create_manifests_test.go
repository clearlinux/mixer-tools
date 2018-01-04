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
	if err = CreateManifests(10, false, 1, testDir); err != nil {
		t.Fatal(err)
	}

	// set last version to 10
	mustInitStandardTest(t, testDir, "10", "20", []string{"test-bundle"})
	mustGenFile(t, testDir, "20", "test-bundle", "foo", "new content")
	if err = CreateManifests(20, false, 1, testDir); err != nil {
		t.Error(err)
	}

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
	if err := CreateManifests(10, false, 1, testDir); err != nil {
		t.Fatal(err)
	}

	mustInitStandardTest(t, testDir, "10", "20", []string{"test-bundle1", "test-bundle2"})
	mustGenFile(t, testDir, "20", "test-bundle1", "foo", "content")
	if err := CreateManifests(20, false, 1, testDir); err != nil {
		t.Error(err)
	}

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
	if err := CreateManifests(10, false, 1, testDir); err != nil {
		t.Error(err)
	}

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

	if err := CreateManifests(10, false, 1, testDir); err != nil {
		t.Error(err)
	}

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
	if err := CreateManifests(10, false, 2, testDir); err != nil {
		t.Fatal(err)
	}

	mustInitStandardTest(t, testDir, "10", "20", []string{})
	if err := CreateManifests(20, true, 1, testDir); err == nil {
		t.Error("CreateManifests successful when decrementing format (error expected)")
	}
}

func TestCreateManifestFormat(t *testing.T) {
	testDir := mustSetupTestDir(t, "format")
	defer removeIfNoErrors(t, testDir)
	mustInitStandardTest(t, testDir, "0", "10", []string{})
	mustGenFile(t, testDir, "10", "os-core", "baz", "bazcontent")
	mustGenFile(t, testDir, "10", "os-core", "foo", "foocontent")
	if err := CreateManifests(10, false, 1, testDir); err != nil {
		t.Fatal(err)
	}

	mustInitStandardTest(t, testDir, "10", "20", []string{})

	if err := CreateManifests(20, true, 2, testDir); err != nil {
		t.Error(err)
	}
}

func TestCreateManifestGhosted(t *testing.T) {
	testDir := mustSetupTestDir(t, "format")
	defer removeIfNoErrors(t, testDir)
	mustInitStandardTest(t, testDir, "0", "10", []string{"test-bundle"})
	mustGenFile(t, testDir, "10", "test-bundle", "usr/lib/kernel/bar", "bar")
	if err := CreateManifests(10, false, 1, testDir); err != nil {
		t.Fatal(err)
	}

	mustInitStandardTest(t, testDir, "10", "20", []string{"test-bundle"})
	mustGenFile(t, testDir, "20", "test-bundle", "usr/lib/kernel/baz", "baz")
	if err := CreateManifests(20, false, 1, testDir); err != nil {
		t.Fatal(err)
	}

	mustInitStandardTest(t, testDir, "20", "30", []string{"test-bundle"})
	if err := CreateManifests(30, false, 1, testDir); err != nil {
		t.Error(err)
	}

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

	if err := CreateManifests(10, false, 1, testDir); err != nil {
		t.Fatal(err)
	}

	dualIncludes := []byte("includes:\ttest-bundle1\nincludes:\ttest-bundle1")
	if fileContains(filepath.Join(testDir, "www/10/Manifest.test-bundle2"), dualIncludes) {
		t.Error("includes not deduplicated for version 10")
	}

	mustInitStandardTest(t, testDir, "10", "20", []string{"test-bundle1", "test-bundle2"})
	mustInitIncludesFile(t, testDir, "20", "test-bundle2", []string{"test-bundle1", "test-bundle1"})
	if err := CreateManifests(20, false, 1, testDir); err != nil {
		t.Fatal(err)
	}

	if fileContains(filepath.Join(testDir, "www/20/Manifest.test-bundle2"), dualIncludes) {
		t.Error("includes not deduplicated for version 20")
	}
}

func TestCreateManifestDeletes(t *testing.T) {
	testDir := mustSetupTestDir(t, "deletes")
	defer removeIfNoErrors(t, testDir)
	mustInitStandardTest(t, testDir, "0", "10", []string{"test-bundle"})
	mustGenFile(t, testDir, "10", "test-bundle", "test", "test")
	if err := CreateManifests(10, false, 1, testDir); err != nil {
		t.Fatal(err)
	}

	mustInitStandardTest(t, testDir, "10", "20", []string{"test-bundle"})
	if err := CreateManifests(20, false, 1, testDir); err != nil {
		t.Fatal(err)
	}

	deletedLine := []byte(".d..\t" + AllZeroHash + "\t20\t/test")
	if !fileContains(filepath.Join(testDir, "www/20/Manifest.test-bundle"), deletedLine) {
		t.Error("file not properly deleted")
	}
}
