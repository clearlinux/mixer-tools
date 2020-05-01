// Copyright © 2018 Intel Corporation
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

	"github.com/clearlinux/mixer-tools/log"

	"github.com/pkg/errors"
)

// UpdatePreviousMixVersion updates the PREVIOUS_MIX_VERSION field in mixer.state
func (b *Builder) UpdatePreviousMixVersion(version string) error {
	b.State.Mix.PreviousMixVer = version

	return b.State.Save()
}

// UpdateMixVer sets the mix version in the builder object and writes it out to file
func (b *Builder) UpdateMixVer(version int) error {
	// Deprecate '.mixversion' --> 'mixversion'
	if _, err := os.Stat(filepath.Join(b.Config.Builder.VersionPath, ".mixversion")); err == nil {
		b.MixVerFile = ".mixversion"
		log.Warning(log.Mixer, "'.mixversion' has been deprecated. Please rename file to 'mixversion'")
	}

	b.MixVer = strconv.Itoa(version)
	b.MixVerUint32 = uint32(version)
	return ioutil.WriteFile(filepath.Join(b.Config.Builder.VersionPath, b.MixVerFile), []byte(b.MixVer), 0644)
}

// ReadVersions will initialise the mix versions (mix and clearlinux) from
// the configuration files in the version directory.
func (b *Builder) ReadVersions() error {
	// Deprecate '.mixversion' --> 'mixversion'
	if _, err := os.Stat(filepath.Join(b.Config.Builder.VersionPath, ".mixversion")); err == nil {
		b.MixVerFile = ".mixversion"
		log.Warning(log.Mixer, "'.mixversion' has been deprecated. Please rename file to 'mixversion'")
	}
	ver, err := ioutil.ReadFile(filepath.Join(b.Config.Builder.VersionPath, b.MixVerFile))
	if err != nil {
		return err
	}
	b.MixVer = strings.TrimSpace(string(ver))
	b.MixVer = strings.Replace(b.MixVer, "\n", "", -1)

	// Deprecate '.clearversion' --> 'upstreamversion'
	if _, err = os.Stat(filepath.Join(b.Config.Builder.VersionPath, ".clearversion")); err == nil {
		b.UpstreamVerFile = ".clearversion"
		log.Warning(log.Mixer, " '.clearversion' has been deprecated. Please rename file to 'upstreamversion'")
	}
	ver, err = ioutil.ReadFile(filepath.Join(b.Config.Builder.VersionPath, b.UpstreamVerFile))
	if err != nil {
		return err
	}
	b.UpstreamVer = strings.TrimSpace(string(ver))
	b.UpstreamVer = strings.Replace(b.UpstreamVer, "\n", "", -1)

	// Deprecate '.clearversion' --> 'upstreamurl'
	if _, err = os.Stat(filepath.Join(b.Config.Builder.VersionPath, ".clearurl")); err == nil {
		b.UpstreamURLFile = ".clearurl"
		log.Warning(log.Mixer, " '.clearurl' has been deprecated. Please rename file to 'upstreamurl'")
	}
	ver, err = ioutil.ReadFile(filepath.Join(b.Config.Builder.VersionPath, b.UpstreamURLFile))
	if err != nil {
		log.Warning(log.Mixer, " %s/%s does not exist, run mixer init to generate", b.Config.Builder.VersionPath, b.UpstreamURLFile)
		b.UpstreamURL = ""
	} else {
		b.UpstreamURL = strings.TrimSpace(string(ver))
		b.UpstreamURL = strings.Replace(b.UpstreamURL, "\n", "", -1)
	}

	// Parse strings into valid version numbers.
	b.MixVerUint32, err = parseUint32(b.MixVer)
	if err != nil {
		return errors.Wrapf(err, "Couldn't parse mix version")
	}
	b.UpstreamVerUint32, err = parseUint32(b.UpstreamVer)
	if err != nil {
		return errors.Wrapf(err, "Couldn't parse upstream version")
	}

	return nil
}

