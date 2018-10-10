// Copyright 2018 Intel Corporation
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
)

// CompressedTarReader is a tar.Reader that also wraps the uncompression Reader.
type CompressedTarReader struct {
	*tar.Reader
	CompressionCloser io.Closer
}

// Close a CompressedTarReader. Unlike regular tar.Reader, the compression algorithm might need Close
// to be called to dispose resources (like external process execution).
func (ctr *CompressedTarReader) Close() error {
	if ctr.CompressionCloser != nil {
		return ctr.CompressionCloser.Close()
	}
	return nil
}

// Compression algorithms have "magic" bytes in the beginning of the file to identify them.
var (
	gzipMagic  = []byte{0x1F, 0x8B}
	xzMagic    = []byte{0xFD, '7', 'z', 'X', 'Z', 0x00}
	bzip2Magic = []byte{'B', 'Z', 'h'}
	// https://github.com/facebook/zstd/blob/dev/lib/zstd.h#L385
	zstdMagic = []byte{0x28, 0xB5, 0x2F, 0xFD}
)

// NewCompressedTarReader creates a struct compatible with tar.Reader reading from uncompressed or
// compressed input. Compressed input type is guessed based on the magic on the input.
func NewCompressedTarReader(rs io.ReadSeeker) (*CompressedTarReader, error) {
	var h [6]byte
	_, err := rs.Read(h[:])
	if err != nil {
		return nil, err
	}
	_, err = rs.Seek(0, io.SeekStart)
	if err != nil {
		return nil, err
	}
	result := &CompressedTarReader{}
	switch {
	case bytes.HasPrefix(h[:], gzipMagic):
		gr, err := gzip.NewReader(rs)
		if err != nil {
			return nil, fmt.Errorf("couldn't decompress using gzip: %s", err)
		}
		result.CompressionCloser = gr
		result.Reader = tar.NewReader(gr)
	case bytes.HasPrefix(h[:], xzMagic):
		xr, err := NewExternalReader(rs, "unxz")
		if err != nil {
			return nil, fmt.Errorf("couldn't decompress using xz: %s", err)
		}
		result.CompressionCloser = xr
		result.Reader = tar.NewReader(xr)
	case bytes.HasPrefix(h[:], bzip2Magic):
		br := bzip2.NewReader(rs)
		result.Reader = tar.NewReader(br)
	case bytes.HasPrefix(h[:], zstdMagic):
		zr, err := NewExternalReader(rs, "zstd", "-d")
		if err != nil {
			return nil, fmt.Errorf("couldn't decompress using zstd: %s", err)
		}
		result.CompressionCloser = zr
		result.Reader = tar.NewReader(zr)
	default:
		// Assume uncompressed tar and let it complain if not valid.
		result.Reader = tar.NewReader(rs)
	}
	return result, nil
}
