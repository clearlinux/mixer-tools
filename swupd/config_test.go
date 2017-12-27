package swupd

import (
	"reflect"
	"testing"
)

func TestReadServerINI(t *testing.T) {
	if c := readServerINI("nowhere", "noINI"); c != defaultConfig {
		// should just leave the defaults in place
		t.Error("generated config was the not the expected default config")
	}

	var c config
	if c = readServerINI("/var/lib/update", "testdata/server.ini"); c == defaultConfig {
		t.Error("generated config was the same as the default config")
	}

	if c.emptyDir != "/var/lib/update/emptytest/" ||
		c.imageBase != "/var/lib/update/imagetest/" ||
		c.outputDir != "/var/lib/update/wwwtest/" ||
		c.debuginfo.banned != true ||
		c.debuginfo.lib != "/usr/lib/debugtest/" ||
		c.debuginfo.src != "/usr/src/debugtest/" {
		t.Errorf("%v\n%v\n%v\n%v\n%v\n",
			c.imageBase, c.outputDir, c.debuginfo.banned, c.debuginfo.lib, c.debuginfo.src)
	}
}

func TestReadGroupsINI(t *testing.T) {
	var err error
	if _, err = readGroupsINI("nowhere"); err == nil {
		// readGroupsINI does raise an error when the groups.ini file does not exist
		// because it is required for the build
		t.Error("readGroupsINI did not raise an error on a non-existent file")
	}

	if _, err = readGroupsINI("testdata/groups.ini.no-os-core"); err == nil {
		t.Error("readGroupsINI did not raise an error for groups.ini file with no os-core listed")
	}

	var groups []string
	if groups, err = readGroupsINI("testdata/groups.ini"); err != nil {
		t.Error(err)
	}

	expected := []string{"os-core", "test-bundle", "test-bundle2"}
	if !reflect.DeepEqual(groups, expected) {
		t.Errorf("groups %v did not match expected %v", groups, expected)
	}
}

func TestReadLastVerFile(t *testing.T) {
	var err error
	if _, err = readLastVerFile("nowhere"); err == nil {
		// readLastVerFile raises an error when the file does not exist
		// because it is necessary for the build
		t.Error("readLastVerFile did not raise an error on a non-existent file")
	}

	if _, err = readLastVerFile("testdata/BAD_LAST_VER"); err == nil {
		t.Error("readLastVerFile did not raise an error on file with invalid content")
	}

	var lastVer uint32
	if lastVer, err = readLastVerFile("testdata/LAST_VER"); err != nil {
		t.Error(err)
	}

	if lastVer != 10 {
		t.Errorf("readLastVer returned %v when 10 was expected", lastVer)
	}
}

func TestReadIncludesFile(t *testing.T) {
	var err error
	if _, err = readIncludesFile("nowhere"); err != nil {
		// this just means there are no includes
		// there should be no error
		t.Error(err)
	}

	var includes []string
	if includes, err = readIncludesFile("testdata/test-bundle-includes"); err != nil {
		t.Error(err)
	}

	expected := []string{"test-bundle1", "test-bundle2"}
	if !reflect.DeepEqual(includes, expected) {
		t.Errorf("includes %v did not match expected %v", includes, expected)
	}
}
