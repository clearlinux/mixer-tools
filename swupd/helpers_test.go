package swupd

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
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

func mustNotExist(t *testing.T, name string) {
	_, err := os.Stat(name)
	if !os.IsNotExist(err) {
		if err == nil {
			t.Fatalf("file %s exists, but want file not to exist", name)
		}
		t.Fatalf("got error %q, but want file does not exist error", err)
	}
}

func mustCreateManifestsStandard(t *testing.T, ver uint32, testDir string) {
	mustCreateManifests(t, ver, 0, 1, testDir)
}

func mustCreateManifests(t *testing.T, ver uint32, minVer uint32, format uint, testDir string) {
	if _, err := CreateManifests(ver, minVer, format, testDir); err != nil {
		t.Fatal(err)
	}
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
