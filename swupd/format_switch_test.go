package swupd

import (
	"strings"
	"testing"
)

// Minversion support added in format 26
func TestFormats25to26Minversion(t *testing.T) {
	ts := newTestSwupd(t, "format25to26minversion")
	defer ts.cleanup()

	ts.Bundles = []string{"test-bundle"}

	// format25 MoM should NOT have minversion in header, which is introduced
	// in format26. (It should also not have it because minversion is set to 0)
	ts.Format = 25
	ts.addFile(10, "test-bundle", "/foo", "content")
	ts.createManifests(10)

	expSubs := []string{
		"MANIFEST\t25",
		"version:\t10",
		"previous:\t0",
		"filecount:\t2",
		"timestamp:\t",
		"contentsize:\t",
		"includes:\tos-core",
		"10\t/foo",
		"10\t/usr/share",
	}
	checkManifestContains(t, ts.Dir, "10", "test-bundle", expSubs...)

	nExpSubs := []string{
		"\t0\t/foo",
		".d..\t",
		"minversion:\t",
	}
	checkManifestNotContains(t, ts.Dir, "10", "test-bundle", nExpSubs...)

	// minversion now set to 20, but the MoM should still NOT have minversion
	// in header due to format25 being used
	ts.MinVersion = 20
	ts.addFile(20, "test-bundle", "/foo", "new content")
	ts.createManifests(20)

	expSubs = []string{
		"MANIFEST\t25",
		"version:\t20",
		"previous:\t10",
		"filecount:\t2",
		"includes:\tos-core",
		"20\t/foo",
	}
	checkManifestContains(t, ts.Dir, "20", "test-bundle", expSubs...)
	checkManifestNotContains(t, ts.Dir, "20", "MoM", "minversion:\t")

	// updated to format26, minversion still set to 20, so we should see
	// minversion  header in the MoM
	ts.Format = 26
	ts.addFile(30, "test-bundle", "/foo", "even newer content")
	ts.createManifests(30)
	expSubs = []string{
		"MANIFEST\t26",
		"version:\t30",
		"previous:\t20",
		"filecount:\t2",
		"includes:\tos-core",
	}
	checkManifestContains(t, ts.Dir, "30", "test-bundle", expSubs...)
	checkManifestContains(t, ts.Dir, "30", "MoM", "minversion:\t20")
}

// Delta manifest support added in format 26
func TestFormats25to26DeltaManifest(t *testing.T) {
	ts := newTestSwupd(t, "format25to26deltaManifest")
	defer ts.cleanup()

	ts.Bundles = []string{"test-bundle"}

	contents := strings.Repeat("large", 1000)
	if len(contents) < minimumSizeToMakeDeltaInBytes {
		t.Fatal("test content size is invalid")
	}

	// Format 25 should not have delta manifest support
	ts.Format = 25
	ts.addFile(10, "test-bundle", "/foo", contents+"A")
	ts.createManifests(10)

	ts.addFile(20, "test-bundle", "/foo", contents+"B")
	ts.createManifests(20)
	checkManifestContains(t, ts.Dir, "20", "MoM", "MANIFEST\t25")

	// Delta manifests should not exist
	ts.mustHashFile("image/10/full/foo")
	ts.mustHashFile("image/20/full/foo")
	ts.createPack("test-bundle", 10, 20, ts.path("image"))
	ts.checkNotExists("www/20/Manifest.test-bundle.D.10")

	// Update to format26
	ts.Format = 26
	ts.addFile(30, "test-bundle", "/foo", contents+"C")
	ts.createManifests(30)

	ts.addFile(40, "test-bundle", "/foo", contents+"D")
	ts.createManifests(40)
	checkManifestContains(t, ts.Dir, "40", "MoM", "MANIFEST\t26")
}

