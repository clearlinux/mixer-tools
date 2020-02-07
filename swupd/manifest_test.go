package swupd

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
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

func TestReadManifestHeaderOptional(t *testing.T) {
	m := Manifest{}
	if err := readManifestFileHeaderLine([]string{"also-add:", "test-bundle"}, &m); err != nil {
		t.Error("failed to read also-add header")
	}

	var expected []*Manifest
	expected = append(expected, &Manifest{Name: "test-bundle"})
	if !reflect.DeepEqual(m.Header.Optional, expected) {
		t.Errorf("manifest also-add set to %v when %v expected", m.Header.Optional, expected)
	}

	if err := readManifestFileHeaderLine([]string{"also-add:", "test-bundle2"}, &m); err != nil {
		t.Error("failed to read second includes header")
	}

	expected = append(expected, &Manifest{Name: "test-bundle2"})
	if !reflect.DeepEqual(m.Header.Optional, expected) {
		t.Errorf("manifest also-add set to %v when %v expected", m.Header.Optional, expected)
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
			if f.Type == 0 || f.Status == 0 || f.Modifier == 0 || f.Misc == MiscUnset {
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
		{"format not set", ManifestHeader{0, 100, 90, 0, 553, time.Unix(1000, 0), 100000, nil, nil}},
		{"version zero", ManifestHeader{10, 0, 90, 0, 553, time.Unix(1000, 0), 100000, nil, nil}},
		{"no files", ManifestHeader{10, 100, 90, 0, 0, time.Unix(1000, 0), 100000, nil, nil}},
		{"no timestamp", ManifestHeader{10, 100, 0, 90, 553, zeroTime, 100000, nil, nil}},
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

func TestParseManifestFromFileGood(t *testing.T) {
	path := "testdata/manifest.good"

	m, err := ParseManifestFile(path)
	if err != nil {
		t.Error(err)
	}

	if expected := uint(21); m.Header.Format != expected {
		t.Errorf("Expected manifest format %d, got %d", expected, m.Header.Format)
	}

	if len(m.Files) == 0 {
		t.Error("ParseManifestFile did not add file entries to the file list")
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
			_, err := ParseManifestFile(name)
			if err == nil {
				t.Error("ParseManifestFile did not raise error for invalid manifest")
			}
		})
	}
}

func TestWriteManifestFile(t *testing.T) {
	path := "testdata/manifest.good"

	m, err := ParseManifestFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if len(m.Files) == 0 {
		t.Fatal("ParseManifestFile did not add file entried to the file list")
	}

	// do not use a tempfile here, we just need the unique name
	newpath := "testdata/manifest.good.result"
	defer func() {
		_ = os.Remove(newpath)
	}()
	if err := m.WriteManifestFile(newpath); err != nil {
		t.Error(err)
	}

	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal("unable to change file permissions for test")
	}

	fh1, _ := Hashcalc(path)
	fh2, _ := Hashcalc(newpath)
	if fh1 != fh2 {
		t.Errorf("generated %v (%v) did not match read %v (%v) file", newpath, fh2, path, fh1)
		// Print some debug information
		cmd := exec.Command("diff", newpath, path)
		var out bytes.Buffer
		cmd.Stdout = &out
		err := cmd.Run()
		if err != nil {
			t.Fatal("diff failed")
		}
		fmt.Println("DIFF")
		fmt.Println(out.String())

		info1, _ := os.Stat(path)
		info2, _ := os.Stat(newpath)
		fmt.Println("FILE PERMISSIONS")
		fmt.Printf("read: %v\n", info1.Mode())
		fmt.Printf("gend: %v\n", info2.Mode())
	}
}

func TestWriteManifestWithBadHeader(t *testing.T) {
	m := Manifest{Header: ManifestHeader{}}

	var output bytes.Buffer
	err := m.WriteManifest(&output)
	if err == nil {
		t.Fatal("WriteManifest did not fail on invalid header")
	}
}

