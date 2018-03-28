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
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/pkg/errors"

	"github.com/clearlinux/mixer-tools/helpers"
)

// GetHostAndUpstreamFormats retreives the formats for the host and the mix's
// upstream version. It attempts to determine the format for the host machine,
// and if successful, looks up the format for the desired upstream version.
func (b *Builder) GetHostAndUpstreamFormats() (string, string, error) {
	// Determine the host's format
	hostFormat, err := ioutil.ReadFile("/usr/share/defaults/swupd/format")
	if err != nil && !os.IsNotExist(err) {
		return "", "", err
	}

	// Get the upstream format
	upstreamFormat, err := b.DownloadFileFromUpstreamAsString(fmt.Sprintf("update/%s/format", b.UpstreamVer))
	if err != nil {
		return "", "", err
	}

	return string(hostFormat), upstreamFormat, nil
}

const dockerfile = `FROM scratch
ADD mixer.tar.xz /
ENV LC_ALL="en_US.UTF-8"
RUN clrtrust generate
CMD ["/bin/bash"]
`

func createDockerfile(dir string) error {
	filename := filepath.Join(dir, "Dockerfile")

	f, err := os.Create(filename)
	if err != nil {
		return errors.Wrap(err, "Failed to create Dockerfile")
	}
	defer func() {
		_ = f.Close()
	}()

	_, err = f.Write([]byte(dockerfile))
	if err != nil {
		return err
	}

	return nil
}

func getCheckSum(filename string) string {
	content, err := ioutil.ReadFile(filename)
	if err != nil {
		return ""
	}
	checksum := sha512.Sum512(content)
	return hex.EncodeToString(checksum[:])
}

func (b *Builder) dockerImageIsStale(upstreamFile, localFile string) bool {
	if _, err := os.Stat(localFile); err == nil {
		checksum, err := b.DownloadFileFromUpstreamAsString(upstreamFile + "-SHA512SUMS")
		if err == nil {
			checksum = strings.Split(checksum, " ")[0]
			if checksum == getCheckSum(localFile) {
				// File exists and is not stale
				return false
			}
		}
	}
	// File does not exist or is stale
	return true
}

func getDockerImageName(format string) string {
	return fmt.Sprintf("mixer-tools/mixer:%s", format)
}

func (b *Builder) buildDockerImage(format, ver string) error {
	// Make docker root dir if it doens't exist
	wd, _ := os.Getwd()
	dockerRoot := filepath.Join(wd, fmt.Sprintf("docker/mixer-%s", format))
	if err := os.MkdirAll(dockerRoot, 0777); err != nil {
		return errors.Wrapf(err, "Failed to generate docker work dir: %s", dockerRoot)
	}

	upstreamFile := fmt.Sprintf("/releases/%s/clear/clear-%s-mixer.tar.xz", ver, ver)
	localFile := filepath.Join(dockerRoot, "mixer.tar.xz")

	// Return early if docker image is already built and is not stale
	cmd := []string{
		"docker",
		"images",
		"-q", getDockerImageName(format),
	}
	output, err := helpers.RunCommandOutput(cmd[0], cmd[1:]...)
	if err != nil {
		return errors.Wrapf(err, "Error checking for docker image %q", getDockerImageName(format))
	}
	stale := b.dockerImageIsStale(upstreamFile, localFile)
	if !stale && output.String() != "" {
		return nil
	}

	// Download the mixer base image from upstream if it's stale
	if stale {
		fmt.Println("Downloading image from upstream...")
		if err := b.DownloadFileFromUpstream(upstreamFile, localFile); err != nil {
			return errors.Wrapf(err, "Failed to download docker image base for ver %s", ver)
		}
	}

	// Generate Dockerfile
	if err := createDockerfile(dockerRoot); err != nil {
		return err
	}

	// Build Docker image
	fmt.Println("Building Docker image...")
	cmd = []string{
		"docker",
		"build",
		"-t", getDockerImageName(format),
		"--rm",
		filepath.Join(dockerRoot, "."),
	}
	if err := helpers.RunCommandSilent(cmd[0], cmd[1:]...); err != nil {
		return errors.Wrap(err, "Failed to build Docker image")
	}

	return nil
}

