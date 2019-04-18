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
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/clearlinux/mixer-tools/swupd"
	"github.com/pkg/errors"
)

// SignManifestMoM will sign the Manifest.MoM file in in place based on the Mix
// version read from builder.conf.
func (b *Builder) SignManifestMoM() error {
	mom := filepath.Join(b.Config.Builder.ServerStateDir, "www", b.MixVer, "Manifest.MoM")
	sig := mom + ".sig"

	// Call openssl because signing and pkcs7 stuff is not well supported in Go yet.
	cmd := exec.Command("openssl", "smime", "-sign", "-binary", "-in", mom,
		"-signer", b.Config.Builder.Cert, "-inkey", filepath.Dir(b.Config.Builder.Cert)+"/private.pem",
		"-outform", "DER", "-out", sig)

	// Capture the output as it is useful in case of errors.
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to sign Manifest.MoM:\n%s", out.String())
	}
	return nil
}

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
	fmt.Printf("MoM version %d\n", mom.Header.Version)
	for _, f := range mom.Files {
		fmt.Printf("- %-20s %d\n", f.Name, f.Version)
	}

	if !params.SkipSigning {
		fmt.Println("Signing manifest.")
		err = b.SignManifestMoM()
		if err != nil {
			return err
		}
	}

	outputDir := filepath.Join(b.Config.Builder.ServerStateDir, "www")
	thisVersionDir := filepath.Join(outputDir, fmt.Sprint(b.MixVerUint32))
	fmt.Println("Compressing Manifest.MoM")
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
	// TODO: handle "too many open files" when mom.UpdatedBundles gets too
	// large. May have to limit the number of workers here.
	workers := len(mom.UpdatedBundles)
	wg.Add(workers)
	bundleChan := make(chan *swupd.Manifest)
	errorChan := make(chan error, workers)
	fmt.Println("Compressing bundle manifests")
	compWorker := func() {
		defer wg.Done()
		for bundle := range bundleChan {
			fmt.Printf("  %s\n", bundle.Name)
			f := filepath.Join(thisVersionDir, "Manifest."+bundle.Name)
			err = createCompressedArchive(f+".tar", f)
			if err != nil {
				errorChan <- err
				break
			}
		}
	}

	for i := 0; i < workers; i++ {
		go compWorker()
	}

	for _, bundle := range mom.UpdatedBundles {
		select {
		case bundleChan <- bundle:
		case err = <-errorChan:
			// break as soon as we see a failure
			break
		}
	}
	close(bundleChan)
	wg.Wait()
	if err == nil && len(errorChan) > 0 {
		err = <-errorChan
	}

	chanLen := len(errorChan)
	for i := 0; i < chanLen; i++ {
		<-errorChan
	}

	if err != nil {
		return err
	}

	// Now tar the full manifest, since it doesn't show up in the MoM
	fmt.Println("  full")
	f := filepath.Join(thisVersionDir, "Manifest.full")
	err = createCompressedArchive(f+".tar", f)
	if err != nil {
		return err
	}

	// TODO: Create manifest tars for Manifest.MoM and the mom.UpdatedBundles.
	timer.Stop()

	if !params.SkipFullfiles {
		timer.Start("CREATE FULLFILES")
		fmt.Printf("Using %d workers\n", b.NumFullfileWorkers)
		fullfilesDir := filepath.Join(outputDir, b.MixVer, "files")
		fullChrootDir := filepath.Join(b.Config.Builder.ServerStateDir, "image", b.MixVer, "full")
		var info *swupd.FullfilesInfo
		info, err = swupd.CreateFullfiles(mom.FullManifest, fullChrootDir, fullfilesDir, b.NumFullfileWorkers)
		if err != nil {
			return err
		}
		// Print summary of fullfile generation.
		{
			total := info.Skipped + info.NotCompressed
			fmt.Printf("- Already created: %d\n", info.Skipped)
			fmt.Printf("- Not compressed:  %d\n", info.NotCompressed)
			fmt.Printf("- Compressed\n")
			for k, v := range info.CompressedCounts {
				total += v
				fmt.Printf("  - %-20s %d\n", k, v)
			}
			fmt.Printf("Total fullfiles: %d\n", total)
		}
		timer.Stop()
	} else {
		fmt.Println("\n=> CREATE FULLFILES - skipped")
	}

	if !params.SkipPacks {
		timer.Start("CREATE ZERO PACKS")
		bundleDir := filepath.Join(b.Config.Builder.ServerStateDir, "image")
		for _, bundle := range mom.Files {
			if bundle.Type != swupd.TypeManifest {
				continue
			}
			// TODO: Evaluate if it's worth using goroutines.
			name := bundle.Name
			version := bundle.Version
			packPath := filepath.Join(outputDir, fmt.Sprint(version), swupd.GetPackFilename(name, 0))
			_, err = os.Lstat(packPath)
			if err == nil {
				fmt.Printf("Zero pack already exists for %s to version %d\n", name, version)
				continue
			}
			if !os.IsNotExist(err) {
				return errors.Wrapf(err, "couldn't access existing pack file %s", packPath)
			}

			fmt.Printf("Creating zero pack for %s to version %d\n", name, version)

			var info *swupd.PackInfo
			info, err = swupd.CreatePack(name, 0, version, outputDir, bundleDir, 0)
			if err != nil {
				return errors.Wrapf(err, "couldn't make pack for bundle %q", name)
			}
			if len(info.Warnings) > 0 {
				fmt.Println("Warnings during pack:")
				for _, w := range info.Warnings {
					fmt.Printf("  %s\n", w)
				}
				fmt.Println()
			}
			fmt.Printf("  Fullfiles in pack: %d\n", info.FullfileCount)
			fmt.Printf("  Deltas in pack: %d\n", info.DeltaCount)
		}
		timer.Stop()
	} else {
		fmt.Println("\n=> CREATE ZERO PACKS - skipped")
	}

	return nil
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
