package main

import (
	"archive/tar"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/clearlinux/mixer-tools/swupd"
)

type clientState struct {
	dir string

	noCache     bool
	baseContent string
	isRemote    bool
}

func newClientState(stateDir, baseContent string) (*clientState, error) {
	contentFilename := filepath.Join(stateDir, "content")
	stateContent, err := ioutil.ReadFile(contentFilename)
	if err == nil {
		if string(stateContent) != baseContent {
			fmt.Printf("- Resetting state in %s previously used for %q\n", stateDir, stateContent)
			// Delete files individually in case stateDir is managed by the user.
			var fis []os.FileInfo
			fis, err = ioutil.ReadDir(stateDir)
			if err != nil {
				return nil, fmt.Errorf("couldn't reset state directory: %s", err)
			}
			for _, fi := range fis {
				err = os.RemoveAll(filepath.Join(stateDir, fi.Name()))
				if err != nil {
					return nil, fmt.Errorf("couldn't reset state directory: %s", err)
				}
			}
		}
	}
	err = os.MkdirAll(filepath.Join(stateDir, "staged/temp"), 0755)
	if err != nil {
		return nil, fmt.Errorf("couldn't create state directory: %s", err)
	}
	err = ioutil.WriteFile(contentFilename, []byte(baseContent), 0644)
	if err != nil {
		return nil, fmt.Errorf("couldn't write %s: %s", contentFilename, err)
	}

	var isRemote bool
	if strings.HasPrefix(baseContent, "https://") || strings.HasPrefix(baseContent, "http://") {
		isRemote = true
	}

	cs := &clientState{
		dir:         stateDir,
		baseContent: baseContent,
		isRemote:    isRemote,
	}

	return cs, nil
}

func (cs *clientState) OpenFile(elem ...string) (io.ReadCloser, error) {
	joined := filepath.Join(elem...)
	if !cs.isRemote {
		return os.Open(filepath.Join(cs.baseContent, joined))
	}

	u := cs.baseContent + "/" + joined
	fmt.Printf("- downloading %s\n", u)
	res, err := http.Get(u)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		_ = res.Body.Close()
		return nil, fmt.Errorf("couldn't download %q: got response with code: %d %s", u, res.StatusCode, http.StatusText(res.StatusCode))
	}
	return res.Body, nil
}

func (cs *clientState) GetFile(elem ...string) (string, error) {
	joined := filepath.Join(elem...)
	if !cs.isRemote {
		return filepath.Join(cs.baseContent, joined), nil
	}
	localFile := filepath.Join(cs.dir, joined)
	if _, err := os.Stat(localFile); err == nil {
		if !cs.noCache {
			return localFile, nil
		}
		err = os.RemoveAll(localFile)
		if err != nil {
			return "", fmt.Errorf("couldn't remove %s to redownload: %s", localFile, err)
		}
	}
	err := os.MkdirAll(filepath.Dir(localFile), 0755)
	if err != nil {
		return "", err
	}
	err = download(cs.baseContent+"/"+joined, localFile)
	if err != nil {
		return "", err
	}
	return localFile, nil
}

func (cs *clientState) Path(elem ...string) string {
	return filepath.Join(cs.dir, filepath.Join(elem...))
}

func (cs *clientState) GetBundleManifest(version, name, expectedHash string) (*swupd.Manifest, error) {
	if name == "MoM" {
		return nil, fmt.Errorf("invalid arguments to GetBundleManifest: MoM is not a bundle")
	}
	filename, err := cs.GetFile(version, "Manifest."+name)
	if err != nil {
		return nil, err
	}
	hash, err := swupd.GetHashForFile(filename)
	if err != nil {
		return nil, fmt.Errorf("couldn't calculate hash for %s: %s", filename, err)
	}
	if hash != expectedHash {
		return nil, fmt.Errorf("hash mismatch in %s got %s but expected %s", filename, hash, expectedHash)
	}
	m, err := swupd.ParseManifestFile(filename)
	if err != nil {
		return nil, fmt.Errorf("couldn't parse bundle manifest file %s: %s", filename, err)
	}
	return m, nil
}

func (cs *clientState) GetFullfile(version, hash string) error {
	tarredFilename, err := cs.GetFile(version, "files", hash+".tar")
	if err != nil {
		return err
	}
	tarred, err := os.Open(tarredFilename)
	if err != nil {
		return err
	}
	defer func() {
		_ = tarred.Close()
	}()

	tr, err := swupd.NewCompressedTarReader(tarred)
	if err != nil {
		return err
	}
	defer func() {
		_ = tr.Close()
	}()

	hdr, err := tr.Next()
	if err != nil {
		return err
	}
	err = cs.extractFullfile(hdr, tr)
	if err != nil {
		return err
	}

	hdr, err = tr.Next()
	if err == nil {
		fmt.Printf("! ignoring unexpected extra content in %s: %s\n", tarredFilename, hdr.Name)
	}

	return nil
}

func newTarXzReader(r io.Reader) (*swupd.CompressedTarReader, error) {
	result := &swupd.CompressedTarReader{}
	xr, err := swupd.NewExternalReader(r, "unxz")
	if err != nil {
		return nil, fmt.Errorf("couldn't decompress using xz: %s", err)
	}
	result.CompressionCloser = xr
	result.Reader = tar.NewReader(xr)
	return result, nil
}

