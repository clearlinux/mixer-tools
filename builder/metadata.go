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
	"bufio"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

// UpdateMixVer sets the mix version in the builder object and writes it out to file
func (b *Builder) UpdateMixVer(version int) error {
	// Deprecate '.mixversion' --> 'mixversion'
	if _, err := os.Stat(filepath.Join(b.Config.Builder.VersionPath, ".mixversion")); err == nil {
		b.MixVerFile = ".mixversion"
		fmt.Println("Warning: '.mixversion' has been deprecated. Please rename file to 'mixversion'")
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
		fmt.Println("Warning: '.mixversion' has been deprecated. Please rename file to 'mixversion'")
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
		fmt.Println("Warning: '.clearversion' has been deprecated. Please rename file to 'upstreamversion'")
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
		fmt.Println("Warning: '.clearurl' has been deprecated. Please rename file to 'upstreamurl'")
	}
	ver, err = ioutil.ReadFile(filepath.Join(b.Config.Builder.VersionPath, b.UpstreamURLFile))
	if err != nil {
		log.Printf("Warning: %s/%s does not exist, run mixer init to generate\n", b.Config.Builder.VersionPath, b.UpstreamURLFile)
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

// UpdateFormatFile update the format number in the full-chroot format file
func (b *Builder) UpdateFormatFile(version int) error {
	formatFile := filepath.Join(b.Config.Builder.ServerStateDir, "image", b.MixVer, "full/usr/share/defaults/swupd/format")
	if _, err := os.Stat(formatFile); err == nil {
		return err
	}

	return ioutil.WriteFile(formatFile, []byte(strconv.Itoa(version)), 0644)
}

// ModifyBundles goes through the bundle directory and performs an action when it finds
// a Deprecated bundle
func (b *Builder) ModifyBundles(action func(string) error) error {
	path := b.Config.Mixer.LocalBundleDir
	files, err := ioutil.ReadDir(path)
	if err != nil {
		return err
	}

	var scanner *bufio.Scanner
	for _, file := range files {
		fmt.Println("CHECKING FILE: " + file.Name())
		fileToScan := filepath.Join(path, file.Name())
		f, err := os.Open(fileToScan)
		if err != nil {
			return err
		}

		// Scan the files and find which bundle definitions are marked deprecated
		scanner = bufio.NewScanner(f)
		var str string
		re := regexp.MustCompile("#\\s\\[STATUS\\]:\\s*Deprecated.*")
		for scanner.Scan() {
			str = scanner.Text()
			fmt.Println("Scanning: " + str)
			// Don't scan past header, stop once we have no more # comments
			if str[0] == '#' {
				if index := re.FindStringIndex(str); index != nil {
					fmt.Println("Found deprecated bundle: " + fileToScan)
					// Call the callback function we need on the file we're scanning
					if err = action(file.Name()); err != nil {
						return err
					}
				}
			} else {
				_ = f.Close()
				break
			}
		}
		_ = f.Close()
	}
	return nil
}

// PrintVersions prints the current mix and upstream versions, and the
// latest version of upstream.
func (b *Builder) PrintVersions() error {
	format, first, latest, err := b.getUpstreamFormatRange(b.UpstreamVer)
	if err != nil {
		return err
	}

	fmt.Printf(`
Current mix:               %d
Current upstream:          %d (format: %s)

First upstream in format:  %d
Latest upstream in format: %d
`, b.MixVerUint32, b.UpstreamVerUint32, format, first, latest)

	return nil
}

// UpdateVersions will validate then update both mix and upstream versions. If
// upstream version is 0, then the latest upstream version in the current
// upstream format will be taken instead.
func (b *Builder) UpdateVersions(nextMix, nextUpstream uint32) error {
	format, _, latest, err := b.getUpstreamFormatRange(b.UpstreamVer)
	if err != nil {
		return err
	}

	if nextMix <= b.MixVerUint32 {
		return fmt.Errorf("invalid mix version to update (%d), need to be greater than current mix version (%d)", nextMix, b.MixVerUint32)
	}

	if nextUpstream == 0 {
		nextUpstream = latest
	}

	nextUpstreamStr := strconv.FormatUint(uint64(nextUpstream), 10)

	nextFormat := format
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

	fmt.Printf(`Old mix:      %d
Old upstream: %d (format: %s)

New mix:      %d
New upstream: %d (format: %s)
`, b.MixVerUint32, b.UpstreamVerUint32, format, nextMix, nextUpstream, nextFormat)

	mixVerContents := []byte(fmt.Sprintf("%d\n", nextMix))
	err = ioutil.WriteFile(filepath.Join(b.Config.Builder.VersionPath, b.MixVerFile), mixVerContents, 0644)
	if err != nil {
		return errors.Wrap(err, "couldn't write updated mix version")
	}
	fmt.Printf("\nWrote %s.\n", b.MixVerFile)

	upstreamVerContents := []byte(nextUpstreamStr + "\n")
	err = ioutil.WriteFile(filepath.Join(b.Config.Builder.VersionPath, b.UpstreamVerFile), upstreamVerContents, 0644)
	if err != nil {
		return errors.Wrap(err, "couldn't write updated upstream version")
	}
	fmt.Printf("Wrote %s.\n", b.UpstreamVerFile)
	b.UpstreamVerUint32 = nextUpstream
	b.UpstreamVer = nextUpstreamStr

	if _, err := b.CheckBumpNeeded(false); err != nil {
		return err
	}

	return nil
}
