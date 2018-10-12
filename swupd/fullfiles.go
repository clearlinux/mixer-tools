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
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
)

// debugFullfiles is a flag to turn on debug statements for fullfile creation.
var debugFullfiles = false

type compressFunc func(dst io.Writer, src io.Reader) error

var fullfileCompressors = []struct {
	Name string
	Func compressFunc
}{
	{"external-bzip2", externalCompressFunc("bzip2")},
	{"external-gzip", externalCompressFunc("gzip")},
	{"external-xz", externalCompressFunc("xz")},
}

// FullfilesInfo holds statistics about a fullfile generation.
type FullfilesInfo struct {
	NotCompressed    uint
	Skipped          uint
	CompressedCounts map[string]uint
}

// CreateFullfiles creates full file compressed tars for files in chrootDir and places
// them in outputDir. It doesn't regenerate full files that already exist. If number
// of workers is zero or less, 1 worker is used.
func CreateFullfiles(m *Manifest, chrootDir, outputDir string, numWorkers int) (*FullfilesInfo, error) {
	var err error
	if _, err = os.Stat(chrootDir); err != nil {
		return nil, fmt.Errorf("couldn't access the full chroot: %s", err)
	}
	err = os.MkdirAll(outputDir, 0777)
	if err != nil {
		return nil, fmt.Errorf("couldn't create the full files directory: %s", err)
	}

	if numWorkers < 1 {
		numWorkers = 1
	}
	var wg sync.WaitGroup
	wg.Add(numWorkers)

	// Used to feed Files to the task runners.
	taskCh := make(chan *File)

	// Used by the task runners to indicate an error happened. The channel is buffered to ensure
	// that all the goroutines can send their failure and finish.
	errorCh := make(chan error, numWorkers)

	infos := make([]FullfilesInfo, numWorkers)
	for i := range infos {
		info := &infos[i]
		info.CompressedCounts = make(map[string]uint)
	}

	taskRunner := func(info *FullfilesInfo) {
		for f := range taskCh {
			var tErr error
			input := filepath.Join(chrootDir, f.Name)
			name := f.Hash.String()
			// NOTE: to make life simpler for the client, always use .tar extension even
			// if the file could be compressed.
			output := filepath.Join(outputDir, name+".tar")

			// Don't regenerate if file exists.
			if _, tErr = os.Stat(output); tErr == nil {
				info.Skipped++
				continue
			}

			switch f.Type {
			case TypeDirectory:
				tErr = createDirectoryFullfile(input, name, output, info)
			case TypeLink:
				tErr = createLinkFullfile(input, name, output, info)
			case TypeFile:
				tErr = createRegularFullfile(input, name, output, info)
			default:
				tErr = fmt.Errorf("file %s is of unsupported type %q", f.Name, f.Type)
			}

			if tErr != nil {
				errorCh <- tErr
				break
			}
		}
		wg.Done()
	}

	for i := 0; i < numWorkers; i++ {
		go taskRunner(&infos[i])
	}

	done := make(map[Hashval]bool)
	for _, f := range m.Files {
		if done[f.Hash] || f.Version != m.Header.Version || f.Status == StatusDeleted || f.Status == StatusGhosted {
			continue
		}
		done[f.Hash] = true

		select {
		case taskCh <- f:
		case err = <-errorCh:
			// Break as soon as there is a failure.
			break
		}
	}
	close(taskCh)
	wg.Wait()

	// Sending loop might finish before any goroutine could send an error back, so check for
	// error again after they are all done.
	if err == nil && len(errorCh) > 0 {
		err = <-errorCh
	}

	if err != nil {
		return nil, err
	}

	total := &FullfilesInfo{
		CompressedCounts: make(map[string]uint),
	}
	for i := range infos {
		info := &infos[i]
		total.NotCompressed += info.NotCompressed
		total.Skipped = info.Skipped
		for k, v := range info.CompressedCounts {
			total.CompressedCounts[k] += v
		}
	}

	return total, nil
}

func createDirectoryFullfile(input, name, output string, info *FullfilesInfo) error {
	fi, err := os.Lstat(input)
	if err != nil {
		return fmt.Errorf("couldn't create fullfile from %s: %s", input, err)
	}
	if !fi.IsDir() {
		return fmt.Errorf("couldn't create fullfile from %s: manifest expected a directory but it is not", input)
	}

	hdr, err := getHeaderFromFileInfo(fi)
	if err != nil {
		return fmt.Errorf("couldn't create fullfile from %s: %s", input, err)
	}
	hdr.Name = name
	hdr.Typeflag = tar.TypeDir

	err = createTarGzipHeaderOnly(output, hdr)
	if err != nil {
		return fmt.Errorf("couldn't create fullfile from %s: %s", input, err)
	}
	info.CompressedCounts["gzip"]++

	return nil
}

