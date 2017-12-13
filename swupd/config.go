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
	"strconv"
	"strings"

	"github.com/go-ini/ini"
)

var emptyDir = "/var/lib/update/empty/"
var imageBase = "/var/lib/update/image/"
var outputDir = "/var/lib/update/www/"
var debuginfoBanned = true
var debuginfoLib = "/usr/lib/debug"
var debuginfoSrc = "/usr/src/debug"

func readServerINI(path string) error {
	if !exists(path) {
		// just use defaults
		return nil
	}

	cfg, err := ini.InsensitiveLoad(path)
	if err != nil {
		// server.ini exists, but we were unable to read it
		return err
	}

	if ed := cfg.Section("Server").Key("emptydir").Value(); ed != "" {
		emptyDir = ed
	}

	if ib := cfg.Section("Server").Key("imagebase").Value(); ib != "" {
		imageBase = ib
	}

	if od := cfg.Section("Server").Key("outputdir").Value(); od != "" {
		outputDir = od
	}

	if db := cfg.Section("Debuginfo").Key("banned").Value(); db != "" {
		debuginfoBanned = (db == "true")
	}

	if dl := cfg.Section("Debuginfo").Key("lib").Value(); dl != "" {
		debuginfoLib = dl
	}

	if ds := cfg.Section("Debuginfo").Key("src").Value(); ds != "" {
		debuginfoSrc = ds
	}

	return nil
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
			// better way to do this?
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
