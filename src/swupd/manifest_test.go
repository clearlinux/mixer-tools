package swupd

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestReadManifestHeaderManifest(t *testing.T) {
	m := Manifest{}
	if err := readManifestFileHeaderLine([]string{"MANIFEST", "2"}, &m); err != nil {
		t.Error("failed to read MANIFEST header")
	}

	if m.Header.Format != 2 {
		t.Errorf("manifest Format header set to %d when 2 was expected", m.Header.Format)
	}
}

func TestReadManifestHeaderManifestBad(t *testing.T) {
	m := Manifest{}
	if err := readManifestFileHeaderLine([]string{"MANIFEST", "i"}, &m); err == nil {
		t.Error("readManifestFileHeaderLine did not fail with invalid format header")
	}

	if m.Header.Format != 0 {
		t.Errorf("manifest Format header set to %d on invalid format", m.Header.Format)
	}
}

func TestReadManifestHeaderVersion(t *testing.T) {
	m := Manifest{}
	if err := readManifestFileHeaderLine([]string{"version:", "10"}, &m); err != nil {
		t.Error("failed to read version header")
	}

	if m.Header.Version != 10 {
		t.Errorf("manifest Version header set to %d when 20 was expected", m.Header.Version)
	}
}

func TestReadManifestHeaderVersionBad(t *testing.T) {
	m := Manifest{}
	if err := readManifestFileHeaderLine([]string{"version:", " "}, &m); err == nil {
		t.Error("readManifestFileHeaderLine did not fail with invalid version header")
	}

	if m.Header.Version != 0 {
		t.Errorf("manifest Version header set to %d on invalid version", m.Header.Version)
	}
}

func TestReadManifestHeaderFilecount(t *testing.T) {
	m := Manifest{}
	if err := readManifestFileHeaderLine([]string{"filecount:", "1000"}, &m); err != nil {
		t.Error("failed to read filecount header")
	}

	if m.Header.FileCount != 1000 {
		t.Errorf("manifest FileCount header set to %d when 1000 was expected", m.Header.FileCount)
	}
}

func TestReadManifestHeaderFilecountBad(t *testing.T) {
	m := Manifest{}
	if err := readManifestFileHeaderLine([]string{"filecount:", "i"}, &m); err == nil {
		t.Error("readManifestFileHeaderLine did not fail with invalid filecount header")
	}

	if m.Header.FileCount != 0 {
		t.Errorf("manifest FileCount header set to %d on invalid filecount", m.Header.FileCount)
	}
}

func TestReadManifestHeaderTimestamp(t *testing.T) {
	m := Manifest{}
	if err := readManifestFileHeaderLine([]string{"timestamp:", "1000"}, &m); err != nil {
		t.Error("failed to read timestamp header")
	}

	if m.Header.TimeStamp != time.Unix(1000, 0) {
		t.Errorf("manifest TimeStamp header set to %v when 1000 was expected", m.Header.TimeStamp)
	}
}

func TestReadManifestHeaderTimestampBad(t *testing.T) {
	m := Manifest{}
	if err := readManifestFileHeaderLine([]string{"timestamp:", "i"}, &m); err == nil {
		t.Error("readManifestFileHeaderLine did not fail with invalid timestamp header")
	}

	if !m.Header.TimeStamp.IsZero() {
		t.Errorf("manifest TimeStamp header set to %v on invalid timestamp", m.Header.TimeStamp)
	}
}

func TestReadManifestHeaderContentsize(t *testing.T) {
	m := Manifest{}
	if err := readManifestFileHeaderLine([]string{"contentsize:", "1000"}, &m); err != nil {
		t.Error("failed to read contentsize header")
	}

	if m.Header.ContentSize != 1000 {
		t.Errorf("manifest ContentSize header set to %d when 1000 was expected", m.Header.ContentSize)
	}
}

