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
	"os"

	"github.com/pkg/errors"
)

const mixDirGitIgnore = `upstream-bundles/
mix-bundles/`

// Get latest published upstream version
func (b *Builder) getLatestUpstreamVersion() (string, error) {
	ver, err := b.DownloadFileFromUpstreamAsString("/latest")
	if err != nil {
		return "", errors.Wrap(err, "Failed to retrieve latest published upstream version. Missing proxy configuration? ")
	}

	return ver, nil
}

// initDirs creates the directories mixer uses
func (b *Builder) initDirs() error {
	// Create folder to store local rpms if defined but doesn't already exist
	if err := os.MkdirAll(b.Config.Mixer.LocalRPMDir, 0777); err != nil {
		return errors.Wrap(err, "Failed to create local rpms directory")
	}

	// Create folder for local dnf repo if defined but doesn't already exist
	if err := os.MkdirAll(b.Config.Mixer.LocalRepoDir, 0777); err != nil {
		return errors.Wrap(err, "Failed to create local rpms directory")
	}

	// Create folder for local bundle files
	if err := os.MkdirAll(b.Config.Mixer.LocalBundleDir, 0777); err != nil {
		return errors.Wrap(err, "Failed to create local bundles directory")
	}

	return nil
}