func TestSortFilesName(t *testing.T) {
	m := Manifest{
		Files: []*File{
			{Name: "c"},
			{Name: "b"},
			{Name: "d"},
			{Name: "a"},
			{Name: "f"},
			{Name: "fa"},
			{Name: "ba"},
		},
	}

	expectedNames := []string{"a", "b", "ba", "c", "d", "f", "fa"}
	mResult := m
	mResult.sortFilesName()
	for i, f := range mResult.Files {
		if f.Name != expectedNames[i] {
			t.Error("manifest files were not sorted correctly")
		}
	}
}

func TestSortFilesVersionName(t *testing.T) {
	m := Manifest{
		Files: []*File{
			{Name: "z", Version: 20},
			{Name: "x", Version: 20},
			{Name: "u", Version: 10},
			{Name: "qa", Version: 30},
			{Name: "qs", Version: 10},
			{Name: "r", Version: 40},
			{Name: "m", Version: 40},
		},
	}

	expectedNames := []string{"qs", "u", "x", "z", "qa", "m", "r"}
	mResult := m
	mResult.sortFilesVersionName()
	for i, f := range mResult.Files {
		if f.Name != expectedNames[i] {
			t.Error("manifest files were not sorted correctly")
		}
	}
}

func TestLinkPeersAndChange(t *testing.T) {
	mOld := Manifest{
		Files: []*File{
			{Name: "1", Status: StatusUnset, Info: sizer(0)},
			{Name: "2", Status: StatusDeleted, Info: sizer(0)},
			{Name: "3", Status: StatusGhosted, Info: sizer(0)},
			{Name: "4", Status: StatusUnset, Info: sizer(0)},
			{Name: "5", Status: StatusUnset, Hash: 1, Info: sizer(0)},
			{Name: "7", Status: StatusDeleted, Info: sizer(0)},
		},
	}

	mNew := Manifest{
		Files: []*File{
			{Name: "1", Status: StatusUnset, Hash: 1, Info: sizer(0)},
			{Name: "2", Status: StatusUnset, Info: sizer(0)},
			{Name: "3", Status: StatusUnset, Info: sizer(0)},
			{Name: "5", Status: StatusUnset, Hash: 2, Info: sizer(0)},
			{Name: "6", Status: StatusUnset, Info: sizer(0)},
		},
	}

	// 1: modified, 2: deleted->added, 3: ghosted->added, 4: newly deleted,
	// 5: modified, 6: newly added, 7: previously deleted
	expectedFiles := []*File{
		{Name: "1", Status: StatusUnset, Hash: 1, Info: sizer(0)},
		{Name: "2", Status: StatusUnset, Info: sizer(0)},
		{Name: "3", Status: StatusUnset, Info: sizer(0)},
		{Name: "4", Status: StatusDeleted, Info: sizer(0)},
		{Name: "5", Status: StatusUnset, Hash: 2, Info: sizer(0)},
		{Name: "6", Status: StatusUnset, Info: sizer(0)},
		{Name: "7", Status: StatusDeleted, Info: sizer(0)},
	}

	testCases := map[string]struct {
		hasPeer  bool
		expected string
	}{
		"1": {true, "1"},
		"2": {false, ""},
		"3": {false, ""},
		"5": {true, "5"},
		"6": {false, ""},
	}

	// linkPeersAndChange requires mNew and mOld to have file lists sorted
	// by name.
	mNew.sortFilesName()
	mOld.sortFilesName()
	changed, added, deleted := mNew.linkPeersAndChange(&mOld, 0)
	if changed != 2 {
		t.Errorf("%v files detected as changed when 2 was expected", changed)
	}
	if added != 3 {
		t.Errorf("%v files detected as added when 3 were expected", added)
	}
	// The previously deleted file will not be counted as a newly deleted file.
	if deleted != 1 {
		t.Errorf("%v files detected as deleted when only 1 was expected", deleted)
	}

	if len(mNew.Files) != len(expectedFiles) {
		t.Errorf("new file len: %d does not match expected len: %d", len(mNew.Files), len(expectedFiles))
	}

	for i, f := range mNew.Files {
		if testCases[f.Name].hasPeer {
			if f.DeltaPeer == nil {
				t.Fatalf("File %v does not have delta peer when expected", f.Name)
			}

			if f.DeltaPeer.Name != testCases[f.Name].expected {
				t.Errorf("File %v has %v delta peer when %v is expected",
					f.Name,
					f.DeltaPeer.Name,
					testCases[f.Name].expected)
			}
		}

		if f.Name != expectedFiles[i].Name && f.Status != expectedFiles[i].Status {
			t.Errorf("file name: %s or file status: %s does not match expected name: %s or expected status: %s",
				f.Name,
				f.Status,
				expectedFiles[i].Name,
				expectedFiles[i].Status)
		}
	}
}

