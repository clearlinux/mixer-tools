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
	"bytes"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/clearlinux/mixer-tools/log"

	"github.com/BurntSushi/toml"
)

type mixSection struct {
	Format         string `toml:"FORMAT"`
	PreviousMixVer string `toml:"PREVIOUS_MIX_VERSION"`
}

// MixState holds the current state of the mix
type MixState struct {
	Mix mixSection

	/* hidden properties */
	filename string
	version  string

	/* Inform the user where the source of mix format*/
	formatSource string
}

// DefaultFormatPath is the default path for the format file specified by swupd
const DefaultFormatPath = "/usr/share/defaults/swupd/format"

// LoadDefaults initialize the state object with sane values
func (state *MixState) LoadDefaults(config MixConfig) {
	state.loadDefaultFormat()
	state.loadDefaultPreviousMixVer(config.Builder.ServerStateDir)

	state.filename = "mixer.state"
	state.version = CurrentStateVersion
}

func (state *MixState) loadDefaultFormat() {
	/* Get format from legacy config file */
	format, err := state.getFormatFromConfig()
	if err == nil && format != "" {
		state.Mix.Format = format
		state.formatSource = "builder.conf"
		return
	}

	/* Get format from system */
	formatBytes, err := ioutil.ReadFile(DefaultFormatPath)
	if err == nil {
		state.Mix.Format = string(formatBytes)
		state.formatSource = DefaultFormatPath
		return
	}

	state.Mix.Format = "1"
	state.formatSource = "Mixer internal value"
}

func (state *MixState) loadDefaultPreviousMixVer(stateDir string) {
	/* The LAST_VER is the default for PREVIOUS_MIX_VERSION */
	lastVer, err := ioutil.ReadFile(filepath.Join(stateDir, "image/LAST_VER"))
	if err == nil && string(lastVer) != "" {
		state.Mix.PreviousMixVer = strings.TrimSuffix(string(lastVer), "\n")
		return
	}
	state.Mix.PreviousMixVer = "0"
}

func (state *MixState) getFormatFromConfig() (string, error) {
	confBytes, err := ioutil.ReadFile("builder.conf")
	if err != nil {
		return "", err
	}

	r := regexp.MustCompile(`FORMAT[\s"=]*([0-9]+)[\s"]*\n`)
	match := r.FindStringSubmatch(string(confBytes))
	if len(match) == 2 {
		return match[1], nil
	}

	return "", nil
}

// Save creates or overwrites the mixer.state file
func (state *MixState) Save() error {
	var buffer bytes.Buffer
	buffer.Write([]byte("#VERSION " + state.version + "\n\n"))

	enc := toml.NewEncoder(&buffer)

	if err := enc.Encode(state); err != nil {
		return err
	}

	w, err := os.OpenFile(state.filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		return err
	}
	defer func() {
		_ = w.Close()
	}()

	_, err = buffer.WriteTo(w)

	return err
}

// Load the mixer.state file
func (state *MixState) Load(config MixConfig) error {
	state.LoadDefaults(config)

	f, err := os.Open(state.filename)
	if err != nil {
		// If state does not exists, create a default state
		log.Warning(log.Mixer, "Mixer state does not exist; setting default state")
		log.Debug(log.Mixer, "Default FORMAT: "+state.Mix.Format)
		log.Debug(log.Mixer, "Default PREVIOUS_MIX_VERSION: "+state.Mix.PreviousMixVer)
		return state.Save()
	}
	defer func() {
		_ = f.Close()
	}()

	if err := state.parseVersionAndConvert(); err != nil {
		return err
	}

	// Read config version
	reader := bufio.NewReader(f)
	found, err := state.parseVersion(reader)
	if err != nil {
		return err
	} else if !found {
		return errors.New("Unable to read mixer state version")
	}

	_, err = toml.DecodeReader(reader, &state)
	return err
}

func (state *MixState) parseVersion(reader *bufio.Reader) (bool, error) {
	verBytes, err := reader.ReadString('\n')
	if err != nil {
		return false, err
	}

	r := regexp.MustCompile("^#VERSION ([0-9]+.[0-9])+\n")
	match := r.FindStringSubmatch(string(verBytes))

	if len(match) != 2 {
		return false, nil
	}

	state.version = match[1]

	return true, nil
}

func (state *MixState) parse() error {
	_, err := toml.DecodeFile(state.filename, &state)
	return err
}
