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

package swupd

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

// BundleHeader describes the meta information of a bundle
type BundleHeader struct {
	Title        string
	Description  string
	Status       string
	Capabilities string
	Maintainer   string
}

// BundleInfo describes the JSON object to be read from the *-info files
type BundleInfo struct {
	Name             string
	Filename         string
	Header           BundleHeader
	DirectIncludes   []string
	OptionalIncludes []string
	DirectPackages   map[string]bool
	AllPackages      map[string]bool
	Files            map[string]bool
}

// GetBundleInfo loads the BundleInfo member of m from the bundle-info file at
// path
func (m *Manifest) GetBundleInfo(stateDir, path string) error {
	var err error
	if _, err = os.Stat(path); os.IsNotExist(err) {
		basePath := filepath.Dir(path)
		err = m.getBundleInfoFromChroot(filepath.Join(basePath, m.Name))
		if err != nil {
			return err
		}

		var includes []string
		includes, err = readIncludesFile(filepath.Join(basePath, "noship", m.Name+"-includes"))
		if err != nil {
			return err
		}
		m.BundleInfo.DirectIncludes = includes
		// TODO: this part of the code seems to be a legacy thing. Keep
		// an eye on this to make sure m.BundleInfo.OptionalIncludes don't
		// need to be added here too
		return nil
	}

	biBytes, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}

	err = json.Unmarshal(biBytes, &m.BundleInfo)
	if err != nil {
		return err
	}

	extraFilesPath := filepath.Join(stateDir, m.Name+"-extra-files")
	if _, err = os.Stat(extraFilesPath); err == nil {
		extraFilesBytes, err := ioutil.ReadFile(extraFilesPath)
		if err != nil {
			return err
		}

		for _, f := range strings.Split(string(extraFilesBytes), "\n") {
			if len(f) == 0 {
				continue
			}
			if !strings.HasPrefix(f, "/") {
				return fmt.Errorf("invalid extra file %s in %s, must start with '/'", f, extraFilesPath)
			}

			// NOTE: The export flag is always set to false here and builder.isExportable() is intentionally not called.
			// This is done to avoid dependency of 'build update' to any steps/artifacts prior to 'build bundles'.
			// Ideally, 'build update' should work from the output of 'build bundles' and should not use artifacts
			// like bundle definitions directly.
			m.BundleInfo.Files[f] = false
		}
	}

	return nil
}

// getBundleInfoFromChroot loads the BundleInfo file list from a bundle chroot
func (m *Manifest) getBundleInfoFromChroot(rootPath string) error {
	m.BundleInfo.Files = make(map[string]bool)

	if _, err := os.Stat(rootPath); os.IsNotExist(err) {
		return err
	}

	err := filepath.Walk(rootPath, func(path string, fi os.FileInfo, err error) error {
		fname := strings.TrimPrefix(path, rootPath)

		// NOTE: The export flag is always set to false here and builder.isExportable() is intentionally not called.
		// This is done to avoid dependency of 'build update' to any steps/artifacts prior to 'build bundles'.
		// Ideally, 'build update' should work from the output of 'build bundles' and should not use artifacts
		// like bundle definitions directly.
		m.BundleInfo.Files[fname] = false

		return nil
	})

	return err
}

func appendUniqueManifest(ms []*Manifest, man *Manifest) []*Manifest {
	for _, m := range ms {
		if m.Name == man.Name {
			return ms
		}
	}
	return append(ms, man)
}

// ReadIncludesFromBundleInfo sets the Header.Includes field for the given manifest.
func (m *Manifest) ReadIncludesFromBundleInfo(bundles []*Manifest) error {
	includes := []*Manifest{}
	optional := []*Manifest{}
	// os-core is added as an include for every bundle
	// handle it manually so we don't have to rely on the includes list having it
	for _, b := range bundles {
		if b.Name == "os-core" {
			includes = append(includes, b)
		}
	}

	for _, bn := range m.BundleInfo.DirectIncludes {
		// just add this one blindly since it is processed later
		if bn == IndexBundle {
			includes = append(includes, &Manifest{Name: IndexBundle})
			continue
		}

		if bn == "os-core" {
			// already added this one
			continue
		}

		for _, b := range bundles {
			if bn == b.Name {
				includes = appendUniqueManifest(includes, b)
			}
		}
	}

	for _, bn := range m.BundleInfo.OptionalIncludes {
		for _, b := range bundles {
			if bn == b.Name {
				optional = appendUniqueManifest(optional, b)
			}
		}
	}
	m.Header.Includes = includes
	m.Header.Optional = optional
	return nil
}