// reduceDockerMounts takes a list of directory paths and reduces it to a
// minimal, non-redundant list. For example, if the list includes both "/foo"
// and "/foo/bar", then "/foo/bar" would be removed, as its parent is already
// in the list. This function requires paths to have no trailing slash.
func reduceDockerMounts(paths []string) []string {
	if len(paths) <= 1 {
		return paths
	}

	sort.Strings(paths) // Puts "/foo" before "/foo/bar"

	for i := 1; i < len(paths); i++ {
		if paths[i] == paths[i-1] || strings.HasPrefix(paths[i], paths[i-1]+"/") { // "/" is to prevent "/foobar" matching "/foo"
			paths = append(paths[:i], paths[i+1:]...)
			i-- // Because removal shifts things left
		}
	}

	return paths
}

// getDockerMounts returns a minimal list of all directories in the config that
// need to be mounted inside the container. Only the "Buiilder" and "Mixer"
// sections of the conf are parsed.
func (b *Builder) getDockerMounts() []string {
	// Returns the longest substring of path that is the path to a directory.
	var getMaxPath func(path string) string
	getMaxPath = func(path string) string {
		f, err := os.Stat(path)
		if os.IsNotExist(err) {
			// Try again on parent. This happens because some values in config
			// are paths to files that get created by the commands, but their
			// parent directory exists and needs to be mounted.
			path = filepath.Dir(path)
			return getMaxPath(path)
		} else if err != nil {
			return ""
		}
		if f.Mode().IsDir() {
			return path
		}
		return filepath.Dir(path)
	}

	wd, _ := os.Getwd()
	mounts := []string{wd}

	config := reflect.ValueOf(b.Config.Builder)
	for i := 0; i < config.NumField(); i++ {
		field := getMaxPath(config.Field(i).String())
		if !strings.HasPrefix(field, "/") {
			continue
		}
		mounts = append(mounts, field)
	}
	config = reflect.ValueOf(b.Config.Mixer)
	for i := 0; i < config.NumField(); i++ {
		field := getMaxPath(config.Field(i).String())
		if !strings.HasPrefix(field, "/") {
			continue
		}
		mounts = append(mounts, field)
	}

	return reduceDockerMounts(mounts)
}

// RunCommandInContainer will pull the content necessary to build a docker
// image capable of running the desired command, build that image, and then
// run the command in that image.
func (b *Builder) RunCommandInContainer(cmd []string) error {
	format, _, latest, err := b.getUpstreamFormatRange(b.UpstreamVer)
	if err != nil {
		return err
	}

	if err := b.buildDockerImage(format, fmt.Sprint(latest)); err != nil {
		return err
	}

	fmt.Printf("Running command in container: %q\n", strings.Join(cmd, " "))

	wd, _ := os.Getwd()

	// Build Docker image
	dockerCmd := []string{
		"docker",
		"run",
		"-i",
		"--network=host",
		"--rm",
		"--workdir", wd,
		"--entrypoint", cmd[0],
	}

	mounts := b.getDockerMounts()
	for _, path := range mounts {
		dockerCmd = append(dockerCmd, "-v", fmt.Sprintf("%s:%s", path, path))
	}

	dockerCmd = append(dockerCmd, getDockerImageName(format))
	dockerCmd = append(dockerCmd, cmd[1:]...)
	dockerCmd = append(dockerCmd, "--native")

	// Run command
	if err := helpers.RunCommand(dockerCmd[0], dockerCmd[1:]...); err != nil {
		return errors.Wrap(err, "Failed to run command in container")
	}

	return nil
}
