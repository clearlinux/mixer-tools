package swupd

import (
	"testing"
)

func TestManifestFormats25to26(t *testing.T) {
	ts := newTestSwupd(t, "format25to26")
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
