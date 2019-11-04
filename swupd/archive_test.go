package swupd

import (
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

func TestNewCompressedTarReaderUncompressedRegularFile(t *testing.T) {
	testCases := []string{
		"/etc/protocols",
		"/usr/share/defaults/etc/protocols",
		"/usr/share/doc/systemd/LICENSE.GPL2",
	}
	for _, tc := range testCases {
		var f *os.File
		var err error
		if f, err = os.Open(tc); err != nil {
			// not checking non-existent files
			// not testing the os.Open call
			continue
		}
		defer func() {
			_ = f.Close()
		}()
		_, err = NewCompressedTarReader(f)
		if err != nil {
			t.Errorf("NewCompressedTarReader returned error %v for uncompressed regular file %s",
				err, tc)
		}
	}
}

func TestNewCompressedTarReaderBzip2(t *testing.T) {
	// create origin uncompressed file
	f, err := ioutil.TempFile("", "bzip2test")
	if err != nil {
		t.Fatalf("couldn't create test file")
	}
	defer func() {
		_ = f.Close()
		_ = os.Remove(f.Name())
	}()
	// write data to test file
	if _, err = f.Write([]byte("testdata\ntestmoredata")); err != nil {
		t.Fatalf("couldn't write to test file")
	}

	// need unique name for tar file, so create now
	tarf, err := ioutil.TempFile("", "bzip2testtar*.tar")
	if err != nil {
		t.Fatalf("couldn't create test tar")
	}
	defer func() {
		_ = tarf.Close()
		_ = os.Remove(tarf.Name())
	}()

	// tarRegularFullfile needs a FileInfo on the origin file
	fi, err := os.Lstat(f.Name())
	if err != nil {
		t.Fatalf("couldn't get file info for test file: %s", err)
	}

	// tar the origin file, result is an uncompressed tar archive
	if err = tarRegularFullfile(tarf, f.Name(), filepath.Base(tarf.Name()), fi); err != nil {
		t.Fatalf("unable to tar a regular fullfile for test: %s", err)
	}

	// create the output compressed tar file
	out, err := ioutil.TempFile("", "bzip2testCompressed*.tar")
	if err != nil {
		t.Fatalf("Couldn't create test compressed tarfile")
	}
	defer func() {
		_ = out.Close()
		_ = os.Remove(out.Name())
	}()

	// select external-bzip2 compressor
	compressor := fullfileCompressors["external-bzip2"]

	if compressor == nil {
		t.Fatalf("unable to find bzip2 compression function")
	}

	// compress using bzip2
	if err := compressor(out, f); err != nil {
		t.Fatalf("failed to compress %s using bzip2", f.Name())
	}

	// need to seek back to the beginning to uncompress
	if _, err := out.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("unable to seek to start of fullfile")
	}

	// test NewCompressedTarReader
	if _, err := NewCompressedTarReader(out); err != nil {
		t.Errorf("NewCompressedTarReader unable to uncompress bzip2 file: %s", err)
	}
}

func TestCompressedTarReaderCloseNil(t *testing.T) {
	ctr := CompressedTarReader{}
	if ctr.Close() != nil {
		t.Error("expected nil return with undefined close")
	}
}
