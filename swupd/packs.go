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
	"archive/tar"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
)

const debugPacks = false

// PackState describes whether and how a file was packed.
type PackState int

// Files can be not packed, packed as a delta or packed as a fullfile.
const (
	NotPacked PackState = iota
	PackedDelta
	PackedFullfile
)

// PackEntry describes a file that was considered to be in a pack.
type PackEntry struct {
	File   *File
	State  PackState
	Reason string
}

// PackInfo contains detailed information about a pack written.
type PackInfo struct {
	FullfileCount uint64
	DeltaCount    uint64

	// Entries contains all the files considered for packing and details about its presence in
	// the pack.
	Entries []PackEntry

	// Warnings contains the issues found. These are not considered errors since the pack could
	// finish by working around the issue, e.g. if file not found in chroot, try to get it from
	// the fullfiles.
	Warnings []string
}

// Empty tells if the pack has contents or not. Packs without contents are not generated.
func (info PackInfo) Empty() bool {
	return info.FullfileCount == 0 && info.DeltaCount == 0
}

func (state PackState) String() string {
	switch state {
	case NotPacked:
		return "not packed"
	case PackedDelta:
		return "packed delta"
	case PackedFullfile:
		return "packed fullfile"
	}
	return "invalid"
}

// CreateAllDeltas builds all of the deltas using the full manifest from one
// version to the next. This allows better concurrency and the pack creation
// code can just worry about adding pre-existing files to packs.
func CreateAllDeltas(outputDir string, fromVersion, toVersion, numWorkers int) error {
	// Don't try to make deltas for zero packs
	if fromVersion == 0 {
		return nil
	}
	fromFile := filepath.Join(outputDir, strconv.Itoa(fromVersion), "Manifest.full")
	toFile := filepath.Join(outputDir, strconv.Itoa(toVersion), "Manifest.full")

	fromManifest, err := ParseManifestFile(fromFile)
	if err != nil {
		return err
	}
	toManifest, err := ParseManifestFile(toFile)
	if err != nil {
		return err
	}

	if fromVersion >= toVersion {
		return fmt.Errorf("fromManifest version (%d) must be smaller than toManifest version (%d)", fromVersion, toVersion)
	}

	var c config
	c, err = getConfig(filepath.Join(outputDir, ".."))
	if err != nil {
		return err
	}

	_, err = createDeltasFromManifests(&c, fromManifest, toManifest, numWorkers)
	if err != nil {
		return err
	}

	return nil
}

