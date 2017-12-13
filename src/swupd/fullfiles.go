package swupd

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
)

const DebugFullfiles = false

type compressFunc func(dst io.Writer, src io.Reader) error

var fullfileCompressors = []struct {
	Name                string
	Func                compressFunc
	ExternalTarExtraArg string
}{
	{"gzip", compressGzip, ""},
	{"external-tar-bzip2", nil, "--bzip2"},
	{"external-tar-gzip", nil, "--gzip"},
	{"external-tar-xz", nil, "--xz"},
}

func CreateFullfiles(m *Manifest, chrootDir, outputDir string) error {
	done := make(map[hashval]bool)
	for _, f := range m.Files {
		if done[f.Hash] || f.Version != m.Header.Version || f.Status == statusDeleted || f.Status == statusGhosted {
			continue
		}

		input := filepath.Join(chrootDir, f.Name)
		name := f.Hash.String()
		// NOTE: to make life simpler for the client, always use .tar extension even if the
		// file could be compressed.
		output := filepath.Join(outputDir, name+".tar")

		var err error
		switch f.Type {
		case typeDirectory:
			err = createDirectoryFullfile(input, name, output)
		case typeLink:
			err = createLinkFullfile(input, name, output)
		case typeFile:
			err = createRegularFullfile(input, name, output)
		default:
			return fmt.Errorf("cannot create fullfiles: file %s is of unsupported type %q", f.Name, f.Type)
		}

		if err != nil {
			return err
		}

		done[f.Hash] = true
	}
	return nil
}

func createDirectoryFullfile(input, name, output string) error {
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

	return nil
}

func createLinkFullfile(input, name, output string) error {
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

	return nil
}

func createRegularFullfile(input, name, output string) (err error) {
	// Ensure this is a regular file.
	fi, err := os.Lstat(input)
	if err != nil {
		return fmt.Errorf("couldn't create fullfile from %s: %s", input, err)
	}
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("couldn't create fullfile from %s: manifest expected a regular file but it is not", input)
	}

	// Create the uncompressed fullfile. We keep this file open to pass to the compressors that
	// use io.Reader. External compressors don't use this file, so we don't need to care about
	// syncing it.
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

	if DebugFullfiles {
		log.Printf("DEBUG: Creating fullfile %s for regular file %s (%d bytes)", name, input, fi.Size())
		log.Printf("DEBUG: %s (%d bytes, uncompressed)", filepath.Base(output), uncompressedSize)
	}

	// Pick the best compression option (or no compression) for that specific fullfile.
	best := ""
	var bestSize int64 = uncompressedSize
	for i, c := range fullfileCompressors {
		var candidateSize int64
		candidate := fmt.Sprintf("%s.%d.%s", output, i, c.Name)

		if c.Func != nil {
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
				out.Close()
				os.RemoveAll(candidate)
				continue
			}
			candidateSize, err = out.Seek(0, io.SeekEnd)
			if err != nil {
				log.Printf("WARNING: couldn't get size of %s: %s", candidate, err)
				out.Close()
				os.RemoveAll(candidate)
				continue
			}
			out.Close()
		} else {
			err = runTarCommand(input, name, candidate, c.ExternalTarExtraArg)
			if err != nil {
				log.Printf("WARNING: couldn't compress %s using compressor %q: %s", input, c.Name, err)
				os.RemoveAll(candidate)
				continue
			}
			candidateInfo, err := os.Stat(candidate)
			if err != nil {
				log.Printf("WARNING: couldn't get size of %s: %s", candidate, err)
				os.RemoveAll(candidate)
				continue
			}
			candidateSize = candidateInfo.Size()
		}

		if candidateSize < bestSize {
			if best != "" {
				os.RemoveAll(best)
			}
			best = candidate
			bestSize = candidateSize
		} else {
			os.RemoveAll(candidate)
		}

		if DebugFullfiles {
			log.Printf("DEBUG: %s (%d bytes)", filepath.Base(candidate), candidateSize)
		}
	}

	if best != "" {
		// Failure during rename might indicate some further problems, so return error
		// instead of ignoring it (since there is an uncompressed version).
		return os.Rename(best, output)
	}
	return nil
}

func runTarCommand(input, name, output, extra string) error {
	var buf bytes.Buffer
	// We know that name is suitable to be part of the transform regexp. If this function gets
	// wider usage, we must revisit this.
	cmd := exec.Command("tar", "--create", extra, "--file="+output, "--transform=s/.*/"+name+"/", "--preserve-permissions", input)
	cmd.Stderr = &buf
	cmd.Stdout = &buf
	err := cmd.Run()
	if err != nil {
		if buf.Len() > 0 {
			return fmt.Errorf("running external command %s failed: %s\nOUTPUT:\n%s", cmd.Args, err, buf.String())
		}
		return fmt.Errorf("running external command %s failed: %s", cmd.Args, err)
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
	defer in.Close()
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

func compressGzip(dst io.Writer, src io.Reader) error {
	gw := gzip.NewWriter(dst)
	_, err := io.Copy(gw, src)
	if err != nil {
		return err
	}
	return gw.Close()
}
