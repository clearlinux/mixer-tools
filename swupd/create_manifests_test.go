package swupd

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

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
	tmpImageBase := imageBase
	defer func() {
		imageBase = tmpImageBase
	}()

	if imageBase, err = ioutil.TempDir("testdata", "image"); err != nil {
		t.Fatalf("Could not initialize image dir for testing: %v", err)
	}

	defer os.RemoveAll(imageBase)

	if err = initBuildDirs(10, bundles); err != nil {
		t.Errorf("initBuildDirs raised unexpected error: %v", err)
	}

	if !dirExistsWithPerm(filepath.Join(imageBase, "10"), 0755) {
		t.Errorf("%v does not exist with correct perms", filepath.Join(imageBase, "10"))
	}

	for _, dir := range bundles {
		if !dirExistsWithPerm(filepath.Join(imageBase, "10", dir), 0755) {
			t.Errorf("%v does not exist with correct perms", filepath.Join(imageBase, "10", dir))
		}
	}
}

func TestCreateManifests(t *testing.T) {
	// init test dir
	if err := os.RemoveAll("./testdata/test-web-dir"); err != nil {
		t.Fatal("Unable to remove testdata/test-web-dir")
	}

	cmd := exec.Command("cp", "-a", "./testdata/test-web-dir.bak", "./testdata/test-web-dir")
	if err := cmd.Run(); err != nil {
		t.Fatal("Unable to copy testdata/test-web-dir.bak to testdata/test-web-dir")
	}

	if err := CreateManifests(10, false, 1, "./testdata/test-web-dir"); err != nil {
		t.Error(err)
	}

	// set last version to 10
	ver := []byte("10\n")
	if err := ioutil.WriteFile("./testdata/test-web-dir/image/LAST_VER", ver, 0755); err != nil {
		t.Fatal(err)
	}

	if err := CreateManifests(20, false, 1, "./testdata/test-web-dir"); err != nil {
		t.Error(err)
	}
}
