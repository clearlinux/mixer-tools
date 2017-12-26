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
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
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
	// TODO: Add stats.

	// Entries contains all the files considered for packing and details about its presence in
	// the pack.
	Entries []PackEntry

	// Warnings contains the issues found. These are not considered errors since the pack could
	// finish by working around the issue, e.g. if file not found in chroot, try to get it from
	// the fullfiles.
	Warnings []string
}

// WritePack writes the pack of a given Manifest from a version to the version of the Manifest. The
// outputDir is used to pick deltas and fullfiles. If not empty, chrootDir is tried first as a fast
// alternative to decompressing the fullfiles.
func WritePack(w io.Writer, m *Manifest, fromVersion uint32, outputDir, chrootDir string) (info *PackInfo, err error) {
	if m.Name == "" {
		return nil, fmt.Errorf("manifest has no name")
	}
	if fromVersion >= m.Header.Version {
		return nil, fmt.Errorf("fromVersion (%d) smaller than toVersion (%d)", fromVersion, m.Header.Version)
	}

	if debugPacks {
		if chrootDir != "" {
			log.Printf("DEBUG: using chrootDir=%s for packing", chrootDir)
		} else {
			log.Printf("DEBUG: not using chrootDir for packing")
		}
	}

	info = &PackInfo{
		Entries: make([]PackEntry, len(m.Files)),
	}

	xw, err := newExternalWriter(w, "xz")
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

	done := make(map[Hashval]bool)
	for i, f := range m.Files {
		entry := &info.Entries[i]
		entry.File = f

		if f.Version <= fromVersion {
			entry.Reason = "already in from manifest"
			continue
		}
		if done[f.Hash] {
			entry.Reason = "hash already packed"
			continue
		}
		if f.Status == statusDeleted {
			entry.Reason = "file deleted"
			continue
		}
		if f.Status == statusGhosted {
			entry.Reason = "file ghosted"
			continue
		}

		done[f.Hash] = true

		// TODO: Pack deltas when available.
		entry.State = PackedFullfile
		entry.Reason = "from fullfile"
		if chrootDir != "" {
			var fallback bool
			fallback, err = copyFromChrootFile(tw, chrootDir, f)
			if (err != nil) && fallback {
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

	err = tw.Close()
	if err != nil {
		return nil, err
	}
	return info, nil
}

func copyFromChrootFile(tw *tar.Writer, chrootDir string, f *File) (fallback bool, err error) {
	realname := filepath.Join(chrootDir, f.Name)
	fi, err := os.Lstat(realname)
	if err != nil {
		return true, err
	}
	hdr, err := getHeaderFromFileInfo(fi)
	if err != nil {
		return true, err
	}
	hdr.Name = f.Hash.String()

	// TODO: Also perform this verification for copyFromFullfile?

	switch f.Type {
	case typeDirectory:
		if !fi.IsDir() {
			return true, fmt.Errorf("couldn't use %s for packing: manifest expected a directory but it is not", realname)
		}
		hdr.Typeflag = tar.TypeDir
	case typeLink:
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
	case typeFile:
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

type compressedTarReader struct {
	*tar.Reader
	compressionCloser io.Closer
}

func (ctr *compressedTarReader) Close() error {
	if ctr.compressionCloser != nil {
		return ctr.compressionCloser.Close()
	}
	return nil
}

// Compression algorithms have "magic" bytes in the beginning of the file to identify them.
var (
	gzipMagic  = []byte{0x1F, 0x8B}
	xzMagic    = []byte{0xFD, '7', 'z', 'X', 'Z', 0x00}
	bzip2Magic = []byte{'B', 'Z', 'h'}
)

// newCompressedTarReader creates a struct compatible with tar.Reader reading from uncompressed or
// compressed input.
func newCompressedTarReader(rs io.ReadSeeker) (*compressedTarReader, error) {
	var h [6]byte
	_, err := rs.Read(h[:])
	if err != nil {
		return nil, err
	}
	_, err = rs.Seek(0, io.SeekStart)
	if err != nil {
		return nil, err
	}
	result := &compressedTarReader{}

	switch {
	case bytes.HasPrefix(h[:], gzipMagic):
		gr, err := gzip.NewReader(rs)
		if err != nil {
			return nil, fmt.Errorf("couldn't decompress using gzip: %s", err)
		}
		result.compressionCloser = gr
		result.Reader = tar.NewReader(gr)
	case bytes.HasPrefix(h[:], xzMagic):
		xr, err := newExternalReader(rs, "unxz")
		if err != nil {
			return nil, fmt.Errorf("couldn't decompress using xz: %s", err)
		}
		result.compressionCloser = xr
		result.Reader = tar.NewReader(xr)
	case bytes.HasPrefix(h[:], bzip2Magic):
		br := bzip2.NewReader(rs)
		result.Reader = tar.NewReader(br)
	default:
		// Assume uncompressed tar and let it complain if not valid.
		result.Reader = tar.NewReader(rs)
	}
	return result, nil
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

	fullfileReader, err := newCompressedTarReader(fullfile)
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
