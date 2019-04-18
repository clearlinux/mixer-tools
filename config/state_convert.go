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
	"os"
)

// CurrentStateVersion is the current revision for the state file structure
const CurrentStateVersion = "1.1"

func (state *MixState) parseVersionAndConvert() error {
	f, err := os.Open(state.filename)
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
	}()

	// Read state version
	reader := bufio.NewReader(f)
	found, err := state.parseVersion(reader)
	if err != nil {
		return err
	}

	// Already on latest version
	if found && state.version == CurrentStateVersion {
		return nil
	}

	fmt.Printf("Converting state to version %s\n", CurrentStateVersion)

	return state.convertCurrent()
}

func (state *MixState) convertCurrent() error {
	// Version only exists in new state, so parse mixer.state as TOML
	if err := state.parse(); err != nil {
		return err
	}

	// Set state to the current version
	state.version = CurrentStateVersion

	return state.Save()
}
