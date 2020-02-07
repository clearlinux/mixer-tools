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
)

// TypeFlag describes the file type of a manifest entry.
// It matches the first byte in the flags field.
type TypeFlag uint8

// Valid values for TypeFlag.
const (
	TypeUnset TypeFlag = iota
	TypeFile
	TypeDirectory
	TypeLink
	TypeManifest
	// TODO: IManifests are deprecated. Remove them on format 30
	TypeIManifest
)

var typeBytes = map[TypeFlag]byte{
	TypeUnset:     '.',
	TypeFile:      'F',
	TypeDirectory: 'D',
	TypeLink:      'L',
	TypeManifest:  'M',
	TypeIManifest: 'I',
}

// StatusFlag describes whether a manifest entry is present or not.
// It matches the second byte in the flags field.
type StatusFlag uint8

// Valid values for StatusFlag.
const (
	StatusUnset StatusFlag = iota
	StatusDeleted
	StatusGhosted
	StatusExperimental
)

var statusBytes = map[StatusFlag]byte{
	StatusUnset:        '.',
	StatusDeleted:      'd',
	StatusGhosted:      'g',
	StatusExperimental: 'e',
}

// ModifierFlag describes specific characteristics of a file, used later by
// swupd client when deciding how to update it.
// It matches the third byte in the flags field.
type ModifierFlag uint8

// Valid values for ModifierFlag.
const (
	ModifierUnset ModifierFlag = iota
	ModifierConfig
	ModifierState
	ModifierBoot
)

var modifierBytes = map[ModifierFlag]byte{
	ModifierUnset:  '.',
	ModifierConfig: 'C',
	ModifierState:  's',
	ModifierBoot:   'b',
}

// MiscFlag is a placeholder for additional flags that can be used by swupd-client.
type MiscFlag uint8

// Valid values for MiscFlag
const (
	MiscUnset       MiscFlag = iota
	MiscRename               // deprecated
	MiscMixManifest          // indicates manifest from mixer integrated swupd-client so that swupd-client can hardlink instead of curling
)

// File represents an entry in a manifest
type File struct {
	Name    string
	Hash    Hashval
	Version uint32

	// flags
	Type     TypeFlag
	Status   StatusFlag
	Modifier ModifierFlag
	Misc     MiscFlag

	Info      os.FileInfo
	DeltaPeer *File
}

// typeFromFlag return file type based on flag byte
func typeFromFlag(flag byte) (TypeFlag, error) {
	switch flag {
	case 'F':
		return TypeFile, nil
	case 'D':
		return TypeDirectory, nil
	case 'L':
		return TypeLink, nil
	case 'I':
		return TypeIManifest, nil
	case 'M':
		return TypeManifest, nil
	case '.':
		return TypeUnset, nil
	default:
		return TypeUnset, fmt.Errorf("invalid file type flag: %v", flag)
	}
}

func (t TypeFlag) String() string {
	switch t {
	case TypeFile:
		return "F"
	case TypeDirectory:
		return "D"
	case TypeLink:
		return "L"
	case TypeManifest:
		return "M"
	case TypeIManifest:
		return "I"
	case TypeUnset:
		return "."
	}
	return "?"
}

// statusFromFlag return status based on flag byte
func statusFromFlag(flag byte) (StatusFlag, error) {
	switch flag {
	case 'd':
		return StatusDeleted, nil
	case 'g':
		return StatusGhosted, nil
	case 'e':
		return StatusExperimental, nil
	case '.':
		return StatusUnset, nil
	default:
		return StatusUnset, fmt.Errorf("invalid file status flag: %v", flag)
	}
}

func (s StatusFlag) String() string {
	switch s {
	case StatusDeleted:
		return "d"
	case StatusGhosted:
		return "g"
	case StatusExperimental:
		return "e"
	case StatusUnset:
		return "."
	}
	return "?"
}

// modifierFromFlag return modifier from flag byte
func modifierFromFlag(flag byte) (ModifierFlag, error) {
	switch flag {
	case 'C':
		return ModifierConfig, nil
	case 's':
		return ModifierState, nil
	case 'b':
		return ModifierBoot, nil
	case '.':
		return ModifierUnset, nil
	default:
		return ModifierUnset, fmt.Errorf("invalid file modifier flag: %v", flag)
	}
}

// miscFromFlag return misc flag from flag byte
func miscFromFlag(flag byte) (MiscFlag, error) {
	switch flag {
	case 'r':
		return MiscRename, nil
	case '.':
		return MiscUnset, nil
	case 'm':
		return MiscMixManifest, nil
	default:
		return MiscUnset, fmt.Errorf("invalid file rename flag: %v", flag)
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
	// set misc
	if f.Misc, err = miscFromFlag(flags[3]); err != nil {
		return err
	}

	return nil
}

// GetFlagString returns the flags in a format suitable for the Manifest
func (f *File) GetFlagString() (string, error) {
	if f.Type == TypeUnset &&
		f.Status == StatusUnset &&
		f.Modifier == ModifierUnset {
		return "", fmt.Errorf("no flags are set on file %s", f.Name)
	}

	// only write a '.' or 'm' to a manifest
	// the 'r' flag is deprecated
	miscByte := byte('.')
	if f.Misc == MiscMixManifest {
		miscByte = 'm'
	}

	flagBytes := []byte{
		typeBytes[f.Type],
		statusBytes[f.Status],
		modifierBytes[f.Modifier],
		miscByte,
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

func (f *File) isUnsupportedTypeChange() bool {
	if f.DeltaPeer == nil {
		// nothing to check, new or deleted file
		return false
	}

	if f.Status == StatusDeleted || f.DeltaPeer.Status == StatusDeleted {
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
	return (f.DeltaPeer.Type == TypeDirectory && f.Type != TypeDirectory)
}

// Present tells if a file is present. Returns false if the file is deleted or ghosted.
func (f *File) Present() bool {
	return f.Status != StatusDeleted && f.Status != StatusGhosted
}