func TestReadManifestHeaderContentsizeBad(t *testing.T) {
	m := Manifest{}
	if err := readManifestFileHeaderLine([]string{"contentsize:", "i"}, &m); err == nil {
		t.Error("readManifestFileHeaderLine did not fail with invalid contentsize header")
	}

	if m.Header.ContentSize != 0 {
		t.Errorf("manifest ContentSize header set to %d on invalid contentsize", m.Header.ContentSize)
	}
}

func TestReadManifestHeaderIncludes(t *testing.T) {
	m := Manifest{}
	if err := readManifestFileHeaderLine([]string{"includes:", "test-bundle"}, &m); err != nil {
		t.Error("failed to read includes header")
	}

	var expected []*Manifest
	expected = append(expected, &Manifest{Name: "test-bundle"})
	if !reflect.DeepEqual(m.Header.Includes, expected) {
		t.Errorf("manifest Includes set to %v when %v expected", m.Header.Includes, expected)
	}

	if err := readManifestFileHeaderLine([]string{"includes:", "test-bundle2"}, &m); err != nil {
		t.Error("failed to read second includes header")
	}

	expected = append(expected, &Manifest{Name: "test-bundle2"})
	if !reflect.DeepEqual(m.Header.Includes, expected) {
		t.Errorf("manifest Includes set to %v when %v expected", m.Header.Includes, expected)
	}
}

func TestReadManifestFileEntry(t *testing.T) {
	validHash := "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
	validManifestLines := [][]string{
		{"Fdbr", validHash, "10", "/usr/testfile"},
		{"FgCr", validHash, "100", "/usr/bin/test"},
		{"Ddsr", validHash, "99990", "/"},
	}

	t.Run("valid", func(t *testing.T) {
		m := Manifest{}
		for _, line := range validManifestLines {
			if err := readManifestFileEntry(line, &m); err != nil {
				t.Errorf("failed to read manifest line: %v", err)
			}
		}

		for _, f := range m.Files {
			if f.Type == 0 || f.Status == 0 || f.Modifier == 0 || !f.Rename {
				t.Error("failed to set flag from manifest line")
			}
		}
	})

	invalidHash := "1234567890abcdef1234567890"
	invalidManifestLines := [][]string{
		{"..i.", validHash, "10", "/usr/testfile"},
		{"...", validHash, "10", "/usr/testfile"},
		{"FgCr", invalidHash, "100", "/usr/bin/test"},
		{"Ddsr", validHash, "i", "/"},
	}

	for _, line := range invalidManifestLines {
		t.Run("valid", func(t *testing.T) {
			m := Manifest{}
			if err := readManifestFileEntry(line, &m); err == nil {
				t.Error("readManifestFileEntry did not fail with invalid input")
			}
		})
	}
}

func TestCheckValidManifestHeader(t *testing.T) {
	m := Manifest{
		Header: ManifestHeader{
			Format:      10,
			Version:     100,
			Previous:    90,
			FileCount:   553,
			ContentSize: 100000,
			TimeStamp:   time.Unix(1000, 0),
			// does not fail when includes not added
		},
	}

	if err := m.CheckHeaderIsValid(); err != nil {
		t.Error("CheckHeaderIsValid returned error for valid header")
	}
}

func TestCheckInvalidManifestHeaders(t *testing.T) {
	zeroTime := time.Time{}

	tests := []struct {
		name   string
		header ManifestHeader
	}{
		{"format not set", ManifestHeader{0, 100, 90, 553, time.Unix(1000, 0), 100000, nil}},
		{"version zero", ManifestHeader{10, 0, 90, 553, time.Unix(1000, 0), 100000, nil}},
		{"no files", ManifestHeader{10, 100, 90, 0, time.Unix(1000, 0), 100000, nil}},
		{"no timestamp", ManifestHeader{10, 100, 90, 553, zeroTime, 100000, nil}},
		{"zero contentsize", ManifestHeader{10, 100, 90, 553, time.Unix(1000, 0), 0, nil}},
		{"version smaller than previous", ManifestHeader{10, 100, 110, 553, time.Unix(1000, 0), 100000, nil}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := Manifest{Header: tt.header}
			if err := m.CheckHeaderIsValid(); err == nil {
				t.Error("CheckHeaderIsValid did not return an error on invalid header")
			}
		})
	}
}

