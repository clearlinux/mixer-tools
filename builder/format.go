// Copyright © 2017 Intel Corporation
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package builder

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/clearlinux/mixer-tools/helpers"
	"github.com/pkg/errors"
)

// UpdateFormatVersion updates the builder.conf file with a new format version
func UpdateFormatVersion(b *Builder, version string) error {
	b.Config.Swupd.Format = version
	newver := "${1}" + b.Config.Swupd.Format
	var re = regexp.MustCompile(`(FORMAT=)[0-9]*`)

	builderData, err := ioutil.ReadFile(b.BuildConf)
	if err != nil {
		return errors.Wrap(err, "Failed to read builder.conf")
	}

	builderEdit := re.ReplaceAllString(string(builderData), newver)

	builderOut := []byte(builderEdit)
	if err = ioutil.WriteFile(b.BuildConf, builderOut, 0644); err != nil {
		return errors.Wrap(err, "Failed to write new builder.conf")
	}

	return nil
}

// CopyFullGroupsINI copies the initial ini file which has ALL bundle definitions
func CopyFullGroupsINI(b *Builder) error {
	return helpers.CopyFile(filepath.Join(b.Config.Builder.ServerStateDir, "full_groups.ini"), filepath.Join(b.Config.Builder.ServerStateDir, "groups.ini"))
}

// CopyTrimmedGroupsINI copies the new ini made with deleted bundles removed
func CopyTrimmedGroupsINI(b *Builder) error {
	return helpers.CopyFile(filepath.Join(b.Config.Builder.ServerStateDir, "trimmed_groups.ini"), filepath.Join(b.Config.Builder.ServerStateDir, "groups.ini"))
}

// RevertFullGroupsINI copies back the full ini to the manifest creator accounts for deleted bundles
func RevertFullGroupsINI(b *Builder) error {
	return helpers.CopyFile(filepath.Join(b.Config.Builder.ServerStateDir, "groups.ini"), filepath.Join(b.Config.Builder.ServerStateDir, "full_groups.ini"))
}

// RevertTrimmedGroupsINI copies back the trimmed INI so manifests are not created anymore for deleted bundles in new format
func RevertTrimmedGroupsINI(b *Builder) error {
	return helpers.CopyFile(filepath.Join(b.Config.Builder.ServerStateDir, "groups.ini"), filepath.Join(b.Config.Builder.ServerStateDir, "trimmed_groups.ini"))
}

func getLastBuildVersion(b *Builder) (string, error) {
	// Likely the first build or some weirdness that requires us to get the container image and build it anyway
	var lastVer []byte
	var err error
	filename := filepath.Join(b.Config.Builder.ServerStateDir, "image/LAST_VER")
	if lastVer, err = ioutil.ReadFile(filename); err != nil {
		return "", errors.Wrap(err, "Cannot find last built version, must get container image")
	}
	data := string(lastVer)
	ver := strings.Split(data, "\n")

	return ver[0], nil
}

// CheckBumpNeeded returns nil if it successfully deduces there is no format
// bump boundary being crossed.
func CheckBumpNeeded(b *Builder) (bool, error) {
	version, err := getLastBuildVersion(b)
	if err != nil {
		return false, err
	}
	// Check what format our last built version is part of
	versionURL := fmt.Sprintf("https://download.clearlinux.org/update/%s/format", version)
	resp, err := http.Get(versionURL)
	if err != nil {
		return false, errors.Wrapf(err, "Failed to http.Get() %s", versionURL)
	}
	oldVer, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return false, errors.Wrapf(err, "Could not read format version from %s", versionURL)
	}

	// Check what format our to-be-built version is part of
	versionURL = fmt.Sprintf("https://download.clearlinux.org/update/%s/format", b.MixVer)
	resp, err = http.Get(versionURL)
	if err != nil {
		return false, errors.Wrapf(err, "Failed to http.Get() %s", versionURL)
	}
	newVer, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return false, errors.Wrapf(err, "Could not read format version from %s", versionURL)
	}

	// Check both formats are real numbers
	oldFmt, err := strconv.ParseInt(string(oldVer), 10, 64)
	if err != nil {
		return false, errors.New("Old format is not a number")
	}
	newFmt, err := strconv.ParseInt(string(newVer), 10, 64)
	if err != nil {
		return false, errors.New("Old format is not a number")
	}

	// We always need to perform a format bump if these are not equal
	if oldFmt != newFmt {
		return true, nil
	}

	return false, nil
}
