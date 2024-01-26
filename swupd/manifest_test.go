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
		{"Fdar", validHash, "10", "/usr/testfile", ""},
		{"Fger", validHash, "100", "/usr/bin/test", "/V3"},
		{"Ddgr", validHash, "99990", "/", "/V4"},
	}

	t.Run("valid", func(t *testing.T) {
		m := Manifest{}
		for _, line := range validManifestLines {
			if err := readManifestFileEntry(line, &m); err != nil {
				t.Errorf("failed to read manifest line: %v", err)
			}
		}

		for i, f := range m.Files {
			if f.Name != validManifestLines[i][4]+validManifestLines[i][3] {
				t.Error("Failed to set filename from manifest line")
			}
			if f.Type == 0 || f.Status == 0 || f.Modifier == 0 || f.Misc == MiscUnset {
				t.Error("failed to set flag from manifest line")
			}
		}
	})

	invalidHash := "1234567890abcdef1234567890"
	invalidManifestLines := [][]string{
		{"...", validHash, "10", "/usr/testfile"},
		{"Fg.r", invalidHash, "100", "/usr/bin/test"},
		{"Ddsr", validHash, "i", "/"},
	}

	for _, line := range invalidManifestLines {
		t.Run("valid", func(t *testing.T) {
			m := Manifest{}
			if err := readManifestFileEntry(line, &m); err == nil {
				t.Errorf("readManifestFileEntry did not fail with invalid input (%v)", line)
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

func TestRemoveOptNonFiles(t *testing.T) {
	testCases := []File{
		{Name: "/V3/", Type: TypeLink},
		{Name: "/V4", Type: TypeDirectory},
		{Name: "/V5", Type: TypeDirectory},
		{Name: "/V3/usr/bin/f1", Type: TypeUnset, Status: StatusDeleted},
		{Name: "/V4/usr/bin/f2", Type: TypeUnset, Status: StatusDeleted},
		{Name: "/V5/usr/bin/f3", Type: TypeUnset, Status: StatusDeleted},
		{Name: "/V3/usr/bin/f4-1", Type: TypeUnset, Status: StatusDeleted},
		{Name: "/V4/usr/bin/f4-2", Type: TypeUnset, Status: StatusDeleted},
		{Name: "/V5/usr/bin/f4-3", Type: TypeUnset, Status: StatusDeleted},
		{Name: "/usr/bin/foo", Type: TypeUnset, Status: StatusDeleted},
		{Name: "/usr/bin/bar", Type: TypeUnset, Status: StatusDeleted},
		{Name: "/usr/bin/f1", Type: TypeFile, Version: 10},
		{Name: "/usr/bin/f2", Type: TypeFile, Version: 10},
		{Name: "/usr/bin/f3", Type: TypeFile, Version: 10},
		{Name: "/usr/bin/f4-1", Type: TypeFile, Version: 10},
		{Name: "/usr/bin/f4-2", Type: TypeFile, Version: 10},
		{Name: "/usr/bin/f4-3", Type: TypeFile, Version: 10},
		{Name: "/V3/usr/bin/f4-1", Type: TypeFile, Version: 10},
		{Name: "/V4/usr/bin/f4-2", Type: TypeFile, Version: 10},
		{Name: "/V5/usr/bin/f4-3", Type: TypeFile, Version: 10},
		{Name: "/usr/bin/aa1-mixer-test-canary-2", Type: TypeFile},
		{Name: "/V4/usr/bin/aa1-mixer-test-canary-2", Type: TypeFile},
		{Name: "/usr/bin/", Type: TypeDirectory},
	}

	m := Manifest{}
	m.Header = ManifestHeader{}
	m.Header.Version = 20
	for i := range testCases {
		m.Files = append(m.Files, &testCases[i])
	}
	m.removeOptNonFiles()
	if len(m.Files) != 14 {
		t.Fatalf("Manifest files incorrectly pruned")
	}
	for i := range m.Files {
		if m.Files[i].Name != testCases[i+9].Name {
			t.Errorf("Manifest file incorrectly pruned, expected: %v | actual: %v",
				testCases[i+3].Name, m.Files[i].Name)
		}
		if (m.Files[i].Name == "/V3/usr/bin/f1" || m.Files[i].Name == "/V4/usr/bin/f2" || m.Files[i].Name == "/V5/usr/bin/f3") && m.Files[i].Version == 20 {
			t.Errorf("Improperly updated version in %v",
				m.Files[i])
		}
		if m.Files[i].Version == 10 {
			t.Errorf("Manifest file %v missing version update, expected 20",
				m.Files[i])
		}
	}
}

func TestSetupModifiers(t *testing.T) {
	testCases := []struct {
		file             File
		expectedName     string
		expectedModifier ModifierFlag
		expectedMisc     MiscFlag
		expectedStatus   StatusFlag
		expectedVersion  uint32
		used             bool
		skipped          bool
	}{
		{File{Name: "/usr/bin", Type: TypeDirectory, Misc: MiscExportFile, Status: StatusExperimental}, "/usr/bin", Sse0, MiscExportFile, StatusExperimental, 0, false, false},
		{File{Name: "/V3/usr/bin", Type: TypeDirectory}, "/usr/bin", Avx2_1, MiscUnset, StatusUnset, 0, false, true},
		{File{Name: "/V4/usr/bin", Type: TypeDirectory}, "/usr/bin", Avx512_2, MiscUnset, StatusUnset, 0, false, true},
		{File{Name: "/V5/usr/bin", Type: TypeDirectory}, "/usr/bin", Apx4, MiscUnset, StatusUnset, 0, false, true},
		{File{Name: "/usr/bin/file00", Type: TypeFile, Misc: MiscExportFile, Status: StatusExperimental}, "/usr/bin/file00", Sse0, MiscExportFile, StatusExperimental, 0, false, false},
		{File{Name: "/usr/bin/file01", Type: TypeFile, Misc: MiscExportFile, Status: StatusExperimental}, "/usr/bin/file01", Sse1, MiscExportFile, StatusExperimental, 0, false, false},
		{File{Name: "/usr/bin/file02", Type: TypeFile, Misc: MiscExportFile, Status: StatusExperimental}, "/usr/bin/file02", Sse2, MiscExportFile, StatusExperimental, 0, false, false},
		{File{Name: "/usr/bin/file03", Type: TypeFile, Misc: MiscExportFile, Status: StatusExperimental}, "/usr/bin/file03", Sse3, MiscExportFile, StatusExperimental, 0, false, false},
		{File{Name: "/usr/bin/file04", Type: TypeFile, Misc: MiscExportFile, Status: StatusExperimental}, "/usr/bin/file04", Sse4, MiscExportFile, StatusExperimental, 0, false, false},
		{File{Name: "/usr/bin/file05", Type: TypeFile, Misc: MiscExportFile, Status: StatusExperimental}, "/usr/bin/file05", Sse5, MiscExportFile, StatusExperimental, 0, false, false},
		{File{Name: "/usr/bin/file06", Type: TypeFile, Misc: MiscExportFile, Status: StatusExperimental}, "/usr/bin/file06", Sse6, MiscExportFile, StatusExperimental, 0, false, false},
		{File{Name: "/usr/bin/file07", Type: TypeFile, Misc: MiscExportFile, Status: StatusExperimental}, "/usr/bin/file07", Sse7, MiscExportFile, StatusExperimental, 0, false, false},
		{File{Name: "/V3/usr/bin/file01", Type: TypeFile, Modifier: Avx2_1}, "/usr/bin/file01", Avx2_1, MiscExportFile, StatusExperimental, 0, false, false},
		{File{Name: "/V3/usr/bin/file03", Type: TypeFile, Modifier: Avx2_1}, "/usr/bin/file03", Avx2_3, MiscExportFile, StatusExperimental, 0, false, false},
		{File{Name: "/V3/usr/bin/file05", Type: TypeFile, Modifier: Avx2_1}, "/usr/bin/file05", Avx2_5, MiscExportFile, StatusExperimental, 0, false, false},
		{File{Name: "/V3/usr/bin/file07", Type: TypeFile, Modifier: Avx2_1}, "/usr/bin/file07", Avx2_7, MiscExportFile, StatusExperimental, 0, false, false},
		{File{Name: "/V4/usr/bin/file02", Type: TypeFile, Modifier: Avx512_2}, "/usr/bin/file02", Avx512_2, MiscExportFile, StatusExperimental, 0, false, false},
		{File{Name: "/V4/usr/bin/file03", Type: TypeFile, Modifier: Avx512_2}, "/usr/bin/file03", Avx512_3, MiscExportFile, StatusExperimental, 0, false, false},
		{File{Name: "/V4/usr/bin/file06", Type: TypeFile, Modifier: Avx512_2}, "/usr/bin/file06", Avx512_6, MiscExportFile, StatusExperimental, 0, false, false},
		{File{Name: "/V4/usr/bin/file07", Type: TypeFile, Modifier: Avx512_2}, "/usr/bin/file07", Avx512_7, MiscExportFile, StatusExperimental, 0, false, false},
		{File{Name: "/V5/usr/bin/file04", Type: TypeFile, Modifier: Apx4}, "/usr/bin/file04", Apx4, MiscExportFile, StatusExperimental, 0, false, false},
		{File{Name: "/V5/usr/bin/file05", Type: TypeFile, Modifier: Apx4}, "/usr/bin/file05", Apx5, MiscExportFile, StatusExperimental, 0, false, false},
		{File{Name: "/V5/usr/bin/file06", Type: TypeFile, Modifier: Apx4}, "/usr/bin/file06", Apx6, MiscExportFile, StatusExperimental, 0, false, false},
		{File{Name: "/V5/usr/bin/file07", Type: TypeFile, Modifier: Apx4}, "/usr/bin/file07", Apx7, MiscExportFile, StatusExperimental, 0, false, false},
		{File{Name: "/usr/bin/dfile00", Type: TypeFile, Misc: MiscExportFile, Status: StatusUnset}, "/usr/bin/dfile00", Sse0, MiscExportFile, StatusUnset, 0, false, false},
		{File{Name: "/usr/bin/dfile01", Type: TypeFile, Misc: MiscExportFile, Status: StatusUnset}, "/usr/bin/dfile01", Sse0, MiscExportFile, StatusUnset, 20, false, false},
		{File{Name: "/V3/usr/bin/dfile01", Type: TypeUnset, Misc: MiscExportFile, Status: StatusDeleted}, "/usr/bin/dfile01", Sse0, MiscExportFile, StatusDeleted, 0, false, true},
		{File{Name: "/usr/bin/dfile02", Type: TypeFile, Misc: MiscExportFile, Status: StatusUnset}, "/usr/bin/dfile02", Sse2, MiscExportFile, StatusUnset, 0, false, false},
		{File{Name: "/V4/usr/bin/dfile02", Type: TypeFile, Misc: MiscExportFile, Status: StatusUnset}, "/usr/bin/dfile02", Avx512_2, MiscExportFile, StatusUnset, 0, false, false},
		{File{Name: "/usr/bin/dfile03", Type: TypeFile, Misc: MiscExportFile, Status: StatusUnset}, "/usr/bin/dfile03", Sse1, MiscExportFile, StatusUnset, 20, false, false},
		{File{Name: "/V3/usr/bin/dfile03", Type: TypeFile, Misc: MiscExportFile, Status: StatusUnset}, "/usr/bin/dfile03", Avx2_1, MiscExportFile, StatusUnset, 20, false, false},
		{File{Name: "/V4/usr/bin/dfile03", Type: TypeUnset, Misc: MiscExportFile, Status: StatusDeleted}, "/usr/bin/dfile03", Sse0, MiscExportFile, StatusDeleted, 0, false, true},
		{File{Name: "/usr/bin/dfile04", Type: TypeFile, Misc: MiscExportFile, Status: StatusUnset}, "/usr/bin/dfile04", Sse2, MiscExportFile, StatusUnset, 20, false, false},
		{File{Name: "/V4/usr/bin/dfile04", Type: TypeFile, Misc: MiscExportFile, Status: StatusUnset}, "/usr/bin/dfile04", Avx512_2, MiscExportFile, StatusUnset, 20, false, false},
		{File{Name: "/V5/usr/bin/dfile04", Type: TypeUnset, Misc: MiscExportFile, Status: StatusDeleted}, "/usr/bin/dfile04", Sse0, MiscExportFile, StatusDeleted, 0, false, true},
	}
	testCaseMap := make(map[string][]struct {
		file             File
		expectedName     string
		expectedModifier ModifierFlag
		expectedMisc     MiscFlag
		expectedStatus   StatusFlag
		expectedVersion  uint32
		used             bool
		skipped          bool
	})
	for _, tc := range testCases {
		testCaseMap[tc.expectedName] = append(testCaseMap[tc.expectedName], tc)
	}

	m := Manifest{}
	m.Header = ManifestHeader{}
	m.Header.Version = 20
	for i := range testCases {
		m.Files = append(m.Files, &testCases[i].file)
	}
	if err := m.setupModifiers(); err != nil {
		t.Errorf("setupModifiers failed %v", err)
	}

	for _, f := range m.Files {
		var tcs []struct {
			file             File
			expectedName     string
			expectedModifier ModifierFlag
			expectedMisc     MiscFlag
			expectedStatus   StatusFlag
			expectedVersion  uint32
			used             bool
			skipped          bool
		}
		var errb bool
		if tcs, errb = testCaseMap[f.Name]; errb == false {
			t.Errorf("Error fixing up filenames for %v", f.Name)
		}
		found := false
		for i := range tcs {
			if f.Modifier == tcs[i].expectedModifier && f.Misc == tcs[i].expectedMisc && f.Status == tcs[i].expectedStatus && f.Version == tcs[i].expectedVersion {
				found = true
				tcs[i].used = true
			}
		}

		if found == false {
			t.Errorf("Missing match for item in modifiers list: %v",
				f)
		}
	}
	for _, tcs := range testCaseMap {
		for i := range tcs {
			if (tcs[i].used && tcs[i].skipped) || (!tcs[i].used && !tcs[i].skipped) {
				t.Errorf("%v test item not used correctly in manifest", tcs[i])
			}
		}
	}
}

func TestSetupModifiersMissingSSE(t *testing.T) {
	testCases := []File{
		{Name: "/V3/usr/bin/file00", Type: TypeFile, Modifier: Avx2_1},
		{Name: "/V4/usr/bin/file01", Type: TypeFile, Modifier: Avx512_2},
		{Name: "/V5/usr/bin/file02", Type: TypeFile, Modifier: Apx4},
	}

	for i := range testCases {
		m := Manifest{}
		m.Files = append(m.Files, &testCases[i])
		if err := m.setupModifiers(); err == nil {
			t.Errorf("Missing SSE file for %v not detected", testCases[i].Name)
		}
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
			{Name: "/V3/a", Status: StatusUnset, Info: sizer(0), Misc: MiscExportFile, Version: 10},
			{Name: "/V3/b", Status: StatusUnset, Info: sizer(0), Misc: MiscExportFile, Version: 10},
			{Name: "/V4/a", Status: StatusUnset, Info: sizer(0), Misc: MiscExportFile, Version: 10},
			{Name: "/V4/b", Status: StatusUnset, Info: sizer(0), Misc: MiscExportFile, Version: 10},
			{Name: "/V5/a", Status: StatusUnset, Info: sizer(0), Misc: MiscExportFile, Version: 10},
			{Name: "/V5/b", Status: StatusUnset, Info: sizer(0), Misc: MiscExportFile, Version: 10},
			{Name: "/dir", Type: TypeDirectory, Status: StatusUnset, Info: sizer(0), Version: 10},
			{Name: "1", Status: StatusUnset, Info: sizer(0)},
			{Name: "2", Status: StatusDeleted, Info: sizer(0)},
			{Name: "3", Status: StatusGhosted, Info: sizer(0)},
			{Name: "4", Status: StatusUnset, Info: sizer(0)},
			{Name: "5", Status: StatusUnset, Hash: 1, Info: sizer(0)},
			{Name: "7", Status: StatusDeleted, Info: sizer(0)},
			{Name: "8", Status: StatusUnset, Info: sizer(0), Misc: MiscExportFile, Version: 10},
		},
	}

	mNew := Manifest{
		Files: []*File{
			{Name: "/V3/a", Status: StatusUnset, Info: sizer(0)},
			{Name: "/V3/b", Status: StatusUnset, Hash: 1, Info: sizer(0)},
			{Name: "/V3/dir", Type: TypeDirectory, Status: StatusUnset, Hash: 1, Info: sizer(0)},
			{Name: "/V4/a", Status: StatusUnset, Info: sizer(0)},
			{Name: "/V4/b", Status: StatusUnset, Hash: 2, Info: sizer(0)},
			{Name: "/V4/dir", Type: TypeDirectory, Status: StatusUnset, Hash: 2, Info: sizer(0)},
			{Name: "/V5/a", Status: StatusUnset, Info: sizer(0)},
			{Name: "/V5/b", Status: StatusUnset, Hash: 2, Info: sizer(0)},
			{Name: "/V5/dir", Type: TypeDirectory, Status: StatusUnset, Hash: 2, Info: sizer(0)},
			{Name: "/dir", Type: TypeDirectory, Status: StatusUnset, Hash: 3, Info: sizer(0)},
			{Name: "1", Status: StatusUnset, Hash: 1, Info: sizer(0)},
			{Name: "2", Status: StatusUnset, Info: sizer(0)},
			{Name: "3", Status: StatusUnset, Info: sizer(0)},
			{Name: "5", Status: StatusUnset, Hash: 2, Info: sizer(0)},
			{Name: "6", Status: StatusUnset, Info: sizer(0)},
			{Name: "8", Status: StatusUnset, Info: sizer(0), Misc: MiscExportFile},
		},
	}

	// 1: modified, 2: deleted->added, 3: ghosted->added, 4: newly deleted,
	// 5: modified, 6: newly added, 7: previously deleted, 8: unchanged,
	// a: prefix but unchanged, b: prefix and changed,
	// /dir's opt prefixes not considered
	expectedFiles := []*File{
		{Name: "/V3/a", Status: StatusUnset, Info: sizer(0), Version: 10},
		{Name: "/V3/b", Status: StatusUnset, Hash: 1, Info: sizer(0)},
		{Name: "/V3/dir", Hash: 1},
		{Name: "/V4/a", Status: StatusUnset, Info: sizer(0), Version: 10},
		{Name: "/V4/b", Status: StatusUnset, Hash: 2, Info: sizer(0)},
		{Name: "/V4/dir", Hash: 2},
		{Name: "/V5/a", Status: StatusUnset, Info: sizer(0), Version: 10},
		{Name: "/V5/b", Status: StatusUnset, Hash: 2, Info: sizer(0)},
		{Name: "/V5/dir", Hash: 3},
		{Name: "/dir", Type: TypeDirectory, Status: StatusUnset, Hash: 3, Info: sizer(0)},
		{Name: "1", Status: StatusUnset, Hash: 1, Info: sizer(0)},
		{Name: "2", Status: StatusUnset, Info: sizer(0)},
		{Name: "3", Status: StatusUnset, Info: sizer(0)},
		{Name: "4", Status: StatusDeleted, Info: sizer(0)},
		{Name: "5", Status: StatusUnset, Hash: 2, Info: sizer(0)},
		{Name: "6", Status: StatusUnset, Info: sizer(0)},
		{Name: "7", Status: StatusDeleted, Info: sizer(0)},
		{Name: "8", Status: StatusUnset, Info: sizer(0), Version: 10},
	}

	testCases := map[string]struct {
		hasPeer  bool
		expected string
	}{
		"1":       {true, "1"},
		"2":       {false, ""},
		"3":       {false, ""},
		"5":       {true, "5"},
		"6":       {false, ""},
		"/V3/a":   {false, ""},
		"/V4/a":   {false, ""},
		"/V5/a":   {false, ""},
		"/V3/b":   {true, "/V3/b"},
		"/V4/b":   {true, "/V4/b"},
		"/V5/b":   {true, "/V5/b"},
		"/V3/dir": {false, ""},
		"/V4/dir": {false, ""},
		"/V5/dir": {false, ""},
		"/dir":    {true, "/dir"},
	}

	// linkPeersAndChange requires mNew and mOld to have file lists sorted
	// by name.
	mNew.sortFilesName()
	mOld.sortFilesName()
	changed, added, deleted := mNew.linkPeersAndChange(&mOld, 0)
	if changed != 6 {
		t.Errorf("%v files detected as changed when 6 was expected", changed)
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
		if !testCases[f.Name].hasPeer && f.DeltaPeer != nil {
			t.Fatalf("File %v with DeltaPeer set that shouldn't be", f.Name)
		} else if testCases[f.Name].hasPeer {
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

		if f.Name != expectedFiles[i].Name || f.Status != expectedFiles[i].Status || f.Version != expectedFiles[i].Version {
			t.Errorf("file name: %s, file status: %s, or file version %v does not match expected name: %s, expected status: %s or expected version %v",
				f.Name,
				f.Status,
				f.Version,
				expectedFiles[i].Name,
				expectedFiles[i].Status,
				expectedFiles[i].Version)
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
