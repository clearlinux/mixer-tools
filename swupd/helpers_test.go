package swupd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"
	"text/template"
)

func removeAllIgnoreErr(dir string) {
	_ = os.RemoveAll(dir)
}

func mustInitStandardTest(t *testing.T, testDir, lastVer, ver string, bundles []string) {
	t.Helper()
	mustInitTestDir(t, testDir)
	mustInitServerINI(t, testDir)
	mustInitGroupsINI(t, testDir, bundles)
	for _, b := range bundles {
		mustTrackBundle(t, testDir, ver, b)
	}
	mustInitOSRelease(t, testDir, ver)
}

func mustInitTestDir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(path, "image"), os.ModePerm); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(path, "www"), os.ModePerm); err != nil {
		t.Fatal(err)
	}
}

func mustInitGroupsINI(t *testing.T, testDir string, bundles []string) {
	t.Helper()
	bs := []byte("[os-core]\ngroup=os-core\nstatus=ACTIVE\n")
	for _, b := range bundles {
		bs = append(bs, []byte(fmt.Sprintf("[%s]\ngroup=%s\nstatus=ACTIVE\n", b, b))...)
	}

	if err := ioutil.WriteFile(filepath.Join(testDir, "groups.ini"), bs, 0644); err != nil {
		t.Fatal(err)
	}
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
	t.Helper()
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
	t.Helper()
	bundlesDir := filepath.Join(testDir, "image", ver, bundle, "usr/share/clear/bundles")
	if err := os.MkdirAll(bundlesDir, os.ModePerm); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Create(filepath.Join(bundlesDir, bundle)); err != nil {
		t.Fatal(err)
	}
}

