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
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/clearlinux/mixer-tools/log"

	"github.com/pkg/errors"
)

const manifestFieldDelim = "\t"

// IndexBundle defines the name of the bundle that contains index information
// about the current update
const IndexBundle = "os-core-update-index"

// ManifestHeader contains metadata for the manifest
type ManifestHeader struct {
	Format      uint
	Version     uint32
	Previous    uint32
	MinVersion  uint32
	FileCount   uint32
	TimeStamp   time.Time
	ContentSize uint64
	Includes    []*Manifest
	Optional    []*Manifest
}

// Manifest represents a bundle or list of bundles (MoM)
type Manifest struct {
	Name         string
	Header       ManifestHeader
	Files        []*File
	DeletedFiles []*File
	BundleInfo   BundleInfo
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
	case "minversion:":
		if parsed, err = strconv.ParseUint(fields[1], 10, 32); err != nil {
			return fmt.Errorf("invalid manifest, %v", err)
		}
		m.Header.MinVersion = uint32(parsed)
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
	case "also-add:":
		m.Header.Optional = append(m.Header.Optional, &Manifest{Name: fields[1]})
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
	file := &File{Name: fname, Version: ver}

	// set the file hash
	file.Hash = internHash(fhash)

	// Set the flags using fflags
	if err = file.setFlags(fflags); err != nil {
		return fmt.Errorf("invalid flags: %v", err)
	}

	// IManifests are deprecated. Ignore them
	// TODO: Remove code on format 30
	if file.Type == TypeIManifest {
		return nil
	}

	// Reset the file prefix back what is in the chroot
	file.setPrefixFromModifier()

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
		if (entry != "includes:" && entry != "also-add:") && parsedEntries[entry] > 0 {
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

func (m *Manifest) removeOptNonFiles() {
	seen := make(map[string]bool)
	i := 0
	for _, f := range m.Files {
		if f.Type == TypeFile || f.Type == TypeUnset || !f.hasOptPrefix() {
			// Looking files files, deleted files or anything else that doesn't have an
			// optimization prefix.
			if f.Status == StatusDeleted {
				if strings.HasPrefix(f.Name, "/V3") || strings.HasPrefix(f.Name, "/V4") {
					// for non-SSE deleted files the versions on matching files need bumping
					// so update will replace the deleted optimized file.
					// Track these files for later
					strippedName := strings.TrimPrefix(f.Name, "/V3")
					strippedName = strings.TrimPrefix(strippedName, "/V4")
					seen[strippedName] = true
					continue
				}
			}
			m.Files[i] = f
			i++
		}
	}
	for j := i; j < len(m.Files); j++ {
		m.Files[j] = nil
	}
	m.Files = m.Files[:i]
	for _, f := range m.Files {
		strippedName := strings.TrimPrefix(f.Name, "/V3")
		strippedName = strings.TrimPrefix(strippedName, "/V4")
		// Only SSE content or non-deleted optimized files should be added to the filelist
		if result := seen[strippedName]; result {
			// Now update any versions for matching files of newly deleted optimized files
			f.Version = m.Header.Version
		}
	}
}

func (m *Manifest) setupModifiers() error {
	m.removeOptNonFiles()
	allfiles := make(map[string][]*File)
	optbin := make(map[string][]*File)
	for _, f := range m.Files {
		if f.Type != TypeFile {
			continue
		}
		f.setModifierFromPathname()
		allfiles[f.Name] = append(allfiles[f.Name], f)
		if len(allfiles[f.Name]) > 1 || f.Modifier != SSE_0 {
			optbin[f.Name] = allfiles[f.Name]
		}
	}

	for k, v := range optbin {
		var m uint64
		ssebin := false
		misc := MiscUnset
		status := StatusUnset
		for _, f := range v {
			if f.Modifier == SSE_0 {
				ssebin = true
				misc = f.Misc
				status = f.Status
			}
			m = m | modifierMasks[f.Modifier]
		}
		for _, f := range v {
			if f.Modifier != SSE_0 {
				f.Misc = misc
				f.Status = status
			}
		}
		if !ssebin {
			return fmt.Errorf("%s missing non-SSE version of binary", k)
		}
		for _, f := range v {
			f.setFullModifier(m)
		}
	}
	m.sortFilesVersionName()
	return nil
}

// WriteManifest writes manifest to a given io.Writer.
func (m *Manifest) WriteManifest(w io.Writer) error {
	err := m.CheckHeaderIsValid()
	if err != nil {
		return err
	}
	t := manifestTemplateForFormat(m.Header.Format)
	// Last step before writing out the manifest
	// the modifiers and files are finalized extremely late
	// as the changes are cosmetic to the filename and are not
	// to be used elsewhere.
	// Some concern over comparing a chroot with paths that
	// are different in the manifest file but otherwise this is
	// low impact.
	err = m.setupModifiers()
	if err != nil {
		return fmt.Errorf("Manifest.%s content error: %s", m.Name, err)
	}
	err = t.Execute(w, m)
	if err != nil {
		return fmt.Errorf("couldn't write Manifest.%s: %s", m.Name, err)
	}
	for _, f := range m.Files {
		// Reset the prefix as the manifest struct might be used by other
		// mixer processes and needs to have the path prefix in those cases.
		f.setPrefixFromModifier()
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
// At this point Manifest m should only have the files that were present in the
// chroot for that manifest. Link delta peers with the oldManifest if the file
// in the oldManifest is not deleted or ghosted. Expects m and oldManifest
// files lists to be sorted by name only.
//
// An important note is that deletes must persist over minversions but not over
// format bumps.
func (m *Manifest) linkPeersAndChange(oldManifest *Manifest, minVersion uint32) (int, int, int) {
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
			if nf.Hash == of.Hash && of.Version >= minVersion && (nf.hasOptPrefix() || nf.Misc == of.Misc) {
				// same contents, version doesn't change.
				// misc is ignored for files with an optimization prefix
				// as that is setup elsewhere to match the base file
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
				if of.Status == StatusDeleted && m.Header.Format == oldManifest.Header.Format {
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

	// anything remaining in oldManifest is deleted in the new manifest
	for _, of := range oldManifest.Files[ox:omFilesLen] {
		if !of.Present() {
			if of.Status == StatusDeleted && m.Header.Format == oldManifest.Header.Format {
				m.Files = append(m.Files, of)
			}
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
	df.Modifier = SSE_0
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
				return errors.Wrapf(err, "error accessing %s to decide whether it can have a delta or not", newPath)
			}
			if fi.Size() < minimumSizeToMakeDeltaInBytes {
				continue
			}

			nf.DeltaPeer = of
			of.DeltaPeer = nf
		}
	}

	// Run rename detection on old and new manifests
	return renameDetection(newManifest, added, removed, *c)
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

	if len(m1.Header.Optional) != len(m2.Header.Optional) {
		return true
	}

	for i := 0; i < len(m1.Header.Optional); i++ {
		if m1.Header.Optional[i].Name != m2.Header.Optional[i].Name {
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

// addAllManifestFiles creates a sorted list of file records for each manifest.
func addAllManifestFiles(manifests []*Manifest, ui UpdateInfo, c config, numWorkers int) error {
	var wg sync.WaitGroup
	if numWorkers <= 0 {
		numWorkers = runtime.NumCPU()
	}
	if numWorkers > len(ui.bundles) {
		numWorkers = len(ui.bundles)
	}
	wg.Add(numWorkers)

	mCh := make(chan *Manifest)
	errorChan := make(chan error, numWorkers)
	defer close(errorChan)

	manifestWorker := func() {
		defer wg.Done()
		for m := range mCh {
			if err := m.addManifestFiles(ui, c); err != nil {
				log.Error(log.Mixer, err.Error())
				errorChan <- err
				return
			}
		}
	}

	for i := 0; i < numWorkers; i++ {
		go manifestWorker()
	}

	var err error
	for _, m := range manifests {
		select {
		case mCh <- m:
		case err = <-errorChan:
			break
		}
		if err != nil {
			break
		}
	}

	close(mCh)
	wg.Wait()

	if err != nil {
		return err
	}

	if len(errorChan) > 0 {
		err = <-errorChan
	}

	return err
}

// addManifestFiles creates a sorted list of file records for a manifest.
// Files in the manifest's bundle info file that exist in included bundles
// will not be added.
func (m *Manifest) addManifestFiles(ui UpdateInfo, c config) error {
	if m.Name == "full" {
		chroot := filepath.Join(c.imageBase, fmt.Sprint(ui.version), "full")
		if err := m.addFilesFromChroot(chroot, ""); err != nil {
			return err
		}
	} else {
		// Add files to manifest that do not exist in included bundles.
		includes := m.GetRecursiveIncludes()
		for f := range m.BundleInfo.Files {
			isIncluded := false
			for _, inc := range includes {
				if _, ok := inc.BundleInfo.Files[f]; ok {
					isIncluded = true
					break
				}
			}
			if !isIncluded {
				if err := m.addFile(f, c, ui.version); err != nil {
					return err
				}
			}
		}

		if m.hasUnsupportedTypeChanges() {
			return errors.New("type changes not yet supported")
		}
	}

	if c.debuginfo.banned {
		m.removeDebuginfo(c.debuginfo)
	}

	m.sortFilesName()

	return nil
}

// addFile creates a file record and adds it to the manifest.
func (m *Manifest) addFile(fpath string, c config, version uint32) error {
	chrootDir := filepath.Join(c.imageBase, fmt.Sprint(version), "full")
	fullPath := filepath.Join(chrootDir, fpath)
	fi, err := os.Lstat(fullPath)
	if os.IsNotExist(err) {
		log.Warning("Missing file, assuming %%ghost: %s", fpath)
		return nil
	}
	if err != nil {
		return err
	}

	err = m.createFileRecord(chrootDir, fpath, "", fi)
	if err != nil {
		if strings.Contains(err.Error(), "hash calculation error") {
			return err
		}
		log.Warning(log.Mixer, "%s", err.Error())
	}

	return nil
}

// GetRecursiveIncludes returns a list of all recursively included bundles
// for the given manifest.
func (m *Manifest) GetRecursiveIncludes() []*Manifest {
	manifests := []*Manifest{}
	visited := make(map[string]bool)

	for _, inc := range m.Header.Includes {
		manifests = append(manifests, inc)
		visited[inc.Name] = true
	}

	for i := 0; i < len(manifests); i++ {
		for _, inc := range manifests[i].Header.Includes {
			if !visited[inc.Name] {
				manifests = append(manifests, inc)
				visited[inc.Name] = true
			}
		}
	}

	return manifests
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

				// Directories can be installed with inconsistent sizes, so the
				// directory size is not included in the contentsize header field.
				if f1.Type != TypeDirectory {
					m.Header.ContentSize -= uint64(f1.Info.Size())
				}
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
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return ioutil.WriteFile(path, contents, 0644)
}

// constructIndexTrackingFile writes an empty tracking file to the bundle chroot
// and to the full chroot.
func constructIndexTrackingFile(c *config, ui *UpdateInfo) error {
	imageVerPath := filepath.Join(c.imageBase, fmt.Sprint(ui.version))
	trackingFile := filepath.Join("/usr/share/clear/bundles", IndexBundle)
	// write to bundle chroot
	if err := createAndWrite(filepath.Join(imageVerPath, IndexBundle, trackingFile), []byte{}); err != nil {
		return err
	}

	// write to full chroot and return any errors
	return createAndWrite(filepath.Join(imageVerPath, "full", trackingFile), []byte{})
}

// writeIndexManifest creates a deprecated and empty index bundle.
// If the previous index isn't deprecated, create a new empty bundle, adding only the tracking file.
// If the previous index is already deprecated or inexistent return without errors because the empty index should be carried out.
func writeIndexManifest(c *config, ui *UpdateInfo, oldMoM *Manifest, bundles []*Manifest) (*Manifest, error) {
	// Get old version of IndexBundle
	ver := ui.previous
	for _, m := range oldMoM.Files {
		if m.Name == IndexBundle {
			ver = m.Version
		}
	}

	// Get old Manifest
	oldMPath := filepath.Join(c.outputDir, fmt.Sprint(ver), "Manifest."+IndexBundle)
	oldM, err := getOldManifest(oldMPath)
	if err != nil || oldM.Header.Version == 0 {
		// No old index, just ignore
		return nil, nil
	}

	if ui.minVersion != ui.version {
		numFiles := 0
		for _, f := range oldM.Files {
			if f.Status == StatusUnset {
				numFiles++
				if numFiles > 1 {
					break
				}
			}
		}

		if numFiles == 1 {
			// This is an empty index
			return nil, nil
		}
	}

	var newOsCore *Manifest
	for _, b := range bundles {
		if b.Name == "os-core" {
			// record for includes list
			newOsCore = b
		}
	}

	// now add a manifest
	idxMan := &Manifest{
		Header: ManifestHeader{
			Format:    ui.format,
			Version:   ui.version,
			Previous:  ui.previous,
			TimeStamp: ui.timeStamp,
		},
		Name: IndexBundle,
	}

	// construct the tracking files in the bundle and full chroots
	if err := constructIndexTrackingFile(c, ui); err != nil {
		return nil, err
	}

	bundleDir := filepath.Join(c.imageBase, fmt.Sprint(ui.version))
	// add files from the chroot created in constructIndex
	err = idxMan.addFilesFromChroot(filepath.Join(bundleDir, IndexBundle), "")
	if err != nil {
		return nil, err
	}

	if newOsCore != nil {
		idxMan.Header.Includes = append(idxMan.Header.Includes, newOsCore)
		idxMan.subtractManifests(newOsCore)
	}

	oldM.sortFilesName()
	_, _, _ = idxMan.linkPeersAndChange(oldM, ui.minVersion)

	// Fix version of deleted files in the case of a minVersion
	for _, f := range idxMan.Files {
		if f.Version < ui.minVersion {
			f.Version = ui.minVersion
		}
	}

	// record file count
	idxMan.Header.FileCount = uint32(len(idxMan.Files))

	// done processing, sort by version before writing
	idxMan.sortFilesVersionName()

	// there were changes at this version, so write the manifest
	manOutput := filepath.Join(c.outputDir, fmt.Sprint(ui.version), "Manifest."+IndexBundle)
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

// AppendFile appends a file to the manifest and updates the ContentSize
func (m *Manifest) AppendFile(file *File) {
	m.Files = append(m.Files, file)
	// Directories can be installed with inconsistent sizes, so the
	// directory size is not included in the contentsize header field.
	if file.Info != nil && file.Type != TypeDirectory {
		m.Header.ContentSize += uint64(file.Info.Size())
	}
}