func createLinkFullfile(input, name, output string, info *FullfilesInfo) error {
	fi, err := os.Lstat(input)
	if err != nil {
		return fmt.Errorf("couldn't create fullfile from %s: %s", input, err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("couldn't create fullfile from %s: manifest expected a link but it is not", input)
	}
	link, err := os.Readlink(input)
	if err != nil {
		return fmt.Errorf("couldn't create fullfile from %s: %s", input, err)
	}

	hdr, err := getHeaderFromFileInfo(fi)
	if err != nil {
		return fmt.Errorf("couldn't create fullfile from %s: %s", input, err)
	}
	hdr.Name = name
	hdr.Typeflag = tar.TypeSymlink
	hdr.Linkname = link

	err = createTarGzipHeaderOnly(output, hdr)
	if err != nil {
		return fmt.Errorf("couldn't create fullfile from %s: %s", input, err)
	}
	info.CompressedCounts["gzip"]++

	return nil
}

func createRegularFullfile(input, name, output string, info *FullfilesInfo) (err error) {
	// Ensure this is a regular file.
	fi, err := os.Lstat(input)
	if err != nil {
		return fmt.Errorf("couldn't create fullfile from %s: %s", input, err)
	}
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("couldn't create fullfile from %s: manifest expected a regular file but it is not", input)
	}

	// Create the uncompressed fullfile. We keep this file open to pass to the compressors that
	// use io.Reader.
	uncompressed, err := os.Create(output)
	if err != nil {
		return fmt.Errorf("couldn't create uncompressed fullfile: %s", err)
	}
	defer func() {
		cerr := uncompressed.Close()
		if err == nil {
			err = cerr
		}
	}()
	err = tarRegularFullfile(uncompressed, input, name, fi)
	if err != nil {
		return fmt.Errorf("couldn't archive the file %s: %s", input, err)
	}
	uncompressedSize, err := uncompressed.Seek(0, io.SeekEnd)
	if err != nil {
		return fmt.Errorf("couldn't find the size of uncompressed fullfile: %s", err)
	}

	if debugFullfiles {
		log.Printf("DEBUG: Creating fullfile %s for regular file %s (%d bytes)", name, input, fi.Size())
		log.Printf("DEBUG: %s (%d bytes, uncompressed)", filepath.Base(output), uncompressedSize)
	}

	// Pick the best compression option (or no compression) for that specific fullfile.
	best := ""
	bestSize := uncompressedSize
	for i, c := range fullfileCompressors {
		var candidateSize int64
		candidate := fmt.Sprintf("%s.%d.%s", output, i, c.Name)

		_, err := uncompressed.Seek(0, io.SeekStart)
		if err != nil {
			return fmt.Errorf("couldn't seek in fullfile %s: %s", input, err)
		}
		out, err := os.Create(candidate)
		if err != nil {
			log.Printf("WARNING: couldn't create output file for %q compressor: %s", c.Name, err)
			continue
		}
		err = c.Func(out, uncompressed)
		if err != nil {
			log.Printf("WARNING: couldn't compress %s using compressor %q: %s", input, c.Name, err)
			_ = out.Close()
			_ = os.RemoveAll(candidate)
			continue
		}
		candidateSize, err = out.Seek(0, io.SeekEnd)
		if err != nil {
			log.Printf("WARNING: couldn't get size of %s: %s", candidate, err)
			_ = out.Close()
			_ = os.RemoveAll(candidate)
			continue
		}
		_ = out.Close()

		if candidateSize < bestSize {
			if best != "" {
				_ = os.RemoveAll(best)
			}
			best = candidate
			bestSize = candidateSize
		} else {
			_ = os.RemoveAll(candidate)
		}

		if debugFullfiles {
			log.Printf("DEBUG: %s (%d bytes)", filepath.Base(candidate), candidateSize)
		}
	}

	var bestName string
	if best != "" {
		bestName = filepath.Ext(best)[1:]
		info.CompressedCounts[bestName]++
	} else {
		bestName = "<uncompressed>"
		info.NotCompressed++
	}

	if debugFullfiles {
		log.Printf("DEBUG: best algorithm was %s", bestName)
	}

	if best != "" {
		// Failure during rename might indicate some further problems, so return error
		// instead of ignoring it (since there is an uncompressed version).
		return os.Rename(best, output)
	}
	return nil
}

func tarRegularFullfile(w io.Writer, input, name string, fi os.FileInfo) error {
	tw := tar.NewWriter(w)
	hdr, err := getHeaderFromFileInfo(fi)
	if err != nil {
		return err
	}
	hdr.Typeflag = tar.TypeReg
	hdr.Name = name
	err = tw.WriteHeader(hdr)
	if err != nil {
		return err
	}
	in, err := os.Open(input)
	if err != nil {
		return err
	}
	defer func() {
		cerr := in.Close()
		if err == nil {
			err = cerr
		}
	}()

	_, err = io.Copy(tw, in)
	if err != nil {
		return err
	}
	return tw.Close()
}

func createTarGzipHeaderOnly(output string, hdr *tar.Header) error {
	out, err := os.Create(output)
	if err != nil {
		return err
	}
	defer func() {
		cerr := out.Close()
		if err == nil {
			err = cerr
		}
	}()
	gw := gzip.NewWriter(out)
	tw := tar.NewWriter(gw)
	err = tw.WriteHeader(hdr)
	if err != nil {
		return err
	}
	err = tw.Close()
	if err != nil {
		return err
	}
	err = gw.Close()
	if err != nil {
		return err
	}
	return nil
}

func getHeaderFromFileInfo(fi os.FileInfo) (*tar.Header, error) {
	// TODO: FileInfoHeader gets as much as it can. Change to explicitly pick only the metadata
	// we care about.
	return tar.FileInfoHeader(fi, "")
}

func externalCompressFunc(program string, args ...string) compressFunc {
	return func(dst io.Writer, src io.Reader) error {
		w, err := NewExternalWriter(dst, program, args...)
		if err != nil {
			return err
		}
		_, err = io.Copy(w, src)
		if err != nil {
			return err
		}
		return w.Close()
	}
}
