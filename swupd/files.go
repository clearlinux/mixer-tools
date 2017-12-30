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
	"errors"
	"fmt"
	"os"
)

type ftype int
type fmodifier int
type fstatus int
type frename bool

const (
	typeUnset ftype = iota
	typeFile
	typeDirectory
	typeLink
	typeManifest
)

var typeBytes = map[ftype]byte{
	typeUnset:     '.',
	typeFile:      'F',
	typeDirectory: 'D',
	typeLink:      'L',
	typeManifest:  'M',
}

const (
	modifierUnset fmodifier = iota
	modifierConfig
	modifierState
	modifierBoot
)

var modifierBytes = map[fmodifier]byte{
	modifierUnset:  '.',
	modifierConfig: 'C',
	modifierState:  's',
	modifierBoot:   'b',
}

const (
	statusUnset fstatus = iota
	statusDeleted
	statusGhosted
)

var statusBytes = map[fstatus]byte{
	statusUnset:   '.',
	statusDeleted: 'd',
	statusGhosted: 'g',
}

const (
	renameUnset = false
	renameSet   = true
)

var renameBytes = map[frename]byte{
	renameUnset: '.',
	renameSet:   'r',
}

// File represents an entry in a manifest
type File struct {
	Name    string
	Hash    Hashval
	Version uint32

	// flags
	Type     ftype
	Status   fstatus
	Modifier fmodifier
	Rename   frename

	// renames
	RenameScore uint16
	RenamePeer  *File

	Info      os.FileInfo
	DeltaPeer *File
}

// typeFromFlag return file type based on flag byte
func typeFromFlag(flag byte) (ftype, error) {
	switch flag {
	case 'F':
		return typeFile, nil
	case 'D':
		return typeDirectory, nil
	case 'L':
		return typeLink, nil
	case 'M':
		return typeManifest, nil
	case '.':
		return typeUnset, nil
	default:
		return typeUnset, fmt.Errorf("invalid file type flag: %v", flag)
	}
}

func (t ftype) String() string {
	switch t {
	case typeFile:
		return "F"
	case typeDirectory:
		return "D"
	case typeLink:
		return "L"
	case typeManifest:
		return "M"
	case typeUnset:
		return "."
	}
	return "?"
}

// statusFromFlag return status based on flag byte
func statusFromFlag(flag byte) (fstatus, error) {
	switch flag {
	case 'd':
		return statusDeleted, nil
	case 'g':
		return statusGhosted, nil
	case '.':
		return statusUnset, nil
	default:
		return statusUnset, fmt.Errorf("invalid file status flag: %v", flag)
	}
}

// modifierFromFlag return modifier from flag byte
func modifierFromFlag(flag byte) (fmodifier, error) {
	switch flag {
	case 'C':
		return modifierConfig, nil
	case 's':
		return modifierState, nil
	case 'b':
		return modifierBoot, nil
	case '.':
		return modifierUnset, nil
	default:
		return modifierUnset, fmt.Errorf("invalid file modifier flag: %v", flag)
	}
}

// setRenameFromFlag set rename flag from flag byte
func renameFromFlag(flag byte) (frename, error) {
	switch flag {
	case 'r':
		return renameSet, nil
	case '.':
		return renameUnset, nil
	default:
		return renameUnset, fmt.Errorf("invalid file rename flag: %v", flag)
	}
}

// setFlags set flags from flag string
func (f *File) setFlags(flags string) error {
	if len(flags) != 4 {
		return fmt.Errorf("invalid number of flags: %v", flags)
	}

	var err error
	// set file type
	if f.Type, err = typeFromFlag(flags[0]); err != nil {
		return err
	}
	// set status
	if f.Status, err = statusFromFlag(flags[1]); err != nil {
		return err
	}
	// set modifier
	if f.Modifier, err = modifierFromFlag(flags[2]); err != nil {
		return err
	}
	// set rename flag
	if f.Rename, err = renameFromFlag(flags[3]); err != nil {
		return err
	}

	return nil
}

// GetFlagString returns the flags in a format suitable for the Manifest
func (f *File) GetFlagString() (string, error) {
	if f.Type == typeUnset &&
		f.Status == statusUnset &&
		f.Modifier == modifierUnset &&
		f.Rename == renameUnset {
		return "", errors.New("no flags are set on file")
	}

	flagBytes := []byte{
		typeBytes[f.Type],
		statusBytes[f.Status],
		modifierBytes[f.Modifier],
		renameBytes[f.Rename],
	}

	return string(flagBytes), nil
}

func (f *File) findFileNameInSlice(fs []*File) *File {
	for _, file := range fs {
		if file.Name == f.Name {
			return file
		}
	}

	return nil
}

func sameFile(f1 *File, f2 *File) bool {
	return f1.Name == f2.Name &&
		f1.Hash == f2.Hash &&
		f1.Type == f2.Type &&
		f1.Status == f2.Status &&
		f1.Modifier == f2.Modifier
}

func (f *File) isUnsupportedTypeChange() bool {
	if f.DeltaPeer == nil {
		// nothing to check, new or deleted file
		return false
	}

	if f.Status == statusDeleted || f.DeltaPeer.Status == statusDeleted {
		return false
	}

	if f.Type == f.DeltaPeer.Type {
		return false
	}

	// file -> link OK
	// file -> directory OK
	// link -> file OK
	// link -> directory OK
	// directory -> anything TYPE CHANGE
	return (f.DeltaPeer.Type == typeDirectory && f.Type != typeDirectory)
}
