package builder

import (
	"io/ioutil"
	"os"
	"path"
	"reflect"
	"testing"
)

func TestAddContentChroots(t *testing.T) {
	testWorkspace, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("could not create valid test dir: %s", err.Error())
	}
	defer func() { _ = os.RemoveAll(testWorkspace) }()

	// passChroot1: new file/directory, existing file/directory, file/directory symlinks
	passChroot1 := path.Join(testWorkspace, "passChroot1")
	if err := os.MkdirAll(path.Join(passChroot1, "usr/unique"), 0755); err != nil {
		t.Fatalf("could not create valid test dir: %s", err.Error())
	}
	if err := ioutil.WriteFile(path.Join(passChroot1, "file"), []byte("foo"), 0755); err != nil {
		t.Fatalf("could not create valid test file: %s", err.Error())
	}
	if err := ioutil.WriteFile(path.Join(passChroot1, "uniqueFile"), []byte("foo"), 0755); err != nil {
		t.Fatalf("could not create valid test file: %s", err.Error())
	}
	if err := os.Symlink(path.Join(passChroot1, "file"), path.Join(passChroot1, "fileLink")); err != nil {
		t.Fatalf("could not create valid test file: %s", err.Error())
	}
	if err := os.Symlink(path.Join(passChroot1, "usr"), path.Join(passChroot1, "dirLink")); err != nil {
		t.Fatalf("could not create valid test file: %s", err.Error())
	}

	// passChroot2: new directory
	passChroot2 := (path.Join(testWorkspace, "passChroot2"))
	if err := os.MkdirAll(path.Join(passChroot2, "usr/unique2"), 0755); err != nil {
		t.Fatalf("could not create valid test dir: %s", err.Error())
	}

	// passChroot3: new directory
	passChroot3 := (path.Join(testWorkspace, "passChroot3"))
	if err := os.MkdirAll(path.Join(passChroot3, "usr/unique3"), 0755); err != nil {
		t.Fatalf("could not create valid test dir: %s", err.Error())
	}

	// failChroot1: file conflict
	failChroot1 := (path.Join(testWorkspace, "failChroot1"))
	if err := os.MkdirAll(failChroot1, 0755); err != nil {
		t.Fatalf("could not create valid testdir")
	}
	if err := ioutil.WriteFile(path.Join(failChroot1, "file"), []byte("invalid"), 0755); err != nil {
		t.Fatalf("could not create valid test file")
	}

	// failChroot2: directory conflict, different permissions
	failChroot2 := (path.Join(testWorkspace, "failChroot2"))
	if err := os.MkdirAll(path.Join(failChroot2, "usr"), 0744); err != nil {
		t.Fatalf("could not create valid testdir")
	}

	tests := []struct {
		name        string
		set         *bundleSet
		expectedSet *bundleSet
		shouldFail  bool
	}{
		{
			name: "Passing with 2 bundles with multiple content chroots",
			set: &bundleSet{
				"bundle1": &bundle{
					ContentChroots: map[string]bool{
						passChroot1: true,
						passChroot2: true,
					},
					Files: map[string]bool{},
				},
				"bundle2": &bundle{
					ContentChroots: map[string]bool{passChroot3: true},
					Files:          map[string]bool{},
				},
			},
			expectedSet: &bundleSet{
				"bundle1": &bundle{
					Files: map[string]bool{
						"/usr": true, "/usr/unique": true, "/file": true, "/uniqueFile": true,
						"/fileLink": true, "/dirLink": true, "/usr/unique2": true,
					},
				},
				"bundle2": &bundle{
					Files: map[string]bool{"/usr": true, "/usr/unique3": true},
				},
			},
			shouldFail: false,
		},
		{
			name: "Failing with file conflict",
			set: &bundleSet{
				"bundle1": &bundle{
					ContentChroots: map[string]bool{
						failChroot1: true,
					},
					Files: map[string]bool{},
				},
			},
			shouldFail: true,
		},
		{
			name: "Failing with directory conflict",
			set: &bundleSet{
				"bundle1": &bundle{
					ContentChroots: map[string]bool{
						failChroot2: true,
					},
					Files: map[string]bool{},
				},
			},
			shouldFail: true,
		},
	}

	for _, tt := range tests {
		fullDir, err := createTestFullChroot(testWorkspace)
		if err != nil {
			t.Fatalf("could not create valid test dir: %s", err.Error())
		}

		err = addBundleContentChroots(tt.set, fullDir)

		if tt.shouldFail {
			if err == nil {
				t.Errorf("%s: unexpected success", tt.name)
			}
			continue
		} else if err != nil {
			t.Errorf("%s: unexpected error: %s", tt.name, err)
		}

		for name, bundle := range *tt.set {
			if !reflect.DeepEqual(bundle.Files, (*tt.expectedSet)[name].Files) {
				t.Errorf("%s: Failed\nCHROOT FILES (%d):\n%v\nEXPECTED FILES (%d):\n%v",
					tt.name,
					len(bundle.Files),
					bundle.Files,
					len((*tt.expectedSet)[name].Files),
					(*tt.expectedSet)[name].Files)
			}
		}
	}
}

func createTestFullChroot(workspace string) (string, error) {
	fullDir := path.Join(workspace, "full")
	if _, err := os.Stat(fullDir); err == nil {
		if err = os.RemoveAll(fullDir); err != nil {
			return "", err
		}
	}

	// full chroot with a directory and file
	if err := os.MkdirAll(path.Join(fullDir, "usr"), 0755); err != nil {
		return "", err
	}
	if err := ioutil.WriteFile(path.Join(fullDir, "file"), []byte("foo"), 0755); err != nil {
		return "", err
	}
	return fullDir, nil
}
