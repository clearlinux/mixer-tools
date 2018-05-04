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

	"github.com/clearlinux/mixer-tools/config"
	"github.com/go-ini/ini"
)

type dbgConfig struct {
	banned bool
	lib    string
	src    string
}

type swupdData struct {
	config config.MixConfig

	emptyDir  string
	imageBase string
	outputDir string
	debuginfo dbgConfig
}

func swupdDataFromConfig(c config.MixConfig) swupdData {
	var sdata swupdData

	sdata.config = c

	sdata.emptyDir = filepath.Join(c.Builder.ServerStateDir, "empty")
	sdata.imageBase = filepath.Join(c.Builder.ServerStateDir, "image")
	sdata.outputDir = filepath.Join(c.Builder.ServerStateDir, "www")

	var dbg dbgConfig
	dbg.banned = c.Server.DebugInfoBanned == "true"
	dbg.lib = c.Server.DebugInfoLib
	dbg.src = c.Server.DebugInfoSrc

	sdata.debuginfo = dbg

	return sdata
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

	osCoreFound := false
	groups := []string{}
	for _, section := range sections {
		if section == "DEFAULT" {
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
