package swupd

import (
	"testing"
)

func TestSetConfigFromPathname(t *testing.T) {
	testCases := []struct {
		file     File
		expected fmodifier
	}{
		{File{Name: "/etc/something"}, modifierConfig},
		{File{Name: "/etc/a"}, modifierConfig},
		{File{Name: "/not/etc"}, modifierUnset},
		{File{Name: "/etc"}, modifierUnset},
		{File{Name: "/something/else/entirely"}, modifierUnset},
	}

	for _, tc := range testCases {
		t.Run(tc.file.Name, func(t *testing.T) {
			tc.file.setConfigFromPathname()
			if tc.file.Modifier != tc.expected {
				t.Errorf("file %v modifier %v did not match expected %v",
					tc.file.Name, tc.file.Modifier, tc.expected)
			}
		})
	}
}

func TestSetStateFromPathname(t *testing.T) {
	pathTestCases := []File{
		{Name: "/usr/src/debug"},
		{Name: "/dev"},
		{Name: "/home"},
		{Name: "/proc"},
		{Name: "/root"},
		{Name: "/run"},
		{Name: "/sys"},
		{Name: "/tmp"},
		{Name: "/var"},
	}

	for _, tc := range pathTestCases {
		t.Run(tc.Name, func(t *testing.T) {
			// actual directories do not get their modifier set
			tc.setStateFromPathname()
			if tc.Modifier != modifierUnset {
				t.Errorf("file %v modifier %v did not match expected %v",
					tc.Name, tc.Modifier, modifierUnset)
			}

			// now check children of the directory
			tc.Name = tc.Name + "/a"
			tc.setStateFromPathname()
			if tc.Modifier != modifierState {
				t.Errorf("file %v modifier %v did not match expected %v",
					tc.Name, tc.Modifier, modifierState)
			}
		})
	}

	allTestCases := []struct {
		file     File
		expected fmodifier
	}{
		{File{Name: "/acct/a"}, modifierState},
		{File{Name: "/cache/a"}, modifierState},
		{File{Name: "/data/a"}, modifierState},
		{File{Name: "/lost+found/a"}, modifierState},
		{File{Name: "/mnt/asec/a"}, modifierState},
		{File{Name: "/a"}, modifierUnset},
		{File{Name: "/acct"}, modifierState},
		{File{Name: "/other"}, modifierUnset},
		{File{Name: "/usr/src/foo"}, modifierState},
	}

	for _, tc := range allTestCases {
		t.Run(tc.file.Name, func(t *testing.T) {
			tc.file.setStateFromPathname()
			if tc.file.Modifier != tc.expected {
				t.Errorf("file %v modifier %v did not match expected %v",
					tc.file.Name, tc.file.Modifier, tc.expected)
			}
		})
	}
}

func TestSetBootFromPathname(t *testing.T) {
	testCases := []struct {
		file     File
		expected fmodifier
	}{
		{File{Name: "/boot/EFI"}, modifierBoot},
		{File{Name: "/usr/lib/modules/module"}, modifierBoot},
		{File{Name: "/usr/lib/kernel/file"}, modifierBoot},
		{File{Name: "/usr/lib/gummiboot/foo"}, modifierBoot},
		{File{Name: "/usr/bin/gummiboot/bar"}, modifierBoot},
		{File{Name: "/usr/gummiboot/bar"}, modifierUnset},
		{File{Name: "/usr/kernel/bar"}, modifierUnset},
	}

	for _, tc := range testCases {
		t.Run(tc.file.Name, func(t *testing.T) {
			tc.file.setBootFromPathname()
			if tc.file.Modifier != tc.expected {
				t.Errorf("file %v modifier %v did not match expected %v",
					tc.file.Name, tc.file.Modifier, tc.expected)
			}
		})
	}
}

func TestSetModifierFromPathname(t *testing.T) {
	testCases := []struct {
		file     File
		expected fmodifier
	}{
		{File{Name: "/etc/file"}, modifierConfig},
		{File{Name: "/usr/src/debug"}, modifierUnset},
		{File{Name: "/dev/foo"}, modifierState},
		{File{Name: "/usr/src/file"}, modifierState},
		{File{Name: "/acct/file"}, modifierState},
		{File{Name: "/boot/EFI"}, modifierBoot},
		{File{Name: "/randomfile"}, modifierUnset},
	}

	for _, tc := range testCases {
		t.Run(tc.file.Name, func(t *testing.T) {
			tc.file.setModifierFromPathname()
			if tc.file.Modifier != tc.expected {
				t.Errorf("file %v modifier %v did not match expected %v",
					tc.file.Name, tc.file.Modifier, tc.expected)
			}
		})
	}
}

func TestApplyHeuristics(t *testing.T) {
	testCases := map[string]fmodifier{
		"/etc/file":      modifierConfig,
		"/usr/src/debug": modifierUnset,
		"/dev/foo":       modifierState,
		"/usr/src/file":  modifierState,
		"/acct/file":     modifierState,
		"/boot/EFI":      modifierBoot,
		"/randomfile":    modifierUnset,
	}

	m := Manifest{}
	for key, _ := range testCases {
		m.Files = append(m.Files, &File{Name: key})
	}

	m.applyHeuristics()
	for _, f := range m.Files {
		if f.Modifier != testCases[f.Name] {
			t.Errorf("file %v modifier %v did not match expected %v",
				f.Name, f.Modifier, testCases[f.Name])
		}
	}
}
