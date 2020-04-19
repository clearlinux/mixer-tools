// Copyright © 2018 Intel Corporation
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

package builder

import (
	"archive/tar"
	"bytes"
	"fmt"
	"github.com/clearlinux/mixer-tools/log"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/clearlinux/mixer-tools/swupd"
	"github.com/pkg/errors"
)

func (b *Builder) buildUpdateContent(params UpdateParameters, timer *stopWatch) error {
	var err error

	// TODO: move this to parsing configuration / parameter time.
	// TODO: should this be uint64?
	var format uint32
	format, err = parseUint32(b.State.Mix.Format)
	if err != nil {
		return errors.Errorf("invalid format")
	}

	minVersion := uint32(params.MinVersion)

	err = writeMetaFiles(filepath.Join(b.Config.Builder.ServerStateDir, "www", b.MixVer), b.State.Mix.Format, Version)
	if err != nil {
		return errors.Wrapf(err, "failed to write update metadata files")
	}

	previous, err := parseUint32(b.State.Mix.PreviousMixVer)
	if err != nil {
		return err
	}

	timer.Start("CREATE MANIFESTS")
	mom, err := swupd.CreateManifests(b.MixVerUint32, previous, minVersion, uint(format), b.Config.Builder.ServerStateDir, b.NumBundleWorkers)
	if err != nil {
		return errors.Wrapf(err, "failed to create update metadata")
	}
	log.Info(log.Mixer, "MoM version %d", mom.Header.Version)
	for _, f := range mom.Files {
		log.Info(log.Mixer, "- %-20s %d", f.Name, f.Version)
	}

	// sign the Manifest.MoM file in place based on the Mix
	// version read from builder.conf.
	if !params.SkipSigning {
		log.Info(log.Mixer, "Signing manifest.")
		err = b.signFile(filepath.Join(b.Config.Builder.ServerStateDir, "www", b.MixVer, "Manifest.MoM"))
		if err != nil {
			return err
		}
	}

	outputDir := filepath.Join(b.Config.Builder.ServerStateDir, "www")
	thisVersionDir := filepath.Join(outputDir, fmt.Sprint(b.MixVerUint32))
	log.Info(log.Mixer, "Compressing Manifest.MoM")
	momF := filepath.Join(thisVersionDir, "Manifest.MoM")
	if params.SkipSigning {
		err = createCompressedArchive(momF+".tar", momF)
	} else {
		err = createCompressedArchive(momF+".tar", momF, momF+".sig")
	}
	if err != nil {
		return err
	}

	var wg sync.WaitGroup

	wg.Add(b.NumBundleWorkers)
	bundleChan := make(chan *swupd.Manifest)
	errorChan := make(chan error, b.NumBundleWorkers)
	defer close(errorChan)

	log.Info(log.Mixer, "Compressing bundle manifests")
	compWorker := func() {
		defer wg.Done()
		for bundle := range bundleChan {
			log.Info(log.Mixer, "  %s", bundle.Name)
			f := filepath.Join(thisVersionDir, "Manifest."+bundle.Name)
			err := createCompressedArchive(f+".tar", f)
			if err != nil {
				log.Error(log.Mixer, err.Error())
				errorChan <- err
				return
			}
		}
	}

	for i := 0; i < b.NumBundleWorkers; i++ {
		go compWorker()
	}

	for _, bundle := range mom.UpdatedBundles {
		select {
		case bundleChan <- bundle:
		case err = <-errorChan:
			// break as soon as we see a failure
			break
		}
		if err != nil {
			break
		}
	}
	close(bundleChan)
	wg.Wait()

	if err != nil {
		return err
	}
	if len(errorChan) > 0 {
		return <-errorChan
	}

	// Now tar the full manifest, since it doesn't show up in the MoM
	log.Info(log.Mixer, "  full")
	f := filepath.Join(thisVersionDir, "Manifest.full")
	err = createCompressedArchive(f+".tar", f)
	if err != nil {
		return err
	}

	// TODO: Create manifest tars for Manifest.MoM and the mom.UpdatedBundles.
	timer.Stop()

	if !params.SkipFullfiles {
		timer.Start("CREATE FULLFILES")
		log.Info(log.Mixer, "Using %d workers", b.NumFullfileWorkers)
		fullfilesDir := filepath.Join(outputDir, b.MixVer, "files")
		fullChrootDir := filepath.Join(b.Config.Builder.ServerStateDir, "image", b.MixVer, "full")
		var info *swupd.FullfilesInfo
		info, err = swupd.CreateFullfiles(mom.FullManifest, fullChrootDir, fullfilesDir, b.NumFullfileWorkers, b.Config.Swupd.Compression)
		if err != nil {
			return err
		}
		// Print summary of fullfile generation.
		{
			total := info.Skipped + info.NotCompressed
			log.Info(log.Mixer, "- Already created: %d", info.Skipped)
			log.Info(log.Mixer, "- Not compressed:  %d", info.NotCompressed)
			log.Info(log.Mixer, "- Compressed")
			for k, v := range info.CompressedCounts {
				total += v
				log.Info(log.Mixer, "  - %-20s %d", k, v)
			}
			log.Info(log.Mixer, "Total fullfiles: %d", total)
		}
		timer.Stop()
	} else {
		log.Info(log.Mixer, "=> CREATE FULLFILES - skipped")
	}

	if !params.SkipPacks {
		if err = b.createZeroPack(timer, mom.Files, outputDir); err != nil {
			return err
		}
	} else {
		log.Info(log.Mixer, "=> CREATE ZERO PACKS - skipped")
	}

	return nil
}