func mustInitOSRelease(t *testing.T, testDir, ver string) {
	t.Helper()
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

func resetHash() {
	Hashes = []*string{&AllZeroHash}
	invHash = map[string]Hashval{AllZeroHash: 0}
}

func mustMkdir(t *testing.T, name string) {
	t.Helper()
	err := os.MkdirAll(name, 0755)
	if err != nil {
		t.Fatal(err)
	}
}

func mustExist(t *testing.T, name string) {
	t.Helper()
	_, err := os.Stat(name)
	if err != nil {
		t.Fatal(err)
	}
}

func existDelta(t *testing.T, testDir, filename string, from, to uint32) string {
	t.Helper()
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
	return deltafile
}

func mustExistDelta(t *testing.T, testDir, filename string, from, to uint32) {
	deltafile := existDelta(t, testDir, filename, from, to)
	mustExist(t, deltafile)
}

func mustNotExistDelta(t *testing.T, testDir, filename string, from, to uint32) {
	deltafile := existDelta(t, testDir, filename, from, to)
	mustNotExist(t, deltafile)
}

func mustNotExist(t *testing.T, name string) {
	t.Helper()
	_, err := os.Stat(name)
	if !os.IsNotExist(err) {
		if err == nil {
			t.Fatalf("file %s exists, but want file not to exist", name)
		}
		t.Fatalf("got error %q, but want file does not exist error", err)
	}
}

func mustCreateManifestsStandard(t *testing.T, ver, previousVer uint32, testDir string) *MoM {
	t.Helper()
	return mustCreateManifests(t, ver, previousVer, 0, 1, testDir)
}

func mustCreateManifests(t *testing.T, ver, previousVer, minVer uint32, format uint, testDir string) *MoM {
	t.Helper()
	mom, err := CreateManifests(ver, previousVer, minVer, format, testDir, runtime.NumCPU())
	if err != nil {
		t.Fatal(err)
	}
	return mom
}

func checkManifestFileCount(ts *testSwupd, version, manifest string, files, deleted int) {

	manifestFile := filepath.Join(ts.Dir, "www", version, "Manifest."+manifest)
	m, err := ParseManifestFile(manifestFile)
	if err != nil {
		ts.t.Fatalf("Couldn't parse manifest %s: %s", manifestFile, err)
	}
	if len(m.Files) != files {
		ts.t.Fatalf(
			"Number of files in Manifest %s: %d is different from the expected: %d.",
			manifestFile, len(m.Files), files)
	}

	if len(m.DeletedFiles) != deleted {
		ts.t.Fatalf(
			"Number of deleted files in Manifest %s: %d is different from the expected: %d.",
			manifestFile, len(m.DeletedFiles), deleted)
	}
}

func checkManifestContainsFile(t *testing.T, manFpath string, subs ...string) {
	t.Helper()
	b, err := ioutil.ReadFile(manFpath)
	if err != nil {
		t.Error(err)
		return
	}

	for _, sub := range subs {
		if !bytes.Contains(b, []byte(sub)) {
			t.Errorf("%s did not contain expected '%s'", manFpath, sub)
		}
	}
}

func checkManifestContains(t *testing.T, testDir, ver, name string, subs ...string) {
	t.Helper()
	manFpath := filepath.Join(testDir, "www", ver, "Manifest."+name)
	checkManifestContainsFile(t, manFpath, subs...)
}

func checkManifestNotContains(t *testing.T, testDir, ver, name string, subs ...string) {
	t.Helper()
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
	t.Helper()
	manFpath := filepath.Join(testDir, "www", ver, "Manifest."+name)
	b, err := ioutil.ReadFile(manFpath)
	if err != nil {
		t.Error(err)
		return
	}

	for _, re := range res {
		if !re.Match(b) {
			t.Errorf("%v not found in %s", re.String(), manFpath)
		}
	}
}

func mustCreateAllDeltas(t *testing.T, manifest, statedir string, from, to uint32) {
	t.Helper()
	deltas, err := CreateDeltasForManifest(manifest, statedir, from, to, 0)
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

func tryCreateAllDeltas(t *testing.T, manifest, statedir string, from, to uint32) {
	t.Helper()
	_, err := CreateDeltasForManifest(manifest, statedir, from, to, 0)
	if err != nil {
		t.Fatalf("couldn't create deltas for %s: %s", manifest, err)
	}
}

func checkFileInManifest(t *testing.T, m *Manifest, version uint32, name string) {
	t.Helper()
	for _, f := range m.Files {
		if f.Name == name {
			if f.Version == version {
				return
			}
			t.Errorf("in manifest %s version %d: file %s has version %d but expected %d",
				m.Name, m.Header.Version, f.Name, f.Version, version)
			return
		}
	}
	t.Errorf("couldn't find file %s in manifest %s version %d", name, m.Name, m.Header.Version)
}

func fileInManifestHash(t *testing.T, m *Manifest, version uint32, name string, hash string) *File {
	t.Helper()
	for _, f := range m.Files {
		if f.Name == name {
			if f.Version != version {
				t.Fatalf("in manifest %s version %d: file %s has version %d but expected %d", m.Name, m.Header.Version, f.Name, f.Version, version)
			}
			if f.Hash.String() != hash {
				t.Fatalf("in manifest %s version %d: file %s has hash %s but expected %s", m.Name, m.Header.Version, f.Name, f.Hash.String(), hash)
			}
			return f
		}
	}
	t.Fatalf("couldn't find file %s in manifest %s version %d", name, m.Name, m.Header.Version)
	return nil
}

func fileInManifest(t *testing.T, m *Manifest, version uint32, name string) *File {
	t.Helper()
	for _, f := range m.Files {
		if f.Name == name {
			if f.Version == version {
				return f
			}
			t.Fatalf("in manifest %s version %d: file %s has version %d but expected %d", m.Name, m.Header.Version, f.Name, f.Version, version)
		}
	}
	t.Fatalf("couldn't find file %s in manifest %s version %d", name, m.Name, m.Header.Version)
	return nil
}

func fileNotInManifest(t *testing.T, m *Manifest, name string) {
	t.Helper()
	for _, f := range m.Files {
		if f.Name == name {
			t.Fatalf("unexpectedly found file %s with version %d in manifest %s version %d", f.Name, f.Version, m.Name, m.Header.Version)
		}
	}
}

func fileDeletedInManifest(t *testing.T, m *Manifest, version uint32, name string) {
	t.Helper()
	f := fileInManifest(t, m, version, name)
	if f.Status != StatusDeleted {
		t.Fatalf("manifest %s version %d has file %s marked as %q, but expected \"d\" (deleted)", m.Name, m.Header.Version, name, f.Status)
	}
	// TODO: Later on worth having a different helper function for handling delete/rename.
	if f.Hash.String() != AllZeroHash {
		t.Fatalf("manifest %s version %d has deleted file %s with hash %s, but expected %s", m.Name, m.Header.Version, f.Name, f.Hash, AllZeroHash)

	}
	if f.Version != version {
		t.Fatalf("manifest %s version %d has deleted file %s with version %d, but expected %d", m.Name, m.Header.Version, f.Name, f.Version, version)
	}
}

func checkIncludes(t *testing.T, m *Manifest, includes ...string) {
	t.Helper()
	if len(m.Header.Includes) != len(includes) {
		t.Errorf("manifest %s in version %d has %d includes but expected %d", m.Name, m.Header.Version, len(m.Header.Includes), len(includes))
	}

	for _, inc := range includes {
		var found bool
		for _, b := range m.Header.Includes {
			if b.Name == inc {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("couldn't find include %s in manifest %s version %d", inc, m.Name, m.Header.Version)
		}
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
//
// See also testSwupd struct, that has a testFileSystem plus other swupd specific facilities.
type testFileSystem struct {
	Dir string
	t   *testing.T
}

func newTestFileSystem(t *testing.T, prefix string) *testFileSystem {
	t.Helper()
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
	fs.t.Helper()
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

func (fs *testFileSystem) initBundleInfo(version uint32, bundle string, includes []string) {
	bi := BundleInfo{
		Name:           bundle,
		DirectIncludes: includes,
		DirectPackages: make(map[string]bool),
		Files:          make(map[string]bool),
	}

	b, err := json.Marshal(&bi)
	if err != nil {
		fs.t.Fatal(err)
	}

	path := filepath.Join(fs.Dir, "image", fmt.Sprint(version), bundle+"-info")
	err = os.MkdirAll(filepath.Dir(path), 0755)
	if err != nil {
		fs.t.Fatal(err)
	}
	err = ioutil.WriteFile(path, b, 0644)
	if err != nil {
		fs.t.Fatal(err)
	}
}

func (fs *testFileSystem) addDirToFullChroot(version uint32, dir string) {
	fileDir := filepath.Join(fs.Dir, "image", fmt.Sprint(version), "full", dir)
	err := os.MkdirAll(fileDir, 0755)
	if err != nil {
		fs.t.Fatal(err)
	}
}

func (fs *testFileSystem) addToFullChroot(version uint32, file, content string) {
	fs.addDirToFullChroot(version, filepath.Dir(file))
	fileDir := filepath.Join(fs.Dir, "image", fmt.Sprint(version), "full", filepath.Dir(file))
	filePath := filepath.Join(fileDir, filepath.Base(file))
	err := ioutil.WriteFile(filePath, []byte(content), 0644)
	if err != nil {
		fs.t.Fatal(err)
	}
}

func (fs *testFileSystem) addToBundleInfo(version uint32, bundle, file string) {
	bundleInfoDir := filepath.Join(fs.Dir, "image", fmt.Sprint(version))
	bundleInfoPath := filepath.Join(bundleInfoDir, bundle+"-info")
	allbundleInfoDir := filepath.Join(bundleInfoDir, "full/usr/share/clear/allbundles")
	allbundleInfoPath := filepath.Join(allbundleInfoDir, bundle+"-info")

	biBytes, err := ioutil.ReadFile(bundleInfoPath)
	if err != nil {
		fs.t.Fatal(err)
	}

	var bi BundleInfo
	err = json.Unmarshal(biBytes, &bi)
	if err != nil {
		fs.t.Fatal(err)
	}

	bi.Files[file] = true

	b, err := json.Marshal(&bi)
	if err != nil {
		fs.t.Fatal(err)
	}

	// update regular info file
	if err = ioutil.WriteFile(bundleInfoPath, b, 0644); err != nil {
		fs.t.Fatal(err)
	}

	// update allbundle info file
	if err = os.MkdirAll(allbundleInfoDir, 0755); err != nil {
		fs.t.Fatal(err)
	}

	if err = ioutil.WriteFile(allbundleInfoPath, b, 0644); err != nil {
		fs.t.Fatal(err)
	}
}

func (fs *testFileSystem) addHeaderToBundleInfo(version uint32, bundle string, header BundleHeader) {
	bundleInfoPath := filepath.Join(fs.Dir, "image", fmt.Sprint(version), bundle+"-info")
	biBytes, err := ioutil.ReadFile(bundleInfoPath)
	if err != nil {
		fs.t.Fatal(err)
	}

	var bi BundleInfo
	err = json.Unmarshal(biBytes, &bi)
	if err != nil {
		fs.t.Fatal(err)
	}

	bi.Header = header

	b, err := json.Marshal(&bi)
	if err != nil {
		fs.t.Fatal(err)
	}

	err = ioutil.WriteFile(bundleInfoPath, b, 0644)
	if err != nil {
		fs.t.Fatal(err)
	}
}

func (fs *testFileSystem) addIncludesToBundleInfo(version uint32, bundle string, includes []string) {
	bundleInfoPath := filepath.Join(fs.Dir, "image", fmt.Sprint(version), bundle+"-info")
	biBytes, err := ioutil.ReadFile(bundleInfoPath)
	if err != nil {
		fs.t.Fatal(err)
	}

	var bi BundleInfo
	err = json.Unmarshal(biBytes, &bi)
	if err != nil {
		fs.t.Fatal(err)
	}

	bi.DirectIncludes = includes

	b, err := json.Marshal(&bi)
	if err != nil {
		fs.t.Fatal(err)
	}

	err = ioutil.WriteFile(bundleInfoPath, b, 0644)
	if err != nil {
		fs.t.Fatal(err)
	}
}

func (fs *testFileSystem) addFile(version uint32, bundle, file, content string) {
	fs.t.Helper()
	path := filepath.Join(fs.Dir, "image", fmt.Sprint(version), bundle+"-info")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		fs.initBundleInfo(version, bundle, []string{})
	}

	fs.addToFullChroot(version, file, content)
	fs.addToBundleInfo(version, bundle, file)
}

func (fs *testFileSystem) addDir(version uint32, bundle, dir string) {
	fs.t.Helper()
	fs.t.Helper()
	path := filepath.Join(fs.Dir, "image", fmt.Sprint(version), bundle+"-info")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		fs.initBundleInfo(version, bundle, []string{})
	}

	fs.addDirToFullChroot(version, dir)
	fs.addToBundleInfo(version, bundle, dir)
}

func (fs *testFileSystem) addExtraFile(version uint32, bundle, file, content string) {
	fs.t.Helper()
	path := filepath.Join(fs.Dir, bundle+"-extra-files")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		fs.t.Fatal(err)
	}

	_, err = f.WriteString(file + "\n")
	if err != nil {
		fs.t.Fatal(err)
	}

	fs.addToFullChroot(version, file, content)
}

func (fs *testFileSystem) addHeader(version uint32, bundle string, header BundleHeader) {
	fs.t.Helper()
	path := filepath.Join(fs.Dir, "image", fmt.Sprint(version), bundle+"-info")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		fs.initBundleInfo(version, bundle, []string{})
	}

	fs.addHeaderToBundleInfo(version, bundle, header)
}

func (fs *testFileSystem) addIncludes(version uint32, bundle string, includes []string) {
	fs.t.Helper()
	path := filepath.Join(fs.Dir, "image", fmt.Sprint(version), bundle+"-info")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		fs.initBundleInfo(version, bundle, []string{})
	}

	fs.addIncludesToBundleInfo(version, bundle, includes)
}

func (fs *testFileSystem) symlink(subpath, linkname string) {
	fs.t.Helper()
	path := filepath.Join(fs.Dir, subpath)
	err := os.MkdirAll(filepath.Dir(path), 0755)
	if err != nil {
		fs.t.Fatalf("couldn't create directory to write file: %s", err)
	}
	err = os.Symlink(linkname, path)
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
	fs.t.Helper()
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
	fs.t.Helper()
	path := filepath.Join(fs.Dir, subpath)
	err := os.RemoveAll(path)
	if err != nil {
		fs.t.Fatalf("error removing %s: %s", subpath, err)
	}
}

func (fs *testFileSystem) mkdir(subpath string) {
	fs.t.Helper()
	path := filepath.Join(fs.Dir, subpath)
	err := os.MkdirAll(path, 0755)
	if err != nil {
		fs.t.Fatalf("error creating directory %s: %s", subpath, err)
	}
}

func (fs *testFileSystem) exists(subpath string) bool {
	fs.t.Helper()
	path := filepath.Join(fs.Dir, subpath)
	_, err := os.Stat(path)
	switch {
	case err == nil:
		return true
	case os.IsNotExist(err):
		return false
	default:
		fs.t.Fatalf("error checking if %s exists: %s", subpath, err)
		return false
	}
}

func (fs *testFileSystem) checkExists(subpath string) {
	fs.t.Helper()
	ok := fs.exists(subpath)
	if !ok {
		fs.t.Errorf("file %s doesn't exist", subpath)
	}
}

func (fs *testFileSystem) checkNotExists(subpath string) {
	fs.t.Helper()
	ok := fs.exists(subpath)
	if ok {
		fs.t.Errorf("file %s exists but expected to not exist", subpath)
	}
}

func (fs *testFileSystem) checkContains(subpath, sub string) {
	fs.t.Helper()
	path := filepath.Join(fs.Dir, subpath)
	contents, err := ioutil.ReadFile(path)
	if err != nil {
		fs.t.Errorf("couldn't open %s to check its contents: %s", subpath, err)
	}
	if !bytes.Contains(contents, []byte(sub)) {
		fs.t.Errorf("%s did not contain expected %q", subpath, sub)
	}
}

func (fs *testFileSystem) checkNotContains(subpath, sub string) {
	fs.t.Helper()
	path := filepath.Join(fs.Dir, subpath)
	contents, err := ioutil.ReadFile(path)
	if err != nil {
		fs.t.Errorf("couldn't open %s to check its contents: %s", subpath, err)
	}
	if bytes.Contains(contents, []byte(sub)) {
		fs.t.Errorf("%s contains unexpected content: %q", subpath, sub)
	}
}

// testSwupd is a struct that keeps track of an update content state and a testing.T. It
// can be used to perform repository construction and operations and handle unexpected
// errors with t.Fatal. It does embed a testFileSystem, so all the filesystem operations
// are also available.
//
// It is to be used in places where the swupd operations (create manifests, create packs)
// are expected to return without errors and the subject of the test is their product. In
// case errors of those operations are to be tested, use the data from the struct but not
// the helper functions.
//
// Simple usage looks like
//
// func TestMyTest(t *testing.T) {
// 	ts := newTestSwupd(t, "my-test-")
// 	defer ts.cleanup()
//
//	ts.Bundles = []string{"bundle"}
//      ts.write("image/10/bundle/file", "content")
//      ts.createManifests(10)
//
//      // ...
// }
//
// For tests that require only filesystem operations, prefer testFileSystem.
type testSwupd struct {
	*testFileSystem

	Bundles         []string
	MinVersion      uint32
	PreviousVersion uint32
	Format          uint
}

func newTestSwupd(t *testing.T, prefix string) *testSwupd {
	t.Helper()
	fs := newTestFileSystem(t, prefix)
	defer func() {
		// If we failed to create a testSwupd, cleanup the fs. If we succeed
		// it is up to the caller to cleanup the testSwupd.
		if t.Failed() {
			fs.cleanup()
		}
	}()

	fs.mkdir("image")
	fs.mkdir("www")

	mustInitServerINI(t, fs.Dir)

	return &testSwupd{
		testFileSystem: fs,
		Format:         1,
	}
}

// Create Manifests and bump to next version.
func (ts *testSwupd) createManifests(version uint32) *MoM {
	ts.t.Helper()
	mustInitGroupsINI(ts.t, ts.Dir, ts.Bundles)

	for _, name := range ts.Bundles {
		ts.addFile(version, name, filepath.Join("/usr/share/clear/bundles", name), "")
	}

	osRelease := fmt.Sprintf("VERSION_ID=%d\n", version)
	ts.addFile(version, "os-core", "/usr/lib/os-release", osRelease)

	mom, err := CreateManifests(version, ts.PreviousVersion, ts.MinVersion, ts.Format, ts.Dir, runtime.NumCPU())
	if err != nil {
		ts.t.Fatalf("error creating manifests for version %d: %s", version, err)
	}

	ts.PreviousVersion = version

	return mom
}

func (ts *testSwupd) createManifestsFromChroots(version uint32) *MoM {
	ts.t.Helper()
	mustInitGroupsINI(ts.t, ts.Dir, ts.Bundles)

	for _, name := range ts.Bundles {
		ts.write(filepath.Join("image", fmt.Sprint(version), name, "usr/share/clear/bundles", name), "")
	}

	osRelease := fmt.Sprintf("VERSION_ID=%d\n", version)
	ts.write(filepath.Join("image", fmt.Sprint(version), "os-core", "usr/lib/os-release"), osRelease)

	mom, err := CreateManifests(version, ts.PreviousVersion, ts.MinVersion, ts.Format, ts.Dir, runtime.NumCPU())
	if err != nil {
		ts.t.Fatalf("error creating manifests for version %d: %s", version, err)
	}

	ts.PreviousVersion = version

	return mom
}

func (ts *testSwupd) copyChroots(fromVersion, toVersion uint32) {
	ts.t.Helper()
	from := fmt.Sprint(fromVersion)
	to := fmt.Sprint(toVersion)
	ts.mkdir(filepath.Join("image", to))
	for _, name := range ts.Bundles {
		fromSubpath := filepath.Join("image", from, name)
		if ts.exists(fromSubpath) {
			ts.cp(fromSubpath, filepath.Join("image", to))
		}
	}
}

func (ts *testSwupd) createPack(name string, from, to uint32, chrootDir string) *PackInfo {
	ts.t.Helper()
	return mustCreatePack(ts.t, name, from, to, ts.path("www"), chrootDir)
}

func (ts *testSwupd) createFullfiles(version uint32) {
	ts.t.Helper()
	filename := ts.path(filepath.Join("www", fmt.Sprint(version), "Manifest.full"))
	m, err := ParseManifestFile(filename)
	if err != nil {
		ts.t.Fatalf("couldn't parse full manifest to generate full files in test: %s", err)
	}
	chrootDir := ts.path(filepath.Join("image", fmt.Sprint(version), "full"))
	outputDir := ts.path(filepath.Join("www", fmt.Sprint(version), "files"))
	_, err = CreateFullfiles(m, chrootDir, outputDir, 0)
	if err != nil {
		ts.t.Fatalf("couldn't create fullfiles: %s", err)
	}
}

func (ts *testSwupd) parseManifest(version uint32, name string) *Manifest {
	ts.t.Helper()
	filename := ts.path(filepath.Join("www", fmt.Sprint(version), "Manifest."+name))
	m, err := ParseManifestFile(filename)
	if err != nil {
		ts.t.Fatalf("couldn't parse manifest %s for version %d: %s", name, version, err)
	}
	return m
}

func (ts *testSwupd) mustHashFile(subpath string) string {
	ts.t.Helper()
	hash, err := GetHashForFile(ts.path(subpath))
	if err != nil {
		ts.t.Fatalf("couldn't calculate hash for file %s: %s", subpath, err)
	}
	return hash
}
