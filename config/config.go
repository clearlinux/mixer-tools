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
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/clearlinux/mixer-tools/helpers"
	"github.com/pkg/errors"
)

// UseNewConfig controls whether to use the new TOML config format.
// This is an experimental feature.
var UseNewConfig = false

// MixConfig represents the config parameters found in the builder config file.
type MixConfig struct {
	Builder builderConf
	Swupd   swupdConf
	Server  serverConf
	Mixer   mixerConf

	/* hidden properties */
	filename string
}

type builderConf struct {
	Cert           string `required:"true" mount:"true" toml:"CERT"`
	ServerStateDir string `required:"true" mount:"true" toml:"SERVER_STATE_DIR"`
	VersionPath    string `required:"true" mount:"true" toml:"VERSIONS_PATH"`
	DNFConf        string `required:"true" mount:"true" toml:"YUM_CONF"`
}

type swupdConf struct {
	Bundle     string `required:"false" toml:"BUNDLE"`
	ContentURL string `required:"false" toml:"CONTENTURL"`
	Format     string `required:"true" toml:"FORMAT"`
	VersionURL string `required:"false" toml:"VERSIONURL"`
}

type serverConf struct {
	DebugInfoBanned string `required:"false" toml:"DEBUG_INFO_BANNED"`
	DebugInfoLib    string `required:"false" toml:"DEBUG_INFO_LIB"`
	DebugInfoSrc    string `required:"false" toml:"DEBUG_INFO_SRC"`
}

type mixerConf struct {
	LocalBundleDir string `required:"false" mount:"true" toml:"LOCAL_BUNDLE_DIR"`
	LocalRepoDir   string `required:"false" mount:"true" toml:"LOCAL_REPO_DIR"`
	LocalRPMDir    string `required:"false" mount:"true" toml:"LOCAL_RPM_DIR"`
	DockerImgPath  string `required:"false" toml:"DOCKER_IMAGE_PATH"`
}

// LoadDefaults sets sane values for the config properties
func (config *MixConfig) LoadDefaults(localrpms bool) error {
	pwd, err := os.Getwd()
	if err != nil {
		return err
	}

	config.LoadDefaultsForPath(localrpms, pwd)
	return nil
}

// LoadDefaultsForPath sets sane values for config properties using `path` as base directory
func (config *MixConfig) LoadDefaultsForPath(localrpms bool, path string) {

	// [Builder]
	config.Builder.Cert = filepath.Join(path, "Swupd_Root.pem")
	config.Builder.ServerStateDir = filepath.Join(path, "update")
	config.Builder.VersionPath = path
	config.Builder.DNFConf = filepath.Join(path, ".yum-mix.conf")

	// [Swupd]
	config.Swupd.Bundle = "os-core-update"
	config.Swupd.ContentURL = "<URL where the content will be hosted>"
	config.Swupd.Format = "1"
	config.Swupd.VersionURL = "<URL where the version of the mix will be hosted>"

	// [Server]
	config.Server.DebugInfoBanned = "true"
	config.Server.DebugInfoLib = "/usr/lib/debug"
	config.Server.DebugInfoSrc = "/usr/src/debug"

	// [Mixer]
	config.Mixer.LocalBundleDir = filepath.Join(path, "local-bundles")

	if localrpms {
		config.Mixer.LocalRPMDir = filepath.Join(path, "local-rpms")
		config.Mixer.LocalRepoDir = filepath.Join(path, "local-yum")
	} else {
		config.Mixer.LocalRPMDir = ""
		config.Mixer.LocalRepoDir = ""
	}

	config.filename = filepath.Join(path, "builder.conf")
}

// CreateDefaultConfig creates a default builder.conf using the active
// directory as base path for the variables values.
func (config *MixConfig) CreateDefaultConfig(localrpms bool) error {
	if !UseNewConfig {
		return config.createLegacyConfig(localrpms)
	}

	if err := config.LoadDefaults(localrpms); err != nil {
		return err
	}

	err := config.initConfigPath("")
	if err != nil {
		return err
	}

	return config.SaveConfig()
}

// SaveConfig saves the properties in MixConfig to a TOML config file
func (config *MixConfig) SaveConfig() error {
	if !UseNewConfig {
		return errors.Errorf("SaveConfig can only be used with --new-config flag")
	}

	w, err := os.OpenFile(config.filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		return err
	}
	defer func() {
		_ = w.Close()
	}()

	enc := toml.NewEncoder(w)
	return enc.Encode(config)
}

func (config *MixConfig) createLegacyConfig(localrpms bool) error {
	if UseNewConfig {
		return errors.Errorf("createLegacyConfig is not compatible with --new-config flag")
	}

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
	data += "DOCKER_IMAGE_PATH=clearlinux/mixer\n"

	if localrpms {
		data += "LOCAL_RPM_DIR=" + filepath.Join(pwd, "local-rpms") + "\n"
		data += "LOCAL_REPO_DIR=" + filepath.Join(pwd, "local-yum") + "\n"
	}

	return ioutil.WriteFile(builderconf, []byte(data), 0666)
}

