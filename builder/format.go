// Copyright © 2017 Intel Corporation
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
	"io/ioutil"
	"path/filepath"
	"regexp"

	"github.com/clearlinux/mixer-tools/helpers"
	"github.com/pkg/errors"
)

// UpdateFormatVersion updates the builder.conf file with a new format version
func UpdateFormatVersion(b *Builder, version string) error {
	b.Config.Swupd.Format = version
	newver := "${1}" + b.Config.Swupd.Format
	var re = regexp.MustCompile(`(FORMAT=)[0-9]*`)

	builderData, err := ioutil.ReadFile(b.BuildConf)
	if err != nil {
		return errors.Wrap(err, "Failed to read builder.conf")
	}

	builderEdit := re.ReplaceAllString(string(builderData), newver)

	builderOut := []byte(builderEdit)
	if err = ioutil.WriteFile(b.BuildConf, builderOut, 0644); err != nil {
		return errors.Wrap(err, "Failed to write new builder.conf")
	}

	return nil
}

// CopyFullGroupsINI copies the initial ini file which has ALL bundle definitions
func CopyFullGroupsINI(b *Builder) error {
	return helpers.CopyFile(filepath.Join(b.Config.Builder.ServerStateDir, "full_groups.ini"), filepath.Join(b.Config.Builder.ServerStateDir, "groups.ini"))
}

// CopyTrimmedGroupsINI copies the new ini made with deleted bundles removed
func CopyTrimmedGroupsINI(b *Builder) error {
	return helpers.CopyFile(filepath.Join(b.Config.Builder.ServerStateDir, "trimmed_groups.ini"), filepath.Join(b.Config.Builder.ServerStateDir, "groups.ini"))
}

// RevertFullGroupsINI copies back the full ini to the manifest creator accounts for deleted bundles
func RevertFullGroupsINI(b *Builder) error {
	return helpers.CopyFile(filepath.Join(b.Config.Builder.ServerStateDir, "groups.ini"), filepath.Join(b.Config.Builder.ServerStateDir, "full_groups.ini"))
}

// RevertTrimmedGroupsINI copies back the trimmed INI so manifests are not created anymore for deleted bundles in new format
func RevertTrimmedGroupsINI(b *Builder) error {
	return helpers.CopyFile(filepath.Join(b.Config.Builder.ServerStateDir, "groups.ini"), filepath.Join(b.Config.Builder.ServerStateDir, "trimmed_groups.ini"))
}

