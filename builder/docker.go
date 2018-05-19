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

func (b *Builder) getDockerImageName(format string) string {
	return fmt.Sprintf("%s:%s", b.Config.Mixer.DockerImgPath, format)
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

// canAccess checks whether mixer has read/write access to the given directory.
// A nil error means success.
func canAccess(dir string) error {
	// Check read access
	if _, err := ioutil.ReadDir(dir); err != nil {
		return err
	}
	// Check write acess
	f, err := ioutil.TempFile(dir, "")
	if err != nil {
		return fmt.Errorf("open %s: permission denied", dir)
	}
	defer func() {
		_ = os.Remove(f.Name())
		_ = f.Close()
	}()

	return nil
}

// getPathDir returns the directory portion of a config file path. If the path
// is already to a directory, the path is returned unchanged. If it is instead
// to a file or does not exist, its parent is returned. This is needed because
// some values in the config are paths to files or directories that get created
// by the commands, but their parent directory exists and needs to be mounted.
func getDirFromConfigPath(path string) (string, error) {
	f, err := os.Stat(path)
	if os.IsNotExist(err) || (err == nil && !f.Mode().IsDir()) {
		path = filepath.Dir(path)
		f, err = os.Stat(path)
	}
	if err != nil {
		return "", err
	}

	return path, nil
}

// addConfigFieldPaths loops through each field in a config section, verifying
// and adding its value to the mounts slice.
func addConfigFieldPaths(config reflect.Value, mounts *[]string) error {
	for i := 0; i < config.NumField(); i++ {
		path, err := getDirFromConfigPath(config.Field(i).String())
		if err != nil {
			return err
		}
		if !strings.HasPrefix(path, "/") { // filepath.Dir can return "."
			continue
		}
		if err := canAccess(path); err != nil {
			return err
		}
		*mounts = append(*mounts, path)
	}
	return nil
}

// getDockerMounts returns a minimal list of all directories in the config that
// need to be mounted inside the container. Only the "Buiilder" and "Mixer"
// sections of the conf are parsed.
func (b *Builder) getDockerMounts() ([]string, error) {
	wd, _ := os.Getwd()
	mounts := []string{wd}

	err := addConfigFieldPaths(reflect.ValueOf(b.Config.Builder), &mounts)
	if err != nil {
		return nil, err
	}

	err = addConfigFieldPaths(reflect.ValueOf(b.Config.Mixer), &mounts)
	if err != nil {
		return nil, err
	}

	return reduceDockerMounts(mounts), nil
}

// RunCommandInContainer will pull the content necessary to build a docker
// image capable of running the desired command, build that image, and then
// run the command in that image.
func (b *Builder) RunCommandInContainer(cmd []string) error {
	format, err := b.getUpstreamFormat(b.UpstreamVer)
	if err != nil {
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

	mounts, err := b.getDockerMounts()
	if err != nil {
		return errors.Wrap(err, "Failed to extract mountable directories from config")
	}
	for _, path := range mounts {
		dockerCmd = append(dockerCmd, "-v", fmt.Sprintf("%s:%s", path, path))
	}

	dockerCmd = append(dockerCmd, b.getDockerImageName(format))
	dockerCmd = append(dockerCmd, cmd[1:]...)
	dockerCmd = append(dockerCmd, "--native")

	// Run command
	if err := helpers.RunCommand(dockerCmd[0], dockerCmd[1:]...); err != nil {
		return errors.Wrap(err, "Failed to run command in container")
	}

	return nil
}
