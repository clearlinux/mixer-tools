package swupd

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateDeltas(t *testing.T) {
	ts := newTestSwupd(t, "deltas")
	defer ts.cleanup()
	ts.Bundles = []string{"test-bundle1", "test-bundle2"}
	ts.addFile(10, "test-bundle1", "/foo", strings.Repeat("foo", 100))
	ts.addFile(10, "test-bundle2", "/bar", strings.Repeat("bar", 100))
	ts.createManifests(10)

	ts.addFile(20, "test-bundle1", "/foo", strings.Repeat("foo", 100))
	ts.addFile(20, "test-bundle2", "/bar", strings.Repeat("bar", 100)+"testingdelta")
	ts.createManifests(20)
	mustMkdir(t, filepath.Join(ts.Dir, "www/20/delta"))

	mustCreateAllDeltas(t, "Manifest.full", ts.Dir, 10, 20)
	mustExistDelta(t, ts.Dir, "/bar", 10, 20)
}

func TestCreateDeltaTooBig(t *testing.T) {
	ts := newTestSwupd(t, "delta-too-big")
	defer ts.cleanup()
	ts.Bundles = []string{"test-bundle"}
	ts.addFile(10, "test-bundle", "/foo", strings.Repeat("foo", 100))
	ts.createManifests(10)
	ts.createFullfiles(10)

	// new one must be large and different enough for the delta to be larger than
	// the compressed version
	ts.addFile(20, "test-bundle", "/foo", strings.Repeat("asdfghasdf", 10000))
	ts.createManifests(20)
	ts.createFullfiles(20)
	mustMkdir(t, filepath.Join(ts.Dir, "www/20/delta"))

	tryCreateAllDeltas(t, "Manifest.full", ts.Dir, 10, 20)
	mustNotExistDelta(t, ts.Dir, "/foo", 10, 20)
}

func TestCreateDeltaFULLDL(t *testing.T) {
	ts := newTestSwupd(t, "delta-fulldl")
	defer ts.cleanup()
	ts.Bundles = []string{"test-bundle"}
	ts.addFile(10, "test-bundle", "/foo", strings.Repeat("0", 300))
	ts.createManifests(10)
	ts.createFullfiles(10)

	ts.addFile(20, "test-bundle", "/foo", `asdfklghaslcvnasfdgjalf ahvas hjkldghasdhf;
aj'sdhfsfjdfh sdfjkhgkhsdfg jshdfljkhasldrhsgiur 12736250q4hgkfy7efhbsd x89v zx,kjhlxlfyb
lk.n cv.srt890u n kgjh l;dsuygoihdrt jdxfgjhd 985rkfjgx c v, xnbx'aweoihdkfjsgh lkjdfhg.
m,cvnxcpowertw54lsi8ydoprf g,mdbng.c,mvnxb,.mxhstu;lwey5o;sdfjklgx;cnvjnxbasdfh`)
	ts.createManifests(20)
	ts.createFullfiles(20)
	mustMkdir(t, filepath.Join(ts.Dir, "www/20/delta"))

	tryCreateAllDeltas(t, "Manifest.full", ts.Dir, 10, 20)
	mustNotExistDelta(t, ts.Dir, "/foo", 10, 20)
}

// Imported from swupd-server/test/functional/no-delta.
func TestNoDeltasForTypeChangesOrDereferencedSymlinks(t *testing.T) {
	ts := newTestSwupd(t, "no-deltas-")
	defer ts.cleanup()
	ts.Bundles = []string{"os-core"}

	// NOTE: Currently the delta is compared to the real file, but a better
	// approximation comparison would be with a compressed version of the real file
	// (fullfile), since the delta itself will already be compressed, so packing won't
	// make it smaller like it might with the real file.

	// Create a content that will get delta.
	before := strings.Repeat("CONTENT", 1000)
	after := strings.ToLower(before[:10]) + before[10:]

	// file1 will remain a regular file.
	ts.write("image/10/os-core/file1", before+"1")
	ts.write("image/20/os-core/file1", after+"1")

	// sym1 is a link that will become a regular file (L->F).
	ts.symlink("image/10/os-core/sym1", "file1")
	ts.cp("image/10/os-core/file1", "image/20/os-core/sym1")

	// file2 will remain a regular file.
	ts.write("image/10/os-core/file2", before+"2")
	ts.write("image/20/os-core/file2", after+"2")

	// sym2 is a regular file that will become a link (F->L).
	ts.cp("image/10/os-core/file2", "image/10/os-core/sym2")
	ts.symlink("image/20/os-core/sym2", "file2")

	// file3 will remain a regular file.
	ts.write("image/10/os-core/file3", before+"3")
	ts.write("image/20/os-core/file3", after+"3")

	// symlink change + symlink target change, no delta for dereferenced sym3.
	ts.cp("image/20/os-core/file3", "image/20/os-core/file4")
	ts.symlink("image/10/os-core/sym3", "file3")
	ts.symlink("image/20/os-core/sym3", "file4")

	ts.createManifestsFromChroots(10)
	ts.createManifestsFromChroots(20)

	info := ts.createPack("os-core", 10, 20, ts.path("image"))

	mustHaveNoWarnings(t, info)
	mustHaveDeltaCount(t, info, 3)

	// NOTE: This was 5 for old swupd-server, but the new code doesn't pack a fullfile
	// if a delta already targets that file.
	mustHaveFullfileCount(t, info, 4)

	// Check that only the regular file to regular file deltas exist.
	{
		hashA := ts.mustHashFile("image/10/os-core/file1")
		hashB := ts.mustHashFile("image/20/os-core/file1")
		ts.checkExists(fmt.Sprintf("www/20/delta/10-20-%s-%s", hashA, hashB))
	}
	{
		hashA := ts.mustHashFile("image/10/os-core/file2")
		hashB := ts.mustHashFile("image/20/os-core/file2")
		ts.checkExists(fmt.Sprintf("www/20/delta/10-20-%s-%s", hashA, hashB))
	}
	{
		hashA := ts.mustHashFile("image/10/os-core/file3")
		hashB := ts.mustHashFile("image/20/os-core/file3")
		ts.checkExists(fmt.Sprintf("www/20/delta/10-20-%s-%s", hashA, hashB))
	}

	// Since pack has 3 deltas, no other delta is there. Double check other deltas
	// were not created in the file system.
	fis, err := ioutil.ReadDir(ts.path("www/20/delta"))
	if err != nil {
		t.Fatal(err)
	}
	if uint64(len(fis)) != info.DeltaCount {
		t.Fatalf("found %d files in %s but expected %d", len(fis), ts.path("www/20/delta"), info.DeltaCount)
	}
}
