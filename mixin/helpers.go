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

	"github.com/clearlinux/mixer-tools/swupd"

	"github.com/pkg/errors"
)

func getCurrentVersion() (int, error) {
	c, err := ioutil.ReadFile("/usr/lib/os-release")
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

func setUpMixDir(bundle string, upstreamVer, mixVer int) error {
	var err error
	err = os.MkdirAll(filepath.Join(mixWS, "local-rpms"), 755)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(filepath.Join(mixWS, "builder.conf"),
		[]byte(builderConf), 0644)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(filepath.Join(mixWS, "mixversion"),
		[]byte(fmt.Sprintf("%d", mixVer)), 0644)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(filepath.Join(mixWS, "mixbundles"),
		[]byte(fmt.Sprintf("%s\nos-core", bundle)), 0644)
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

func setUpMixDirIfNeeded(bundle string, ver, mixVer int) error {
	var err error
	if _, err = os.Stat(filepath.Join(mixWS, "builder.conf")); os.IsNotExist(err) {
		err = setUpMixDir(bundle, ver, mixVer)
		if err != nil {
			return err
		}
	}
	return nil
}
