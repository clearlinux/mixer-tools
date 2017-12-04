package swupd

import (
	"reflect"
	"testing"
	"time"
)

func TestReadManifestHeader(t *testing.T) {
	t.Run("MANIFEST,2", func(t *testing.T) {
		m := Manifest{}
		if err := readManifestFileHeaderLine([]string{"MANIFEST", "2"}, &m); err != nil {
			t.Error("failed to read MANIFEST header")
		}

		if m.Header.Format != 2 {
			t.Errorf("manifest Format header set to %d when 2 was expected", m.Header.Format)
		}
	})

	t.Run("MANIFEST,i", func(t *testing.T) {
		m := Manifest{}
		if err := readManifestFileHeaderLine([]string{"MANIFEST", "i"}, &m); err == nil {
			t.Error("readManifestFileHeaderLine did not fail with invalid format header")
		}

		if m.Header.Format != 0 {
			t.Errorf("manifest Format header set to %d on invalid format", m.Header.Format)
		}
	})

	t.Run("version:,10", func(t *testing.T) {
		m := Manifest{}
		if err := readManifestFileHeaderLine([]string{"version:", "10"}, &m); err != nil {
			t.Error("failed to read version header")
		}

		if m.Header.Version != 10 {
			t.Errorf("manifest Version header set to %d when 20 was expected", m.Header.Version)
		}
	})

	t.Run("version:,i", func(t *testing.T) {
		m := Manifest{}
		if err := readManifestFileHeaderLine([]string{"version:", " "}, &m); err == nil {
			t.Error("readManifestFileHeaderLine did not fail with invalid version header")
		}

		if m.Header.Version != 0 {
			t.Errorf("manifest Version header set to %d on invalid version", m.Header.Version)
		}
	})

	t.Run("filecount:,1000", func(t *testing.T) {
		m := Manifest{}
		if err := readManifestFileHeaderLine([]string{"filecount:", "1000"}, &m); err != nil {
			t.Error("failed to read filecount header")
		}

		if m.Header.FileCount != 1000 {
			t.Errorf("manifest FileCount header set to %d when 1000 was expected", m.Header.FileCount)
		}
	})

	t.Run("filecount:,i", func(t *testing.T) {
		m := Manifest{}
		if err := readManifestFileHeaderLine([]string{"filecount:", "i"}, &m); err == nil {
			t.Error("readManifestFileHeaderLine did not fail with invalid filecount header")
		}

		if m.Header.FileCount != 0 {
			t.Errorf("manifest FileCount header set to %d on invalid filecount", m.Header.FileCount)
		}
	})

	t.Run("timestamp:,1000", func(t *testing.T) {
		m := Manifest{}
		if err := readManifestFileHeaderLine([]string{"timestamp:", "1000"}, &m); err != nil {
			t.Error("failed to read timestamp header")
		}

		if m.Header.TimeStamp != time.Unix(1000, 0) {
			t.Errorf("manifest TimeStamp header set to %v when 1000 was expected", m.Header.TimeStamp)
		}
	})

	t.Run("timestamp:,i", func(t *testing.T) {
		m := Manifest{}
		if err := readManifestFileHeaderLine([]string{"timestamp:", "i"}, &m); err == nil {
			t.Error("readManifestFileHeaderLine did not fail with invalid timestamp header")
		}

		if !m.Header.TimeStamp.IsZero() {
			t.Errorf("manifest TimeStamp header set to %v on invalid timestamp", m.Header.TimeStamp)
		}
	})

	t.Run("contentsize:,1000", func(t *testing.T) {
		m := Manifest{}
		if err := readManifestFileHeaderLine([]string{"contentsize:", "1000"}, &m); err != nil {
			t.Error("failed to read contentsize header")
		}

		if m.Header.ContentSize != 1000 {
			t.Errorf("manifest ContentSize header set to %d when 1000 was expected", m.Header.ContentSize)
		}
	})

	t.Run("contentsize:,i", func(t *testing.T) {
		m := Manifest{}
		if err := readManifestFileHeaderLine([]string{"contentsize:", "i"}, &m); err == nil {
			t.Error("readManifestFileHeaderLine did not fail with invalid contentsize header")
		}

		if m.Header.ContentSize != 0 {
			t.Errorf("manifest ContentSize header set to %d on invalid contentsize", m.Header.ContentSize)
		}
	})

	t.Run("includes:,test-bundle", func(t *testing.T) {
		m := Manifest{}
		if err := readManifestFileHeaderLine([]string{"includes:", "test-bundle"}, &m); err != nil {
			t.Error("failed to read includes header")
		}

		var expected []Manifest
		expected = append(expected, Manifest{Name: "test-bundle"})
		if !reflect.DeepEqual(m.Header.Includes, expected) {
			t.Errorf("manifest Includes set to %v when %v expected", m.Header.Includes, expected)
		}

		if err := readManifestFileHeaderLine([]string{"includes:", "test-bundle2"}, &m); err != nil {
			t.Error("failed to read second includes header")
		}

		expected = append(expected, Manifest{Name: "test-bundle2"})
		if !reflect.DeepEqual(m.Header.Includes, expected) {
			t.Errorf("manifest Includes set to %v when %v expected", m.Header.Includes, expected)
		}
	})
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
			if f.Type == 0 || f.Status == 0 || f.Modifier == 0 || !f.Rename {
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
