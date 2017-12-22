package swupd

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

func TestCreateFullfiles(t *testing.T) {
	dir, err := ioutil.TempDir("", "fullfiles-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	chrootDir := filepath.Join(dir, "chroot")
	mustMkdir(t, chrootDir)

	outputDir := filepath.Join(dir, "output")
	mustMkdir(t, outputDir)

	files := map[string]*struct {
		data    []byte
		version uint32
		hash    string
	}{
		"A": {data: []byte(`file1`), version: 20},
		"B": {data: []byte(`file2`), version: 20},
		"C": {data: []byte(`file3`), version: 20},

		// File from previous version will not have a fullfile.
		"D": {data: []byte(`DDD`), version: 10},

		// File with same content (and hash).
		"E": {data: []byte(`file1`), version: 20},
	}

	m := &Manifest{}
	m.Header.Version = 20

	unique := make(map[hashval]bool)
	for name, desc := range files {
		err := ioutil.WriteFile(filepath.Join(chrootDir, name), desc.data, 0644)
		if err != nil {
			t.Fatal(err)
		}

		// TODO: Use the proper hash function to derive this, then check the output later.
		desc.hash = "hash" + string(desc.data)

		f := &File{
			Name:    name,
			Hash:    internHash(desc.hash),
			Type:    typeFile,
			Version: desc.version,
		}
		if m.Header.Version == f.Version {
			unique[f.Hash] = true
		}
		m.Files = append(m.Files, f)
	}

	err = CreateFullfiles(m, chrootDir, outputDir)
	if err != nil {
		t.Fatal(err)
	}

	for _, desc := range files {
		tarName := filepath.Join(outputDir, desc.hash+".tar")
		if desc.version != m.Header.Version {
			mustNotExist(t, tarName)
		} else {
			mustExist(t, tarName)
		}
	}

	fis, err := ioutil.ReadDir(outputDir)
	if err != nil {
		t.Fatal(err)
	}

	if len(fis) != len(unique) {
		t.Fatalf("generated %d fullfiles, but want %d", len(fis), len(unique))
	}

	// TODO: Extract the tar files and retake the hash to see if it matches.
}

func mustMkdir(t *testing.T, name string) {
	err := os.Mkdir(name, 0755)
	if err != nil {
		t.Fatal(err)
	}
}

func mustExist(t *testing.T, name string) {
	_, err := os.Stat(name)
	if err != nil {
		t.Fatal(err)
	}
}

func mustNotExist(t *testing.T, name string) {
	_, err := os.Stat(name)
	if !os.IsNotExist(err) {
		if err == nil {
			t.Fatalf("file %s exists, but want file not to exist", name)
		}
		t.Fatalf("got error %q, but want file does not exist error", err)
	}
}