func (cs *clientState) GetZeroPack(version, name string) error {
	cachedName := cs.Path(fmt.Sprintf("pack-%s-from-0-to-%s.tar", name, version))
	if !cs.noCache {
		if _, err := os.Stat(cachedName); err == nil {
			return nil
		}
	}

	pack, err := cs.OpenFile(version, fmt.Sprintf("pack-%s-from-0.tar", name))
	if err != nil {
		return err
	}
	defer func() {
		_ = pack.Close()
	}()

	// Compression time is known ahead of time, so avoid the need of Seeker interface.
	tr, err := newTarXzReader(pack)
	if err != nil {
		return err
	}
	for {
		var hdr *tar.Header
		hdr, err = tr.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("invalid pack for %s", name)
		}
		if !strings.HasPrefix(hdr.Name, "staged/") || hdr.Name == "staged/" {
			continue
		}
		err = cs.extractFullfile(hdr, tr)
		if err != nil {
			return err
		}
	}

	err = tr.Close()
	if err != nil {
		return err
	}

	return ioutil.WriteFile(cachedName, nil, 0600)
}

func download(u, path string) (err error) {
	tempPath := path + ".downloading"
	f, err := os.OpenFile(tempPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("couldn't open temporary file to write downloaded contents: %s", err)
	}
	defer func() {
		if err != nil {
			_ = os.Remove(tempPath)
		}
	}()
	fmt.Printf("- downloading %s\n", u)
	res, err := http.Get(u)
	if err != nil {
		_ = f.Close()
		return err
	}
	defer func() {
		_ = res.Body.Close()
	}()
	if res.StatusCode != http.StatusOK {
		_ = f.Close()
		return fmt.Errorf("couldn't download %q: got response with code: %d %s", u, res.StatusCode, http.StatusText(res.StatusCode))
	}
	_, err = io.Copy(f, res.Body)
	if err != nil {
		_ = f.Close()
		return fmt.Errorf("couldn't download %q: %s", u, err)
	}

	err = f.Close()
	if err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}

func (cs *clientState) extractFullfile(hdr *tar.Header, r io.Reader) error {
	basename := filepath.Base(hdr.Name)
	filename := cs.Path("staged", basename)

	// First check if file already exists.
	_, err := os.Lstat(filename)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("couldn't access existing file %s: %s", filename, err)
	}

	// File exists, check the hash.
	if err == nil {
		hash, herr := swupd.GetHashForFile(filename)
		if herr == nil && hash == basename {
			if !cs.noCache {
				// No work needed!
				return nil
			}
		} else if herr != nil {
			fmt.Printf("! couldn't calculate hash for existing file %s, removing to extract it again\n", filename)
		} else {
			fmt.Printf("! existing file %s has invalid hash %s, removing to extract it again\n", filename, hash)
		}
		err = os.Remove(filename)
		if err != nil {
			return fmt.Errorf("couldn't remove file for extracting new one: %s", err)
		}
	}

	// Write to a temporary filename.
	tempFilename := cs.Path("staged/temp", basename)

	switch hdr.Typeflag {
	case tar.TypeReg:
		mode := hdr.FileInfo().Mode()
		var f *os.File
		f, err = os.OpenFile(tempFilename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, mode)
		if err != nil {
			return fmt.Errorf("couldn't create temporary file: %s", err)
		}
		_, err = io.Copy(f, r)
		if err != nil {
			_ = f.Close()
			return fmt.Errorf("couldn't extract data to temporary file %s: %s", tempFilename, err)
		}
		err = f.Chown(hdr.Uid, hdr.Gid)
		if err != nil {
			_ = f.Close()
			return fmt.Errorf("couldn't change ownership of temporary file: %s", err)
		}
		if mode&(os.ModeSticky|os.ModeSetgid|os.ModeSetuid) != 0 {
			err = f.Chmod(mode)
			if err != nil {
				_ = f.Close()
				return fmt.Errorf("couldn't change mode of temporary file: %s", err)
			}
		}
		err = f.Close()
		if err != nil {
			return fmt.Errorf("couldn't close temporary file: %s", err)
		}

	case tar.TypeSymlink:
		err = os.Remove(tempFilename)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("couldn't remove previous temporary file: %s", err)
		}
		err = os.Symlink(hdr.Linkname, tempFilename)
		if err != nil {
			return fmt.Errorf("couldn't create temporary file: %s", err)
		}

	case tar.TypeDir:
		err = os.Remove(tempFilename)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("couldn't remove previous temporary file: %s", err)
		}
		err = os.Mkdir(tempFilename, hdr.FileInfo().Mode())
		if err != nil {
			return fmt.Errorf("couldn't create temporary file: %s", err)
		}
		err = os.Chown(tempFilename, hdr.Uid, hdr.Gid)
		if err != nil {
			return fmt.Errorf("couldn't change ownership of temporary file: %s", err)
		}
		err = os.Chmod(tempFilename, hdr.FileInfo().Mode())
		if err != nil {
			return fmt.Errorf("couldn't change mode of temporary file: %s", err)
		}

	default:
		return fmt.Errorf("unsupported type %c in fullfile %s", hdr.Typeflag, basename)
	}

	// Now validate the file.
	hash, err := swupd.GetHashForFile(tempFilename)
	if err != nil {
		return err
	}

	if hash != basename {
		return fmt.Errorf("staged file %s has invalid hash %s", filename, hash)
	}

	return os.Rename(tempFilename, filename)
}
