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
	Hash    hashval
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

// setHash intern hashes of correct length and add index to f.Hash
func (f *File) setHash(hash string) error {
	if len(hash) != 64 {
		return fmt.Errorf("hash %v incorrect length", hash)
	}

	f.Hash = internHash(hash)
	return nil
}

func (f *File) setHashZero() {
	f.Hash = 0
}

func (f *File) getHashString() string {
	return *Hashes[f.Hash]
}

func (f *File) getFlagString() (string, error) {
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
