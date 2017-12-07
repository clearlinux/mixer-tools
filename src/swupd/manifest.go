package swupd

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const manifestFieldDelim = "\t"

// ManifestHeader contains metadata for the manifest
type ManifestHeader struct {
	Format      uint
	Version     uint32
	Previous    uint32
	FileCount   uint32
	TimeStamp   time.Time
	ContentSize uint64
	Includes    []*Manifest
}

// Manifest represents a bundle or list of bundles (MoM)
type Manifest struct {
	Name   string
	Header ManifestHeader
	Files  []*File
}

// MoM is a manifest of manifests with the same header information
type MoM struct {
	Header       ManifestHeader
	SubManifests []*Manifest
}

// readManifestFileHeaderLine Read a header line from a manifest
func readManifestFileHeaderLine(fields []string, m *Manifest) error {
	var err error
	var parsed uint64

	// Only search for defined fields
	switch fields[0] {
	case "MANIFEST":
		if parsed, err = strconv.ParseUint(fields[1], 10, 16); err != nil {
			return err
		}
		m.Header.Format = uint(parsed)
	case "version:":
		if parsed, err = strconv.ParseUint(fields[1], 10, 32); err != nil {
			return err
		}
		m.Header.Version = uint32(parsed)
	case "previous:":
		if parsed, err = strconv.ParseUint(fields[1], 10, 32); err != nil {
			return err
		}
		m.Header.Previous = uint32(parsed)
	case "filecount:":
		if parsed, err = strconv.ParseUint(fields[1], 10, 32); err != nil {
			return err
		}
		m.Header.FileCount = uint32(parsed)
	case "timestamp:":
		var timestamp int64
		if timestamp, err = strconv.ParseInt(fields[1], 10, 64); err != nil {
			return err
		}
		// parsed is already int64
		m.Header.TimeStamp = time.Unix(timestamp, 0)
	case "contentsize:":
		if parsed, err = strconv.ParseUint(fields[1], 10, 64); err != nil {
			return err
		}
		// parsed is already uint64
		m.Header.ContentSize = parsed
	case "includes:":
		m.Header.Includes = append(m.Header.Includes, &Manifest{Name: fields[1]})
	}

	return nil
}

// readManifestFileEntry
// fields: "<fflags, 4 chars>", "<hash, 64 chars>", "<version>", "<filename>"
func readManifestFileEntry(fields []string, m *Manifest) error {
	fflags := fields[0]
	fhash := fields[1]
	fver := fields[2]
	fname := fields[3]

	// check length of fflags and fhash
	if len(fflags) != 4 {
		return fmt.Errorf("invalid number of flags: %v", fflags)
	}

	var parsed uint64
	var err error
	// fver must be a valid uint32
	if parsed, err = strconv.ParseUint(fver, 10, 32); err != nil {
		return fmt.Errorf("invalid version: %v", err)
	}
	ver := uint32(parsed)

	// create a file record
	var file *File
	file = &File{Name: fname, Version: ver}

	// set the file hash
	if err = file.setHash(fhash); err != nil {
		return fmt.Errorf("invalid hash: %v", err)
	}

	// Set the flags using fflags
	if err = file.setFlags(fflags); err != nil {
		return fmt.Errorf("invalid flags: %v", err)
	}

	// add file to manifest
	m.Files = append(m.Files, file)

	return nil
}

// CheckHeaderIsValid verifies that all header fields in the manifest are valid.
func (m *Manifest) CheckHeaderIsValid() error {
	if m.Header.Format == 0 {
		return errors.New("manifest format not set")
	}

	if m.Header.Version == 0 {
		return errors.New("manifest has version zero, version must be positive")
	}

	if m.Header.Version < m.Header.Previous {
		return errors.New("version is smaller than previous")
	}

	if m.Header.FileCount == 0 {
		return errors.New("manifest has a zero file count")
	}

	if m.Header.ContentSize == 0 {
		return errors.New("manifest has zero contentsize")
	}

	if m.Header.TimeStamp.IsZero() {
		return errors.New("manifest timestamp not set")
	}

	// Includes are not required.
	return nil
}

// ReadManifestFromFile reads a manifest file into memory
func (m *Manifest) ReadManifestFromFile(f string) error {
	var err error
	manifestFile, err := os.Open(f)
	if err != nil {
		return err
	}

	// handle Close() errors
	defer func() {
		cerr := manifestFile.Close()
		if err == nil {
			err = cerr
		}
	}()

	fstat, err := manifestFile.Stat()
	if err != nil {
		return err
	}

	if fstat.Size() == 0 {
		return fmt.Errorf("%v is an empty file", f)
	}

	input := bufio.NewScanner(manifestFile)

	inHeader := true
	for input.Scan() {
		manifestLine := input.Text()
		// empty line means end of header
		if len(manifestLine) == 0 {
			if inHeader {
				inHeader = false
				// reached end of header, validate that everything was set
				if err = m.CheckHeaderIsValid(); err != nil {
					return err
				}
				continue
			} else {
				// we already had a blank line, this is an error
				return errors.New("extra blank line in manifest")
			}
		}

		manifestFields := strings.Split(manifestLine, manifestFieldDelim)

		// In the header until an empty line is encountered
		if inHeader {
			if err = readManifestFileHeaderLine(manifestFields, m); err != nil {
				return err
			}
			continue
		}

		// body if we got this far
		if err = readManifestFileEntry(manifestFields, m); err != nil {
			return err
		}
	}

	if err = m.CheckHeaderIsValid(); err != nil {
		return err
	}

	if len(m.Files) == 0 {
		return errors.New("manifest does not have any file entries")
	}

	// return err so the deferred close can modify it
	return err
}
