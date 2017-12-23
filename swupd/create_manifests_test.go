package swupd

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
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

func TestCreateManifests(t *testing.T) {
	// init test dir
	if err := os.RemoveAll("./testdata/testdata-basic"); err != nil {
		t.Fatal("Unable to remove testdata/testdata-basic")
	}

	cmd := exec.Command("cp", "-a", "./testdata/testdata-basic.bak", "./testdata/testdata-basic")
	if err := cmd.Run(); err != nil {
		t.Fatal("Unable to copy testdata/testdata-basic.bak to testdata/testdata-basic")
	}

	if err := CreateManifests(10, false, 1, "./testdata/testdata-basic"); err != nil {
		t.Error(err)
	}

	// set last version to 10
	ver := []byte("10\n")
	if err := ioutil.WriteFile("./testdata/testdata-basic/image/LAST_VER", ver, 0755); err != nil {
		t.Fatal(err)
	}

	if err := CreateManifests(20, false, 1, "./testdata/testdata-basic"); err != nil {
		t.Error(err)
	}
}
