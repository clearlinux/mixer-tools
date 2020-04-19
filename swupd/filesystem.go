// Copyright 2017 Intel Corporation
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
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/clearlinux/mixer-tools/log"
)

const illegalChars = ";&|*`/<>\\\"'\a"

// FilenameBlacklisted checks for illegal characters in filename
func FilenameBlacklisted(fname string) bool {
	return strings.ContainsAny(fname, illegalChars)
}

// createFileRecord creates a manifest File entry from a file
func (m *Manifest) createFileRecord(rootPath, path, removePrefix string, fi os.FileInfo) error {
	file, err := recordFromFile(rootPath, path, removePrefix, fi)
	if err != nil {
		return err
	}

	// this is a file to skip
	if file == nil {
		return nil
	}

	if flag, ok := m.BundleInfo.Files[path]; ok {
		if flag == true {
			// set Misc flag
			if file.Misc, err = miscFromFlag('x'); err != nil {
				return err
			}
		}
	}

	m.AppendFile(file)

	return nil
}

var hashMap sync.Map

// recordFromFile creates a struct File record from an os.FileInfo object
// this function sets the Name, Info, Type, and Hash fields
func recordFromFile(rootPath, path, removePrefix string, fi os.FileInfo) (*File, error) {
	var file *File
	var fname string
	if removePrefix != "" {
		fname = strings.TrimPrefix(path, removePrefix)
		rootPath = removePrefix
	} else {
		fname = strings.TrimPrefix(path, rootPath)
	}
	if fname == "" {
		return nil, nil
	}

	if FilenameBlacklisted(filepath.Base(fname)) {
		return nil, fmt.Errorf("%s is a blacklisted file name", fname)
	}

	file = &File{
		Name: fname,
		Info: fi,
	}

	switch mode := fi.Mode(); {
	case mode.IsRegular():
		file.Type = TypeFile
	case mode.IsDir():
		file.Type = TypeDirectory
	case mode&os.ModeSymlink != 0:
		file.Type = TypeLink
	default:
		return nil, fmt.Errorf("%v is an unsupported file type", file.Name)
	}
	filePath := filepath.Join(rootPath, file.Name)
	if val, ok := hashMap.Load(filePath); ok {
		file.Hash = val.(Hashval)
	} else {
		fh, err := Hashcalc(filePath)
		if err != nil {
			return nil, fmt.Errorf("hash calculation error: %v", err)
		}
		file.Hash = fh
		hashMap.Store(filePath, fh)
	}

	return file, nil
}

// createManifestRecord wraps createFileRecord to create a Manifest record for a MoM
func (m *Manifest) createManifestRecord(rootPath, path string, version uint32, manifestType TypeFlag, bundleStatus string) error {
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}

	file, err := recordFromFile(rootPath, path, "", fi)
	if err != nil {
		if strings.Contains(err.Error(), "hash calculation error") {
			return err
		}
		log.Warning(log.Mixer, "%s", err)
	}

	// this is a file to skip
	if file == nil {
		return nil
	}

	// Only the bundle name should be part of the name in the manifest
	file.Name = strings.Replace(file.Name, "/Manifest.", "", -1)
	file.Type = manifestType
	setManifestStatusForFormat(m.Header.Format, bundleStatus, &file.Status)
	file.Version = version
	m.Files = append(m.Files, file)
	return nil
}

func (m *Manifest) addFilesFromChroot(rootPath, removePrefix string) error {
	if _, err := os.Stat(rootPath); os.IsNotExist(err) {
		return err
	}

	err := filepath.Walk(rootPath, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		err = m.createFileRecord(rootPath, path, removePrefix, fi)
		if err != nil {
			if strings.Contains(err.Error(), "hash calculation error") {
				return err
			}
			log.Warning(log.Mixer, "%s", err.Error())
		}
		return nil
	})

	return err
}

func exists(path string) bool {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return false
	}

	return true
}
