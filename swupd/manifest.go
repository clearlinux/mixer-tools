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
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"text/template"
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
	Name         string
	Header       ManifestHeader
	Files        []*File
	DeletedFiles []*File
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
			return fmt.Errorf("invalid manifest, %v", err)
		}
		m.Header.Format = uint(parsed)
	case "version:":
		if parsed, err = strconv.ParseUint(fields[1], 10, 32); err != nil {
			return fmt.Errorf("invalid manifest, %v", err)
		}
		m.Header.Version = uint32(parsed)
	case "previous:":
		if parsed, err = strconv.ParseUint(fields[1], 10, 32); err != nil {
			return fmt.Errorf("invalid manifest, %v", err)
		}
		m.Header.Previous = uint32(parsed)
	case "filecount:":
		if parsed, err = strconv.ParseUint(fields[1], 10, 32); err != nil {
			return fmt.Errorf("invalid manifest, %v", err)
		}
		m.Header.FileCount = uint32(parsed)
	case "timestamp:":
		var timestamp int64
		if timestamp, err = strconv.ParseInt(fields[1], 10, 64); err != nil {
			return fmt.Errorf("invalid manifest, %v", err)
		}
		// parsed is already int64
		m.Header.TimeStamp = time.Unix(timestamp, 0)
	case "contentsize:":
		if parsed, err = strconv.ParseUint(fields[1], 10, 64); err != nil {
			return fmt.Errorf("invalid manifest, %v", err)
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

	if len(fhash) != 64 {
		return fmt.Errorf("invalid hash: %v", fhash)
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
	file.Hash = internHash(fhash)

	// Set the flags using fflags
	if err = file.setFlags(fflags); err != nil {
		return fmt.Errorf("invalid flags: %v", err)
	}

	// add file to manifest
	m.Files = append(m.Files, file)

	// track deleted file
	if file.Status == statusDeleted {
		m.DeletedFiles = append(m.DeletedFiles, file)
	}

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
			return errors.New("invalid manifest, extra blank line")
		}

		fields := strings.Split(text, manifestFieldDelim)
		if err = readManifestFileEntry(fields, m); err != nil {
			return err
		}
	}

	if len(m.Files) == 0 {
		return errors.New("invalid manifest, does not have any file entries")
	}

	return err
}

// Correct visability
func (f *File) FlagString() (string, error) {
	return f.getFlagString()
}

// what a manifest file looks like
// could replace the tabs with \t if we convert this to a normal rather than raw string.
var manifestTemplate = template.Must(template.New("manifest").Parse(`
{{- with .Header -}}
MANIFEST	{{.Format}}
version:	{{.Version}}
previous:	{{.Previous}}
filecount:	{{.FileCount}}
timestamp:	{{(.TimeStamp.Unix)}}
contentsize:	{{.ContentSize -}}
{{range .Includes}}
includes:	{{.Name}}
{{- end}}
{{- end}}
{{ range .Files}}
{{.FlagString}}	{{.Hash}}	{{.Version}}	{{.Name}}
{{- end}}
`))

// WriteManifest writes manifest to a given io.Writer.
func (m *Manifest) WriteManifest(w io.Writer) error {
	err := m.CheckHeaderIsValid()
	if err != nil {
		return err
	}
	err = manifestTemplate.Execute(w, m)
	if err != nil {
		return fmt.Errorf("couldn't write manifest: %s", err)
	}
	return nil
}

// WriteManifestFile writes manifest m to a new file at path.
func (m *Manifest) WriteManifestFile(path string) error {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	err = m.WriteManifest(f)
	if err != nil {
		f.Close()
		_ = os.Remove(path)
		return err
	}
	return f.Close()
}

func (m *Manifest) sortFilesName() {
	// files
	sort.Slice(m.Files, func(i, j int) bool {
		return m.Files[i].Name < m.Files[j].Name
	})
	// and deleted files
	sort.Slice(m.DeletedFiles, func(i, j int) bool {
		return m.DeletedFiles[i].Name < m.DeletedFiles[j].Name
	})
}

func (m *Manifest) sortFilesVersionName() {
	// files
	sort.Slice(m.Files, func(i, j int) bool {
		if m.Files[i].Version < m.Files[j].Version {
			return true
		}
		if m.Files[i].Version > m.Files[j].Version {
			return false
		}
		return m.Files[i].Name < m.Files[j].Name
	})
	// and deleted files
	sort.Slice(m.DeletedFiles, func(i, j int) bool {
		if m.DeletedFiles[i].Version < m.DeletedFiles[j].Version {
			return true
		}
		if m.DeletedFiles[i].Version > m.DeletedFiles[j].Version {
			return false
		}
		return m.DeletedFiles[i].Name < m.DeletedFiles[j].Name
	})
}

