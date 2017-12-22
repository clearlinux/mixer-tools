package swupd

import "testing"

func TestTypeFromFlagFile(t *testing.T) {
	testCases := []struct {
		flag     byte
		expected ftype
	}{
		{'F', typeFile},
		{'D', typeDirectory},
		{'L', typeLink},
		{'M', typeManifest},
		{'.', typeUnset},
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

		if f.Type != typeUnset {
			t.Errorf("file type was set to %v from invalid flag", f.Type)
		}
	})
}

func TestStatusFromFlag(t *testing.T) {
	testCases := []struct {
		flag     byte
		expected fstatus
	}{
		{'d', statusDeleted},
		{'g', statusGhosted},
		{'.', statusUnset},
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

		if f.Status != statusUnset {
			t.Errorf("file modifier was set to %v from invalid flag", f.Status)
		}
	})
}

func TestModifierFromFlag(t *testing.T) {
	testCases := []struct {
		flag     byte
		expected fmodifier
	}{
		{'C', modifierConfig},
		{'s', modifierState},
		{'b', modifierBoot},
		{'.', modifierUnset},
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

		if f.Modifier != modifierUnset {
			t.Errorf("file modifier was set to %v from invalid flag", f.Modifier)
		}
	})
}

func TestRenameFromFlag(t *testing.T) {
	testCases := []struct {
		flag     byte
		expected frename
	}{
		{'r', renameSet},
		{'.', renameUnset},
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

		if f.Rename != renameUnset {
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

func TestSetHashValid(t *testing.T) {
	// reset Hashes so we get the expected indices
	Hashes = []*string{}
	invHash = make(map[string]hashval)
	f := File{}
	validHash := "9bcc1718757db298fb656ae6e2ee143dde746f49fbf6805db7683cb574c36729"
	if err := f.setHash(validHash); err != nil {
		t.Error("setHash failed on valid hash")
	}

	if f.Hash != 0 {
		t.Errorf("f.Hash set to %v when 0 expected", f.Hash)
	}
}

func TestSetHashInvalid(t *testing.T) {
	f := File{}
	invalidHash := "9bcc1718757db298fb656ae6e2ee143dde746f49fbf6805db"
	if err := f.setHash(invalidHash); err == nil {
		t.Error("setHash did not fail on invalid hash")
	}
}

func TestGetFlagString(t *testing.T) {
	f := File{}
	var err error
	if err := f.setFlags("F.br"); err != nil {
		t.Fatal(err)
	}

	var flags string
	if flags, err = f.getFlagString(); err != nil {
		t.Error(err)
	}

	if flags != "F.br" {
		t.Errorf("%s did not match expected F.br", flags)
	}
}

func TestGetFlagStringFlagsUnset(t *testing.T) {
	f := File{}
	if _, err := f.getFlagString(); err == nil {
		t.Error("getFlagString did not raise an error on unset flags")
	}
}