func TestFormat25BadContentSize(t *testing.T) {
	testCases := []struct {
		testName    string
		format      uint
		contentsize uint64
		expected    uint64
	}{
		// broken format
		{"format25: badMax + 1", 25, badMax + 1, badMax - 1},
		{"format25: badMax * 2", 25, badMax * 2, badMax - 1},
		{"format25: badMax", 25, badMax, badMax - 1},
		{"format25: badMax / 2", 25, badMax / 2, badMax / 2},
		// good format
		{"format26: badMax + 1", 26, badMax + 1, badMax + 1},
		{"format26: badMax * 2", 26, badMax * 2, badMax * 2},
		{"format26: badMax", 26, badMax, badMax},
		{"format26: badMax / 2", 26, badMax / 2, badMax / 2},
		// older good format
		{"format24: badMax + 1", 24, badMax + 1, badMax + 1},
		{"format24: badMax * 2", 24, badMax * 2, badMax * 2},
		{"format24: badMax", 24, badMax, badMax},
		{"format24: badMax / 2", 24, badMax / 2, badMax / 2},
	}

	for _, tc := range testCases {
		t.Run(tc.testName, func(t *testing.T) {
			m := &Manifest{
				Header: ManifestHeader{
					Format:      tc.format,
					ContentSize: tc.contentsize,
				},
			}
			m.setMaxContentSizeForFormat()
			if m.Header.ContentSize != tc.expected {
				t.Errorf("%d contentsize set to %d, expected %d",
					tc.contentsize,
					m.Header.ContentSize,
					tc.expected,
				)
			}
		})
	}
}

// Experimental bundles added in format 27
func TestFormats26to27ExperimentalBundles(t *testing.T) {
	ts := newTestSwupd(t, "format26to27ExperimentalBundles")
	defer ts.cleanup()

	var header BundleHeader
	header.Status = "Experimental"

	// Format 26 should not recognize experimental bundles
	ts.Format = 26
	ts.Bundles = []string{"test-bundle1"}
	ts.addFile(10, "test-bundle1", "/foo", "content")
	ts.addHeader(10, "test-bundle1", header)
	ts.createManifests(10)
	checkManifestNotContains(t, ts.Dir, "10", "MoM", "Me..\t")

	// Format 27 should recognize experimental bundles
	ts.Format = 27
	ts.Bundles = []string{"test-bundle2"}
	ts.addFile(20, "test-bundle2", "/foo", "content")
	ts.addHeader(20, "test-bundle2", header)
	ts.createManifests(20)
	checkManifestContains(t, ts.Dir, "20", "MoM", "Me..\t")
}

// optional (also-add) support added in format 29
func TestFormats28to29Optional(t *testing.T) {
	ts := newTestSwupd(t, "format28to29optional")
	defer ts.cleanup()

	ts.Bundles = []string{"test-bundle", "test-bundle-2"}

	// format28 manifest should NOT have also-add bundles in header,
	// which is introduced in format29.
	ts.Format = 28
	ts.addFile(10, "test-bundle", "/foo", "content")
	ts.addOptional(10, "test-bundle", []string{"test-bundle-2"})
	ts.createManifests(10)

	expSubs := []string{
		"MANIFEST\t28",
		"version:\t10",
		"previous:\t0",
		"filecount:\t2",
		"timestamp:\t",
		"contentsize:\t",
		"includes:\tos-core",
	}
	checkManifestContains(t, ts.Dir, "10", "test-bundle", expSubs...)
	checkManifestNotContains(t, ts.Dir, "10", "test-bundle", "also-add:\ttest-bundle-2")

	// updated to format29, the manifest should include the optional bundle
	ts.Format = 29
	ts.addFile(20, "test-bundle", "/foo", "new content")
	ts.addOptional(20, "test-bundle", []string{"test-bundle-2"})
	ts.createManifests(20)

	expSubs = []string{
		"MANIFEST\t29",
		"version:\t20",
		"previous:\t10",
		"filecount:\t2",
		"includes:\tos-core",
		"also-add:\ttest-bundle-2",
	}
	checkManifestContains(t, ts.Dir, "20", "test-bundle", expSubs...)
}