// SetProperty parse a property in the format "Section.Property", finds and sets it within the
// config structure and saves the config file.
func (config *MixConfig) SetProperty(propertyStr string, value string) error {
	if !UseNewConfig {
		return errors.Errorf("SetProperty requires --new-config flag")
	}

	tokens := strings.Split(propertyStr, ".")
	property, sections := tokens[len(tokens)-1], tokens[:len(tokens)-1]

	sectionV := reflect.ValueOf(config).Elem()
	for i := 0; i < len(sections); i++ {
		sectionV = sectionV.FieldByName(sections[i])

		if !sectionV.IsValid() {
			return errors.Errorf("Unknown config sectionV: '%s'", tokens[i])
		}
	}

	sectionT := reflect.TypeOf(sectionV.Interface())
	for i := 0; i < sectionV.NumField(); i++ {
		tag, ok := sectionT.Field(i).Tag.Lookup("toml")

		if ok && tag == property {
			sectionV.Field(i).SetString(value)
			return config.SaveConfig()
		}
	}

	return errors.Errorf("Property not found in config file: '%s'", property)
}

// LoadConfig loads a configuration file from a provided path or from local directory
// is none is provided
func (config *MixConfig) LoadConfig(filename string) error {
	if err := config.initConfigPath(filename); err != nil {
		return err
	}
	if err := config.Parse(); err != nil {
		return err
	}
	if err := config.expandEnv(); err != nil {
		return err
	}

	return config.validate()
}

// Parse reads the values from a config file without performing validation or env expansion
func (config *MixConfig) Parse() error {
	if !UseNewConfig {
		if err := config.legacyParse(); err != nil {
			return err
		}
	} else {
		if _, err := toml.DecodeFile(config.filename, &config); err != nil {
			return err
		}
	}

	return nil
}

func (config *MixConfig) legacyParse() error {
	if UseNewConfig {
		return errors.Errorf("legacyParse is not compatible with --new-config flag")
	}

	lines, err := helpers.ReadFileAndSplit(config.filename)
	if err != nil {
		return errors.Wrap(err, "Failed to read buildconf")
	}

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
		{`^FORMAT\s*=\s*`, &config.Swupd.Format, true},
		{`^VERSIONURL\s*=\s*`, &config.Swupd.VersionURL, false},
		// [Server]
		{`^debuginfo_banned\s*=\s*`, &config.Server.DebugInfoBanned, false},
		{`^debuginfo_lib\s*=\s*`, &config.Server.DebugInfoLib, false},
		{`^debuginfo_src\s*=\s*`, &config.Server.DebugInfoSrc, false},
		// [Mixer]
		{`^LOCAL_BUNDLE_DIR\s*=\s*`, &config.Mixer.LocalBundleDir, false},
		{`^LOCAL_REPO_DIR\s*=\s*`, &config.Mixer.LocalRepoDir, false},
		{`^LOCAL_RPM_DIR\s*=\s*`, &config.Mixer.LocalRPMDir, false},
		{`^DOCKER_IMAGE_PATH\s*=\s*`, &config.Mixer.DockerImgPath, false},
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

		/* ignore unexported fields */
		if !sectionV.CanSet() {
			continue
		}

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
		sectionV := rv.Field(i)
		/* ignore unexported fields */
		if !sectionV.CanSet() {
			continue
		}

		sectionT := reflect.TypeOf(rv.Field(i).Interface())

		for j := 0; j < sectionT.NumField(); j++ {
			tag, ok := sectionT.Field(j).Tag.Lookup("required")

			if ok && tag == "true" && sectionV.Field(j).String() == "" {
				name, ok := sectionT.Field(j).Tag.Lookup("toml")
				if !ok || name == "" {
					// Default back to variable name if no TOML tag is defined
					name = sectionT.Field(j).Name
				}

				return errors.Errorf("Missing required field in config file: %s", name)
			}
		}
	}

	return nil
}

// Convert parses an old config file and converts it to TOML format
func (config *MixConfig) Convert(filename string) error {
	if err := config.initConfigPath(filename); err != nil {
		return err
	}

	// Force UseNewConfig to false
	UseNewConfig = false
	if err := config.Parse(); err != nil {
		return err
	}

	if err := helpers.CopyFile(config.filename+".bkp", config.filename); err != nil {
		return err
	}

	// Force UseNewConfig to true
	UseNewConfig = true

	return config.SaveConfig()
}

// Print print variables and values of a MixConfig struct
func (config *MixConfig) Print() error {
	sb := bytes.NewBufferString("")

	enc := toml.NewEncoder(sb)
	if err := enc.Encode(config); err != nil {
		return err
	}

	fmt.Println(sb.String())

	return nil
}

func (config *MixConfig) initConfigPath(path string) error {
	if path != "" {
		config.filename = path
		return nil
	}

	pwd, err := os.Getwd()
	if err != nil {
		return err
	}

	config.filename = filepath.Join(pwd, "builder.conf")

	return nil
}

// GetConfigFileName returns the file name of current config
func (config *MixConfig) GetConfigFileName() string {
	/* This variable cannot be public or else it will be added to the config file */
	return config.filename
}
