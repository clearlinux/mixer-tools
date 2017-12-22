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
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-ini/ini"
)

type dbgConfig struct {
	banned bool
	lib    string
	src    string
}

type config struct {
	emptyDir  string
	imageBase string
	outputDir string
	debuginfo dbgConfig
}

// StateDir is the directory under which swupd will create the update
// defaults to /var/lib/swupd unless overridden
var StateDir = "/var/lib/swupd"
var defaultConfig config

func setDefaultConfig() {
	defaultConfig = config{
		emptyDir:  filepath.Join(StateDir, "empty"),
		imageBase: filepath.Join(StateDir, "image"),
		outputDir: filepath.Join(StateDir, "www"),
		debuginfo: dbgConfig{
			banned: true,
			lib:    "/usr/lib/debug",
			src:    "/usr/src/debug",
		},
	}
}

func getConfig() config {
	setDefaultConfig()
	return readServerINI(filepath.Join(StateDir, "server.ini"))
}

func readServerINI(path string) config {
	if !exists(path) {
		// just use defaults
		return defaultConfig
	}

	userConfig := defaultConfig

	cfg, err := ini.InsensitiveLoad(path)
	if err != nil {
		// server.ini exists, but we were unable to read it
		return defaultConfig
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

	return userConfig
}

func readGroupsINI(path string) ([]string, error) {
	if !exists(path) {
		return nil, errors.New("no groups.ini file to define bundles")
	}

	cfg, err := ini.InsensitiveLoad(path)
	if err != nil {
		return nil, err
	}

	sections := cfg.SectionStrings()

	osCoreFound := false
	groups := []string{}
	for _, section := range sections {
		if section == "default" {
			// skip "default" set by go-ini/ini
			continue
		}

		// we don't appear to need the status key at the moment
		groups = append(groups, section)
		if !osCoreFound && section == "os-core" {
			osCoreFound = true
		}
	}

	if !osCoreFound {
		err = errors.New("os-core bundle is not listed in groups.ini")
	}

	return groups, err
}

func readLastVerFile(path string) (uint32, error) {
	if !exists(path) {
		return 0, fmt.Errorf("unable to detect last version (%v file does not exist)", path)
	}

	lastVerOut, err := ioutil.ReadFile(path)
	if err != nil {
		return 0, err
	}

	lastVerString := strings.TrimSpace(string(lastVerOut))
	parsed, err := strconv.ParseUint(lastVerString, 10, 32)
	if err != nil {
		return 0, err
	}

	return uint32(parsed), nil
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

	includes := strings.Split(string(allIncludes), "\n")
	if includes[len(includes)-1] == "" {
		// remove trailing empty string caused by trailing newline
		includes = includes[:len(includes)-1]
	}

	return includes, nil
}
