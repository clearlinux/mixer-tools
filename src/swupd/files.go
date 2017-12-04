package swupd

import (
	"fmt"
	"os"
)

type ftype int
type fmodifier int
type fstatus int

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

// File represents an entry in a manifest
type File struct {
	Name    string
	Hash    string
	Version uint32

	// flags
	Type     ftype
	Status   fstatus
	Modifier fmodifier
	Rename   bool

	// renames
	RenameScore uint16
	RenamePeer  *File

	Info      os.FileInfo
	DeltaPeer *File
}

// setFileTypeFromFlag set file type based on flag byte
func setFileTypeFromFlag(flag byte, f *File) error {
	switch flag {
	case 'F':
		f.Type = FILE
	case 'D':
		f.Type = DIRECTORY
	case 'L':
		f.Type = LINK
	case 'M':
		f.Type = MANIFEST
	case '.':
	default:
		return fmt.Errorf("invalid file type flag: %v", flag)
	}

	return nil
}

// setStatusFromFlag set status based on flag byte
func setStatusFromFlag(flag byte, f *File) error {
	switch flag {
	case 'd':
		f.Status = DELETED
	case 'g':
		f.Status = GHOSTED
	case '.':
	default:
		return fmt.Errorf("invalid file status flag: %v", flag)
	}

	return nil
}

// setModifierFromFlag set modifier from flag byte
func setModifierFromFlag(flag byte, f *File) error {
	switch flag {
	case 'C':
		f.Modifier = CONFIG
	case 's':
		f.Modifier = STATE
	case 'b':
		f.Modifier = BOOT
	case '.':
	default:
		return fmt.Errorf("invalid file modifier flag: %v", flag)
	}

	return nil
}

// setRenameFromFlag set rename flag from flag byte
func setRenameFromFlag(flag byte, f *File) error {
	switch flag {
	case 'r':
		f.Rename = true
	case '.':
		f.Rename = false
	default:
		return fmt.Errorf("invalid file rename flag: %v", flag)
	}

	return nil
}

// setFlags set flags from flag string
func (f *File) setFlags(flags string) error {
	if len(flags) != 4 {
		return fmt.Errorf("invalid number of flags: %v", flags)
	}

	var err error
	// set file type
	if err = setFileTypeFromFlag(flags[0], f); err != nil {
		return err
	}
	// set status
	if err = setStatusFromFlag(flags[1], f); err != nil {
		return err
	}
	// set modifier
	if err = setModifierFromFlag(flags[2], f); err != nil {
		return err
	}
	// set rename flag
	if err = setRenameFromFlag(flags[3], f); err != nil {
		return err
	}

	return nil
}
