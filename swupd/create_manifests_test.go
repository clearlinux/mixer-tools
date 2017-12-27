package swupd

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func dirExistsWithPerm(path string, perm os.FileMode) bool {
	var err error
	var info os.FileInfo
	if info, err = os.Stat(path); err != nil {
		// assume it doesn't exist here
		return false
	}

	// check if it is a directory or the perms don't match
	if !info.Mode().IsDir() || info.Mode().Perm() != perm {
		return false
	}

	return true
}

func TestInitBuildEnv(t *testing.T) {
	var err error
	tmpStateDir := StateDir
	defer func() {
		StateDir = tmpStateDir
	}()

	if StateDir, err = ioutil.TempDir("testdata", "state"); err != nil {
		t.Fatalf("Could not initialize state dir for testing: %v", err)
	}

	defer os.RemoveAll(StateDir)

	if err = initBuildEnv(); err != nil {
		t.Errorf("initBuildEnv raised unexpected error: %v", err)
	}

	if !exists(filepath.Join(StateDir, "temp")) {
		t.Error("initBuildEnv failed to set up temporary directory")
	}
}

func TestInitBuildDirs(t *testing.T) {
	var err error
	bundles := []string{"os-core", "os-core-update", "test-bundle"}
	c := getConfig()
	if c.imageBase, err = ioutil.TempDir("testdata", "image"); err != nil {
		t.Fatalf("Could not initialize image dir for testing: %v", err)
	}

	defer os.RemoveAll(c.imageBase)

	if err = initBuildDirs(10, bundles, c.imageBase); err != nil {
		t.Errorf("initBuildDirs raised unexpected error: %v", err)
	}

	if !dirExistsWithPerm(filepath.Join(c.imageBase, "10"), 0755) {
		t.Errorf("%v does not exist with correct perms", filepath.Join(c.imageBase, "10"))
	}

	for _, dir := range bundles {
		if !dirExistsWithPerm(filepath.Join(c.imageBase, "10", dir), 0755) {
			t.Errorf("%v does not exist with correct perms", filepath.Join(c.imageBase, "10", dir))
		}
	}
}

func fileContains(path string, sub string) bool {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return false
	}

	return strings.Contains(string(b), sub)
}

func fileContainsRe(path string, re *regexp.Regexp) string {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return ""
	}

	return re.FindString(string(b))
}

