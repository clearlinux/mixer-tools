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
	"github.com/clearlinux/mixer-tools/log"
	"path/filepath"

	"github.com/clearlinux/mixer-tools/helpers"
	"github.com/pkg/errors"
)

// ValidateLocalBundles runs bundle parsing validation on all local bundles.
func (b *Builder) ValidateLocalBundles(lvl ValidationLevel) error {
	files, err := helpers.ListVisibleFiles(b.Config.Mixer.LocalBundleDir)
	if err != nil {
		return errors.Wrap(err, "Failed to read local-bundles")
	}

	return b.ValidateBundles(files, lvl)
}

// ValidateBundles runs bundle parsing validation on a list of local bundles. In
// addition to parsing errors, errors are generated if the bundle is not found
// in local-bundles.
func (b *Builder) ValidateBundles(bundles []string, lvl ValidationLevel) error {
	invalid := false
	for _, bundle := range bundles {
		path := filepath.Join(b.Config.Mixer.LocalBundleDir, bundle)

		if err := validateBundleFile(path, lvl); err != nil {
			invalid = true
			log.Error(log.Mca, "Invalid: %q:\n%s\n", bundle, err)
		}
	}

	if invalid {
		return errors.New("Invalid bundles found")
	}

	return nil
}