func TestHasTypeChanges(t *testing.T) {
	mUnchanged := Manifest{
		Files: []*File{
			{ // no delta peer, no type change
				Name:      "1",
				Type:      TypeFile,
				Status:    StatusUnset,
				DeltaPeer: nil,
			},
			{ // same type, no type change
				Name:   "2",
				Type:   TypeFile,
				Status: StatusUnset,
				DeltaPeer: &File{
					Name:   "2",
					Type:   TypeFile,
					Status: StatusUnset,
				},
			},
			{ // File -> Link OK, no change reported
				Name:   "3",
				Type:   TypeLink,
				Status: StatusUnset,
				DeltaPeer: &File{
					Name:   "3",
					Type:   TypeFile,
					Status: StatusUnset,
				},
			},
			{ // File -> Directory OK, no change reported
				Name:   "4",
				Type:   TypeDirectory,
				Status: StatusUnset,
				DeltaPeer: &File{
					Name:   "4",
					Type:   TypeFile,
					Status: StatusUnset,
				},
			},
			{ // Link -> File OK, no change reported
				Name:   "5",
				Type:   TypeFile,
				Status: StatusUnset,
				DeltaPeer: &File{
					Name:   "5",
					Type:   TypeLink,
					Status: StatusUnset,
				},
			},
			{ // Link -> Directory OK, no change reported
				Name:   "6",
				Type:   TypeDirectory,
				Status: StatusUnset,
				DeltaPeer: &File{
					Name:   "6",
					Type:   TypeLink,
					Status: StatusUnset,
				},
			},
			{ // file deleted, no type change reported
				Name:   "7",
				Type:   TypeFile,
				Status: StatusDeleted,
				DeltaPeer: &File{
					Name:   "7",
					Type:   TypeLink,
					Status: StatusUnset,
				},
			},
			{ // delta peer deleted, no type change reported
				Name:   "8",
				Type:   TypeFile,
				Status: StatusUnset,
				DeltaPeer: &File{
					Name:   "8",
					Type:   TypeLink,
					Status: StatusDeleted,
				},
			},
		},
	}
	msChanged := []Manifest{
		{
			Files: []*File{ // Directory -> File TYPE CHANGE
				{
					Name:   "1",
					Type:   TypeFile,
					Status: StatusUnset,
					DeltaPeer: &File{
						Name:   "1",
						Type:   TypeDirectory,
						Status: StatusUnset,
					},
				},
			},
		},
		{
			Files: []*File{ // Directory -> Link TYPE CHANGE
				{
					Name:   "2",
					Type:   TypeLink,
					Status: StatusUnset,
					DeltaPeer: &File{
						Name:   "2",
						Type:   TypeDirectory,
						Status: StatusUnset,
					},
				},
			},
		},
	}

	if mUnchanged.hasUnsupportedTypeChanges() {
		t.Error("Manifest with no type changes detected to have type changes")
	}

	for _, m := range msChanged {
		if !m.hasUnsupportedTypeChanges() {
			t.Error("Manifest with type changes detected to have no type changes")
		}
	}
}

func TestGetNameForManifestFile(t *testing.T) {
	tests := []struct {
		Filename     string
		ExpectedName string
	}{
		{"Manifest.MoM", "MoM"},
		{"Manifest.c-basic", "c-basic"},
		{"12/Manifest.MoM", "MoM"},

		{"manifest.good", ""},
		{"Othername", ""},
		{"Othername.MoM", ""},
	}

	for _, tt := range tests {
		name := getNameForManifestFile(tt.Filename)
		if name != tt.ExpectedName {
			t.Errorf("Manifest with filename %s got name %q, but want %q", tt.Filename, name, tt.ExpectedName)
		}
	}
}
