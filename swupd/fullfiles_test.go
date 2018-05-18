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
		Contents string
		Version  uint32
		Mode     os.FileMode
		IsDir    bool
		Linkname string

		ExpectedHash string
	}{
		"A": {Contents: `file1`, Version: 20, Mode: 0755},
		"B": {Contents: `file2`, Version: 20, Mode: 0644},
		"C": {Contents: `file3`, Version: 20, Mode: 0644},

		// File from previous version will not have a fullfile.
		"D": {Contents: `DDD`, Version: 10, Mode: 0755},

		// File with same content and mode (and hash).
		"E": {Contents: `file1`, Version: 20, Mode: 0755},

		// File with same content but different mode.
		"F": {Contents: `file1`, Version: 20, Mode: 0644},

		// Directories.
		"G": {IsDir: true, Version: 20},
		"H": {IsDir: true, Version: 10},

		// Links.
		"I": {Linkname: "A", Version: 20},
		"J": {Linkname: "A", Version: 10},
	}

	m := &Manifest{}
	m.Header.Version = 20

	unique := make(map[Hashval]bool)
	for name, desc := range files {
		var typeFlag TypeFlag
		path := filepath.Join(chrootDir, name)
		switch {
		case desc.IsDir:
			typeFlag = TypeDirectory
			if desc.Mode == 0 {
				desc.Mode = 0755
			}
			err = os.Mkdir(path, desc.Mode)
			if err != nil {
				t.Fatal(err)
			}
			err = os.Chmod(path, desc.Mode)
		case desc.Linkname != "":
			typeFlag = TypeLink
			err = os.Symlink(desc.Linkname, path)
		default:
			typeFlag = TypeFile
			if desc.Mode == 0 {
				desc.Mode = 0644
			}
			err = ioutil.WriteFile(path, []byte(desc.Contents), desc.Mode)
			if err != nil {
				t.Fatal(err)
			}
			err = os.Chmod(path, desc.Mode)
		}
		if err != nil {
			t.Fatal(err)
		}

		desc.ExpectedHash, err = GetHashForFile(path)
		if err != nil {
			t.Fatalf("couldn't get hashes for the test file %s: %s", path, err)
		}

		f := &File{
			Name:    name,
			Hash:    internHash(desc.ExpectedHash),
			Type:    typeFlag,
			Version: desc.Version,
		}
		if m.Header.Version == f.Version {
			unique[f.Hash] = true
		}
		m.Files = append(m.Files, f)
	}

	_, err = CreateFullfiles(m, chrootDir, outputDir, 0)
	if err != nil {
		t.Fatal(err)
	}

	// All correct files were created.
	for _, desc := range files {
		if desc.Version != m.Header.Version {
			continue
		}
		tarName := filepath.Join(outputDir, desc.ExpectedHash+".tar")
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
	t.Helper()
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

	tr, err := NewCompressedTarReader(f)
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
