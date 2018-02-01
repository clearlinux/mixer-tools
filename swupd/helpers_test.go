package swupd

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"text/template"
)

func removeAllIgnoreErr(dir string) {
	_ = os.RemoveAll(dir)
}

func mustDirExistsWithPerm(t *testing.T, path string, perm os.FileMode) {
	var err error
	var info os.FileInfo
	if info, err = os.Stat(path); err != nil {
		t.Fatal(err)
	}

	// check if it is a directory or the perms don't match
	if !info.Mode().IsDir() || info.Mode().Perm() != perm {
		t.Fatal(err)
	}
}

func mustSetupTestDir(t *testing.T, testName string) string {
	oldDirs, _ := filepath.Glob("./testdata/cmtest-" + testName + ".*")
	for _, d := range oldDirs {
		removeAllIgnoreErr(d)
	}

	testDir, err := ioutil.TempDir("./testdata", "cmtest-"+testName+".")
	if err != nil {
		t.Fatal(err)
	}

	return testDir
}

func mustInitStandardTest(t *testing.T, testDir, lastVer, ver string, bundles []string) {
	mustInitTestDir(t, testDir)
	mustInitServerINI(t, testDir)
	mustInitGroupsINI(t, testDir, bundles)
	for _, b := range bundles {
		mustTrackBundle(t, testDir, ver, b)
	}
	mustInitOSRelease(t, testDir, ver)
	mustSetLatestVer(t, testDir, lastVer)
}

func mustInitTestDir(t *testing.T, path string) {
	if err := os.MkdirAll(filepath.Join(path, "image"), os.ModePerm); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(path, "www"), os.ModePerm); err != nil {
		t.Fatal(err)
	}
}

func mustInitGroupsINI(t *testing.T, testDir string, bundles []string) {
	bs := []byte("[os-core]\ngroup=os-core\nstatus=ACTIVE\n")
	for _, b := range bundles {
		bs = append(bs, []byte(fmt.Sprintf("[%s]\ngroup=%s\nstatus=ACTIVE\n", b, b))...)
	}

	if err := ioutil.WriteFile(filepath.Join(testDir, "groups.ini"), bs, 0644); err != nil {
		t.Fatal(err)
	}
}

func mustGenFile(t *testing.T, testDir, ver, bundle, fname, content string) {
	var err error
	fpath := filepath.Join(testDir, "image", ver, bundle, filepath.Dir(fname))
	err = os.MkdirAll(fpath, os.ModePerm)
	if err != nil {
		t.Fatal(err)
	}

	err = ioutil.WriteFile(filepath.Join(fpath, filepath.Base(fname)), []byte(content), 0644)
	if err != nil {
		t.Fatal(err)
	}
}

func removeIfNoErrors(t *testing.T, testDir string) {
	if !t.Failed() {
		_ = os.RemoveAll(testDir)
	}
}

func mustGenBundleDir(t *testing.T, testDir, ver, bundle, dirName string) {
	dirPath := filepath.Join(testDir, "image", ver, bundle, dirName)
	mustMkdir(t, dirPath)
}

var serverINITemplate = template.Must(template.New("server.ini").Parse(`
[Server]
emptydir={{.testDir}}/empty/
imagebase={{.testDir}}/image/
outputdir={{.testDir}}/www/

[Debuginfo]
banned=true
lib=/usr/lib/debug/
src=/usr/src/debug/
`))

func mustInitServerINI(t *testing.T, testDir string) {
	f, err := os.Create(filepath.Join(testDir, "server.ini"))
	if err != nil {
		t.Fatal(err)
	}

	err = serverINITemplate.Execute(f, map[string]interface{}{"testDir": testDir})
	if err != nil {
		t.Fatal(err)
	}
}

func mustTrackBundle(t *testing.T, testDir, ver, bundle string) {
	bundlesDir := filepath.Join(testDir, "image", ver, bundle, "usr/share/clear/bundles")
	if err := os.MkdirAll(bundlesDir, os.ModePerm); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Create(filepath.Join(bundlesDir, bundle)); err != nil {
		t.Fatal(err)
	}
}

