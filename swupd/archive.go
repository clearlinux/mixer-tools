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
	default:
		// Assume uncompressed tar and let it complain if not valid.
		result.Reader = tar.NewReader(rs)
	}
	return result, nil
}
