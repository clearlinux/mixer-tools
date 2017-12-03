package swupd

import "testing"

func TestTypeFromFlagFile(t *testing.T) {
	testCases := []struct {
		flag     byte
		expected ftype
	}{
		{'F', FILE},
		{'D', DIRECTORY},
		{'L', LINK},
		{'M', MANIFEST},
		{'.', TYPE_UNSET},
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

		if f.Type != TYPE_UNSET {
			t.Errorf("file type was set to %v from invalid flag", f.Type)
		}
	})
}

func TestStatusFromFlag(t *testing.T) {
	testCases := []struct {
		flag     byte
		expected fstatus
	}{
		{'d', DELETED},
		{'g', GHOSTED},
		{'.', STATUS_UNSET},
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

		if f.Status != STATUS_UNSET {
			t.Errorf("file modifier was set to %v from invalid flag", f.Status)
		}
	})
}

func TestModifierFromFlag(t *testing.T) {
	testCases := []struct {
		flag     byte
		expected fmodifier
	}{
		{'C', CONFIG},
		{'s', STATE},
		{'b', BOOT},
		{'.', MODIFIER_UNSET},
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

		if f.Modifier != MODIFIER_UNSET {
			t.Errorf("file modifier was set to %v from invalid flag", f.Modifier)
		}
	})
}

func TestRenameFromFlag(t *testing.T) {
	testCases := []struct {
		flag     byte
		expected frename
	}{
		{'r', RENAME},
		{'.', RENAME_UNSET},
	}

	for _, tc := range testCases {
		t.Run(string(tc.flag), func(t *testing.T) {
			f := File{}
			var err error
			if f.Rename, err = renameFromFlag(tc.flag); err != nil {
				t.Errorf("failed to set %v rename flag on file", tc.flag)
			}

			if f.Rename != tc.expected {
				t.Errorf("file rename was set to %t from %v flag", f.Rename, tc.flag)
			}
		})
	}

	// space is never valid
	t.Run("' '", func(t *testing.T) {
		f := File{}
		var err error
		if f.Rename, err = renameFromFlag(' '); err == nil {
			t.Error("setRenameFromFlag did not fail with invalid input")
		}

		if f.Rename != RENAME_UNSET {
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
