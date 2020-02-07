package swupd

import "testing"

func TestTypeFromFlagFile(t *testing.T) {
	testCases := []struct {
		flag     byte
		expected TypeFlag
	}{
		{'F', TypeFile},
		{'D', TypeDirectory},
		{'L', TypeLink},
		{'M', TypeManifest},
		{'.', TypeUnset},
	}

	for _, tc := range testCases {
		t.Run(string(tc.flag), func(t *testing.T) {
			f := File{}
			var err error
			if f.Type, err = typeFromFlag(tc.flag); err != nil {
				t.Errorf("failed to set %v type flag on file", tc.flag)
			}

			if f.Type != tc.expected {
				t.Errorf("file type was set to %v from %v flag", f.Type, tc.flag)
			}
		})
	}

	// space is never valid
	t.Run("' '", func(t *testing.T) {
		f := File{}
		var err error
		if f.Type, err = typeFromFlag(' '); err == nil {
			t.Error("typeFromFlag did not fail with invalid input")
		}

		if f.Type != TypeUnset {
			t.Errorf("file type was set to %v from invalid flag", f.Type)
		}
	})
}

func TestStatusFromFlag(t *testing.T) {
	testCases := []struct {
		flag     byte
		expected StatusFlag
	}{
		{'d', StatusDeleted},
		{'g', StatusGhosted},
		{'.', StatusUnset},
	}

	for _, tc := range testCases {
		t.Run(string(tc.flag), func(t *testing.T) {
			f := File{}
			var err error
			if f.Status, err = statusFromFlag(tc.flag); err != nil {
				t.Errorf("failed to set %v status flag on file", tc.flag)
			}

			if f.Status != tc.expected {
				t.Errorf("file status was set to %v from %v flag", f.Status, tc.flag)
			}
		})
	}

	// space is never valid
	t.Run("' '", func(t *testing.T) {
		f := File{}
		var err error
		if f.Status, err = statusFromFlag(' '); err == nil {
			t.Error("statusFromFlag did not fail with invalid input")
		}

		if f.Status != StatusUnset {
			t.Errorf("file modifier was set to %v from invalid flag", f.Status)
		}
	})
}

func TestModifierFromFlag(t *testing.T) {
	testCases := []struct {
		flag     byte
		expected ModifierFlag
	}{
		{'C', ModifierConfig},
		{'s', ModifierState},
		{'b', ModifierBoot},
		{'.', ModifierUnset},
	}

	for _, tc := range testCases {
		t.Run(string(tc.flag), func(t *testing.T) {
			f := File{}
			var err error
			if f.Modifier, err = modifierFromFlag(tc.flag); err != nil {
				t.Errorf("failed to set %v modifier flag on file", tc.flag)
			}

			if f.Modifier != tc.expected {
				t.Errorf("file modifier was set to %v from %v flag", f.Modifier, tc.flag)
			}
		})
	}

	// space is never valid
	t.Run("' '", func(t *testing.T) {
		f := File{}
		var err error
		if f.Modifier, err = modifierFromFlag(' '); err == nil {
			t.Error("setModifierFromFlag did not fail with invalid input")
		}

		if f.Modifier != ModifierUnset {
			t.Errorf("file modifier was set to %v from invalid flag", f.Modifier)
		}
	})
}

func TestMiscFromFlag(t *testing.T) {
	testCases := []struct {
		flag     byte
		expected MiscFlag
	}{
		{'r', MiscRename},
		{'.', MiscUnset},
	}

	for _, tc := range testCases {
		t.Run(string(tc.flag), func(t *testing.T) {
			f := File{}
			var err error
			if f.Misc, err = miscFromFlag(tc.flag); err != nil {
				t.Errorf("failed to set %v rename flag on file", tc.flag)
			}

			if f.Misc != tc.expected {
				t.Errorf("file rename was set to %v from %v flag", f.Misc, tc.flag)
			}
		})
	}

	// space is never valid
	t.Run("' '", func(t *testing.T) {
		f := File{}
		var err error
		if f.Misc, err = miscFromFlag(' '); err == nil {
			t.Error("setMiscFromFlag did not fail with invalid input")
		}

		if f.Misc != MiscUnset {
			t.Error("file rename was set to true from invalid flag")
		}
	})
}

