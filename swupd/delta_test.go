package swupd

import (
	"path/filepath"
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
