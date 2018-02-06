package swupd

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"
)

var largeString = "Lorem ipsum dolor sit amet, consectetur adipiscing elit. Quisque tempor quam id convallis convallis. Proin vehicula nisi in augue posuere lobortis. Quisque tincidunt elit ac facilisis auctor. Praesent facilisis ex eros, nec blandit dui suscipit porta. Nunc id nunc rhoncus, condimentum nibh at, fermentum sem. Nam mollis justo ac iaculis gravida. Vestibulum convallis congue dolor, vitae rutrum ex finibus vel. Phasellus convallis sem nunc, ac laoreet risus rhoncus fermentum. Interdum et malesuada fames ac ante ipsum primis in faucibus. Nullam eget pellentesque nulla. Etiam tristique non magna quis consectetur. In ac dolor sagittis, vulputate ante tincidunt, suscipit leo."

func TestCreateDeltas(t *testing.T) {
	testDir := mustSetupTestDir(t, "deltas")
	defer removeIfNoErrors(t, testDir)
	mustInitStandardTest(t, testDir, "0", "10", []string{"test-bundle1", "test-bundle2"})
	mustGenFile(t, testDir, "10", "test-bundle1", "foo", largeString)
	mustGenFile(t, testDir, "10", "test-bundle2", "bar", largeString)
	mustCreateManifestsStandard(t, 10, testDir)

	mustInitStandardTest(t, testDir, "10", "20", []string{"test-bundle1", "test-bundle2"})
	mustGenFile(t, testDir, "20", "test-bundle1", "foo", largeString)
	mustGenFile(t, testDir, "20", "test-bundle2", "bar", largeString+"testingdelta")
	mustCreateManifestsStandard(t, 20, testDir)
	mustMkdir(t, filepath.Join(testDir, "www/20/delta"))

	mustCreateAllDeltas(t, "Manifest.full", testDir, 10, 20)
	mustExistDelta(t, testDir, "/bar", 10, 20)
}

// Imported from swupd-server/test/functional/no-delta.
func TestNoDeltasForTypeChangesOrDereferencedSymlinks(t *testing.T) {
	ts := newTestSwupd(t, "no-deltas-")
	defer ts.cleanup()

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

	ts.createManifests(10)
	ts.createManifests(20)

	info := ts.createPack("os-core", 10, 20, ts.path("image"))

	mustHaveNoWarnings(t, info)
	mustHaveDeltaCount(t, info, 3)
	mustHaveFullfileCount(t, info, 5)

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
