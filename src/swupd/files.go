package swupd

import (
	"fmt"
	"os"
)

type ftype int
type fmodifier int
type fstatus int
type frename bool

const (
	TYPE_UNSET ftype = iota
	FILE
	DIRECTORY
	LINK
	MANIFEST
)

const (
	MODIFIER_UNSET fmodifier = iota
	CONFIG
	STATE
	BOOT
)

const (
	STATUS_UNSET fstatus = iota
	DELETED
	GHOSTED
)

const (
	RENAME_UNSET = false
	RENAME       = true
)

// File represents an entry in a manifest
type File struct {
	Name    string
	Hash    string
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
		return FILE, nil
	case 'D':
		return DIRECTORY, nil
	case 'L':
		return LINK, nil
	case 'M':
		return MANIFEST, nil
	case '.':
		return TYPE_UNSET, nil
	default:
		return TYPE_UNSET, fmt.Errorf("invalid file type flag: %v", flag)
	}
}

// statusFromFlag return status based on flag byte
func statusFromFlag(flag byte) (fstatus, error) {
	switch flag {
	case 'd':
		return DELETED, nil
	case 'g':
		return GHOSTED, nil
	case '.':
		return STATUS_UNSET, nil
	default:
		return STATUS_UNSET, fmt.Errorf("invalid file status flag: %v", flag)
	}
}

// modifierFromFlag return modifier from flag byte
func modifierFromFlag(flag byte) (fmodifier, error) {
	switch flag {
	case 'C':
		return CONFIG, nil
	case 's':
		return STATE, nil
	case 'b':
		return BOOT, nil
	case '.':
		return MODIFIER_UNSET, nil
	default:
		return MODIFIER_UNSET, fmt.Errorf("invalid file modifier flag: %v", flag)
	}
}

// setRenameFromFlag set rename flag from flag byte
func renameFromFlag(flag byte) (frename, error) {
	switch flag {
	case 'r':
		return RENAME, nil
	case '.':
		return RENAME_UNSET, nil
	default:
		return RENAME_UNSET, fmt.Errorf("invalid file rename flag: %v", flag)
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
