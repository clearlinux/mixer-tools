// Copyright © 2017 Intel Corporation
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

package helpers

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/big"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/pkg/errors"
)

// CreateCertTemplate will construct the template for needed openssl metadata
// instead of using an attributes.cnf file
func CreateCertTemplate() *x509.Certificate {
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialnumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		log.Println(errors.Wrap(err, "ERROR: Failed to generate serial number"))
	}

	template := x509.Certificate{
		SerialNumber:          serialnumber,
		Subject:               pkix.Name{Organization: []string{"Mixer"}},
		SignatureAlgorithm:    x509.SHA256WithRSA,
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(1, 0, 0),
		BasicConstraintsValid: true,
		IsCA:                  false, // This could be true since we are self signed, but set false for correctness
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature | x509.KeyUsageCRLSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageCodeSigning},
	}

	return &template
}

// CreateKeyPair constructs an RSA keypair in memory
func CreateKeyPair() (*rsa.PrivateKey, error) {
	rootKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		log.Println(errors.Wrap(err, "ERROR: Failed to generate random key"))
	}
	return rootKey, nil
}

// GenerateCertificate will create the private signing key and public
// certificate for clients to use and writes them to disk
func GenerateCertificate(cert string, template, parent *x509.Certificate, pubkey interface{}, privkey interface{}) error {
	if _, err := os.Stat(cert); os.IsNotExist(err) {
		der, err := x509.CreateCertificate(rand.Reader, template, parent, pubkey, privkey)
		if err != nil {
			return err
		}

		// Write the public certficiate out for clients to use
		certOut, err := os.Create(cert)
		if err != nil {
			return err
		}
		err = pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: der})
		if err != nil {
			return err
		}
		err = certOut.Close()
		if err != nil {
			return err
		}

		// Write the private signing key out
		keyOut, err := os.OpenFile(filepath.Dir(cert)+"/private.pem", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
		if err != nil {
			return err
		}

		defer func() {
			_ = keyOut.Close()
		}()

		// Need type assertion for Marshal to work
		priv := privkey.(*rsa.PrivateKey)
		err = pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
		if err != nil {
			return err
		}
	}
	return nil
}

// ReadFileAndSplit tokenizes the given file and converts in into a slice split
// by the newline character.
func ReadFileAndSplit(filename string) ([]string, error) {
	builder, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	data := string(builder)
	lines := strings.Split(data, "\n")

	return lines, nil
}

// UnpackFile unpacks a .tar or .tar.gz/.tgz file to a given directory.
// Should be roughly equivalent to "tar -x[z]f file -C dest". Does not
// overwrite; returns error if file being unpacked already exists.
func UnpackFile(file string, dest string) error {
	fr, err := os.Open(file)
	if err != nil {
		return err
	}
	defer func() {
		_ = fr.Close()
	}()

	var tr *tar.Reader

	// If it's a compressed tarball
	if strings.HasSuffix(file, ".tar.gz") || strings.HasSuffix(file, ".tgz") {
		gzr, err := gzip.NewReader(fr)
		if err != nil {
			return errors.Wrapf(err, "Error decompressing tarball: %s", file)
		}
		defer func() {
			_ = gzr.Close()
		}()
		tr = tar.NewReader(gzr)
	} else {
		tr = tar.NewReader(fr)
	}

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			// end of archive
			break
		} else if err != nil {
			return errors.Wrapf(err, "Error reading contents of tarball: %s", file)
		}

		out := filepath.Join(dest, hdr.Name)

		switch hdr.Typeflag {
		// Skip GitHub generated "extended header" file
		case tar.TypeXGlobalHeader:
			continue
		case tar.TypeDir:
			if err = os.MkdirAll(out, os.FileMode(hdr.Mode)); err != nil {
				return errors.Wrapf(err, "Error unpacking directory: %s", out)
			}
		case tar.TypeReg:
			of, err := os.OpenFile(out, os.O_CREATE|os.O_RDWR|os.O_EXCL, os.FileMode(hdr.Mode))
			if err != nil {
				return errors.Wrapf(err, "Error unpacking file: %s", out)
			}

			_, err = io.Copy(of, tr)
			_ = of.Close()
			if err != nil {
				return errors.Wrapf(err, "Error unpacking file: %s", out)
			}
		default:
			return errors.Errorf("Error unpacking file: %s", out)
		}
	}
	return nil
}

// CopyFile copies a file, overwriting the destination if it exists.
func CopyFile(dest, src string) error {
	return copyFileWithFlags(dest, src, os.O_RDWR|os.O_CREATE|os.O_TRUNC, true, true, false)
}

// CopyFileNoOverwrite copies a file only if the destination file does not exist.
func CopyFileNoOverwrite(dest, src string) error {
	return copyFileWithFlags(dest, src, os.O_RDWR|os.O_CREATE|os.O_EXCL, true, true, false)
}

// CopyFileWithOptions copies a file, overwriting the destination if it exist and allows
// options to be set for following links, syncing to disk, or preserving file permissions.
func CopyFileWithOptions(dest, src string, resolveLinks, sync, useSrcPerms bool) error {
	return copyFileWithFlags(dest, src, os.O_RDWR|os.O_CREATE|os.O_TRUNC, resolveLinks, sync, useSrcPerms)
}

