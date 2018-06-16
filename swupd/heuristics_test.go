package swupd

import (
	"testing"
)

func TestSetConfigFromPathname(t *testing.T) {
	testCases := []struct {
		file     File
		expected ModifierFlag
	}{
		{File{Name: "/etc/something"}, ModifierConfig},
		{File{Name: "/etc/a"}, ModifierConfig},
		{File{Name: "/not/etc"}, ModifierUnset},
		{File{Name: "/etc"}, ModifierUnset},
		{File{Name: "/something/else/entirely"}, ModifierUnset},
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
			if tc.Modifier != ModifierUnset {
				t.Errorf("file %v modifier %v did not match expected %v",
					tc.Name, tc.Modifier, ModifierUnset)
			}

			// now check children of the directory
			tc.Name = tc.Name + "/a"
			tc.setStateFromPathname()
			if tc.Modifier != ModifierState {
				t.Errorf("file %v modifier %v did not match expected %v",
					tc.Name, tc.Modifier, ModifierState)
			}
		})
	}

	allTestCases := []struct {
		file     File
		expected ModifierFlag
	}{
		{File{Name: "/lost+found/a"}, ModifierState},
		{File{Name: "/a"}, ModifierUnset},
		{File{Name: "/other"}, ModifierUnset},
		{File{Name: "/usr/src/foo"}, ModifierState},
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
		expected ModifierFlag
	}{
		{File{Name: "/boot/EFI"}, ModifierBoot},
		{File{Name: "/usr/lib/modules/module"}, ModifierBoot},
		{File{Name: "/usr/lib/kernel/file"}, ModifierBoot},
		{File{Name: "/usr/kernel/bar"}, ModifierUnset},
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
		expected ModifierFlag
	}{
		{File{Name: "/etc/file"}, ModifierConfig},
		{File{Name: "/usr/src/debug"}, ModifierUnset},
		{File{Name: "/dev/foo"}, ModifierState},
		{File{Name: "/usr/src/file"}, ModifierState},
		{File{Name: "/boot/EFI"}, ModifierBoot},
		{File{Name: "/randomfile"}, ModifierUnset},
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
	testCases := map[string]ModifierFlag{
		"/etc/file":      ModifierConfig,
		"/usr/src/debug": ModifierUnset,
		"/dev/foo":       ModifierState,
		"/usr/src/file":  ModifierState,
		"/boot/EFI":      ModifierBoot,
		"/randomfile":    ModifierUnset,
	}

	m := Manifest{}
	for key := range testCases {
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
