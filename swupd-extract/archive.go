package main

import (
	"archive/tar"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"fmt"
	"io"

	"github.com/clearlinux/mixer-tools/swupd"
)

// TODO: Evaluate where to put this, partially duplicate with code in packs.

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
		return newTarXzReader(rs)
	case bytes.HasPrefix(h[:], bzip2Magic):
		br := bzip2.NewReader(rs)
		result.Reader = tar.NewReader(br)
	default:
		// Assume uncompressed tar and let it complain if not valid.
		result.Reader = tar.NewReader(rs)
	}
	return result, nil
}

func newTarXzReader(r io.Reader) (*compressedTarReader, error) {
	result := &compressedTarReader{}
	xr, err := swupd.NewExternalReader(r, "unxz")
	if err != nil {
		return nil, fmt.Errorf("couldn't decompress using xz: %s", err)
	}
	result.compressionCloser = xr
	result.Reader = tar.NewReader(xr)
	return result, nil
}