func (m *Manifest) addDeleted(oldManifest *Manifest) {
	for _, df := range oldManifest.DeletedFiles {
		if df.findFileNameInSlice(m.Files) == nil {
			m.Files = append(m.Files, df)
			m.DeletedFiles = append(m.DeletedFiles, df)
		}
	}
}

// linkPeersAndChange
// At this point Manifest m should only have the files that were present
// in the chroot for that manifest. Link delta peers with the oldManifest
// if the file in the oldManifest is not deleted or ghosted
func (m *Manifest) linkPeersAndChange(oldManifest *Manifest) int {
	changed := 0
	for _, of := range oldManifest.Files {
		if match := of.findFileNameInSlice(m.Files); match != nil {
			if of.Status == statusDeleted || of.Status == statusGhosted {
				continue
			}

			match.DeltaPeer = of
			of.DeltaPeer = match
			if !sameFile(match, of) {
				match.Version = m.Header.Version
				changed++
			} else {
				match.Version = of.Version
			}
		}
	}

	return changed
}

func (m *Manifest) filesAdded(oldManifest *Manifest) int {
	added := 0
	for _, af := range m.Files {
		if af.findFileNameInSlice(oldManifest.Files) == nil {
			af.Version = m.Header.Version
			added++
		}
	}

	return added
}

func (m *Manifest) newDeleted(oldManifest *Manifest) int {
	deleted := 0
	for _, df := range oldManifest.Files {
		if df.Status != statusDeleted && df.findFileNameInSlice(m.Files) == nil {
			if df.Status == statusGhosted {
				continue
			}
			df.Version = m.Header.Version
			df.Status = statusDeleted
			df.Modifier = modifierUnset
			df.Type = typeUnset
			m.Files = append(m.Files, df)
			m.DeletedFiles = append(m.DeletedFiles, df)
			deleted++
		}
	}

	return deleted
}

func (m *Manifest) readIncludes(bundles []*Manifest, c config) error {
	// read in <imageBase>/<version>/noship/<bundle>-includes
	var err error
	path := filepath.Join(c.imageBase, fmt.Sprint(m.Header.Version), "noship", m.Name+"-includes")
	bundleNames, err := readIncludesFile(path)
	if err != nil {
		return err
	}

	includes := []*Manifest{}
	for _, b := range bundles {
		if b.Name == "os-core" {
			includes = append(includes, b)
		}
	}

	for _, bn := range bundleNames {
		for _, b := range bundles {
			if bn == b.Name {
				includes = append(includes, b)
			}
		}
	}

	m.Header.Includes = includes
	return nil
}

func compareIncludes(m1 *Manifest, m2 *Manifest) bool {
	return reflect.DeepEqual(m1.Header.Includes, m2.Header.Includes)
}

func (m *Manifest) hasUnsupportedTypeChanges() bool {
	for _, f := range m.Files {
		if f.isUnsupportedTypeChange() {
			return true
		}
	}

	return false
}

func (m *Manifest) subtractManifestFromManifest(m2 *Manifest) {
	m.sortFilesName()
	m2.sortFilesName()

	i := 0
	j := 0
	for i < len(m.Files) && j < len(m2.Files) {
		f1 := m.Files[i]
		f2 := m2.Files[j]
		if f1.Name == f2.Name {
			// When both files are marked deleted, skip subtraction.
			// Preserving the deleted entries in both manifests is
			// required for "swupd update" to know when to delete the
			// file, because the m2 bundle may be installed with or
			// without the m1 bundle.
			if f1.Status == statusDeleted && f2.Status == statusDeleted {
				i++
				j++
				continue
			}

			// TODO: figure out why only the is_deleted and is_file fields
			// are checked in the original swupd-server code
			if f1.Status == f2.Status && f1.Type == f2.Type {
				// this is expensive because we care about order at this point
				m.Files = append(m.Files[:i], m.Files[i+1:]...)
			}

			// only need to advance the m2.Files index since i now points to the next
			// file in m.Files
			j++
		} else if f1.Name < f2.Name {
			// check next filename in m
			i++
		} else {
			// check next filename in m2
			j++
		}
	}
}

func (m *Manifest) subtractManifests(m2 *Manifest) {
	if m != m2 {
		m.subtractManifestFromManifest(m2)
	}

	for _, mi := range m2.Header.Includes {
		m.subtractManifestFromManifest(mi)
	}
}

func (m *Manifest) removeDebuginfo(d dbgConfig) {
	for i, f := range m.Files {
		if strings.HasPrefix(f.Name, d.src) && len(f.Name) > len(d.src) {
			copy(m.Files[i:], m.Files[i+1:])
			m.Files[len(m.Files)-1] = &File{}
			m.Files = m.Files[:len(m.Files)-1]
			continue
		}

		if strings.HasPrefix(f.Name, d.lib) && len(f.Name) > len(d.lib) {
			copy(m.Files[i:], m.Files[i+1:])
			m.Files[len(m.Files)-1] = &File{}
			m.Files = m.Files[:len(m.Files)-1]
			continue
		}
	}
}
