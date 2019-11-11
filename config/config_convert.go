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

package config

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"

	"github.com/BurntSushi/toml"
	"github.com/clearlinux/mixer-tools/helpers"
	"github.com/pkg/errors"
)

// CurrentConfigVersion holds the current version of the config file
const CurrentConfigVersion = "1.2"

func (config *MixConfig) parseVersion(reader *bufio.Reader) (bool, error) {
	verBytes, err := reader.ReadString('\n')
	if err != nil {
		return false, err
	}

	r := regexp.MustCompile("^#VERSION ([0-9]+.[0-9])+\n")
	match := r.FindStringSubmatch(string(verBytes))

	if len(match) != 2 {
		return false, nil
	}

	config.version = match[1]

	return true, nil
}

func (config *MixConfig) parseVersionAndConvert() error {
	// Reset version for files without versioning
	config.version = "0.0"

	f, err := os.Open(config.filename)
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
	}()

	// Read config version
	reader := bufio.NewReader(f)
	found, err := config.parseVersion(reader)
	if err != nil {
		return err
	}

	//Already on latest version
	if found && config.version == CurrentConfigVersion {
		return nil
	}

	fmt.Printf("Converting config to version %s\n", CurrentConfigVersion)

	if err := helpers.CopyFile(config.filename+".bkp", config.filename); err != nil {
		return err
	}

	fmt.Printf("Old config saved as %s\n", config.filename+".bkp")

	if !found {
		return config.convertLegacy()
	}

	return config.convertCurrent()
}

func (config *MixConfig) resetValuesForFile() error {
	//reset values for all properties but preserve the file location.
	filename := config.filename

	// Load default values for new properties
	if err := config.LoadDefaults(); err != nil {
		return err
	}

	config.filename = filename

	return nil
}

func (config *MixConfig) convertCurrent() error {
	// Load default values for new properties
	if err := config.resetValuesForFile(); err != nil {
		return err
	}

	// Version only exists in New Config, so parse builder.conf as TOML
	if err := config.parse(); err != nil {
		return err
	}

	// Set config to the current format
	config.version = CurrentConfigVersion

	return config.SaveConfig()
}

func (config *MixConfig) convertLegacy() error {
	// Load default values for new properties
	if err := config.resetValuesForFile(); err != nil {
		return err
	}

	// Assume missing version and try to parse as TOML
	if _, err := toml.DecodeFile(config.filename, &config); err != nil {
		// Try parsing as INI
		if err := config.parseLegacy(); err != nil {
			return err
		}
	} else {
		config.hasFormatField = true
	}

	// If the config still has the FORMAT value in it, check if
	// it was already transferred to mixer.state
	if config.hasFormatField {
		if err := config.convertFormat(); err != nil {
			return err
		}
	}

	// Set config to the current format
	config.version = CurrentConfigVersion

	return config.SaveConfig()
}

func (config *MixConfig) convertFormat() error {
	confBytes, err := ioutil.ReadFile(config.filename)
	if err != nil {
		return err
	}

	r := regexp.MustCompile(`FORMAT[\s"=]*([0-9]+)[\s"]*\n`)
	match := r.FindStringSubmatch(string(confBytes))
	if len(match) != 2 {
		return nil
	}

	var state MixState
	state.LoadDefaults(*config)

	_, err = os.Stat(state.filename)
	if err == nil {
		// mixer.state already exist, so don't overwrite it
		return nil
	}

	state.Mix.Format = match[1]
	return state.Save()
}

func (config *MixConfig) parseLegacy() error {
	lines, err := helpers.ReadFileAndSplit(config.filename)
	if err != nil {
		return errors.Wrap(err, "Failed to read buildconf")
	}

	var format string

	// Map the builder values to the regex here to make it easier to assign
	fields := []struct {
		re       string
		dest     *string
		required bool
	}{
		// [Builder]
		{`^CERT\s*=\s*`, &config.Builder.Cert, true},
		{`^SERVER_STATE_DIR\s*=\s*`, &config.Builder.ServerStateDir, true},
		{`^VERSIONS_PATH\s*=\s*`, &config.Builder.VersionPath, true},
		{`^YUM_CONF\s*=\s*`, &config.Builder.DNFConf, true},
		// [Swupd]
		{`^BUNDLE\s*=\s*`, &config.Swupd.Bundle, false},
		{`^CONTENTURL\s*=\s*`, &config.Swupd.ContentURL, false},
		{`^FORMAT\s*=\s*`, &format, true},
		{`^VERSIONURL\s*=\s*`, &config.Swupd.VersionURL, false},
		// [Server]
		{`^debuginfo_banned\s*=\s*`, &config.Server.DebugInfoBanned, false},
		{`^debuginfo_lib\s*=\s*`, &config.Server.DebugInfoLib, false},
		{`^debuginfo_src\s*=\s*`, &config.Server.DebugInfoSrc, false},
		// [Mixer]
		{`^LOCAL_BUNDLE_DIR\s*=\s*`, &config.Mixer.LocalBundleDir, false},
		{`^LOCAL_REPO_DIR\s*=\s*`, &config.Mixer.LocalRepoDir, false},
		{`^LOCAL_RPM_DIR\s*=\s*`, &config.Mixer.LocalRPMDir, false},
	}

	for _, h := range fields {
		r := regexp.MustCompile(h.re)
		for _, i := range lines {
			if m := r.FindIndex([]byte(i)); m != nil {
				*h.dest = i[m[1]:]
			}
		}
	}

	config.hasFormatField = format != ""

	if config.Mixer.LocalBundleDir == "" {
		pwd, err := os.Getwd()
		if err != nil {
			return err
		}
		config.Mixer.LocalBundleDir = filepath.Join(pwd, "local-bundles")
		log.Printf("Warning: LOCAL_BUNDLE_DIR not found in builder.conf. Falling back to %q.\n", config.Mixer.LocalBundleDir)
		log.Println("Please set this value to the location you want local bundle definition files to be stored.")
	}

	return nil
}
