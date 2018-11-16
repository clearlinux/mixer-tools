package swupd

import (
	"strings"
	"testing"
)

// Format 25 should not support minversions, iterative manifests, or delta manifests.
// Support for these features was added in format 26.
func TestManifestFormats25to26(t *testing.T) {
	ts := newTestSwupd(t, "format25to26")
	defer ts.cleanup()

	ts.Bundles = []string{"test-bundle"}

	contents := strings.Repeat("large", 1000)
	if len(contents) < minimumSizeToMakeDeltaInBytes {
		t.Fatal("test content size is invalid")
	}

	// format25 MoM should NOT have minversion in header, which is introduced
	// in format26. (It should also not have it because minversion is set to 0)
	ts.Format = 25
	ts.addFile(10, "test-bundle", "/foo", contents+"A")
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
	ts.addFile(20, "test-bundle", "/foo", contents+"B")
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

	// Iterative manifests should not have entries in the MoM or be generated
	checkManifestNotContains(t, ts.Dir, "20", "MoM", "minversion:\t20", "I...\t")
	ts.checkNotExists("www/20/Manifest.test-bundle.I.10")
	ts.checkNotExists("www/20/os-core.I.10")

	// Delta manifests should not exist in format 25
	ts.mustHashFile("image/10/full/foo")
	ts.mustHashFile("image/20/full/foo")
	ts.createPack("test-bundle", 10, 20, ts.path("image"))
	ts.checkNotExists("www/20/Manifest.test-bundle.D.10")

	// updated to format26, minversion still set to 20, so we should see
	// minversion  header in the MoM
	ts.Format = 26
	ts.addFile(30, "test-bundle", "/foo", contents+"C")
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

	ts.addFile(40, "test-bundle", "/foo", contents+"D")
	ts.createManifests(40)

	// Updates in format 26 should support iterative manifests
	checkManifestContains(t, ts.Dir, "40", "MoM", "\ttest-bundle.I.30", "\tos-core.I.30")
	ts.checkExists("www/40/Manifest.test-bundle.I.30")
	ts.checkExists("www/40/Manifest.os-core.I.30")

	// Delta manifests should be created in format 26
	ts.mustHashFile("image/30/full/foo")
	ts.mustHashFile("image/40/full/foo")
	ts.createPack("test-bundle", 30, 40, ts.path("image"))
	checkDeltaManifest(ts, 30, 40, "test-bundle", 1)
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
