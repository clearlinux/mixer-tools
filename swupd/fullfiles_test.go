package swupd

import (
	"archive/tar"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestCreateFullfiles(t *testing.T) {
	dir, err := ioutil.TempDir("", "fullfiles-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer removeAllIgnoreErr(dir)

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

	unique := make(map[Hashval]bool)
	for name, desc := range files {
		path := filepath.Join(chrootDir, name)
		err = ioutil.WriteFile(path, desc.data, 0644)
		if err != nil {
			t.Fatal(err)
		}

		desc.hash, err = GetHashForFile(path)
		if err != nil {
			t.Fatalf("couldn't get hashes for the test file %s: %s", path, err)
		}

		f := &File{
			Name:    name,
			Hash:    internHash(desc.hash),
			Type:    TypeFile,
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

	// All correct files were created.
	for _, desc := range files {
		tarName := filepath.Join(outputDir, desc.hash+".tar")
		if desc.version != m.Header.Version {
			mustNotExist(t, tarName)
			continue
		}

		mustExist(t, tarName)
		mustHaveMatchingHash(t, tarName)
	}

	// No extra files were created.
	fis, err := ioutil.ReadDir(outputDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(fis) != len(unique) {
		t.Fatalf("generated %d fullfiles, but want %d", len(fis), len(unique))
	}
}

func mustHaveMatchingHash(t *testing.T, path string) {
	expectedHash := filepath.Base(path)
	// Take the ".tar" extension off.
	expectedHash = expectedHash[:len(expectedHash)-4]

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("couldn't open %s to check contents hash: %s", path, err)
	}
	defer func() {
		_ = f.Close()
	}()

	tr, err := newCompressedTarReader(f)
	if err != nil {
		t.Fatalf("couldn't uncompress %s to check contents hash: %s", path, err)
	}
	defer func() {
		_ = tr.Close()
	}()

	hdr, err := tr.Next()
	if err != nil {
		t.Fatalf("couldn't read archive in %s: %s", path, err)
	}

	h, err := newHashFromTarHeader(hdr)
	if err != nil {
		t.Fatalf("couldn't create hash struct from %s: %s", path, err)
	}

	_, err = io.Copy(h, tr)
	if err != nil {
		t.Fatalf("couldn't read archive %s contents: %s", path, err)
	}

	hash := h.Sum()
	if hash != expectedHash {
		t.Fatalf("unexpected hash %s for contents of %s", hash, path)
	}
}

func newHashFromTarHeader(hdr *tar.Header) (*Hash, error) {
	info := &HashFileInfo{
		Mode:     uint32(hdr.Mode),
		UID:      uint32(hdr.Uid),
		GID:      uint32(hdr.Gid),
		Size:     hdr.Size,
		Linkname: hdr.Linkname,
	}
	switch hdr.Typeflag {
	case tar.TypeReg, tar.TypeRegA:
		info.Mode |= syscall.S_IFREG
	case tar.TypeDir:
		info.Mode |= syscall.S_IFDIR
	case tar.TypeSymlink:
		info.Mode |= syscall.S_IFLNK
	}
	return NewHash(info)
}
