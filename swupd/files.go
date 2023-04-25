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
	"strings"
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
// Variable name style is optname_bitflagvalue.
// This is append only, so when new bitflagvalues are added, make new combinations
// from the bottom of the current list.
const (
	SSE_0 ModifierFlag = iota
	SSE_1
	SSE_2
	SSE_3
	AVX2_1
	AVX2_3
	AVX512_2
	AVX512_3
)

var modifierPrefixes = map[ModifierFlag]string{
	SSE_0:    "",
	SSE_1:    "",
	SSE_2:    "",
	SSE_3:    "",
	AVX2_1:   "/V3",
	AVX2_3:   "/V3",
	AVX512_2: "/V4",
	AVX512_3: "/V4",
}

// The three maps below were generated using the following:
// a := ".acdefghijklmnopqrtuvwxyzABDEFGHIJKLMNOPQRSTUVWXYZ0123456789!#^*"

// fmt.Println("var modifierBytes = map[ModifierFlag]byte{")
// for i, c := range a {
// 	fmt.Printf("\t%d: '%c',\n", i, c)
// }
// fmt.Println("}\n")
// fmt.Println("var byteModifiers = map[byte]ModifierFlag{")
// for i, c := range a {
// 	fmt.Printf("\t'%c': %d,\n", c, i)
// }
// fmt.Println("\t'b': 0,")
// fmt.Println("\t's': 0,")
// fmt.Println("\t'C': 0,")
// fmt.Println("}\n")

// fmt.Println("var modifierMasks = map[ModifierFlag]uint64{")
// fmt.Println("\tSSE_0: 0,")
// fmt.Println("\tAVX2_1: 1 << 0,")
// fmt.Println("\tAVX512_2: 1 << 1,")
// fmt.Println("}")
var modifierBytes = map[ModifierFlag]byte{
	0:  '.',
	1:  'a',
	2:  'c',
	3:  'd',
	4:  'e',
	5:  'f',
	6:  'g',
	7:  'h',
	8:  'i',
	9:  'j',
	10: 'k',
	11: 'l',
	12: 'm',
	13: 'n',
	14: 'o',
	15: 'p',
	16: 'q',
	17: 'r',
	18: 't',
	19: 'u',
	20: 'v',
	21: 'w',
	22: 'x',
	23: 'y',
	24: 'z',
	25: 'A',
	26: 'B',
	27: 'D',
	28: 'E',
	29: 'F',
	30: 'G',
	31: 'H',
	32: 'I',
	33: 'J',
	34: 'K',
	35: 'L',
	36: 'M',
	37: 'N',
	38: 'O',
	39: 'P',
	40: 'Q',
	41: 'R',
	42: 'S',
	43: 'T',
	44: 'U',
	45: 'V',
	46: 'W',
	47: 'X',
	48: 'Y',
	49: 'Z',
	50: '0',
	51: '1',
	52: '2',
	53: '3',
	54: '4',
	55: '5',
	56: '6',
	57: '7',
	58: '8',
	59: '9',
	60: '!',
	61: '#',
	62: '^',
	63: '*',
}

var byteModifiers = map[byte]ModifierFlag{
	'.': 0,
	'a': 1,
	'c': 2,
	'd': 3,
	'e': 4,
	'f': 5,
	'g': 6,
	'h': 7,
	'i': 8,
	'j': 9,
	'k': 10,
	'l': 11,
	'm': 12,
	'n': 13,
	'o': 14,
	'p': 15,
	'q': 16,
	'r': 17,
	't': 18,
	'u': 19,
	'v': 20,
	'w': 21,
	'x': 22,
	'y': 23,
	'z': 24,
	'A': 25,
	'B': 26,
	'D': 27,
	'E': 28,
	'F': 29,
	'G': 30,
	'H': 31,
	'I': 32,
	'J': 33,
	'K': 34,
	'L': 35,
	'M': 36,
	'N': 37,
	'O': 38,
	'P': 39,
	'Q': 40,
	'R': 41,
	'S': 42,
	'T': 43,
	'U': 44,
	'V': 45,
	'W': 46,
	'X': 47,
	'Y': 48,
	'Z': 49,
	'0': 50,
	'1': 51,
	'2': 52,
	'3': 53,
	'4': 54,
	'5': 55,
	'6': 56,
	'7': 57,
	'8': 58,
	'9': 59,
	'!': 60,
	'#': 61,
	'^': 62,
	'*': 63,
	'b': 0,
	's': 0,
	'C': 0,
}

var modifierMasks = map[ModifierFlag]uint64{
	SSE_0:    0,
	AVX2_1:   1 << 0,
	AVX512_2: 1 << 1,
}

// MiscFlag is a placeholder for additional flags that can be used by swupd-client.
type MiscFlag uint8

// Valid values for MiscFlag
const (
	MiscUnset       MiscFlag = iota
	MiscRename               // deprecated
	MiscMixManifest          // indicates manifest from mixer integrated swupd-client so that swupd-client can hardlink instead of curling
	MiscExportFile           // indicates file that can be exported in swupd-client
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

// miscFromFlag return misc flag from flag byte
func miscFromFlag(flag byte) (MiscFlag, error) {
	switch flag {
	case 'r':
		return MiscRename, nil
	case '.':
		return MiscUnset, nil
	case 'm':
		return MiscMixManifest, nil
	case 'x':
		return MiscExportFile, nil
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
	var errb bool
	// set file type
	if f.Type, err = typeFromFlag(flags[0]); err != nil {
		return err
	}
	// set status
	if f.Status, err = statusFromFlag(flags[1]); err != nil {
		return err
	}
	// set modifier
	if f.Modifier, errb = byteModifiers[flags[2]]; errb == false {
		return fmt.Errorf("Invalid file modifier flag: %v", flags[2])
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
		f.Status == StatusUnset {
		return "", fmt.Errorf("no flags are set on file %s", f.Name)
	}

	// the 'r' flag is deprecated
	miscByte := byte('.')
	if f.Misc == MiscMixManifest {
		miscByte = 'm'
	} else if f.Misc == MiscExportFile {
		miscByte = 'x'
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

func (f *File) hasOptPrefix() bool {
	return strings.HasPrefix(f.Name, "/V3") || strings.HasPrefix(f.Name, "/V4")
}
