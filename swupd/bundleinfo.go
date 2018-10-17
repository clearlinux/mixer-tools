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
	"log"
	"os"
	"path/filepath"
	"strings"
)

// BundleInfo describes the JSON object to be read from the *-info files
type BundleInfo struct {
	Name           string
	Filename       string
	DirectIncludes []string
	DirectPackages map[string]bool
	AllPackages    map[string]bool
	Files          map[string]bool
}

// GetBundleInfo loads the BundleInfo member of m from the bundle-info file at
// path
func (m *Manifest) GetBundleInfo(stateDir, path string) error {
	var err error
	if _, err = os.Stat(path); os.IsNotExist(err) {
		basePath := filepath.Dir(path)
		err = m.addFilesFromChroot(filepath.Join(filepath.Dir(path), m.Name), "")
		if err != nil {
			return err
		}

		m.BundleInfo.Files = make(map[string]bool)
		for _, f := range m.Files {
			m.BundleInfo.Files[f.Name] = true
		}

		var includes []string
		includes, err = readIncludesFile(filepath.Join(basePath, "noship", m.Name+"-includes"))
		if err != nil {
			return err
		}
		m.BundleInfo.DirectIncludes = includes
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
			m.BundleInfo.Files[f] = true
		}
	}

	return nil
}

func (m *Manifest) addFilesFromBundleInfo(c config, version uint32) error {
	chrootDir := filepath.Join(c.imageBase, fmt.Sprint(version), "full")
	for fpath := range m.BundleInfo.Files {
		fullPath := filepath.Join(chrootDir, fpath)
		fi, err := os.Lstat(fullPath)
		if os.IsNotExist(err) {
			log.Printf("Warning: Missing file, assuming %%ghost: %s\n", fpath)
			continue
		}
		if err != nil {
			return err
		}

		err = m.createFileRecord(chrootDir, fpath, "", fi)
		if err != nil {
			if strings.Contains(err.Error(), "hash calculation error") {
				return err
			}
			log.Printf("Warning: %s\n", err)
		}
	}

	return nil
}

func appendUniqueManifest(ms []*Manifest, man *Manifest) []*Manifest {
	for _, m := range ms {
		if m.Name == man.Name {
			return ms
		}
	}
	return append(ms, man)
}

func (m *Manifest) readIncludesFromBundleInfo(bundles []*Manifest) error {
	includes := []*Manifest{}
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

	m.Header.Includes = includes
	return nil
}
