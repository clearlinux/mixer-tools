package swupd

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
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
	testDir, err := ioutil.TempDir("./testdata", "cmtest-"+testName)
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

func fileContains(path string, sub []byte) bool {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return false
	}

	return bytes.Contains(b, sub)
}

func fileContainsRe(path string, re *regexp.Regexp) string {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return ""
	}

	return string(re.Find(b))
}
