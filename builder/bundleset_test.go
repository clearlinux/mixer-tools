package builder

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseBundle(t *testing.T) {
	tests := []struct {
		Contents         []byte
		ExpectedIncludes []string
		ExpectedPackages []string
		ShouldFail       bool
	}{
		{
			Contents: []byte(`# Simple fake bundle
include(a)
include(b)
pkg1     # Comment
pkg2
`),
			ExpectedIncludes: []string{"a", "b"},
			ExpectedPackages: []string{"pkg1", "pkg2"},
		},

		// Error cases.
		{Contents: []byte(`include(`), ShouldFail: true},
		{Contents: []byte(`()`), ShouldFail: true},
		{Contents: []byte(`Include(`), ShouldFail: true},
		{Contents: []byte(`include())`), ShouldFail: true},
		{Contents: []byte(`include(abc))`), ShouldFail: true},
	}

	for _, tt := range tests {
		includes, packages, err := parseBundle(tt.Contents)
		failed := err != nil
		if failed != tt.ShouldFail {
			if tt.ShouldFail {
				t.Errorf("unexpected success when parsing bundle\nCONTENTS:\n%s\nPARSED INCLUDES: %s\nPARSED PACKAGES:\n%s", tt.Contents, includes, packages)
			} else {
				t.Errorf("unexpected error parsing bundle: %s\nCONTENTS:\n%s", err, tt.Contents)
			}
			continue
		}
		if tt.ShouldFail {
			continue
		}

		if !reflect.DeepEqual(includes, tt.ExpectedIncludes) {
			t.Errorf("got wrong includes when parsing bundle\nCONTENTS:\n%s\nPARSED INCLUDES (%d): %s\nEXPECTED INCLUDES (%d): %s", tt.Contents, len(includes), includes, len(tt.ExpectedIncludes), tt.ExpectedIncludes)
		}

		if !reflect.DeepEqual(packages, tt.ExpectedPackages) {
			t.Errorf("got wrong packages when parsing bundle\nCONTENTS:\n%s\nPARSED PACKAGES (%d):\n%s\nEXPECTED PACKAGES (%d):\n%s", tt.Contents, len(packages), packages, len(tt.ExpectedPackages), tt.ExpectedPackages)
		}
	}
}

func TestParseBundleFile(t *testing.T) {
	tests := []struct {
		Filename         string
		Contents         []byte
		ExpectedIncludes []string
		ExpectedPackages []string
		ShouldFail       bool
	}{
		{
			Filename: "simple-bundle",
			Contents: []byte(`# Simple fake bundle
include(a)
include(b)
pkg1     # Comment
pkg2
`),
			ExpectedIncludes: []string{"a", "b"},
			ExpectedPackages: []string{"pkg1", "pkg2"},
		},

		// Bundle name errors
		{Filename: "b&ndle", Contents: []byte(`pkg1`), ShouldFail: true},
		{Filename: "full", Contents: []byte(`pkg1`), ShouldFail: true},
		{Filename: "MoM", Contents: []byte(`pkg1`), ShouldFail: true},
		// Bundle contents error (catching parseBundle's error)
		{Filename: "b", Contents: []byte(`()`), ShouldFail: true},
	}

	testDir, err := ioutil.TempDir("", "bundleset-test-")
	if err != nil {
		t.Fatalf("couldn't create temporary directory to write test cases: %s", err)
	}
	defer func() {
		_ = os.RemoveAll(testDir)
	}()

	for _, tt := range tests {
		bundleFile := filepath.Join(testDir, tt.Filename)
		err = ioutil.WriteFile(bundleFile, []byte(tt.Contents), 0600)
		if err != nil {
			t.Fatalf("couldn't create temporary file for test case: %s", err)
		}

		bundle, err := parseBundleFile(bundleFile)
		failed := err != nil
		if failed != tt.ShouldFail {
			if tt.ShouldFail {
				t.Errorf("unexpected success when parsing bundle file\nFILE: %s\nCONTENTS:\n%s\nPARSED INCLUDES: %s\nPARSED PACKAGES:\n%s", tt.Filename, tt.Contents, bundle.DirectIncludes, bundle.DirectPackages)
			} else {
				t.Errorf("unexpected error parsing bundle: %s\nCONTENTS:\n%s", err, tt.Contents)
			}
			continue
		}
		if tt.ShouldFail {
			continue
		}

		if !reflect.DeepEqual(bundle.DirectIncludes, tt.ExpectedIncludes) {
			t.Errorf("got wrong includes when parsing bundle\nCONTENTS:\n%s\nPARSED INCLUDES (%d): %s\nEXPECTED INCLUDES (%d): %s", tt.Contents, len(bundle.DirectIncludes), bundle.DirectIncludes, len(tt.ExpectedIncludes), tt.ExpectedIncludes)
		}

		if !reflect.DeepEqual(bundle.DirectPackages, tt.ExpectedPackages) {
			t.Errorf("got wrong packages when parsing bundle\nCONTENTS:\n%s\nPARSED PACKAGES (%d):\n%s\nEXPECTED PACKAGES (%d):\n%s", tt.Contents, len(bundle.DirectPackages), bundle.DirectPackages, len(tt.ExpectedPackages), tt.ExpectedPackages)
		}
	}
}

