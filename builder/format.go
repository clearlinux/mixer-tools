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
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/clearlinux/mixer-tools/helpers"
	"github.com/pkg/errors"
)

// UpdateFormatVersion updates the builder.conf file with a new format version
func (b *Builder) UpdateFormatVersion(version string) error {
	b.Config.Swupd.Format = version

	newver := "${1}" + b.Config.Swupd.Format
	var re = regexp.MustCompile(`(FORMAT=)[0-9]*`)

	builderData, err := ioutil.ReadFile(b.BuildConf)
	if err != nil {
		return errors.Wrap(err, "Failed to read builder.conf")
	}

	builderEdit := re.ReplaceAllString(string(builderData), newver)

	var filename string
	if UseNewConfig {
		filename, err = GetConfigPath("")
		if err != nil {
			return err
		}

		return b.Config.SaveConfig(filename)
	}

	builderOut := []byte(builderEdit)
	if err = ioutil.WriteFile(b.BuildConf, builderOut, 0644); err != nil {
		return errors.Wrap(err, "Failed to write new builder.conf")
	}

	return nil
}

// CopyFullGroupsINI copies the initial ini file which has ALL bundle definitions
func (b *Builder) CopyFullGroupsINI() error {
	return helpers.CopyFile(filepath.Join(b.Config.Builder.ServerStateDir, "full_groups.ini"), filepath.Join(b.Config.Builder.ServerStateDir, "groups.ini"))
}

// RevertFullGroupsINI copies back the full ini to the manifest creator accounts for deleted bundles
func (b *Builder) RevertFullGroupsINI() error {
	return helpers.CopyFile(filepath.Join(b.Config.Builder.ServerStateDir, "groups.ini"), filepath.Join(b.Config.Builder.ServerStateDir, "full_groups.ini"))
}

func (b *Builder) getLastBuildVersion() (string, error) {
	var lastVer []byte
	var err error

	filename := filepath.Join(b.Config.Builder.ServerStateDir, "image/LAST_VER")
	if lastVer, err = ioutil.ReadFile(filename); os.IsNotExist(err) {
		// Likely the first build
		return "", nil
	} else if err != nil {
		return "", errors.Wrap(err, "Cannot find last built version")
	}
	data := string(lastVer)
	ver := strings.Split(data, "\n")

	return ver[0], nil
}

func (b *Builder) getLastBuildUpstreamVersion() (string, error) {
	lastMix, err := b.getLastBuildVersion()
	if err != nil {
		return "", err
	} else if lastMix == "" {
		return "", nil
	}

	var lastVer []byte

	filename := filepath.Join(b.Config.Builder.ServerStateDir, "update/www", lastMix, "upstreamver")
	if lastVer, err = ioutil.ReadFile(filename); os.IsNotExist(err) {
		// Likely the first build
		return "", nil
	} else if err != nil {
		return "", errors.Wrap(err, "Cannot find last built version's upstream version")
	}
	data := string(lastVer)
	ver := strings.Split(data, "\n")

	return ver[0], nil
}

// StageMixForBump prepares the mix for the two format bumps required to pass an
// upstream format boundary. The current upstreamversion is saved in a temporary
// ".bump" file, and replaced with the latest version in the format range of the
// most recent build. This process is undone via UnstageMixFromBump.
func (b *Builder) stageMixForBump() error {
	lastBuildVer, err := b.getLastBuildUpstreamVersion()
	if err != nil {
		return err
	}
	_, _, latest, err := b.getUpstreamFormatRange(lastBuildVer)
	if err != nil {
		return err
	}

	// Copy current upstreamversion to upstreamversion.bump
	vFile := filepath.Join(b.Config.Builder.VersionPath, b.MixVerFile)
	vBFile := filepath.Join(b.Config.Builder.VersionPath, b.MixVerFile+".bump")
	if err := helpers.CopyFile(vBFile, vFile); err != nil {
		return err
	}

	// Set current upstreamversion to latest
	return ioutil.WriteFile(vFile, []byte(strconv.FormatUint(uint64(latest), 10)), 0644)
}

// UnstageMixFromBump resets the upstreamversion file from the temporary ".bump"
// file, if it exists. This returns the user to their desired upstream version
// after having completed the upstream format boundary bump builds.
func (b *Builder) UnstageMixFromBump() error {
	vFile := filepath.Join(b.Config.Builder.VersionPath, b.MixVerFile)
	vBFile := filepath.Join(b.Config.Builder.VersionPath, b.MixVerFile+".bump")

	// No bump file; return early
	if _, err := os.Stat(vBFile); os.IsNotExist(err) {
		return nil
	}

	// Copy upstreamversion.bump to upstreamversion
	if err := helpers.CopyFile(vFile, vBFile); err != nil {
		return err
	}

	return os.Remove(vBFile)
}

// CheckBumpNeeded returns nil if it successfully deduces there is no format
// bump boundary being crossed.
func (b *Builder) CheckBumpNeeded() (bool, error) {
	version, err := b.getLastBuildUpstreamVersion()
	if err != nil {
		return false, err
	} else if version == "" {
		return false, nil
	}
	// Check what format our last built version is part of
	oldVer, err := b.DownloadFileFromUpstreamAsString(filepath.Join("/update", version, "format"))
	if err != nil {
		return false, errors.Wrapf(err, "Could not read format version from %s", b.UpstreamURL)
	}
	// Check what format our to-be-built version is part of
	newVer, err := b.DownloadFileFromUpstreamAsString(filepath.Join("/update", b.UpstreamVer, "format"))
	if err != nil {
		return false, errors.Wrapf(err, "Could not read format version from %s", b.UpstreamURL)
	}

	// Check both formats are real numbers
	oldFmt, err := strconv.ParseUint(string(oldVer), 10, 32)
	if err != nil {
		return false, errors.New("Old format is not a number")
	}
	newFmt, err := strconv.ParseUint(string(newVer), 10, 32)
	if err != nil {
		return false, errors.New("Old format is not a number")
	}

	// We always need to perform a format bump if these are not equal
	if oldFmt != newFmt {
		// Stage the upstreamversion file for bump
		if err = b.stageMixForBump(); err != nil {
			return false, errors.Wrap(err, "Failed to stage mix for format bump")
		}

		format, first, latest, err := b.getUpstreamFormatRange(version)
		if err != nil {
			return false, err
		}
		fmt.Printf("The upstream version for this build (%s) is outside the format range of your last mix "+
			"(format %s, upstream versions %d to %d). This build cannot be done until you complete a "+
			"format-bump build. Please run the following two commands to complete the format bump:\nmixer "+
			"build format-old\nmixer build format-new\nOnce these have completed, you can re-run this build.\n",
			b.UpstreamVer, format, first, latest)

		return true, nil
	}

	return false, nil
}