func TestSetFlags(t *testing.T) {
	flagValid := []string{
		"F...",
		"F.C.",
		"F..r",
		"D.b.",
		".d.r",
		".d..",
		".gb.",
		".gsr",
	}

	var f File

	for _, flags := range flagValid {
		t.Run(flags, func(t *testing.T) {
			f = File{}
			if err := f.setFlags(flags); err != nil {
				t.Errorf("failed to set flags %v on file", flags)
			}
		})
	}

	flagsInvalid := []string{
		" ...",
		". ..",
		".. .",
		"... ",
		"...",
	}

	for _, flags := range flagsInvalid {
		t.Run(flags, func(t *testing.T) {
			f = File{}
			if err := f.setFlags(flags); err == nil {
				t.Error("setFlags did not fail with invalid input")
			}
		})
	}
}

func TestGetFlagString(t *testing.T) {
	f := File{}
	var err error
	if err = f.setFlags("F.br"); err != nil {
		t.Fatal(err)
	}

	var flags string
	if flags, err = f.GetFlagString(); err != nil {
		t.Error(err)
	}

	if flags != "F.b." {
		t.Errorf("%s did not match expected F.b.", flags)
	}
}

func TestGetFlagStringFlagsUnset(t *testing.T) {
	f := File{}
	if _, err := f.GetFlagString(); err == nil {
		t.Error("getFlagString did not raise an error on unset flags")
	}
}

func TestFindFileNameInSlice(t *testing.T) {
	fs := []*File{
		{Name: "1"},
		{Name: "2"},
		{Name: "3"},
	}

	testCases := []struct {
		name        string
		hasMatch    bool
		expectedIdx int
	}{
		{"1", true, 0},
		{"2", true, 1},
		{"3", true, 2},
		{"4", false, 9},
		{"notpresent", false, 9},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := File{Name: tc.name}
			found := f.findFileNameInSlice(fs)
			if tc.hasMatch {
				if found.Name != fs[tc.expectedIdx].Name {
					t.Errorf("findFileNameInSlice returned %v when %v expected",
						found.Name, fs[tc.expectedIdx].Name)
				}
			}
		})
	}
}

func TestTypeHasChanged(t *testing.T) {
	testCases := []struct {
		file     File
		expected bool
	}{
		{
			File{
				Status: StatusDeleted,
				Type:   TypeFile,
				DeltaPeer: &File{
					Status: StatusDeleted,
					Type:   TypeDirectory,
				},
			},
			false,
		},
		{
			File{
				Status: StatusUnset,
				Type:   TypeFile,
				DeltaPeer: &File{
					Status: StatusDeleted,
					Type:   TypeDirectory,
				},
			},
			false,
		},
		{
			File{
				Status: StatusUnset,
				Type:   TypeDirectory,
				DeltaPeer: &File{
					Status: StatusUnset,
					Type:   TypeDirectory,
				},
			},
			false,
		},
		{
			File{
				Status: StatusUnset,
				Type:   TypeLink,
				DeltaPeer: &File{
					Status: StatusUnset,
					Type:   TypeFile,
				},
			},
			false,
		},
		{
			File{
				Status: StatusUnset,
				Type:   TypeDirectory,
				DeltaPeer: &File{
					Status: StatusUnset,
					Type:   TypeFile,
				},
			},
			false,
		},
		{
			File{
				Status: StatusUnset,
				Type:   TypeFile,
				DeltaPeer: &File{
					Status: StatusUnset,
					Type:   TypeLink,
				},
			},
			false,
		},
		{
			File{
				Status: StatusUnset,
				Type:   TypeDirectory,
				DeltaPeer: &File{
					Status: StatusUnset,
					Type:   TypeLink,
				},
			},
			false,
		},
		{
			File{
				Status: StatusUnset,
				Type:   TypeFile,
				DeltaPeer: &File{
					Status: StatusUnset,
					Type:   TypeDirectory,
				},
			},
			true,
		},
		{
			File{
				Status: StatusUnset,
				Type:   TypeLink,
				DeltaPeer: &File{
					Status: StatusUnset,
					Type:   TypeDirectory,
				},
			},
			true,
		},
	}

	for _, tc := range testCases {
		t.Run("isUnsupportedTypeChange", func(t *testing.T) {
			if tc.file.isUnsupportedTypeChange() != tc.expected {
				t.Errorf("isUnsupportedTypeChange returned %v when %v was expected",
					!tc.expected, tc.expected)
			}
		})
	}
}
