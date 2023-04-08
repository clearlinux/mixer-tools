package swupd

import (
	"testing"
)

func TestSetModifierFromPathname(t *testing.T) {
	testCases := []struct {
		file     File
		newName string
		newFlag ModifierFlag
	}{
		{File{Name: "/V3/etc/file"}, "/etc/file", AVX2_1},
		{File{Name: "/V4/usr/src/debug"}, "/usr/src/debug", AVX512_2},
		{File{Name: "/dev/foo"}, "/dev/foo", SSE_0},
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
		{File{Name: "/bin/file00", Modifier: SSE_0}, 0, SSE_0},
		{File{Name: "/bin/file01", Modifier: SSE_0}, 1, SSE_1},
		{File{Name: "/bin/file02", Modifier: SSE_0}, 2, SSE_2},
		{File{Name: "/bin/file03", Modifier: SSE_0}, 3, SSE_3},
		{File{Name: "/bin/file04", Modifier: AVX2_1}, 1, AVX2_1},
		{File{Name: "/bin/file05", Modifier: AVX2_1}, 3, AVX2_3},
		{File{Name: "/bin/file06", Modifier: AVX512_2}, 2, AVX512_2},
		{File{Name: "/bin/file07", Modifier: AVX512_2}, 3, AVX512_3},
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
	testCaseMap := make(map[string]struct{file File; expected StatusFlag})
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