func TestReadManifestFromFileGood(t *testing.T) {
	path := "testdata/manifest.good"
	var m Manifest
	if err := m.ReadManifestFromFile(path); err != nil {
		t.Error(err)
	}

	if expected := uint(21); m.Header.Format != expected {
		t.Errorf("Expected manifest format %d, got %d", expected, m.Header.Format)
	}

	if len(m.Files) == 0 {
		t.Error("ReadManifestFromFile did not add file entries to the file list")
	}
}

func TestInvalidManifests(t *testing.T) {
	files, err := filepath.Glob("testdata/invalid_manifests/*")
	if err != nil {
		t.Errorf("error while reading testdata: %s", err)
	}
	if len(files) == 0 {
		t.Error("no files available for this test")
	}
	for _, name := range files {
		t.Run(path.Base(name), func(t *testing.T) {
			var m Manifest
			if err := m.ReadManifestFromFile(name); err == nil {
				t.Error("ReadManifestFromFile did not raise error for invalid manifest")
			}
		})
	}
}

func compareFiles(file1path string, file2path string) (bool, error) {
	var err error
	var f1Stat, f2Stat os.FileInfo
	var f1, f2 *os.File

	chunkSize := 65536

	f1Stat, err = os.Lstat(file1path)
	if err != nil {
		return false, err
	}

	f2Stat, err = os.Lstat(file2path)
	if err != nil {
		return false, err
	}

	if f1Stat.Size() != f2Stat.Size() {
		return false, nil
	}

	f1, err = os.Open(file1path)
	if err != nil {
		return false, err
	}
	defer f1.Close()

	f2, err = os.Open(file2path)
	if err != nil {
		return false, err
	}
	defer f2.Close()

	b1 := bufio.NewReader(f1)
	b2 := bufio.NewReader(f2)
	for {
		bytesRead1 := make([]byte, chunkSize)
		b1BytesIn, err1 := b1.Read(bytesRead1)

		bytesRead2 := make([]byte, chunkSize)
		b2BytesIn, err2 := b2.Read(bytesRead2)

		if b1BytesIn != b2BytesIn {
			return false, nil
		}

		if err1 != nil || err2 != nil {
			if err1 == io.EOF && err2 == io.EOF {
				return true, nil
			} else if err1 == io.EOF || err2 == io.EOF {
				return false, nil
			} else {
				return false, fmt.Errorf("%v - %v", err1, err2)
			}
		}

		if !bytes.Equal(bytesRead1, bytesRead2) {
			return false, nil
		}
	}
}

func TestWriteManifestFile(t *testing.T) {
	path := "testdata/manifest.good"

	var m Manifest
	if err := m.ReadManifestFromFile(path); err != nil {
		t.Fatal(err)
	}

	if len(m.Files) == 0 {
		t.Fatal("ReadManifestFromFile did not add file entried to the file list")
	}

	f, err := ioutil.TempFile("testdata", "manifest.result")
	if err != nil {
		t.Fatal("unable to open file for write")
	}
	defer os.Remove(f.Name())

	newpath := f.Name()
	if err := m.WriteManifestFile(newpath); err != nil {
		t.Error(err)
	}

	match, err := compareFiles(path, newpath)
	if err != nil {
		t.Fatal("unable to compare old and new manifest")
	}

	if !match {
		t.Errorf("generated %v did not match read %v file", newpath, path)
	}
}

func TestWriteManifestFileBadHeader(t *testing.T) {
	m := Manifest{Header: ManifestHeader{}}

	f, err := ioutil.TempFile("testdata", "manifest.result")
	if err != nil {
		t.Fatal("unable to open file for write")
	}
	defer os.Remove(f.Name())

	path := f.Name()
	if err = m.WriteManifestFile(path); err == nil {
		t.Error("WriteManifestFile did not fail on invalid header")
	}

	if err = os.Remove(path); err != nil {
		t.Error("unable to remove file, did it not close properly?")
	}
}
