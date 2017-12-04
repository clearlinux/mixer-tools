package swupd

import "testing"

func TestSetFileTypeFromFlagFile(t *testing.T) {
	flags := map[byte]ftype{
		'F': FILE,
		'D': DIRECTORY,
		'L': LINK,
		'M': MANIFEST,
		'.': 0,
	}

	for flag, ftype := range flags {
		t.Run(string(flag), func(t *testing.T) {
			f := File{}
			if err := setFileTypeFromFlag(flag, &f); err != nil {
				t.Errorf("failed to set %v type flag on file", flag)
			}

			if f.Type != ftype {
				t.Errorf("file type was set to %v from %v flag", f.Type, flag)
			}
		})
	}

	// space is never valid
	t.Run("' '", func(t *testing.T) {
		f := File{}
		if err := setFileTypeFromFlag(' ', &f); err == nil {
			t.Error("setFileTypeFromFlag did not fail with invalid input")
		}

		if f.Type != TYPE_UNSET {
			t.Errorf("file type was set to %v from invalid flag", f.Type)
		}
	})
}

func TestSetStatusFromFlag(t *testing.T) {
	flags := map[byte]fstatus{
		'd': DELETED,
		'g': GHOSTED,
		'.': 0,
	}

	for flag, fstatus := range flags {
		t.Run(string(flag), func(t *testing.T) {
			f := File{}
			if err := setStatusFromFlag(flag, &f); err != nil {
				t.Errorf("failed to set %v status flag on file", flag)
			}

			if f.Status != fstatus {
				t.Errorf("file status was set to %v from %v flag", f.Status, flag)
			}
		})
	}

	// space is never valid
	t.Run("' '", func(t *testing.T) {
		f := File{}
		if err := setStatusFromFlag(' ', &f); err == nil {
			t.Error("setStatusFromFlag did not fail with invalid input")
		}

		if f.Status != STATUS_UNSET {
			t.Errorf("file modifier was set to %v from invalid flag", f.Status)
		}
	})
}

func TestSetModifierFromFlag(t *testing.T) {
	flags := map[byte]fmodifier{
		'C': CONFIG,
		's': STATE,
		'b': BOOT,
		'.': 0,
	}

	for flag, fmod := range flags {
		t.Run(string(flag), func(t *testing.T) {
			f := File{}
			if err := setModifierFromFlag(flag, &f); err != nil {
				t.Errorf("failed to set %v modifier flag on file", flag)
			}

			if f.Modifier != fmod {
				t.Errorf("file modifier was set to %v from %v flag", f.Modifier, flag)
			}
		})
	}

	// space is never valid
	t.Run("' '", func(t *testing.T) {
		f := File{}
		if err := setModifierFromFlag(' ', &f); err == nil {
			t.Error("setModifierFromFlag did not fail with invalid input")
		}

		if f.Modifier != MODIFIER_UNSET {
			t.Errorf("file modifier was set to %v from invalid flag", f.Modifier)
		}
	})
}

func TestSetRenameFromFlag(t *testing.T) {
	flags := map[byte]bool{
		'r': true,
		'.': false,
	}

	for flag, frename := range flags {
		t.Run(string(flag), func(t *testing.T) {
			f := File{}
			if err := setRenameFromFlag(flag, &f); err != nil {
				t.Errorf("failed to set %v rename flag on file", flag)
			}

			if f.Rename != frename {
				t.Errorf("file rename was set to %t from %v flag", f.Rename, flag)
			}
		})
	}

	// space is never valid
	t.Run("' '", func(t *testing.T) {
		f := File{}
		if err := setRenameFromFlag(' ', &f); err == nil {
			t.Error("setRenameFromFlag did not fail with invalid input")
		}

		if f.Rename {
			t.Error("file rename was set to true from invalid flag")
		}
	})
}

func TestSetFlags(t *testing.T) {
	flagValid := [...]string{
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

	flagsInvalid := [...]string{
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
