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

var requiredManifestHeaderEntries = []string{
	"MANIFEST",
	"version:",
	"previous:",
	"filecount:",
	"timestamp:",
	"contentsize:",
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

	// Read the header.
	parsedEntries := make(map[string]uint)
	for input.Scan() {
		text := input.Text()
		if text == "" {
			// Empty line means end of the header.
			break
		}

		fields := strings.Split(text, manifestFieldDelim)
		entry := fields[0]
		if entry != "includes:" && parsedEntries[entry] > 0 {
			return fmt.Errorf("invalid manifest, duplicate entry %q in header", entry)
		}
		parsedEntries[entry]++

		if err = readManifestFileHeaderLine(fields, m); err != nil {
			return err
		}
	}

	// Validate the header.
	for _, e := range requiredManifestHeaderEntries {
		if parsedEntries[e] == 0 {
			return fmt.Errorf("invalid manifest, missing entry %q in header", e)
		}
	}
	err = m.CheckHeaderIsValid()
	if err != nil {
		return err
	}

	// Read the body.
	for input.Scan() {
		text := input.Text()
		if text == "" {
			return errors.New("extra blank line in manifest")
		}

		fields := strings.Split(text, manifestFieldDelim)
		if err = readManifestFileEntry(fields, m); err != nil {
			return err
		}
	}

	if len(m.Files) == 0 {
		return errors.New("manifest does not have any file entries")
	}

	// return err so the deferred close can modify it
	return err
}

// writeManifestFileHeader writes the header of a manifest to the file
func writeManifestFileHeader(m *Manifest, w *bufio.Writer) error {
	var err error
	if err = m.CheckHeaderPopulated(); err != nil {
		return err
	}

	// bufio.Writer is an errWriter, so errors will be returned when the calling
	// function calls w.Flush()
	w.WriteString(fmt.Sprintf("MANIFEST\t%v\n", m.Header.Format))
	w.WriteString(fmt.Sprintf("version:\t%v\n", m.Header.Version))
	w.WriteString(fmt.Sprintf("previous:\t%v\n", m.Header.Previous))
	w.WriteString(fmt.Sprintf("filecount:\t%v\n", m.Header.FileCount))
	w.WriteString(fmt.Sprintf("timestamp:\t%v\n", m.Header.TimeStamp.Unix()))
	w.WriteString(fmt.Sprintf("contentsize:\t%v\n", m.Header.ContentSize))

	for _, include := range m.Header.Includes {
		w.WriteString(fmt.Sprintf("includes:\t%v\n", include.Name))
	}

	return nil
}

// writeManifestFileEntry writes the file entry line to the file
func writeManifestFileEntry(file *File, w *bufio.Writer) error {
	var flags string
	var err error
	if flags, err = file.getFlagString(); err != nil {
		return err
	}

	line := fmt.Sprintf("%v\t%v\t%v\t%v\n",
		flags,
		file.Hash,
		file.Version,
		file.Name)
	_, err = w.WriteString(line)
	return err
}

// WriteManifestFile writes manifest m to a new file at path
func (m *Manifest) WriteManifestFile(path string) error {
	var err error
	var f *os.File
	if f, err = os.Create(path); err != nil {
		return err
	}

	// handle close errors
	defer func() {
		cerr := f.Close()
		if err == nil {
			err = cerr
		}
	}()

	w := bufio.NewWriter(f)

	if err = writeManifestFileHeader(m, w); err != nil {
		return err
	}

	// write separator between header and body
	w.WriteString("\n")

	for _, entry := range m.Files {
		if err := writeManifestFileEntry(entry, w); err != nil {
			return err
		}
	}

	// return error code or nil from w.Flush()
	err = w.Flush()
	// a nil error may be replaced by an f.Close() error in the deferred function
	return err
}
