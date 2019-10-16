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
	"log"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"
)

// MixConfig represents the config parameters found in the builder config file.
type MixConfig struct {
	Builder builderConf
	Swupd   swupdConf
	Server  serverConf
	Mixer   mixerConf

	/* hidden properties */
	filename string
	version  string

	// Format moved into mixer.state file. This variable is set
	// if the value is still present in the parsed config to
	// print a warning for the user.
	hasFormatField bool
}

type builderConf struct {
	Cert           string `required:"true" mount:"true" toml:"CERT"`
	ServerStateDir string `required:"true" mount:"true" toml:"SERVER_STATE_DIR"`
	VersionPath    string `required:"true" mount:"true" toml:"VERSIONS_PATH"`
	DNFConf        string `required:"true" mount:"true" toml:"YUM_CONF"`
}

type swupdConf struct {
	Bundle             string   `required:"false" toml:"BUNDLE"`
	ContentURL         string   `required:"false" toml:"CONTENTURL"`
	VersionURL         string   `required:"false" toml:"VERSIONURL"`
	Compression        []string `required:"false" toml:"COMPRESSION"`
	UpstreamBundlesURL string   `required:"false" toml:"UPSTREAM_BUNDLES_URL"`
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
	OSReleasePath  string `required:"false" mount:"true" toml:"OS_RELEASE_PATH"`
}

// LoadDefaults sets sane values for the config properties
func (config *MixConfig) LoadDefaults() error {
	pwd, err := os.Getwd()
	if err != nil {
		return err
	}

	config.LoadDefaultsForPath(pwd)
	return nil
}

// LoadDefaultsForPath sets sane values for config properties using `path` as base directory
func (config *MixConfig) LoadDefaultsForPath(path string) {

	// [Builder]
	config.Builder.Cert = filepath.Join(path, "Swupd_Root.pem")
	config.Builder.ServerStateDir = filepath.Join(path, "update")
	config.Builder.VersionPath = path
	config.Builder.DNFConf = filepath.Join(path, ".yum-mix.conf")

	// [Swupd]
	config.Swupd.Bundle = "os-core-update"
	config.Swupd.ContentURL = "<URL where the content will be hosted>"
	config.Swupd.VersionURL = "<URL where the version of the mix will be hosted>"
	config.Swupd.Compression = []string{"external-xz"}
	config.Swupd.UpstreamBundlesURL = "https://github.com/clearlinux/clr-bundles/archive/"

	// [Server]
	config.Server.DebugInfoBanned = "true"
	config.Server.DebugInfoLib = "/usr/lib/debug"
	config.Server.DebugInfoSrc = "/usr/src/debug"

	// [Mixer]
	config.Mixer.LocalBundleDir = filepath.Join(path, "local-bundles")
	config.Mixer.DockerImgPath = "clearlinux/mixer"

	config.Mixer.LocalRPMDir = filepath.Join(path, "local-rpms")
	config.Mixer.LocalRepoDir = filepath.Join(path, "local-yum")

	config.version = CurrentConfigVersion
	config.filename = filepath.Join(path, "builder.conf")

	config.hasFormatField = false
}

// CreateDefaultConfig creates a default builder.conf using the active
// directory as base path for the variables values.
func (config *MixConfig) CreateDefaultConfig() error {
	if err := config.LoadDefaults(); err != nil {
		return err
	}

	err := config.InitConfigPath("")
	if err != nil {
		return err
	}

	return config.SaveConfig()
}

// SaveConfig saves the properties in MixConfig to a TOML config file
func (config *MixConfig) SaveConfig() error {
	var buffer bytes.Buffer
	buffer.Write([]byte("#VERSION " + config.version + "\n\n"))

	enc := toml.NewEncoder(&buffer)

	if err := enc.Encode(config); err != nil {
		return err
	}

	w, err := os.OpenFile(config.filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		return err
	}
	defer func() {
		_ = w.Close()
	}()

	_, err = buffer.WriteTo(w)

	return err
}

// SetProperty parse a property in the format "Section.Property", finds and sets it within the
// config structure and saves the config file.
func (config *MixConfig) SetProperty(propertyStr string, value string) error {
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
			switch sectionV.Field(i).Kind() {
			case reflect.Slice:
				items := strings.Split(value, ",")
				sectionV.Field(i).Set(reflect.ValueOf(items))
			case reflect.Int:
				intV, err := strconv.ParseInt(value, 10, 64)
				if err != nil {
					return err
				}
				sectionV.Field(i).SetInt(intV)
			default:
				val := reflect.ValueOf(value)
				sectionV.Field(i).Set(val.Convert(sectionV.Field(i).Type()))
			}
			return config.SaveConfig()
		}
	}

	return errors.Errorf("Property not found in config file: '%s'", property)
}

// LoadConfig loads a configuration file from a provided path or from local directory
// if none is provided
func (config *MixConfig) LoadConfig(filename string) error {
	if err := config.InitConfigPath(filename); err != nil {
		return err
	}
	if err := config.parseVersionAndConvert(); err != nil {
		return err
	}
	if err := config.parse(); err != nil {
		return err
	}
	if err := config.expandEnv(); err != nil {
		return err
	}

	return config.validate()
}

func (config *MixConfig) parse() error {
	_, err := toml.DecodeFile(config.filename, &config)
	return err
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
			if sectionV.Field(j).Kind() != reflect.String {
				continue
			}
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

	if config.hasFormatField {
		log.Println("Warning: FORMAT value was transferred to mixer.state file")
	}

	return nil
}

// Convert parses an old config file and converts it to TOML format
func (config *MixConfig) Convert(filename string) error {
	if err := config.InitConfigPath(filename); err != nil {
		return err
	}

	if err := config.LoadDefaults(); err != nil {
		return err
	}

	return config.parseVersionAndConvert()
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

// InitConfigPath sets the main config name to what was passed in,
// or defaults to the current working directory + builder.conf
func (config *MixConfig) InitConfigPath(fullpath string) error {
	if fullpath != "" {
		config.filename = fullpath
		return nil
	}
	// Create a builder.conf in the current directory if none is passed in
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