func mustInitOSRelease(t *testing.T, testDir, ver string) {
	var err error
	osReleaseDir := filepath.Join(testDir, "image", ver, "os-core", "usr/lib")
	err = os.MkdirAll(osReleaseDir, os.ModePerm)
	if err != nil {
		t.Fatal(err)
	}

	err = ioutil.WriteFile(filepath.Join(osReleaseDir, "os-release"), []byte(ver), 0644)
	if err != nil {
		t.Fatal(err)
	}
}

func mustSetLatestVer(t *testing.T, testDir, ver string) {
	err := ioutil.WriteFile(filepath.Join(testDir, "image/LAST_VER"), []byte(ver), 0644)
	if err != nil {
		t.Fatal(err)
	}
}

func mustInitIncludesFile(t *testing.T, testDir, ver, bundle string, includes []string) {
	noshipDir := filepath.Join(testDir, "image", ver, "noship")
	if err := os.MkdirAll(noshipDir, os.ModePerm); err != nil {
		t.Fatal(err)
	}

	ib := []byte(strings.Join(includes, "\n") + "\n")
	if err := ioutil.WriteFile(filepath.Join(noshipDir, bundle+"-includes"), ib, 0644); err != nil {
		t.Fatal(err)
	}
}

func resetHash() {
	Hashes = []*string{&AllZeroHash}
	invHash = map[string]Hashval{AllZeroHash: 0}
}

func mustMkdir(t *testing.T, name string) {
	err := os.Mkdir(name, 0755)
	if err != nil {
		t.Fatal(err)
	}
}

func mustExist(t *testing.T, name string) {
	_, err := os.Stat(name)
	if err != nil {
		t.Fatal(err)
	}
}

func mustExistDelta(t *testing.T, testDir, filename string, from, to uint32) {
	var fromFull *Manifest
	var toFull *Manifest
	var err error
	if fromFull, err = ParseManifestFile(filepath.Join(testDir, "www", fmt.Sprintf("%d", from), "Manifest.full")); err != nil {
		t.Fatalf("Failed to load from manifest to read hash from: %q", err)
	}
	if toFull, err = ParseManifestFile(filepath.Join(testDir, "www", fmt.Sprintf("%d", to), "Manifest.full")); err != nil {
		t.Fatalf("Failed to load to manifest to read hash from: %q", err)
	}

	var fileNeeded = &File{Name: filename}
	fromHash := fileNeeded.findFileNameInSlice(fromFull.Files).Hash
	toHash := fileNeeded.findFileNameInSlice(toFull.Files).Hash

	suffix := fmt.Sprintf("%d-%d-%s-%s", from, to, fromHash, toHash)
	deltafile := filepath.Join(testDir, "www", fmt.Sprintf("%d", to), "delta", suffix)

	mustExist(t, deltafile)
}

func mustNotExist(t *testing.T, name string) {
	_, err := os.Stat(name)
	if !os.IsNotExist(err) {
		if err == nil {
			t.Fatalf("file %s exists, but want file not to exist", name)
		}
		t.Fatalf("got error %q, but want file does not exist error", err)
	}
}

func mustCreateManifestsStandard(t *testing.T, ver uint32, testDir string) *MoM {
	return mustCreateManifests(t, ver, 0, 1, testDir)
}

func mustCreateManifests(t *testing.T, ver uint32, minVer uint32, format uint, testDir string) *MoM {
	mom, err := CreateManifests(ver, minVer, format, testDir)
	if err != nil {
		t.Fatal(err)
	}
	return mom
}

func checkManifestContains(t *testing.T, testDir, ver, name string, subs ...string) {
	manFpath := filepath.Join(testDir, "www", ver, "Manifest."+name)
	b, err := ioutil.ReadFile(manFpath)
	if err != nil {
		t.Error(err)
		return
	}

	for _, sub := range subs {
		if !bytes.Contains(b, []byte(sub)) {
			t.Errorf("%s/Manifest.%s did not contain expected '%s'", ver, name, sub)
		}
	}
}

func checkManifestNotContains(t *testing.T, testDir, ver, name string, subs ...string) {
	manFpath := filepath.Join(testDir, "www", ver, "Manifest."+name)
	b, err := ioutil.ReadFile(manFpath)
	if err != nil {
		t.Error(err)
		return
	}

	for _, sub := range subs {
		if bytes.Contains(b, []byte(sub)) {
			t.Errorf("%s/Manifest.%s contained unexpected '%s'", ver, name, sub)
		}
	}
}

