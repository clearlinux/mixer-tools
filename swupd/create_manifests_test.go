package swupd

import (
	"bytes"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"
)

func mustDirExistsWithPerm(t *testing.T, path string, perm os.FileMode) {
	var err error
	var info os.FileInfo
	if info, err = os.Stat(path); err != nil {
		t.Fatal(err)
	}

	// check if it is a directory or the perms don't match
	if !info.Mode().IsDir() || info.Mode().Perm() != perm {
		t.Fatal(err)
	}
}

func TestInitBuildEnv(t *testing.T) {
	var err error
	var sdir string
	if sdir, err = ioutil.TempDir("testdata", "state"); err != nil {
		t.Fatalf("Could not initialize state dir for testing: %v", err)
	}

	defer os.RemoveAll(sdir)

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

	defer os.RemoveAll(c.imageBase)

	if err = initBuildDirs(10, bundles, c.imageBase); err != nil {
		t.Errorf("initBuildDirs raised unexpected error: %v", err)
	}

	mustDirExistsWithPerm(t, filepath.Join(c.imageBase, "10"), 0755)

	for _, dir := range bundles {
		mustDirExistsWithPerm(t, filepath.Join(c.imageBase, "10", dir), 0755)
	}
}

func fileContains(path string, sub []byte) bool {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return false
	}

	return bytes.Contains(b, sub)
}

func fileContainsRe(path string, re *regexp.Regexp) string {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return ""
	}

	return string(re.Find(b))
}

func mustCopyDir(t *testing.T, testDir string) {
	cmd := exec.Command("cp", "-a", testDir+".in", testDir)
	if err := cmd.Run(); err != nil {
		t.Fatalf("Unable to copy %s.in to %s", testDir, testDir)
	}
}

func mustRemoveAll(t *testing.T, testDir string) {
	if err := os.RemoveAll(testDir); err != nil {
		t.Fatalf("unable to remove old %s for testing", testDir)
	}
}

func TestCreateManifests(t *testing.T) {
	testDir := "./testdata/testdata-basic"
	mustRemoveAll(t, testDir)
	mustCopyDir(t, testDir)
	if err := CreateManifests(10, false, 1, testDir); err != nil {
		t.Fatal(err)
	}

	// set last version to 10
	ver := []byte("10\n")
	if err := ioutil.WriteFile(filepath.Join(testDir, "image/LAST_VER"), ver, 0755); err != nil {
		t.Fatal(err)
	}

	if err := CreateManifests(20, false, 1, testDir); err != nil {
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
	// init test dir
	testDir := "./testdata/testdata-delete-no-version-bump"
	mustRemoveAll(t, testDir)
	mustCopyDir(t, testDir)
	if err := CreateManifests(10, false, 1, testDir); err != nil {
		t.Fatal(err)
	}

	ver := []byte("10\n")
	if err := ioutil.WriteFile(filepath.Join(testDir, "image", "LAST_VER"), ver, 0755); err != nil {
		t.Fatal(err)
	}

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
	testDir := "./testdata/testdata-filename-blacklisted"
	mustRemoveAll(t, testDir)
	mustCopyDir(t, testDir)
	if err := CreateManifests(10, false, 1, testDir); err != nil {
		t.Error(err)
	}

	if fileContains(filepath.Join(testDir, "www/10/Manifest.os-core"), []byte("semicolon;")) {
		t.Error("illegal filename 'semicolon;' not blacklisted")
	}
}

func TestCreateManifestDebuginfo(t *testing.T) {
	testDir := "./testdata/testdata-filename-debuginfo"
	mustRemoveAll(t, testDir)
	mustCopyDir(t, testDir)
	if err := CreateManifests(10, false, 1, testDir); err != nil {
		t.Error(err)
	}

	if fileContains(filepath.Join(testDir, "www/10/Manifest.test-bundle"), []byte("/usr/lib/debug/foo")) {
		t.Error("debuginfo file '/usr/lib/debug/foo' not banned")
	}

	if fileContains(filepath.Join(testDir, "www/10/Manifest.test-bundle"), []byte("/usr/src/debug/bar")) {
		t.Error("debuginfo file '/usr/src/debug/bar' not banned")
	}
}

func TestCreateManifestFormatNoDecrement(t *testing.T) {
	testDir := "./testdata/testdata-format-no-decrement"
	mustRemoveAll(t, testDir)
	cmd := exec.Command("cp", "-a", "./testdata/testdata-format.in", testDir)
	if err := cmd.Run(); err != nil {
		t.Fatalf("unable to copy %s.in to %s", testDir, testDir)
	}

	cmd = exec.Command("/bin/cp", "./testdata/format-no-dec-server.ini",
		testDir+"/server.ini")
	if err := cmd.Run(); err != nil {
		t.Fatalf("unable to copy server.ini to %s", testDir)
	}

	if err := CreateManifests(10, false, 2, testDir); err != nil {
		t.Fatal(err)
	}

	ver := []byte("10\n")
	if err := ioutil.WriteFile(filepath.Join(testDir, "image", "LAST_VER"), ver, 0755); err != nil {
		t.Fatal(err)
	}

	if err := CreateManifests(20, true, 1, testDir); err == nil {
		t.Error("CreateManifests successful when decrementing format (error expected)")
	}
}

func TestCreateManifestFormat(t *testing.T) {
	testDir := "./testdata/testdata-format"
	mustRemoveAll(t, testDir)
	mustCopyDir(t, testDir)
	if err := CreateManifests(10, false, 1, testDir); err != nil {
		t.Fatal(err)
	}

	ver := []byte("10\n")
	if err := ioutil.WriteFile(filepath.Join(testDir, "image", "LAST_VER"), ver, 0755); err != nil {
		t.Fatal(err)
	}

	if err := CreateManifests(20, true, 2, testDir); err != nil {
		t.Error(err)
	}
}

func TestCreateManifestGhosted(t *testing.T) {
	testDir := "./testdata/testdata-ghosting"
	mustRemoveAll(t, testDir)
	mustCopyDir(t, testDir)
	if err := CreateManifests(10, false, 1, testDir); err != nil {
		t.Fatal(err)
	}

	ver := []byte("10\n")
	if err := ioutil.WriteFile(filepath.Join(testDir, "image", "LAST_VER"), ver, 0755); err != nil {
		t.Fatal(err)
	}

	if err := CreateManifests(20, false, 1, testDir); err != nil {
		t.Fatal(err)
	}

	ver = []byte("20\n")
	if err := ioutil.WriteFile(filepath.Join(testDir, "image", "LAST_VER"), ver, 0755); err != nil {
		t.Fatal(err)
	}

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
