package swupd

import (
	"os"
	"testing"
)

func TestCreateFileFromPath(t *testing.T) {
	Hashes = []*string{}
	invHash = make(map[string]hashval)
	path := "testdata/manifest.good"
	expected := File{
		Name: path,
		Type: typeFile,
	}

	var fh hashval
	var err error
	fh, err = Hashcalc(expected.Name)
	if err != nil {
		t.Fatal(err)
	}

	expected.Hash = fh

	m := Manifest{}
	var fi os.FileInfo
	if fi, err = os.Lstat(path); err != nil {
		t.Fatal(err)
	}

	err = m.createFileRecord("", path, fi)
	if err != nil {
		t.Error(err)
	}

	newFile := m.Files[0]
	if newFile.Name != expected.Name ||
		newFile.Type != expected.Type ||
		!HashEquals(newFile.Hash, expected.Hash) {
		t.Error("created File did not match expected")
	}
}

func TestAddFilesFromChroot(t *testing.T) {
	rootPath := "testdata/testbundle"
	m := Manifest{}
	if err := m.addFilesFromChroot(rootPath); err != nil {
		t.Error(err)
	}

	if len(m.Files) == 0 {
		t.Error("No files added from chroot")
	}
}

func TestAddFilesFromChrootNotExist(t *testing.T) {
	rootPath := "testdata/nowhere"
	m := Manifest{}
	if err := m.addFilesFromChroot(rootPath); err == nil {
		t.Errorf("addFilesFromChroot did not fail on missing root")
	}
}

func TestExists(t *testing.T) {
	if !exists("testdata/manifest.good") {
		t.Error("exists() did not return true for existing file")
	}

	if exists("testdata/nowhere") {
		t.Error("exists() returned true for non-existant file")
	}
}