// writeMetaFiles writes mixer and format metadata to files
func writeMetaFiles(path, format, version string) error {
	err := os.MkdirAll(path, 0777)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(filepath.Join(path, "format"), []byte(format), 0644)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(filepath.Join(path, "mixer-src-version"), []byte(version), 0644)
}

func (b *Builder) getUpstreamFormat(version string) (string, error) {
	format, err := b.DownloadFileFromUpstreamAsString(fmt.Sprintf("update/%s/format", version))
	if err != nil {
		return "", errors.Wrapf(err, "Failed to get format for version %q", version)
	}
	return format, nil
}

func (b *Builder) getUpstreamFormatRange(version string) (format string, first, latest uint32, err error) {
	format, err = b.getUpstreamFormat(version)
	if err != nil {
		return "", 0, 0, errors.Wrap(err, "couldn't download information about upstream")
	}

	readUint32 := func(subpath string) (uint32, error) {
		str, rerr := b.DownloadFileFromUpstreamAsString(subpath)
		if rerr != nil {
			return 0, rerr
		}
		val, rerr := parseUint32(str)
		if rerr != nil {
			return 0, rerr
		}
		return val, nil
	}

	latest, err = readUint32(fmt.Sprintf("update/version/format%s/latest", format))
	if err != nil {
		return "", 0, 0, errors.Wrap(err, "couldn't read information about upstream")
	}

	// TODO: Clear Linux does produce the "first" files, but not sure mixes got
	// those. We should add those (or change this to walk previous format latest).
	first, err = readUint32(fmt.Sprintf("update/version/format%s/first", format))
	if err != nil {
		return "", 0, 0, errors.Wrap(err, "couldn't read information about upstream")
	}

	return format, first, latest, err
}

// ReplaceInfoBundles wipes the <bundle>-info files and replaces them with
// <bundle>/ directories that are empty. When the manifest creation happens it will
// mark all files in that bundles as deleted.
func (b *Builder) ReplaceInfoBundles(filenames []string) error {
	for _, f := range filenames {
		bdir := filepath.Join(b.Config.Builder.ServerStateDir, "image", b.MixVer, f)
		// This bundle is deprecated, remove the info file
		if err := os.Remove(bdir + "-info"); err != nil {
			return err
		}
		// Create the empty dir for update to mark all files as deleted
		if err := os.MkdirAll(bdir, 0755); err != nil {
			return errors.Wrapf(err, "Failed to create bundle directory: %s", bdir)
		}
	}
	return nil
}

// RemoveBundlesGroupINI removes the bundles from groups.ini and mixbundles so they are not
// tracked anymore and manifests do not get created for them
func (b *Builder) RemoveBundlesGroupINI(bundles []string) error {
	var groups []byte
	filename := filepath.Join(b.Config.Builder.ServerStateDir, "groups.ini")
	groups, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}
	groupsini := string(groups)
	for _, bundle := range bundles {
		bundleToRemove := fmt.Sprintf(`\[%s\]\ngroup=%s`, bundle, bundle)
		re := regexp.MustCompile(bundleToRemove)
		groupsini = re.ReplaceAllString(groupsini, "")
	}
	err = ioutil.WriteFile(filename, []byte(groupsini), 0644)
	if err != nil {
		return err
	}
	// Remove bundles from: 		mix   local  git    (mixbundles list)
	err = b.RemoveBundles(bundles, true, false, false)
	if err != nil {
		return err
	}

	return nil
}

// ModifyBundles goes through the mix bundle list and performs an action when it finds
// a Deprecated bundle
func (b *Builder) ModifyBundles(action func([]string) error) error {
	set, err := b.getMixBundlesListAsSet()
	if err != nil {
		return err
	}

	var deprecatedBundles []string
	for _, bundle := range set {
		if bundle.Header.Status == "Deprecated" {
			log.Info(log.Mixer, "Found deprecated bundle: "+bundle.Name)
			deprecatedBundles = append(deprecatedBundles, bundle.Name)
		}
	}
	// Call the callback function we need on the deprecated bundles
	return action(deprecatedBundles)
}