func (b *Builder) createZeroPack(timer *stopWatch, bundles []*swupd.File, outputDir string) error {
	timer.Start("CREATE ZERO PACKS")
	log.Info(log.Mixer, "Using %d workers", b.NumDeltaWorkers)

	bundleDir := filepath.Join(b.Config.Builder.ServerStateDir, "image")
	bundleChan := make(chan *swupd.File)
	errorChan := make(chan error, b.NumDeltaWorkers)
	defer close(errorChan)

	var wg sync.WaitGroup

	// Define the worker
	zeroPackWorker := func() {
		defer wg.Done()
		for bundle := range bundleChan {
			name := bundle.Name
			version := fmt.Sprint(bundle.Version)
			packPath := filepath.Join(outputDir, version, swupd.GetPackFilename(name, 0))
			_, zErr := os.Lstat(packPath)
			if zErr == nil {
				log.Info(log.Mixer, "Zero pack %s already exists for version %s", name, version)
				continue
			}
			if !os.IsNotExist(zErr) {
				zErr = errors.Wrapf(zErr, "couldn't access existing pack file %s", packPath)
				log.Info(log.Mixer, zErr.Error())
				errorChan <- zErr
				return
			}

			log.Info(log.Mixer, "Creating zero pack %s for version %s", name, version)
			var info *swupd.PackInfo
			info, zErr = swupd.CreatePack(name, 0, bundle.Version, outputDir, bundleDir)
			if zErr != nil {
				zErr = errors.Wrapf(zErr, "couldn't make pack %s for version %s", name, version)
				log.Error(log.Mixer, zErr.Error())
				errorChan <- zErr
				return
			}
			if len(info.Warnings) > 0 {
				log.Info(log.Mixer, "Warnings during pack:")
				for _, w := range info.Warnings {
					log.Info(log.Mixer, "  %s", w)
				}
			}
			log.Info(log.Mixer, "Fullfiles in pack %s: %d", name, info.FullfileCount)
			log.Info(log.Mixer, "Deltas in pack %s: %d", name, info.DeltaCount)
		}
	}

	// Call the worker
	for i := 0; i < b.NumDeltaWorkers; i++ {
		wg.Add(1)
		go zeroPackWorker()
	}

	var err error
	// Create feed for the worker
	for _, bundle := range bundles {
		select {
		case bundleChan <- bundle:
		case err = <-errorChan:
			break
		}
		if err != nil {
			break
		}
	}
	close(bundleChan)
	wg.Wait()

	if len(errorChan) > 0 {
		err = <-errorChan
	}

	timer.Stop()
	return err
}

// createCompressedArchive will use tar and xz to create a compressed
// file. It does not stream the sources contents, doing all the work
// in memory before writing the destination file.
func createCompressedArchive(dst string, srcs ...string) error {
	err := createCompressedArchiveInternal(dst, srcs...)
	return errors.Wrapf(err, "couldn't create compressed archive %s", dst)
}

func createCompressedArchiveInternal(dst string, srcs ...string) error {
	archive := &bytes.Buffer{}
	xw, err := swupd.NewExternalWriter(archive, "xz")
	if err != nil {
		return err
	}

	err = archiveFiles(xw, srcs)
	if err != nil {
		_ = xw.Close()
		return err
	}

	err = xw.Close()
	if err != nil {
		return err
	}

	return ioutil.WriteFile(dst, archive.Bytes(), 0644)
}

func archiveFiles(w io.Writer, srcs []string) error {
	tw := tar.NewWriter(w)
	for _, src := range srcs {
		fi, err := os.Lstat(src)
		if err != nil {
			return err
		}
		if !fi.Mode().IsRegular() {
			return errors.Errorf("%s has unsupported type of file", src)
		}
		var hdr *tar.Header
		hdr, err = tar.FileInfoHeader(fi, "")
		if err != nil {
			return err
		}

		err = tw.WriteHeader(hdr)
		if err != nil {
			return err
		}
		var srcData []byte
		srcData, err = ioutil.ReadFile(src)
		if err != nil {
			return err
		}
		_, err = tw.Write(srcData)
		if err != nil {
			return err
		}
	}
	return tw.Close()
}

func (b *Builder) signFile(fileName string) error {
	sig := fileName + ".sig"
	// Call openssl because signing and pkcs7 stuff is not well supported in Go yet.
	cmd := exec.Command("openssl", "smime", "-sign", "-binary", "-in", fileName,
		"-signer", b.Config.Builder.Cert, "-inkey", filepath.Dir(b.Config.Builder.Cert)+"/private.pem",
		"-outform", "DER", "-out", sig)

	// Capture the output as it is useful in case of errors.
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if err != nil {
		log.Debug(log.Ssl, out.String())
		return fmt.Errorf("failed to sign file:\n%s", fileName)
	}
	log.Verbose(log.Ssl, out.String())
	return nil
}