// WritePack writes the pack between two Manifests, or a zero pack if fromManifest is
// nil. The toManifest should always be non nil. The outputDir is used to pick deltas and
// fullfiles. If not empty, chrootDir is tried first as a fast alternative to
// decompressing the fullfiles. Multiple workers are used to parallelize delta creation.
// If number of workers is zero or less, 1 worker is used.
func WritePack(w io.Writer, fromManifest, toManifest *Manifest, outputDir, chrootDir string, numWorkers int) (info *PackInfo, err error) {
	if toManifest == nil {
		return nil, fmt.Errorf("need a valid toManifest")
	}
	if toManifest.Name == "" {
		return nil, fmt.Errorf("toManifest has no name")
	}
	toVersion := toManifest.Header.Version

	var fromVersion uint32
	var deltas []Delta
	if fromManifest != nil {
		fromVersion = fromManifest.Header.Version
		if fromVersion >= toVersion {
			return nil, fmt.Errorf("fromManifest version (%d) must be smaller than toManifest version (%d)", fromVersion, toVersion)
		}

	}

	if debugPacks {
		log.Printf("DEBUG: WritePack for bundle %s from %d to %d", toManifest.Name, fromVersion, toVersion)
	}

	if fromManifest != nil {
		// TODO: Make WritePack itself take a Config.
		var c config
		c, err = getConfig(filepath.Join(outputDir, ".."))
		if err != nil {
			return nil, err
		}

		deltas, err = findDeltas(&c, fromManifest, toManifest)
		if err != nil {
			return nil, err
		}

		if debugPacks {
			log.Printf("DEBUG: %d potential deltas to use in pack", len(deltas))
		}
	}

	if debugPacks {
		if chrootDir != "" {
			log.Printf("DEBUG: using chrootDir=%s for packing", chrootDir)
		} else {
			log.Printf("DEBUG: not using chrootDir for packing")
		}
	}

	info = &PackInfo{
		Entries: make([]PackEntry, len(toManifest.Files)),
	}

	xw, err := NewExternalWriter(w, "xz")
	if err != nil {
		return nil, err
	}
	defer func() {
		cerr := xw.Close()
		if err == nil && cerr != nil {
			info = nil
			err = cerr
		}
	}()

	tw := tar.NewWriter(xw)
	err = tw.WriteHeader(&tar.Header{
		Name:     "delta/",
		Mode:     0700,
		Typeflag: tar.TypeDir,
	})
	if err != nil {
		return nil, fmt.Errorf("write packed failed: %s", err)
	}
	err = tw.WriteHeader(&tar.Header{
		Name:     "staged/",
		Mode:     0700,
		Typeflag: tar.TypeDir,
	})
	if err != nil {
		return nil, fmt.Errorf("write packed failed: %s", err)
	}

	var fullChrootDir string
	if chrootDir != "" {
		fullChrootDir = filepath.Join(chrootDir, fmt.Sprint(toVersion), "full")
	}

	// Add all deltas that have not failed.
	hasDelta := make(map[Hashval]*Delta)
	for i := range deltas {
		d := &deltas[i]
		if d.Error != nil {
			info.Warnings = append(info.Warnings, d.Error.Error())
			continue
		}
		var fallback bool
		fallback, err = copyFromDelta(tw, d)
		if err != nil {
			// If copy from delta fails before writing to the pack, we can
			// fallback to use the fullfile later.
			if fallback {
				info.Warnings = append(info.Warnings, err.Error())
				continue
			}
			return nil, err
		}

		info.DeltaCount++
		hasDelta[d.to.Hash] = d
	}

	// TODO: In some cases we could be packing both a delta and the fullfile. Should
	// we avoid packing the delta in this case?

	done := make(map[Hashval]bool)
	for i, f := range toManifest.Files {
		entry := &info.Entries[i]
		entry.File = f

		if f.Version <= fromVersion {
			entry.Reason = "already in from manifest"
			continue
		}
		if delta, ok := hasDelta[f.Hash]; ok {
			entry.State = PackedDelta
			entry.Reason = filepath.Base(delta.Path)
			continue
		}
		if done[f.Hash] {
			entry.Reason = "hash already packed"
			continue
		}
		if f.Status == StatusDeleted {
			entry.Reason = "file deleted"
			continue
		}
		if f.Status == StatusGhosted {
			entry.Reason = "file ghosted"
			continue
		}

		done[f.Hash] = true

		entry.State = PackedFullfile
		entry.Reason = "from fullfile"
		info.FullfileCount++
		if fullChrootDir != "" {
			var fallback bool
			fallback, err = copyFromFullChrootFile(tw, fullChrootDir, f)
			if (err != nil) && fallback {
				// If copy from chroot file fails before writing to the pack, we can
				// fallback to try copying from the fullfile.
				info.Warnings = append(info.Warnings, err.Error())
				err = copyFromFullfile(tw, outputDir, f)
			} else {
				entry.Reason = "from chroot"
			}
		} else {
			err = copyFromFullfile(tw, outputDir, f)
		}
		if err != nil {
			return nil, err
		}
	}

	if debugPacks {
		log.Printf("DEBUG: pack created with %d fullfiles and %d deltas", info.FullfileCount, info.DeltaCount)
	}

	err = tw.Close()
	if err != nil {
		return nil, err
	}
	return info, nil
}

