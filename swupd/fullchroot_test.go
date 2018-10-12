package swupd

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

func TestSyncToFull(t *testing.T) {
	d, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatal("couldn't create test directory")
	}

	if err = os.MkdirAll(filepath.Join(d, "10/testbundle/test"), 0755); err != nil {
		t.Fatal("couldn't create bundle test directory")
	}

	if err = syncToFull(10, "testbundle", d); err != nil {
		t.Errorf("syncToFull failed with valid input: %s", err)
	}

	if _, err = os.Stat(filepath.Join(d, "10/full/test")); err != nil {
		t.Errorf("syncToFull failed to sync test file to full chroot: %s", err)
	}

}