// copyFileWithFlags General purpose copy file function
func copyFileWithFlags(dest, src string, flags int, resolveLinks, sync, useSrcPerms bool) error {
	srcInfo, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if !resolveLinks && (srcInfo.Mode()&os.ModeSymlink) == os.ModeSymlink {
		srcLink, err := os.Readlink(src)
		if err != nil {
			return err
		}
		return os.Symlink(srcLink, dest)
	}

	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() {
		_ = source.Close()
	}()

	var perms os.FileMode
	if useSrcPerms {
		perms = srcInfo.Mode()
	} else {
		perms = 0666
	}

	destination, err := os.OpenFile(dest, flags, perms)
	if err != nil {
		return err
	}
	defer func() {
		_ = destination.Close()
	}()

	_, err = io.Copy(destination, source)
	if err != nil {
		return err
	}

	if sync {
		err = destination.Sync()
		if err != nil {
			return err
		}
	}

	return nil
}

// Git runs git with arguments and returns in case of failure.
// IMPORTANT: the 'args' passed to this function _must_ be validated,
// as to avoid cases where input is received from a third party source.
// Such inputs could be something the likes of 'status; rm -rf .*'
// and need to be escaped or avoided properly.
func Git(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		return errors.Wrapf(err, "ERROR: failed to run: git %s", strings.Join(args, " "))
	}
	return nil
}

// RunCommand runs the given command with args and prints output
func RunCommand(cmdname string, args ...string) error {
	return RunCommandInput(nil, cmdname, args...)
}

// RunCommandInput runs the given command with args and input from an io.Reader,
// and prints output
func RunCommandInput(in io.Reader, cmdname string, args ...string) error {
	cmd := exec.Command(cmdname, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = in
	err := cmd.Run()
	if err != nil {
		return errors.Wrapf(err, "failed to execute %s", strings.Join(cmd.Args, " "))
	}

	return nil
}

// RunCommandSilent runs the given command with args and does not print output
func RunCommandSilent(cmdname string, args ...string) error {
	_, err := RunCommandOutput(cmdname, args...)
	return err
}

// RunCommandTimeout runs the given command with timeout + args and does not print command output
func RunCommandTimeout(timeout int, cmdname string, args ...string) error {
	ctx := context.Background()
	// 0 means infinite timeout, ONLY set timeouts when value is > 0
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, cmdname, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	err := cmd.Run()

	if ctx.Err() == context.DeadlineExceeded {
		return errors.Errorf("Command: %s timed out\n", cmdname)
	}

	return err
}

// RunCommandOutput executes the command with arguments and stores its output in
// memory. If the command succeeds returns that output, if it fails, return err that
// contains both the out and err streams from the execution.
func RunCommandOutput(cmdname string, args ...string) (*bytes.Buffer, error) {
	return RunCommandOutputEnv(cmdname, args, []string{})
}

// RunCommandOutputEnv executes the command with arguments and environment and stores
// its output in memory. If the command succeeds returns that output, if it fails,
// return err that contains both the out and err streams from the execution.
func RunCommandOutputEnv(cmdname string, args []string, envs []string) (*bytes.Buffer, error) {
	cmd := exec.Command(cmdname, args...)
	var outBuf bytes.Buffer
	var errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	cmd.Env = append(os.Environ(), envs...)
	runError := cmd.Run()

	if runError != nil {
		var buf bytes.Buffer
		logger := log.New(&buf, "", log.Ldate|log.Ltime)
		logger.Printf("failed to execute %s", strings.Join(cmd.Args, " "))
		if outBuf.Len() > 0 {
			logger.Printf("\nSTDOUT:\n%s", outBuf.Bytes())
		}
		if errBuf.Len() > 0 {
			logger.Printf("\nSTDERR:\n%s", errBuf.Bytes())
		}
		if outBuf.Len() > 0 || errBuf.Len() > 0 {
			// Finish without a newline to wrap well with the err.
			logger.Printf("failed to execute")
		}
		return &outBuf, errors.Wrap(runError, buf.String())
	}
	return &outBuf, nil
}

// ListVisibleFiles reads the directory named by dirname and returns a sorted list
// of names
func ListVisibleFiles(dirname string) ([]string, error) {
	f, err := os.Open(dirname)
	if err != nil {
		return nil, err
	}

	list, err := f.Readdirnames(-1)
	_ = f.Close()
	if err != nil && err != io.EOF {
		return nil, err
	}
	filtered := make([]string, 0, len(list))
	for i := range list {
		if list[i][0] != '.' {
			filtered = append(filtered, list[i])
		}
	}
	sort.Strings(filtered)
	return filtered, nil
}

func getDownloadFileReader(url string) (*io.ReadCloser, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("got status %q when downloading: %s", resp.Status, url)
	}

	return &resp.Body, nil
}

// DownloadFileAsString will download a file from the passed URL and return the
// result as a string.
func DownloadFileAsString(url string) (string, error) {
	fr, err := getDownloadFileReader(url)
	if err != nil {
		return "", err
	}

	defer func() {
		_ = (*fr).Close()
	}()

	content, err := ioutil.ReadAll(*fr)
	if err != nil {
		return "", err
	}

	return string(content), nil
}

// DownloadFile will download a file from the passed URL and write that file to
// the supplied file path. If the path is left empty, the file name will be
// inferred from the source and written to PWD.
func DownloadFile(url string, filePath string) error {
	fr, err := getDownloadFileReader(url)
	if err != nil {
		return errors.Wrap(err, "Failed to download file")
	}
	defer func() {
		_ = (*fr).Close()
	}()

	// If no filePath, infer from url
	if filePath == "" {
		_, filePath = filepath.Split(url)
	}

	out, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer func() {
		_ = out.Close()
	}()

	_, err = io.Copy(out, *fr)
	if err != nil {
		if rmErr := os.RemoveAll(filePath); rmErr != nil {
			return errors.Wrap(err, rmErr.Error())
		}
		return err
	}

	return nil
}