func copyFromDelta(tw *tar.Writer, delta *Delta) (fallback bool, err error) {
	f, err := os.Open(delta.Path)
	if err != nil {
		return true, err
	}
	defer func() {
		_ = f.Close()
	}()
	fi, err := f.Stat()
	if err != nil {
		return true, err
	}
	if !fi.Mode().IsRegular() {
		return true, fmt.Errorf("delta %s is not a regular file", delta.Path)
	}
	hdr, err := getHeaderFromFileInfo(fi)
	if err != nil {
		return true, err
	}
	hdr.Name = "delta/" + fi.Name()
	hdr.Typeflag = tar.TypeReg

	// After we start writing on the tar writer, we can't let the caller fallback to another
	// option anymore.

	err = tw.WriteHeader(hdr)
	if err != nil {
		return false, err
	}
	_, err = io.Copy(tw, f)
	if err != nil {
		return false, err
	}
	return false, nil
}

func copyFromFullChrootFile(tw *tar.Writer, fullChrootDir string, f *File) (fallback bool, err error) {
	realname := filepath.Join(fullChrootDir, f.Name)
	fi, err := os.Lstat(realname)
	if err != nil {
		return true, err
	}
	hdr, err := getHeaderFromFileInfo(fi)
	if err != nil {
		return true, err
	}
	hdr.Name = "staged/" + f.Hash.String()

	// TODO: Also perform this verification for copyFromFullfile?

	switch f.Type {
	case TypeDirectory:
		if !fi.IsDir() {
			return true, fmt.Errorf("couldn't use %s for packing: manifest expected a directory but it is not", realname)
		}
		hdr.Name = hdr.Name + "/"
		hdr.Typeflag = tar.TypeDir
	case TypeLink:
		if fi.Mode()&os.ModeSymlink == 0 {
			return true, fmt.Errorf("couldn't use %s for packing: manifest expected a link but it is not", realname)
		}
		var link string
		link, err = os.Readlink(realname)
		if err != nil {
			return true, fmt.Errorf("couldn't use %s for packing: %s", realname, err)
		}
		hdr.Typeflag = tar.TypeSymlink
		hdr.Linkname = link
	case TypeFile:
		if !fi.Mode().IsRegular() {
			return true, fmt.Errorf("couldn't use %s for packing: manifest expected a regular file but it is not", realname)
		}
		hdr.Typeflag = tar.TypeReg
	default:
		return true, fmt.Errorf("unsupported file %s in chroot with type %q", f.Name, f.Type)
	}

	// After we start writing on the tar writer, we can't let the caller fallback to another
	// option anymore.

	err = tw.WriteHeader(hdr)
	if err != nil {
		return false, err
	}

	if hdr.Typeflag == tar.TypeReg {
		realfile, err := os.Open(realname)
		if err != nil {
			return false, err
		}
		_, err = io.Copy(tw, realfile)
		if err != nil {
			return false, err
		}
		err = realfile.Close()
		if err != nil {
			return false, err
		}
	}
	return false, nil
}

func copyFromFullfile(tw *tar.Writer, outputDir string, f *File) (err error) {
	fullfilePath := filepath.Join(outputDir, fmt.Sprintf("%d", f.Version), "files", f.Hash.String()+".tar")
	defer func() {
		if err != nil {
			_ = os.RemoveAll(fullfilePath)
		}
	}()

	fullfile, err := os.Open(fullfilePath)
	if err != nil {
		return fmt.Errorf("failed to open fullfile for %s in version %d: %s", f.Name, f.Version, err)
	}
	defer func() {
		cerr := fullfile.Close()
		if err == nil {
			err = cerr
		}
	}()

	fullfileReader, err := NewCompressedTarReader(fullfile)
	if err != nil {
		return fmt.Errorf("failed to read fullfile %s: %s", fullfilePath, err)
	}
	defer func() {
		cerr := fullfileReader.Close()
		if err == nil {
			err = cerr
		}
	}()

	hdr, err := fullfileReader.Next()
	if err != nil {
		return fmt.Errorf("failed to read fullfile %s: %s", fullfilePath, err)
	}
	hdr.Name = "staged/" + hdr.Name
	// Sanitize Uname and Gname in case the fullfile hasn't for some reason.
	// TODO: Consider enforcing this as validation and failing.
	hdr.Uname = ""
	hdr.Gname = ""

	err = tw.WriteHeader(hdr)
	if err != nil {
		return fmt.Errorf("failed reading fullfile %s: %s", fullfilePath, err)
	}
	_, err = io.Copy(tw, fullfileReader)
	if err != nil {
		return fmt.Errorf("failed while copying fullfile %s: %s", fullfilePath, err)
	}

	// TODO: Should we really enforce this fullfile validation here?
	_, err = fullfileReader.Next()
	if err != io.EOF {
		return fmt.Errorf("invalid fullfile %s: expected EOF but got %s", fullfilePath, err)
	}

	return nil
}

