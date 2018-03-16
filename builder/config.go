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
	BundleDir      string `required:"true"`
	Cert           string `required:"true"`
	ServerStateDir string `required:"true"`
	VersionPath    string `required:"true"`
	DNFConf        string `required:"true"`
}

type swupdConf struct {
	Format string `required:"true"`
}

type mixerConf struct {
	LocalBundleDir string `required:"false"`
	LocalRepoDir   string `required:"false"`
	LocalRPMDir    string `required:"false"`
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
	if err := config.parse(filename); err != nil {
		return err
	}

	if err := config.expandEnv(); err != nil {
		return err
	}

	if err := config.validate(); err != nil {
		return err
	}

	return nil

}

func (config *MixConfig) parse(filename string) error {
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
		for _, i := range lines {
			if m := r.FindIndex([]byte(i)); m != nil {
				*h.dest = i[m[1]:]
			}
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

func (config *MixConfig) expandEnv() error {
	re := regexp.MustCompile(`\$\{?([[:word:]]+)\}?`)
	rv := reflect.ValueOf(config).Elem()

	for i := 0; i < rv.NumField(); i++ {
		sectionV := rv.Field(i)

		for j := 0; j < sectionV.NumField(); j++ {
			val := sectionV.Field(j).String()
			matches := re.FindAllStringSubmatch(val, -1)

			for _, s := range matches {
				if _, ok := os.LookupEnv(s[1]); !ok {
					return errors.Errorf("buildconf contains an undefined environment variable: %s\n", s[1])
				}
			}

			sectionV.Field(j).SetString(os.ExpandEnv(val))
		}

	}

	return nil
}

func (config *MixConfig) validate() error {
	rv := reflect.ValueOf(config).Elem()

	for i := 0; i < rv.NumField(); i++ {
		sectionT := reflect.TypeOf(rv.Field(i).Interface())
		sectionV := rv.Field(i)

		for j := 0; j < sectionT.NumField(); j++ {
			tag, ok := sectionT.Field(j).Tag.Lookup("required")

			if ok && tag == "true" && sectionV.Field(j).String() == "" {
				return errors.Errorf("Missing required field in config file: %s", sectionT.Field(j).Name)
			}
		}
	}

	return nil
}
