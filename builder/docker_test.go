package builder

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestGetDirFromConfigPath(t *testing.T) {
	tests := []struct {
		Filepath        string
		ExpectedDirPath string
		ShouldFail      bool
	}{
		{
			Filepath:        "some/path/to/directory",
			ExpectedDirPath: "some/path/to/directory",
		},
		{
			Filepath:        "some/path/to/file",
			ExpectedDirPath: "some/path/to",
		},
		{
			Filepath:        "some/path/to/doesnotexist",
			ExpectedDirPath: "some/path/to",
		},

		// Error cases.
		{Filepath: "not/a/real/path", ShouldFail: true},
	}

	testDir, err := ioutil.TempDir("", "getdirfrompath-test-")
	if err != nil {
		t.Fatalf("couldn't create temporary directory to write test cases: %s", err)
	}
	defer func() {
		_ = os.RemoveAll(testDir)
	}()

	// Set up the dirs/files for testing
	path := filepath.Join(testDir, "some/path/to/directory")
	if err = os.MkdirAll(path, 0777); err != nil {
		t.Fatalf("Failed to create test directory: %q\n%s", path, err)
	}
	path = filepath.Join(testDir, "some/path/to/file") // "some/path/to" already exists from previous command
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("Failed to create test file: %q\n%s", path, err)
	}
	defer func() {
		_ = f.Close()
	}()

	for _, tt := range tests {
		fullPath := filepath.Join(testDir, tt.Filepath)

		dir, err := getDirFromConfigPath(fullPath)
		dir = strings.TrimPrefix(dir, testDir+"/")
		failed := err != nil
		if failed != tt.ShouldFail {
			if tt.ShouldFail {
				t.Errorf("unexpected success when parsing filepath\nFILEPATH: %s\nPARSED DIR: %s\n", tt.Filepath, dir)
			} else {
				t.Errorf("unexpected error parsing bundle: %s\nFILEPATH: %s\nEXPECTED DIR: %s\n", err, tt.Filepath, tt.ExpectedDirPath)
			}
			continue
		}
		if tt.ShouldFail {
			continue
		}

		if !reflect.DeepEqual(dir, tt.ExpectedDirPath) {
			t.Errorf("got wrong dir when parsing filepath\nFILEPATH: %s\nPARSED DIR: %s\nEXPECTED DIR: %s", tt.Filepath, dir, tt.ExpectedDirPath)
		}
	}
}

func TestCanAccess(t *testing.T) {
	tests := []struct {
		Filepath   string
		ShouldFail bool
	}{
		{Filepath: "shouldwork"},

		// Error cases.
		{Filepath: "noread", ShouldFail: true},
		{Filepath: "nowrite", ShouldFail: true},
		{Filepath: "noreadwrite", ShouldFail: true},
	}

	testDir, err := ioutil.TempDir("", "canaccess-test-")
	if err != nil {
		t.Fatalf("couldn't create temporary directory to write test cases: %s", err)
	}
	defer func() {
		_ = os.RemoveAll(testDir)
	}()

	// Set up the dirs/files for testing
	path := filepath.Join(testDir, "shouldwork")
	if err := os.Mkdir(path, 0777); err != nil {
		t.Fatalf("Failed to create test directory: %q\n%s", path, err)
	}
	path = filepath.Join(testDir, "noread")
	if err := os.Mkdir(path, 0333); err != nil {
		t.Fatalf("Failed to create test directory: %q\n%s", path, err)
	}
	path = filepath.Join(testDir, "nowrite")
	if err := os.Mkdir(path, 0555); err != nil {
		t.Fatalf("Failed to create test directory: %q\n%s", path, err)
	}
	path = filepath.Join(testDir, "noreadwrite")
	if err := os.Mkdir(path, 0111); err != nil {
		t.Fatalf("Failed to create test directory: %q\n%s", path, err)
	}

	for _, tt := range tests {
		err := canAccess(filepath.Join(testDir, tt.Filepath))
		failed := err != nil
		if failed != tt.ShouldFail {
			if tt.ShouldFail {
				t.Errorf("unexpected success when checking access\nFILEPATH: %s\n", tt.Filepath)
			} else {
				t.Errorf("unexpected error when checking access: %s\nFILEPATH: %s\n", err, tt.Filepath)
			}
		}
	}
}

func TestReduceDockerMounts(t *testing.T) {
	tests := []struct {
		Mounts         []string
		ExpectedMounts []string
	}{
		{ // Test /foo matches as parent to /foo/bar but not /foobar
			Mounts:         []string{"/foo", "/foo/bar", "/foobar"},
			ExpectedMounts: []string{"/foo", "/foobar"},
		},
		{ // Test order doesn't matter
			Mounts:         []string{"/foo/bar", "/foo"},
			ExpectedMounts: []string{"/foo"},
		},
		{ // Test duplicates
			Mounts:         []string{"/foo", "/foo"},
			ExpectedMounts: []string{"/foo"},
		},
		{ // Test ancestor (not just parent)
			Mounts:         []string{"/foo", "/foo/bar/baz"},
			ExpectedMounts: []string{"/foo"},
		},
	}

	for _, tt := range tests {
		mounts := reduceDockerMounts(tt.Mounts)

		if !reflect.DeepEqual(mounts, tt.ExpectedMounts) {
			t.Errorf("got wrong mounts when reducing mounts\nMOUNTS:\n%v\nREDUCED MOUNTS:\n%v\nEXPECTED MOUNTS:\n%v\n", tt.Mounts, mounts, tt.ExpectedMounts)
		}
	}
}