// PrintVersions prints the current mix and upstream versions, and the
// latest version of upstream.
func (b *Builder) PrintVersions() error {
	if Offline {
		b.printVersionsOffline()
		return nil
	}

	format, first, latest, err := b.getUpstreamFormatRange(b.UpstreamVer)
	if err != nil {
		return err
	}

	log.Info(log.Mixer, `
Current mix:               %d
Current upstream:          %d (format: %s)

First upstream in format:  %d
Latest upstream in format: %d
`, b.MixVerUint32, b.UpstreamVerUint32, format, first, latest)

	return nil
}

func (b *Builder) printVersionsOffline() {
	log.Info(log.Mixer, `
Current mix:               %d
Current upstream:          %d (format: %s)

First upstream in format: (not available in offline mode)
Latest upstream in format: (not available in offline mode)
`, b.MixVerUint32, b.UpstreamVerUint32, b.State.Mix.Format)
}

// UpdateVersions will validate then update both mix and upstream versions. If
// upstream version is 0, then the latest upstream version in the current
// upstream format will be taken instead.
func (b *Builder) UpdateVersions(nextMix, nextUpstream uint32, skipFormatCheck bool) error {
	var format string
	var latest uint32
	var err error

	checkUpstream := (nextUpstream != b.UpstreamVerUint32) && !Offline
	if checkUpstream {
		format, _, latest, err = b.getUpstreamFormatRange(b.UpstreamVer)
		if err != nil {
			return err
		}
	} else {
		format = b.State.Mix.Format
		latest = b.UpstreamVerUint32
	}

	if nextMix <= b.MixVerUint32 {
		return fmt.Errorf("invalid mix version to update (%d), need to be greater than current mix version (%d)", nextMix, b.MixVerUint32)
	}

	if nextUpstream == 0 {
		nextUpstream = latest
	}

	nextUpstreamStr := strconv.FormatUint(uint64(nextUpstream), 10)

	nextFormat := format
	if checkUpstream {
		if nextUpstream > latest {
			nextFormat, err = b.getUpstreamFormat(nextUpstreamStr)
			if err != nil {
				return err
			}
		}

		// Verify the version exists by checking if its Manifest.MoM is around.
		_, err = b.DownloadFileFromUpstreamAsString(fmt.Sprintf("/update/%d/Manifest.MoM", nextUpstream))
		if err != nil {
			return errors.Wrapf(err, "invalid upstream version %d", nextUpstream)
		}
	}

	log.Info(log.Mixer, `Old mix:      %d
Old upstream: %d (format: %s)

New mix:      %d
New upstream: %d (format: %s)
`, b.MixVerUint32, b.UpstreamVerUint32, format, nextMix, nextUpstream, nextFormat)

	// When changing the mix version, update PREVIOUS_MIX_VERSION so the last version
	// will be used as the previous version for the next update.
	lastVer, err := b.GetLastBuildVersion()
	if err != nil {
		// No available LAST_VER
		lastVer = "0"
	}
	err = b.UpdatePreviousMixVersion(lastVer)
	if err != nil {
		return err
	}

	mixVerContents := []byte(fmt.Sprintf("%d\n", nextMix))
	log.Info(log.Mixer, "Writing %s", b.MixVerFile)
	err = ioutil.WriteFile(filepath.Join(b.Config.Builder.VersionPath, b.MixVerFile), mixVerContents, 0644)
	if err != nil {
		return errors.Wrap(err, "couldn't write updated mix version")
	}

	upstreamVerContents := []byte(nextUpstreamStr + "\n")
	log.Info(log.Mixer, "Writing %s", b.UpstreamVerFile)
	err = ioutil.WriteFile(filepath.Join(b.Config.Builder.VersionPath, b.UpstreamVerFile), upstreamVerContents, 0644)
	if err != nil {
		return errors.Wrap(err, "couldn't write updated upstream version")
	}
	b.UpstreamVerUint32 = nextUpstream
	b.UpstreamVer = nextUpstreamStr

	if !skipFormatCheck {
		if _, err := b.CheckBumpNeeded(false); err != nil {
			return err
		}
	}

	return nil
}