func checkManifestMatches(t *testing.T, testDir, ver, name string, res ...*regexp.Regexp) {
	manFpath := filepath.Join(testDir, "www", ver, "Manifest."+name)
	b, err := ioutil.ReadFile(manFpath)
	if err != nil {
		t.Error(err)
		return
	}

	for _, re := range res {
		if !re.Match(b) {
			t.Errorf("%v not found in %s/Manifest.%s", re.String(), ver, name)
		}
	}
}

func mustCreateAllDeltas(t *testing.T, manifest, statedir string, from, to uint32) {
	deltas, err := CreateDeltas(manifest, statedir, from, to)
	if err != nil {
		t.Fatalf("couldn't create deltas for %s: %s", manifest, err)
	}

	for _, d := range deltas {
		if d.Error != nil {
			t.Errorf("couldn't create delta for %s %d -> %s %d: %s", d.from.Name, d.from.Version, d.to.Name, d.to.Version, err)
		}
	}

	if t.Failed() {
		t.Fatalf("couldn't create all deltas due to errors above")
	}
}

// testFileSystem is a struct that has a base directory and a testing.T and can be used to
// perform file system actions and handling unexpected errors with t.Fatal. It is to be
// used in places where filesystem is expected to work correctly and the subject of the test
// is something else. Use it like
//
// func TestMyTest(t *testing.T) {
// 	fs := newTestFileSystem(t, "my-test-")
// 	defer fs.cleanup()
// 	// ...
// }
type testFileSystem struct {
	Dir string
	t   *testing.T
}

// TODO: Consider making a swupdTest struct, similar in spirit to testFileSystem, which
// would contain a filesystem and provide easier access to the functionality, i.e. it
// could keep a state for versions we are creating, and have support code to copy the
// bundle files around.

func newTestFileSystem(t *testing.T, prefix string) *testFileSystem {
	dir, err := ioutil.TempDir("", prefix)
	if err != nil {
		t.Fatalf("couldn't create test temporary directory: %s", err)
	}
	return &testFileSystem{
		Dir: dir,
		t:   t,
	}
}

func (fs *testFileSystem) cleanup() {
	if fs.t.Failed() {
		fmt.Printf("Keeping directory %s because test failed\n", fs.Dir)
		return
	}
	_ = os.RemoveAll(fs.Dir)
}

func (fs *testFileSystem) write(subpath, content string) {
	path := filepath.Join(fs.Dir, subpath)
	err := os.MkdirAll(filepath.Dir(path), 0755)
	if err != nil {
		fs.t.Fatalf("couldn't create directory to write file: %s", err)
	}
	err = ioutil.WriteFile(path, []byte(content), 0644)
	if err != nil {
		fs.t.Fatal(err)
	}
}

func (fs *testFileSystem) path(subpath string) string {
	return filepath.Join(fs.Dir, subpath)
}

// Use shell cp command order instead of assignment order. Calling it cp to make readers
// remember that order. Change if we are getting confused.
func (fs *testFileSystem) cp(src, dst string) {
	dstPath := filepath.Join(fs.Dir, dst)
	err := os.MkdirAll(filepath.Dir(dstPath), 0755)
	if err != nil {
		fs.t.Fatalf("error creating target directory to copy files from %s to %s: %s", src, dst, err)
	}

	srcPath := filepath.Join(fs.Dir, src)
	cmd := exec.Command("cp", "-a", "--preserve=all", srcPath, dstPath)
	err = cmd.Run()
	if err != nil {
		fs.t.Fatalf("error copying files from %s to %s: %s", src, dst, err)
	}
}

func (fs *testFileSystem) rm(subpath string) {
	path := filepath.Join(fs.Dir, subpath)
	err := os.RemoveAll(path)
	if err != nil {
		fs.t.Fatalf("error removing %s: %s", subpath, err)
	}
}

func (fs *testFileSystem) mkdir(subpath string) {
	path := filepath.Join(fs.Dir, subpath)
	err := os.MkdirAll(path, 0755)
	if err != nil {
		fs.t.Fatalf("error creating directory %s: %s", subpath, err)
	}
}
