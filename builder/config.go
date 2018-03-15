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
	"regexp"
	"strings"

	"github.com/clearlinux/mixer-tools/helpers"
	"github.com/pkg/errors"
)

// MixConfig represents the config parameters found in the builder config file.
type MixConfig struct {
	Builder builderConf
	Swupd   swupdConf
	Mixer   mixerConf
}

type builderConf struct {
	BundleDir      string
	Cert           string
	ServerStateDir string
	VersionPath    string
	DNFConf        string
}

type swupdConf struct {
	Format string
}

type mixerConf struct {
	LocalBundleDir string
	LocalRepoDir   string
	LocalRPMDir    string
}

// CreateDefaultConfig creates a default builder.conf using the active
// directory as base path for the variables values.
func (config *MixConfig) CreateDefaultConfig(localrpms bool) error {
	pwd, err := os.Getwd()
	if err != nil {
		return err
	}

	builderconf := filepath.Join(pwd, "builder.conf")

	err = helpers.CopyFileNoOverwrite(builderconf, "/usr/share/defaults/bundle-chroot-builder/builder.conf")
	if os.IsExist(err) {
		// builder.conf already exists. Skip creation.
		return nil
	} else if err != nil {
		return err
	}

	fmt.Println("Creating new builder.conf configuration file...")

	raw, err := ioutil.ReadFile(builderconf)
	if err != nil {
		return err
	}

	// Patch all default path prefixes to PWD
	data := strings.Replace(string(raw), "/home/clr/mix", pwd, -1)

	// Add [Mixer] section
	data += "\n[Mixer]\n"
	data += "LOCAL_BUNDLE_DIR=" + filepath.Join(pwd, "local-bundles") + "\n"

	if localrpms {
		data += "LOCAL_RPM_DIR=" + filepath.Join(pwd, "local-rpms") + "\n"
		data += "LOCAL_REPO_DIR=" + filepath.Join(pwd, "local-yum") + "\n"
	}

	return ioutil.WriteFile(builderconf, []byte(data), 0666)
}

// LoadConfig loads a configuration file from a provided path or from local directory
// is none is provided
func (config *MixConfig) LoadConfig(filename string) error {
	lines, err := helpers.ReadFileAndSplit(filename)
	if err != nil {
		return errors.Wrap(err, "Failed to read buildconf")
	}

	// Map the builder values to the regex here to make it easier to assign
	fields := []struct {
		re       string
		dest     *string
		required bool
	}{
		{`^BUNDLE_DIR\s*=\s*`, &config.Builder.BundleDir, true}, //Note: Can be removed once UseNewChrootBuilder is obsolete
		{`^CERT\s*=\s*`, &config.Builder.Cert, true},
		{`^FORMAT\s*=\s*`, &config.Swupd.Format, true},
		{`^LOCAL_BUNDLE_DIR\s*=\s*`, &config.Mixer.LocalBundleDir, false},
		{`^LOCAL_REPO_DIR\s*=\s*`, &config.Mixer.LocalRepoDir, false},
		{`^LOCAL_RPM_DIR\s*=\s*`, &config.Mixer.LocalRPMDir, false},
		{`^SERVER_STATE_DIR\s*=\s*`, &config.Builder.ServerStateDir, true},
		{`^VERSIONS_PATH\s*=\s*`, &config.Builder.VersionPath, true},
		{`^YUM_CONF\s*=\s*`, &config.Builder.DNFConf, true},
	}

	for _, h := range fields {
		r := regexp.MustCompile(h.re)
		// Look for Environment variables in the config file
		re := regexp.MustCompile(`\$\{?([[:word:]]+)\}?`)
		for _, i := range lines {
			if m := r.FindIndex([]byte(i)); m != nil {
				// We want the variable without the $ or {} for lookup checking
				matches := re.FindAllStringSubmatch(i[m[1]:], -1)
				for _, s := range matches {
					if _, ok := os.LookupEnv(s[1]); !ok {
						return errors.Errorf("buildconf contains an undefined environment variable: %s", s[1])
					}
				}

				// Replace valid Environment Variables
				*h.dest = os.ExpandEnv(i[m[1]:])
			}
		}

		if h.required && *h.dest == "" {
			missing := h.re
			re := regexp.MustCompile(`([[:word:]]+)\\s\*=`)
			if matches := re.FindStringSubmatch(h.re); matches != nil {
				missing = matches[1]
			}

			return errors.Errorf("buildconf missing entry for variable: %s", missing)
		}
	}

	if config.Mixer.LocalBundleDir == "" {
		pwd, err := os.Getwd()
		if err != nil {
			return err
		}
		config.Mixer.LocalBundleDir = filepath.Join(pwd, "local-bundles")
		fmt.Printf("WARNING: LOCAL_BUNDLE_DIR not found in builder.conf. Falling back to %q.\n", config.Mixer.LocalBundleDir)
		fmt.Println("Please set this value to the location you want local bundle definition files to be stored.")
	}

	return nil
}
