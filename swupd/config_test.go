package swupd

import (
	"reflect"
	"testing"
)

func TestReadServerINI(t *testing.T) {
	defaultEmptyDir := emptyDir
	defaultImageBase := imageBase
	defaultOutputDir := outputDir
	defaultDebuginfoBanned := debuginfoBanned
	defaultDebuginfoLib := debuginfoLib
	defaultDebuginfoSrc := debuginfoSrc
	defer func() {
		emptyDir = defaultEmptyDir
		imageBase = defaultImageBase
		outputDir = defaultOutputDir
		debuginfoBanned = defaultDebuginfoBanned
		debuginfoLib = defaultDebuginfoLib
		debuginfoSrc = defaultDebuginfoSrc
	}()

	if err := readServerINI("nowhere"); err != nil {
		// readServerINI should not raise an error for a nonexistent file
		// it should just leave the defaults in place
		t.Error(err)
	}

	if err := readServerINI("testdata/server.ini"); err != nil {
		t.Error(err)
	}

	if emptyDir != "/var/lib/update/emptytest/" ||
		imageBase != "/var/lib/update/imagetest/" ||
		outputDir != "/var/lib/update/wwwtest/" ||
		debuginfoBanned != true ||
		debuginfoLib != "/usr/lib/debugtest/" ||
		debuginfoSrc != "/usr/src/debugtest/" {
		t.Errorf("%v\n%v\n%v\n%v\n%v\n%v\n%v",
			imageBase, outputDir, debuginfoBanned, debuginfoLib, debuginfoSrc)
	}
}

func TestReadGroupsINI(t *testing.T) {
	var groups []string
	var err error
	if groups, err = readGroupsINI("nowhere"); err == nil {
		// readGroupsINI does raise an error when the groups.ini file does not exist
		// because it is required for the build
		t.Error("readGroupsINI did not raise an error on a non-existent file")
	}

	if groups, err = readGroupsINI("testdata/groups.ini.no-os-core"); err == nil {
		t.Error("readGroupsINI did not raise an error for groups.ini file with no os-core listed")
	}

	if groups, err = readGroupsINI("testdata/groups.ini"); err != nil {
		t.Error(err)
	}

	expected := []string{"os-core", "test-bundle", "test-bundle2"}
	if !reflect.DeepEqual(groups, expected) {
		t.Errorf("groups %v did not match expected %v", groups, expected)
	}
}

func TestReadLastVerFile(t *testing.T) {
	var lastVer uint32
	var err error
	if lastVer, err = readLastVerFile("nowhere"); err == nil {
		// readLastVerFile raises an error when the file does not exist
		// because it is necessary for the build
		t.Error("readLastVerFile did not raise an error on a non-existent file")
	}

	if lastVer, err = readLastVerFile("testdata/BAD_LAST_VER"); err == nil {
		t.Error("readLastVerFile did not raise an error on file with invalid content")
	}

	if lastVer, err = readLastVerFile("testdata/LAST_VER"); err != nil {
		t.Error(err)
	}

	if lastVer != 10 {
		t.Errorf("readLastVer returned %v when 10 was expected", lastVer)
	}
}

func TestReadIncludesFile(t *testing.T) {
	var includes []string
	var err error
	if includes, err = readIncludesFile("nowhere"); err != nil {
		// this just means there are no includes
		// there should be no error
		t.Error(err)
	}

	if includes, err = readIncludesFile("testdata/test-bundle-includes"); err != nil {
		t.Error(err)
	}

	expected := []string{"test-bundle1", "test-bundle2"}
	if !reflect.DeepEqual(includes, expected) {
		t.Errorf("includes %v did not match expected %v", includes, expected)
	}
}