// BundleToPack contains a bundle and the to/from versions to pack.
type BundleToPack struct {
	Name        string
	FromVersion uint32
	ToVersion   uint32
}

// GetPackFilename returns the filename used for a pack of a bundle from a specific version.
func GetPackFilename(name string, fromVersion uint32) string {
	return fmt.Sprintf("pack-%s-from-%d.tar", name, fromVersion)
}

// FindBundlesToPack will read two MoM manifests and return a set of bundles that must be packed
// (and their corresponding versions).
//
// Note that a MoM can contain bundles in an old version, so each bundle needs its own From/To
// version pair.
func FindBundlesToPack(from *Manifest, to *Manifest) (map[string]*BundleToPack, error) {
	if to == nil {
		return nil, fmt.Errorf("to manifest not specified")
	}

	bundles := make(map[string]*BundleToPack, len(to.Files))
	for _, b := range to.Files {
		bundles[b.Name] = &BundleToPack{b.Name, 0, b.Version}
	}

	// If this is not a zero pack, we might be able to skip some bundles.
	if from != nil {
		for _, oldBundle := range from.Files {
			bundle, ok := bundles[oldBundle.Name]
			if !ok {
				// Bundle doesn't exist in new version, no pack needed.
				continue
			}
			if bundle.ToVersion == oldBundle.Version {
				// Versions match, so no pack required.
				delete(bundles, bundle.Name)
				continue
			}
			if bundle.ToVersion < oldBundle.Version {
				return nil, fmt.Errorf("invalid bundle versions for bundle %s, check the MoMs", bundle.Name)
			}
			bundle.FromVersion = oldBundle.Version
		}
	}

	return bundles, nil
}

// CreatePack creates the pack file for a specific bundle between two versions. The pack is written
// in the TO version subdirectory of outputDir (e.g. a pack from 10 to 20 is written to "www/20").
// Empty packs will lead to not creating the pack.
// Multiple workers are used to parallelize delta creation. If number of workers is zero or
// less, 1 worker is used.
func CreatePack(name string, fromVersion, toVersion uint32, outputDir, chrootDir string, numWorkers int) (*PackInfo, error) {
	toDir := filepath.Join(outputDir, fmt.Sprint(toVersion))
	toM, err := ParseManifestFile(filepath.Join(toDir, "Manifest."+name))
	if err != nil {
		return nil, err
	}
	var fromM *Manifest
	if fromVersion > 0 {
		fromDir := filepath.Join(outputDir, fmt.Sprint(fromVersion))
		fromM, err = ParseManifestFile(filepath.Join(fromDir, "Manifest."+name))
		if err != nil {
			return nil, err
		}
	}

	packPath := filepath.Join(toDir, GetPackFilename(name, fromVersion))
	output, err := os.Create(packPath)
	if err != nil {
		return nil, err
	}
	info, err := WritePack(output, fromM, toM, outputDir, chrootDir, numWorkers)
	if err != nil {
		_ = output.Close()
		_ = os.RemoveAll(packPath)
		return nil, err
	}
	err = output.Close()
	if err != nil {
		_ = os.RemoveAll(packPath)
		return nil, err
	}

	if info.Empty() {
		// Don't bother leaving empty packs around. Not failing if remove fails since an
		// empty pack is not incorrect.
		_ = os.Remove(packPath)
	}

	return info, nil
}
