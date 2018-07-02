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
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/pkg/errors"
)

const manifestFieldDelim = "\t"

// TODO make this configurable (optional and configurable bundle/filepath)
// this should be done when configuration is in a more stable state
const (
	indexBundle       = "os-core-update-index"
	indexFileName     = "/usr/share/clear/os-core-update-index"
	indexAllBundleDir = "/usr/share/clear/allbundles"
)

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
	bundleInfo   bundleInfo
}

// MoM is a manifest that holds references to bundle manifests.
type MoM struct {
	Manifest

	// UpdatedBundles has the manifests of bundles that are new to this
	// version. To get a list of all the bundles, use Files from embedded
	// Manifest struct.
	UpdatedBundles []*Manifest

	// FullManifest contains information about all the files in this version.
	FullManifest *Manifest
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
	if file.Status == StatusDeleted {
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

// ParseManifestFile creates a Manifest from file in path.
func ParseManifestFile(path string) (*Manifest, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	m, err := ParseManifest(f)
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	m.Name = getNameForManifestFile(path)
	err = f.Close()
	if err != nil {
		return nil, err
	}
	return m, nil
}

func getNameForManifestFile(path string) string {
	prefix := "Manifest."
	idx := strings.LastIndex(path, prefix)
	if idx != -1 {
		return path[idx+len(prefix):]
	}
	return ""
}

// ParseManifest creates a Manifest from an io.Reader.
func ParseManifest(r io.Reader) (*Manifest, error) {
	m := &Manifest{}
	input := bufio.NewScanner(r)

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
			return nil, fmt.Errorf("invalid manifest, duplicate entry %q in header", entry)
		}
		parsedEntries[entry]++

		if err := readManifestFileHeaderLine(fields, m); err != nil {
			return nil, err
		}
	}

	// Validate the header.
	for _, e := range requiredManifestHeaderEntries {
		if parsedEntries[e] == 0 {
			return nil, fmt.Errorf("invalid manifest, missing entry %q in header", e)
		}
	}
	err := m.CheckHeaderIsValid()
	if err != nil {
		return nil, err
	}

	// Read the body.
	for input.Scan() {
		text := input.Text()
		if text == "" {
			return nil, errors.New("invalid manifest, extra blank line")
		}

		fields := strings.Split(text, manifestFieldDelim)
		if err := readManifestFileEntry(fields, m); err != nil {
			return nil, err
		}
	}

	if len(m.Files) == 0 {
		return nil, errors.New("invalid manifest, does not have any file entries")
	}

	return m, nil
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
{{.GetFlagString}}	{{.Hash}}	{{.Version}}	{{.Name}}
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
		return fmt.Errorf("couldn't write Manifest.%s: %s", m.Name, err)
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
		_ = f.Close()
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

// linkPeersAndChange
// At this point Manifest m should only have the files that were present
// in the chroot for that manifest. Link delta peers with the oldManifest
// if the file in the oldManifest is not deleted or ghosted.
// Expects m and oldManifest files lists to be sorted by name only
func (m *Manifest) linkPeersAndChange(oldManifest *Manifest, c config, minVersion uint32) (int, int, int) {
	// set previous version to oldManifest version
	m.Header.Previous = oldManifest.Header.Version

	var changed, removed, added []*File
	nx := 0 // new manifest file index
	ox := 0 // old manifest file index
	mFilesLen := len(m.Files)
	omFilesLen := len(oldManifest.Files)
	for nx < mFilesLen && ox < omFilesLen {
		nf := m.Files[nx]
		of := oldManifest.Files[ox]
		if nf.Name == of.Name {
			// if it is the same name check if anything about the file has changed.
			// if something has changed update the version to current version and
			// record that the file has changed
			if of.Status == StatusDeleted || of.Status == StatusGhosted {
				nf.Version = m.Header.Version
				added = append(added, nf)
				ox++
				nx++
				continue
			}
			if nf.Hash == of.Hash && of.Version >= minVersion {
				// same contents, version doesn't change.
				nf.Version = of.Version
			} else {
				// if the file isn't exactly the same, record the change
				// and update the version
				// this case is also hit when the old file is older than the
				// minversion
				nf.Version = m.Header.Version
				changed = append(changed, nf)
				// set up peers since old file exists
				nf.DeltaPeer = of
				of.DeltaPeer = nf
			}
			// advance indices for both file lists since we had a match
			nx++
			ox++
		} else if nf.Name < of.Name {
			// if the file does not exist in the old manifest it is a new
			// file in this manifest. Update the version and record that
			// it is added.
			nf.Version = m.Header.Version
			added = append(added, nf)
			// look at next file in current manifest
			nx++
		} else {
			// if the file exists in the old manifest and does not *yet* exist
			// in the new manifest, it was deleted or is an old deleted/ghosted
			// file
			if !of.Present() {
				if of.Status == StatusDeleted && of.Version >= minVersion {
					// copy over old deleted files
					// append as-is
					m.Files = append(m.Files, of)
				}
				ox++
				continue
			}
			// this is a new deleted file
			m.newDeleted(of)
			removed = append(removed, of)
			// look at next file in old manifest
			ox++
		}
	}

	// anything remaining in m does not exist in oldManifest
	for _, nf := range m.Files[nx:mFilesLen] {
		nf.Version = m.Header.Version
		added = append(added, nf)
	}

	// anything remaining in oldManifest is newly deleted in the new manifest
	for _, of := range oldManifest.Files[ox:omFilesLen] {
		if of.Status == StatusDeleted || of.Status == StatusGhosted {
			continue
		}
		m.newDeleted(of)
		removed = append(removed, of)
	}

	// finally re-sort since we changed the order
	m.sortFilesName()
	// return the number of changed, added, or deleted files in this
	// manifest
	return len(changed), len(added), len(removed)
}

func (m *Manifest) newDeleted(df *File) {
	df.Version = m.Header.Version
	df.Status = StatusDeleted
	df.Type = TypeUnset
	df.Modifier = ModifierUnset
	df.Hash = 0
	// Add file to manifest
	m.Files = append(m.Files, df)
}

// linkDeltaPeersForPack sets the DeltaPeer of the files in newManifest that have the corresponding files
// in oldManifest.
func linkDeltaPeersForPack(c *config, oldManifest, newManifest *Manifest) error {
	newIndex := 0
	oldIndex := 0
	added := []*File{}
	removed := []*File{}

	for newIndex < len(newManifest.Files) && oldIndex < len(oldManifest.Files) {
		nf := newManifest.Files[newIndex]
		of := oldManifest.Files[oldIndex]

		switch {
		case nf.Name < of.Name:
			// New file in new manifest, try for rename
			if nf.Present() && nf.Type == TypeFile {
				added = append(added, nf)
			}
			newIndex++

		case nf.Name > of.Name:
			// Old file not in new manifest, try for rename
			if of.Present() && of.Type == TypeFile {
				removed = append(removed, of)
			}
			oldIndex++

		default:
			newIndex++
			oldIndex++
			if !nf.Present() && of.Present() && of.Type == TypeFile {
				removed = append(removed, of)
				continue
			}

			if nf.Present() && !of.Present() && nf.Type == TypeFile {
				added = append(added, nf)
				continue
			}

			// Matching names, we can have a delta if pass all the
			// requirements below.
			if nf.Version <= of.Version {
				continue
			}

			if nf.Hash == of.Hash {
				continue
			}

			if !nf.Present() || !of.Present() {
				continue
			}

			if nf.Type != TypeFile || nf.Type != of.Type {
				continue
			}

			// Check file size to decide whether it should have a delta.
			newPath := filepath.Join(c.imageBase, fmt.Sprint(nf.Version), "full", nf.Name)
			fi, err := os.Stat(newPath)
			if err != nil {
				return errors.Wrapf(err, "error accessing %s to decide whether it can have a delta or not")
			}
			if fi.Size() < minimumSizeToMakeDeltaInBytes {
				continue
			}

			nf.DeltaPeer = of
			of.DeltaPeer = nf
		}
	}

	// Run rename detection on old and new manifests
	renameDetection(newManifest, added, removed, *c)

	return nil
}

func includesChanged(m1 *Manifest, m2 *Manifest) bool {
	if len(m1.Header.Includes) != len(m2.Header.Includes) {
		return true
	}

	for i := 0; i < len(m1.Header.Includes); i++ {
		if m1.Header.Includes[i].Name != m2.Header.Includes[i].Name {
			return true
		}
	}

	return false
}

func (m *Manifest) hasUnsupportedTypeChanges() bool {
	for _, f := range m.Files {
		if f.isUnsupportedTypeChange() {
			return true
		}
	}

	return false
}

// subtractManifestFromManifest removes all files present in m2 from m.
// Expects m and m2 files lists to be sorted by name only
func (m *Manifest) subtractManifestFromManifest(m2 *Manifest) {
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
			if f1.Status == StatusDeleted && f2.Status == StatusDeleted {
				i++
				j++
				continue
			}

			// TODO: figure out why only the is_deleted and is_file fields
			// are checked in the original swupd-server code
			if f1.Status == f2.Status && f1.Type == f2.Type {
				// this is expensive because we care about order at this point
				m.Files = append(m.Files[:i], m.Files[i+1:]...)
				m.Header.FileCount--
				m.Header.ContentSize -= uint64(f1.Info.Size())
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
		m.subtractManifests(mi)
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

// getManifestVerFromMoM Find last version for b manifest from mom
func getManifestVerFromMoM(mom *Manifest, b *Manifest) uint32 {
	for _, m := range mom.Files {
		if m.Name == b.Name {
			return m.Version
		}
	}

	return 0
}

// maxFullFromManifest maximizes all file entries in mf that are also in bundle
// Expects mf and bundle file lists to be sorted by name only.
func maxFullFromManifest(mf *Manifest, bundle *Manifest) {
	i := 0
	j := 0

	for i < len(mf.Files) && j < len(bundle.Files) {
		ff := mf.Files[i]
		bf := bundle.Files[j]
		if ff.Name == bf.Name {
			// files match, maximize versions if appropriate
			if bf.Status != StatusDeleted && bf.Version > ff.Version {
				ff.Version = bf.Version
			}
			// advance both indices
			i++
			j++
		} else if ff.Name < bf.Name {
			// check next filename in mf
			i++
		} else {
			// check next filename in bundle
			j++
		}
	}
}

// maximizeFull maximizes all file entries in mf for each bundle in bundles
// Expects mf and bundles file lists to be sorted by name only.
func maximizeFull(mf *Manifest, bundles []*Manifest) {
	for _, b := range bundles {
		maxFullFromManifest(mf, b)
	}
}

func createAndWrite(path string, contents []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0777); err != nil {
		return err
	}
	return ioutil.WriteFile(path, contents, 0655)
}

func fileContains(path string, sub string) bool {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return false
	}

	return bytes.Contains(b, []byte(sub))
}

type bundleIndex struct {
	fname string
	bname string
	bsize uint64
}

func constructIndex(c *config, ui *UpdateInfo, f2b []*bundleIndex) error {
	// sort by filename then by bundle size
	sort.Slice(f2b[:], func(i, j int) bool {
		if f2b[i].fname == f2b[j].fname {
			return f2b[i].bsize < f2b[j].bsize
		}
		return f2b[i].fname < f2b[j].fname
	})

	// construct content for the file
	var output []byte
	for _, line := range f2b {
		strLine := fmt.Sprintf("%s\t%s\n", line.fname, line.bname)
		output = append(output, []byte(strLine)...)
	}

	imageVerPath := filepath.Join(c.imageBase, fmt.Sprint(ui.version))
	// write to bundle chroot
	outputFileName := filepath.Join(imageVerPath, indexBundle, indexFileName)
	if err := createAndWrite(outputFileName, output); err != nil {
		return err
	}

	trackingFile := filepath.Join("/usr/share/clear/bundles", indexBundle)
	if err := createAndWrite(filepath.Join(imageVerPath, indexBundle, trackingFile), []byte{}); err != nil {
		return err
	}

	// write to full chroot
	fullFileName := filepath.Join(imageVerPath, "full", indexFileName)
	if err := createAndWrite(fullFileName, output); err != nil {
		return err
	}

	return createAndWrite(filepath.Join(imageVerPath, "full", trackingFile), []byte{})
}

// writeIndexManifest creates a file that is an index of all files -> bundle mappings in
// the current update. This file excludes directories and files that are not present and
// sorts the index first by filename then by bundle name. writeIndexManifest creates a new
// bundle in which the file will live. This bundle is added to the "full" manifest which
// is part of the bundles slice. A pointer to the new manifest is returned on success.
func writeIndexManifest(c *config, ui *UpdateInfo, bundles []*Manifest) (*Manifest, error) {
	fileToBundles := []*bundleIndex{}
	var newFull, newOsCore *Manifest
	for _, b := range bundles {
		if b.Name == "full" {
			// record for later and skip
			newFull = b
			continue
		}

		if b.Name == "os-core" {
			// record for includes list
			newOsCore = b
		}

		for _, f := range b.Files {
			// only record present files that are TypeFile or TypeSymlink
			if f.Present() && f.Type != TypeDirectory {
				ftb := &bundleIndex{
					f.Name,
					b.Name,
					b.Header.ContentSize,
				}
				fileToBundles = append(fileToBundles, ftb)
			}
		}
	}

	// no full manifest in bundles list
	if newFull == nil {
		return nil, errors.New("no full manifest found")
	}

	// construct the index file from the bundleIndex slice
	if err := constructIndex(c, ui, fileToBundles); err != nil {
		return nil, err
	}

	// now add a manifest
	idxMan := &Manifest{
		Header: ManifestHeader{
			Format:    ui.format,
			Version:   ui.version,
			Previous:  ui.lastVersion,
			TimeStamp: ui.timeStamp,
			Includes:  []*Manifest{newOsCore},
		},
		Name: indexBundle,
	}

	bundleDir := filepath.Join(c.imageBase, fmt.Sprint(ui.version))
	// add files from the chroot created in constructIndex
	err := idxMan.addFilesFromChroot(filepath.Join(bundleDir, indexBundle), "")
	if err != nil {
		return nil, err
	}

	// if the allbundles directory was created add all those bundle files
	// to the index as well
	metaRoot := filepath.Join(bundleDir, "full", indexAllBundleDir)
	if _, err = os.Stat(metaRoot); err == nil {
		err = idxMan.addFilesFromChroot(metaRoot, filepath.Join(bundleDir, "full"))
		if err != nil {
			return nil, err
		}
	}
	// record file count
	idxMan.Header.FileCount = uint32(len(idxMan.Files))
	// sort file list for processing
	idxMan.sortFilesName()
	// now subtract out the included os-core
	idxMan.subtractManifests(newOsCore)
	// figure out if we need to update any includes
	for _, b := range bundles {
		if b.Header.Version < ui.version {
			continue
		}

		includesPath := filepath.Join(c.imageBase, fmt.Sprint(ui.version), "noship", b.Name+"-includes")
		if fileContains(includesPath, indexBundle) {
			b.Header.Includes = append(b.Header.Includes, idxMan)
			b.subtractManifests(idxMan)
		}
	}

	// link in old manifest for version information
	// first get old MoM
	oldMoMPath := filepath.Join(c.outputDir, fmt.Sprint(ui.lastVersion), "Manifest.MoM")
	oldMoM, err := getOldManifest(oldMoMPath)
	if err != nil {
		return nil, err
	}

	// get old version
	ver := getManifestVerFromMoM(oldMoM, idxMan)
	if ver == 0 {
		ver = ui.lastVersion
	}

	oldMPath := filepath.Join(c.outputDir, fmt.Sprint(ver), "Manifest."+idxMan.Name)
	oldM, err := getOldManifest(oldMPath)
	if err != nil {
		return nil, err
	}
	oldM.sortFilesName()
	// linkPeersAndChange will update file versions correctly
	_, _, _ = idxMan.linkPeersAndChange(oldM, *c, ui.minVersion)
	// now add any new files to the full manifest
	for _, idxF := range idxMan.Files {
		i := sort.Search(len(newFull.Files), func(i int) bool {
			return newFull.Files[i].Name >= idxF.Name
		})

		if i < len(newFull.Files) && newFull.Files[i].Name == idxF.Name {
			// overwrite existing
			newFull.Files[i] = idxF
			continue
		}

		// add to full manifest
		newFull.Files = append(newFull.Files, idxF)
	}

	// done processing, sort by version before writing
	idxMan.sortFilesVersionName()
	manOutput := filepath.Join(c.outputDir, fmt.Sprint(ui.version), "Manifest."+indexBundle)
	if err := idxMan.WriteManifestFile(manOutput); err != nil {
		return nil, err
	}

	return idxMan, nil
}

func fileContentInManifest(f *File, m *Manifest) bool {
	if m == nil {
		return false
	}
	for i := range m.Files {
		if f.Name != m.Files[i].Name {
			continue
		}
		if f.Hash != m.Files[i].Hash {
			continue
		}
		if f.Type != m.Files[i].Type {
			continue
		}
		if f.Status != m.Files[i].Status {
			continue
		}
		if f.Modifier != m.Files[i].Modifier {
			continue
		}
		return true
	}
	return false
}

// this is a hack to allow users to update using swupd-client v3.15.3 which performs a
// check on contentsize with a maximum a couple of orders off the intended maximum.
// Remove this code (and the caller) when a format bump has occurred in Clear.
var badMax uint64 = 2000000000

func (m *Manifest) setMaxContentSizeHack() {
	if m.Header.ContentSize >= badMax {
		m.Header.ContentSize = badMax - 1
	}
}

// end hack
