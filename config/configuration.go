// Copyright 2018 Intel Corporation
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
	"os"
	"reflect"
	"regexp"
	"strings"
)

// Configuration is a generic inerface for TOML configs in mixer
type Configuration interface {
	Save() error
	Load(filename string) error

	SetFilename(filename string)
	GetFilename() string

	SetVersion(version string)
	GetVersion() string
	GetLatestVersion() string
}

// SetProperty parses a property in the format "Section.Property", finds and sets it within the
// config structure and saves the config file.
func SetProperty(c Configuration, propertyStr string, value string) error {
	tokens := strings.Split(propertyStr, ".")
	property, sections := tokens[len(tokens)-1], tokens[:len(tokens)-1]
	sectionV := reflect.ValueOf(c).Elem()
	for i := 0; i < len(sections); i++ {
		sectionV = sectionV.FieldByName(sections[i])
		if !sectionV.IsValid() {
			return fmt.Errorf("Unknown config sectionV: '%s'", tokens[i])
		}
	}
	sectionT := reflect.TypeOf(sectionV.Interface())
	for i := 0; i < sectionV.NumField(); i++ {
		tag, ok := sectionT.Field(i).Tag.Lookup("toml")
		if ok && tag == property {
			sectionV.Field(i).SetString(value)
			return c.Save()
		}
	}
	return fmt.Errorf("Property not found in config file: '%s'", property)
}

// GetProperty parses a property in the format Section.Property, finds the property and returns its
// current value
func GetProperty(c Configuration, propertyStr string) (string, error) {
	tokens := strings.Split(propertyStr, ".")
	property, sections := tokens[len(tokens)-1], tokens[:len(tokens)-1]
	sectionV := reflect.ValueOf(c).Elem()
	for i := 0; i < len(sections); i++ {
		sectionV = sectionV.FieldByName(sections[i])
		if !sectionV.IsValid() {
			return "", fmt.Errorf("Unknown config sectionV: '%s'", tokens[i])
		}
	}
	sectionT := reflect.TypeOf(sectionV.Interface())
	for i := 0; i < sectionV.NumField(); i++ {
		tag, ok := sectionT.Field(i).Tag.Lookup("toml")
		if ok && tag == property {
			return sectionV.Field(i).String(), nil
		}
	}
	return "", fmt.Errorf("Property not found in config file: '%s'", property)
}

// ParseVersion checks the version header in the file and returns
// true if it matches the latest known version for that configuration
// file
func ParseVersion(c Configuration) (bool, error) {
	// Reset version for files without versioning
	c.SetVersion("0.0")

	f, err := os.Open(c.GetFilename())
	if err != nil {
		return false, err
	}
	defer func() {
		_ = f.Close()
	}()

	// Parse version
	reader := bufio.NewReader(f)
	verBytes, err := reader.ReadString('\n')
	if err != nil {
		return false, err
	}

	r := regexp.MustCompile("^#VERSION ([0-9]+.[0-9])+\n")
	match := r.FindStringSubmatch(string(verBytes))

	if len(match) != 2 {
		return false, nil
	}

	c.SetVersion(match[1])

	return c.GetVersion() == c.GetLatestVersion(), nil
}
