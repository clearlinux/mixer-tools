// Copyright 2017 Intel Corporation
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

package swupd

import (
	"errors"
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/go-ini/ini"
)

type dbgConfig struct {
	banned bool
	lib    string
	src    string
}

type config struct {
	stateDir  string
	emptyDir  string
	imageBase string
	outputDir string
	debuginfo dbgConfig
}

var defaultConfig = config{
	stateDir:  "/var/lib/swupd",
	emptyDir:  "/var/lib/swupd/empty",
	imageBase: "/var/lib/swupd/image",
	outputDir: "/var/lib/swupd/www",
	debuginfo: dbgConfig{
		banned: true,
		lib:    "/usr/lib/debug",
		src:    "/usr/src/debug",
	},
}

func getConfig(stateDir string) (config, error) {
	var s string
	if stateDir != "" {
		s = stateDir
	} else {
		s = defaultConfig.stateDir
	}

	return readServerINI(s, filepath.Join(s, "server.ini"))
}

// readServerINI reads the server.ini file from path. Raises an error when the file
// exists but it was unable to be loaded
func readServerINI(stateDir, path string) (config, error) {
	if !exists(path) {
		// just use defaults
		return defaultConfig, nil
	}

	userConfig := defaultConfig
	userConfig.stateDir = stateDir

	cfg, err := ini.InsensitiveLoad(path)
	if err != nil {
		// server.ini exists, but we were unable to read it
		return defaultConfig, err
	}

	if key, err := cfg.Section("Server").GetKey("emptydir"); err == nil {
		userConfig.emptyDir = key.Value()
	}

	if key, err := cfg.Section("Server").GetKey("imagebase"); err == nil {
		userConfig.imageBase = key.Value()
	}

	if key, err := cfg.Section("Server").GetKey("outputdir"); err == nil {
		userConfig.outputDir = key.Value()
	}

	if key, err := cfg.Section("Debuginfo").GetKey("banned"); err == nil {
		userConfig.debuginfo.banned = (key.Value() == "true")
	}

	if key, err := cfg.Section("Debuginfo").GetKey("lib"); err == nil {
		userConfig.debuginfo.lib = key.Value()
	}

	if key, err := cfg.Section("Debuginfo").GetKey("src"); err == nil {
		userConfig.debuginfo.src = key.Value()
	}

	return userConfig, nil
}

// readGroupsINI reads the groups.ini file from path. Raises an error when the
// groups.ini file does not exist because it is required for the build
func readGroupsINI(path string) ([]string, error) {
	if !exists(path) {
		return nil, errors.New("no groups.ini file to define bundles")
	}

	cfg, err := ini.Load(path)
	if err != nil {
		return nil, err
	}

	sections := cfg.SectionStrings()

	groups := []string{}
	for _, section := range sections {
		if section == "DEFAULT" {
			// skip "default" set by go-ini/ini
			continue
		}

		// we don't appear to need the status key at the moment
		groups = append(groups, section)
	}

	return groups, err
}

func appendUnique(ss []string, s string) []string {
	for _, e := range ss {
		if e == s {
			return ss
		}
	}

	return append(ss, s)
}

func readIncludesFile(path string) ([]string, error) {
	if !exists(path) {
		return []string{}, nil
	}

	var allIncludes []byte
	var err error
	if allIncludes, err = ioutil.ReadFile(path); err != nil {
		return []string{}, err
	}

	includes := []string{}
	for _, s := range strings.Split(string(allIncludes), "\n") {
		if s != "" {
			includes = appendUnique(includes, s)
		}
	}

	return includes, nil
}
