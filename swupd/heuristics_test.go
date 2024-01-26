package swupd

import (
	"testing"
)

func TestSetModifierFromPathname(t *testing.T) {
	testCases := []struct {
		file    File
		newName string
		newFlag ModifierFlag
	}{
		{File{Name: "/V3/etc/file"}, "/etc/file", Avx2_1},
		{File{Name: "/V4/usr/src/debug"}, "/usr/src/debug", Avx512_2},
		{File{Name: "/V5/usr/bin/foo"}, "/usr/bin/foo", Apx4},
		{File{Name: "/dev/foo"}, "/dev/foo", Sse0},
	}

	for _, tc := range testCases {
		t.Run(tc.file.Name, func(t *testing.T) {
			tc.file.setModifierFromPathname()
			if tc.file.Name != tc.newName || tc.file.Modifier != tc.newFlag {
				t.Errorf("file %v (%v) modifier %v (%v) did not match expected (value in parens)",
					tc.file.Name, tc.newName, tc.file.Modifier, tc.newFlag)
			}
		})
	}
}

func TestSetFullModifier(t *testing.T) {
	testCases := []struct {
		file     File
		bits     uint64
		expected ModifierFlag
	}{
		{File{Name: "/bin/file00", Modifier: Sse0}, 0, Sse0},
		{File{Name: "/bin/file01", Modifier: Sse0}, 1, Sse1},
		{File{Name: "/bin/file02", Modifier: Sse0}, 2, Sse2},
		{File{Name: "/bin/file03", Modifier: Sse0}, 3, Sse3},
		{File{Name: "/bin/file04", Modifier: Sse0}, 4, Sse4},
		{File{Name: "/bin/file05", Modifier: Sse0}, 5, Sse5},
		{File{Name: "/bin/file06", Modifier: Sse0}, 6, Sse6},
		{File{Name: "/bin/file07", Modifier: Sse0}, 7, Sse7},
		{File{Name: "/bin/file08", Modifier: Avx2_1}, 1, Avx2_1},
		{File{Name: "/bin/file09", Modifier: Avx2_1}, 3, Avx2_3},
		{File{Name: "/bin/file10", Modifier: Avx2_1}, 5, Avx2_5},
		{File{Name: "/bin/file11", Modifier: Avx2_1}, 7, Avx2_7},
		{File{Name: "/bin/file12", Modifier: Avx512_2}, 2, Avx512_2},
		{File{Name: "/bin/file13", Modifier: Avx512_2}, 3, Avx512_3},
		{File{Name: "/bin/file14", Modifier: Avx512_2}, 6, Avx512_6},
		{File{Name: "/bin/file15", Modifier: Avx512_2}, 7, Avx512_7},
		{File{Name: "/bin/file16", Modifier: Apx4}, 4, Apx4},
		{File{Name: "/bin/file17", Modifier: Apx4}, 5, Apx5},
		{File{Name: "/bin/file18", Modifier: Apx4}, 6, Apx6},
		{File{Name: "/bin/file19", Modifier: Apx4}, 7, Apx7},
	}

	for _, tc := range testCases {
		t.Run(tc.file.Name, func(t *testing.T) {
			tc.file.setFullModifier(tc.bits)
			if tc.file.Modifier != tc.expected {
				t.Errorf("file %v modifier %v with bits %v did not match expected %v",
					tc.file.Name, tc.file.Modifier, tc.bits, tc.expected)
			}
		})
	}
}

func TestSetGhostedFromPathname(t *testing.T) {
	testCases := []struct {
		file     File
		expected StatusFlag
	}{
		{File{Name: "/boot/file", Status: StatusDeleted}, StatusGhosted},
		{File{Name: "/boot/file2", Status: StatusUnset}, StatusUnset},
		{File{Name: "/usr/lib/modules/foo", Status: StatusDeleted}, StatusGhosted},
		{File{Name: "/usr/lib/kernel/foo", Status: StatusDeleted}, StatusGhosted},
	}

	for _, tc := range testCases {
		t.Run(tc.file.Name, func(t *testing.T) {
			tc.file.setGhostedFromPathname()
			if tc.file.Status != tc.expected {
				t.Errorf("file %v status %v did not match expected %v",
					tc.file.Name, tc.file.Status, tc.expected)
			}
		})
	}
}

func TestApplyHeuristics(t *testing.T) {
	testCases := []struct {
		file     File
		expected StatusFlag
	}{
		{File{Name: "/boot/file", Status: StatusDeleted}, StatusGhosted},
		{File{Name: "/boot/file2", Status: StatusUnset}, StatusUnset},
		{File{Name: "/usr/lib/modules/foo", Status: StatusDeleted}, StatusGhosted},
		{File{Name: "/usr/lib/kernel/foo", Status: StatusDeleted}, StatusGhosted},
	}
	testCaseMap := make(map[string]struct {
		file     File
		expected StatusFlag
	})
	for _, tc := range testCases {
		testCaseMap[tc.file.Name] = tc
	}

	m := Manifest{}
	for _, val := range testCaseMap {
		m.Files = append(m.Files, &val.file)
	}

	m.applyHeuristics()
	for _, f := range m.Files {
		if f.Status != testCaseMap[f.Name].expected {
			t.Errorf("file %v status %v did not match expected %v",
				f.Name, f.Status, testCaseMap[f.Name].expected)
		}
	}
}