func TestCreateManifests(t *testing.T) {
	testDir := "./testdata/testdata-basic"
	// init test dir
	if err := os.RemoveAll(testDir); err != nil {
		t.Fatalf("Unable to remove %s", testDir)
	}

	cmd := exec.Command("cp", "-a", testDir+".bak", testDir)
	if err := cmd.Run(); err != nil {
		t.Fatalf("Unable to copy %s.bak to %s", testDir, testDir)
	}

	if err := CreateManifests(10, false, 1, testDir); err != nil {
		t.Error(err)
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
		sub      string
		expected bool
	}{
		{"MANIFEST\t1", true},
		{"version:\t10", true},
		{"previous:\t0", true},
		{"filecount:\t5", true},
		{"timestamp:\t", true},
		{"contentsize:\t", true},
		{"includes:\tos-core", true},
		{"10\t/foo", true},
		{"\t0\t/foo", false},
		{"10\t/usr/share", true},
		{".d..\t", false},
	}

	// do not use the fileContains helper for this because we are checking a lot
	// of strings in the same file
	b, err := ioutil.ReadFile(filepath.Join(testDir, "www/10/Manifest.test-bundle"))
	if err != nil {
		t.Fatal("could not read test file for checking")
	}

	s := string(b)
	for _, l := range lines {
		if strings.Contains(s, l.sub) != l.expected {
			if l.expected {
				t.Errorf("'%s' not found in 10/Manifest.test-bundle", l.sub)
			} else {
				t.Errorf("invalid '%s' found in 10/Manifest.test-bundle", l.sub)
			}
		}
	}

	lines20 := []struct {
		sub      string
		expected bool
	}{
		{"MANIFEST\t1", true},
		{"version:\t20", true},
		{"previous:\t10", true},
		{"filecount:\t5", true},
		{"includes:\tos-core", true},
		{"20\t/foo", true},
		{"10\t/foo", false},
	}

	// do not use the fileContains helper for this because we are checking a lot
	// of strings in the same file
	b, err = ioutil.ReadFile(filepath.Join(testDir, "www/20/Manifest.test-bundle"))
	if err != nil {
		t.Fatal("could not read test file for checking")
	}

	s = string(b)
	for _, l := range lines20 {
		if strings.Contains(s, l.sub) != l.expected {
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
	if err := os.RemoveAll(testDir); err != nil {
		t.Fatal("unable to remove " + testDir)
	}

	cmd := exec.Command("cp", "-a", testDir+".bak", testDir)
	if err := cmd.Run(); err != nil {
		t.Fatalf("unable to copy %s.bak to %s", testDir, testDir)
	}

	if err := CreateManifests(10, false, 1, testDir); err != nil {
		t.Error(err)
	}

	ver := []byte("10\n")
	if err := ioutil.WriteFile(filepath.Join(testDir, "image", "LAST_VER"), ver, 0755); err != nil {
		t.Fatal(err)
	}

	if err := CreateManifests(20, false, 1, testDir); err != nil {
		t.Error(err)
	}

	if !fileContains(filepath.Join(testDir, "www/10/Manifest.full"), "10\t/foo") {
		t.Error("'10\t/foo' not found in 10/Manifest.full")
	}

	if !fileContains(filepath.Join(testDir, "www/20/Manifest.full"), "10\t/foo") {
		t.Error("'10\t/foo' not found in 20/Manifest.full")
	}

	if fileContains(filepath.Join(testDir, "www/20/Manifest.full"), "20\t/foo") {
		t.Error("invalid '20\t/foo' found in 20/Manifest.full")
	}
}

func TestCreateManifestIllegalChar(t *testing.T) {
	testDir := "./testdata/testdata-filename-blacklisted"
	if err := os.RemoveAll(testDir); err != nil {
		t.Fatal("unable to remove " + testDir)
	}

	cmd := exec.Command("cp", "-a", testDir+".bak", testDir)
	if err := cmd.Run(); err != nil {
		t.Fatalf("unable to copy %s.bak to %s", testDir, testDir)
	}

	if err := CreateManifests(10, false, 1, testDir); err != nil {
		t.Error(err)
	}

	if fileContains(filepath.Join(testDir, "www/10/Manifest.os-core"), "semicolon;") {
		t.Error("illegal filename 'semicolon;' not blacklisted")
	}
}

func TestCreateManifestDebuginfo(t *testing.T) {
	testDir := "./testdata/testdata-filename-debuginfo"
	if err := os.RemoveAll(testDir); err != nil {
		t.Fatal("unable to remove " + testDir)
	}

	cmd := exec.Command("cp", "-a", testDir+".bak", testDir)
	if err := cmd.Run(); err != nil {
		t.Fatalf("unable to copy %s.bak to %s", testDir, testDir)
	}

	if err := CreateManifests(10, false, 1, testDir); err != nil {
		t.Error(err)
	}

	if fileContains(filepath.Join(testDir, "www/10/Manifest.test-bundle"), "/usr/lib/debug/foo") {
		t.Error("debuginfo file '/usr/lib/debug/foo' not banned")
	}

	if fileContains(filepath.Join(testDir, "www/10/Manifest.test-bundle"), "/usr/src/debug/bar") {
		t.Error("debuginfo file '/usr/src/debug/bar' not banned")
	}
}

func TestCreateManifestFormatNoDecrement(t *testing.T) {
	testDir := "./testdata/testdata-format-no-decrement"
	if err := os.RemoveAll(testDir); err != nil {
		t.Fatal("unable to remove " + testDir)
	}

	cmd := exec.Command("cp", "-a", "./testdata/testdata-format.bak", testDir)
	if err := cmd.Run(); err != nil {
		t.Fatalf("unable to copy %s.bak to %s", testDir, testDir)
	}

	cmd = exec.Command("cp", "./testdata/format-no-dec-server.ini", testDir)
	if err := cmd.Run(); err != nil {
		t.Fatalf("unable to copy server.ini to %s", testDir)
	}

	if err := CreateManifests(10, false, 2, testDir); err != nil {
		t.Error(err)
	}

	ver := []byte("10\n")
	if err := ioutil.WriteFile(filepath.Join(testDir, "image", "LAST_VER"), ver, 0755); err != nil {
		t.Fatal(err)
	}

	if err := CreateManifests(20, true, 1, testDir); err == nil {
		t.Error("CreateManifests successful when decrementing format")
	}
}

func TestCreateManifestFormat(t *testing.T) {
	testDir := "./testdata/testdata-format"
	if err := os.RemoveAll(testDir); err != nil {
		t.Fatal("unable to remove " + testDir)
	}

	cmd := exec.Command("cp", "-a", testDir+".bak", testDir)
	if err := cmd.Run(); err != nil {
		t.Fatalf("unable to copy %s.bak to %s", testDir, testDir)
	}

	if err := CreateManifests(10, false, 1, testDir); err != nil {
		t.Error(err)
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
	if err := os.RemoveAll(testDir); err != nil {
		t.Fatal("unable to remove " + testDir)
	}

	cmd := exec.Command("cp", "-a", testDir+".bak", testDir)
	if err := cmd.Run(); err != nil {
		t.Fatalf("unable to copy %s.bak to %s", testDir, testDir)
	}

	if err := CreateManifests(10, false, 1, testDir); err != nil {
		t.Error(err)
	}

	ver := []byte("10\n")
	if err := ioutil.WriteFile(filepath.Join(testDir, "image", "LAST_VER"), ver, 0755); err != nil {
		t.Fatal(err)
	}

	if err := CreateManifests(20, false, 1, testDir); err != nil {
		t.Error(err)
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

	if fileContains(filepath.Join(testDir, "www/30/Manifest.full"), "/usr/lib/kernel/bar") {
		t.Error("/usr/lib/kernel/bar not cleaned up in 30/Manifest.full")
	}

	re = regexp.MustCompile("\\.gb\\.\t.*\t30\t/usr/lib/kernel/baz")
	if fileContainsRe(filepath.Join(testDir, "www/30/Manifest.full"), re) == "" {
		t.Errorf("%v not found in 30/Manifest.full", re.String())
	}
}
