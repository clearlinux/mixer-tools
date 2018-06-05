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

package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/clearlinux/mixer-tools/config"
	"github.com/clearlinux/mixer-tools/helpers"
	"github.com/clearlinux/mixer-tools/swupd"

	"github.com/pkg/errors"
)

func getCurrentVersion() (int, error) {
	// if upstreamversion exists, use that
	c, err := ioutil.ReadFile(filepath.Join(mixWS, "upstreamversion"))
	if err == nil {
		return strconv.Atoi(strings.TrimSpace(string(c)))
	}

	// if upstreamversion does not exist, this is the first time
	// the workspace has been set up; read from /usr/lib/os-release
	c, err = ioutil.ReadFile("/usr/lib/os-release")
	if err != nil {
		return -1, err
	}

	re := regexp.MustCompile(`(?m)^VERSION_ID=(\d+)$`)
	m := re.FindStringSubmatch(string(c))
	if len(m) == 0 {
		return -1, errors.New("unable to determine OS version")
	}

	v, err := strconv.Atoi(m[1])
	if err != nil {
		return -1, err
	}

	return v, nil
}

func getLastVersion() int {
	c, err := ioutil.ReadFile(filepath.Join(mixWS, "update/image/LAST_VER"))
	if err != nil {
		return 0
	}

	v, err := strconv.Atoi(string(c))
	if err != nil {
		return 0
	}

	return v
}

func appendToFile(filename, text string) error {
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}

	defer func() {
		_ = f.Close()
	}()

	if _, err = f.WriteString(text); err != nil {
		return err
	}

	return nil
}

func excludeName(man *swupd.Manifest, exclude string) {
	for i := range man.Files {
		if man.Files[i].Name == exclude {
			man.Files = append(man.Files[:i], man.Files[i+1:]...)
			break
		}
	}
}

func setUpMixDir(upstreamVer, mixVer int) error {
	var err error
	err = os.MkdirAll(filepath.Join(mixWS, "local-rpms"), 755)
	if err != nil {
		return err
	}
	var c config.MixConfig
	c.LoadDefaultsForPath(true, "/usr/share/mix")
	c.Swupd.Bundle = "os-core"
	c.Swupd.ContentURL = "file:///usr/share/mix/update/www"
	c.Swupd.VersionURL = "file:///usr/share/mix/update/www"
	if err = c.SaveConfig(); err != nil {
		return err
	}
	err = ioutil.WriteFile(filepath.Join(mixWS, "mixversion"),
		[]byte(fmt.Sprintf("%d", mixVer)), 0644)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(filepath.Join(mixWS, "upstreamurl"),
		[]byte("https://download.clearlinux.org"), 0644)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(filepath.Join(mixWS, "upstreamversion"), []byte(fmt.Sprintf("%d", upstreamVer)), 0644)
}

func setUpMixDirIfNeeded(ver, mixVer int) error {
	var err error
	if _, err = os.Stat(filepath.Join(mixWS, "builder.conf")); os.IsNotExist(err) {
		err = setUpMixDir(ver, mixVer)
		if err != nil {
			return err
		}
	}
	return nil
}

func parseHeaderNoopInstall(pkg, installOut string) (string, error) {
	parts := strings.Split(installOut, "Installing:\n")
	if len(parts) < 2 {
		// dnf failure - no such package
		return "", errors.New("no such package")
	}
	var r = regexp.MustCompile(fmt.Sprintf(` (%s)\s+\S+\s+\S+\s+(\S+)\s+\S+ \S+\n`, pkg))
	matches := r.FindStringSubmatch(parts[1])
	if len(matches) == 3 {
		return matches[2], nil
	}
	return "", errors.New("unable to find repo for package")
}

func getPackageRepo(pkg string, ver int, configFile string) (string, error) {
	packagerCmd := []string{
		"dnf",
		"--config=" + configFile,
		fmt.Sprintf("--releasever=%d", ver),
		"install",
		"--assumeno",
		pkg,
	}

	// ignore error here because passing --assumeno to dnf install always
	// results in an error due to the aborted install. Instead rely on the
	// error to come from parseHeaderNoopInstall
	outBuf, _ := helpers.RunCommandOutput(packagerCmd[0], packagerCmd[1:]...)
	return parseHeaderNoopInstall(pkg, outBuf.String())
}