func TestParseBundleSet(t *testing.T) {
	type FilesMap map[string]string
	type CountsMap map[string]int

	// Repurpose empty CountsMap as error to make the test entries less verbose, since we can avoid
	// putting the attribute names in the literal map.
	Error := CountsMap{}

	// Replace spaces with new lines.
	Lines := func(s string) string {
		return strings.Replace(s, " ", "\n", -1)
	}

	tests := []struct {
		Name  string
		Files FilesMap
		// TODO: Replace this with the actual map.
		ExpectedAllPackageCounts CountsMap
	}{
		{
			"simple include",
			FilesMap{
				"a": Lines("A1 A2"),
				"b": "include(a)",
			},
			CountsMap{
				"a": 2,
				"b": 2,
			},
		},
		{
			"redundant includes",
			FilesMap{
				"a": Lines("A1 A2 A3 A4"),
				"b": Lines("include(a) B1 B2 B3 B4"),
				"c": Lines("include(b) C1 C2 C3 C4"),
				"d": Lines("include(a) include(b) include(c) A1 B1 C1 D1"),
			},
			CountsMap{
				"a": 4,
				"b": 8,
				"c": 12,
				"d": 13,
			},
		},

		{
			"all packages don't have duplicates",
			FilesMap{
				"a": Lines("A A A A"),
				"b": Lines("include(a) A A A A"),
				"c": Lines("include(b) A A A A include(a)"),
				"d": Lines("A"),
				"e": Lines("include(a) include(d) E"),
			},
			CountsMap{
				"a": 1,
				"b": 1,
				"c": 1,
				"d": 1,
				"e": 2,
			},
		},

		{"cyclic error two bundles",
			FilesMap{"a": "include(b)", "b": "include(a)"}, Error},

		{"cyclic error three bundles",
			FilesMap{"a": "include(b)", "b": "include(c)", "c": "include(a)"}, Error},

		{"bundle not available",
			FilesMap{"a": "include(c)"}, Error},

		{"bundle not available 2",
			FilesMap{"a": "include(b)", "b": "include(c)"}, Error},
	}

	testDir, err := ioutil.TempDir("", "bundleset-test-")
	if err != nil {
		t.Fatalf("couldn't create temporary directory to write test cases: %s", err)
	}
	defer func() {
		_ = os.RemoveAll(testDir)
	}()

	for i, tt := range tests {
		dir := filepath.Join(testDir, fmt.Sprint(i))
		err = os.Mkdir(dir, 0700)
		if err != nil {
			t.Fatalf("couldn't create temporary directory to write test case: %s", err)
		}

		set := make(bundleSet)
		for name, contents := range tt.Files {
			bundleFile := filepath.Join(dir, name)
			err = ioutil.WriteFile(bundleFile, []byte(contents), 0600)
			if err != nil {
				t.Fatalf("couldn't create temporary file for test case: %s", err)
			}
			var bundle *bundle
			bundle, err = parseBundleFile(bundleFile)
			if err != nil {
				t.Errorf("unexpected error when parsing bundle set from test case %q: %s", tt.Name, err)
			}
			set[bundle.Name] = bundle
		}

		err = validateAndFillBundleSet(set)
		shouldFail := (len(tt.ExpectedAllPackageCounts) == 0)
		failed := err != nil

		if failed != shouldFail {
			if shouldFail {
				t.Errorf("expected error but parsed bundle set from test case %q", tt.Name)
			} else {
				t.Errorf("unexpected error when parsing bundle set from test case %q: %s", tt.Name, err)
			}
			continue
		}
		if shouldFail {
			continue
		}

		for _, b := range set {
			expectedCount := tt.ExpectedAllPackageCounts[b.Name]
			count := len(b.AllPackages)
			if count != expectedCount {
				t.Errorf("got %d all packages but expected %d all packages in bundle %s for test case %q", count, expectedCount, b.Name, tt.Name)
			}
		}
	}
}
