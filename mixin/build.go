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

package main

import (
	"crypto/rsa"
	"crypto/x509"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/clearlinux/mixer-tools/builder"
	"github.com/clearlinux/mixer-tools/helpers"
	"github.com/clearlinux/mixer-tools/swupd"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build a mix from local or remote mix content",
	Long: `Build a mix from local or remote mix content existing/configured under
/usr/share/mix. The output of this command can be used by swupd via the
'swupd update --migrate' command.`,
	Run: runBuildMix,
}

func init() {
	RootCmd.AddCommand(buildCmd)
}

func runBuildMix(cmd *cobra.Command, args []string) {
	err := buildMix(true)
	if err != nil {
		fail(err)
	}
	fmt.Println("Successfully built mix content")
}

func buildBundles(b *builder.Builder) error {
	var privkey *rsa.PrivateKey
	var template *x509.Certificate

	if _, err := os.Stat(b.Config.Builder.Cert); os.IsNotExist(err) {
		fmt.Println("Generating certificate for signature validation...")
		privkey, err = helpers.CreateKeyPair()
		if err != nil {
			return errors.Wrap(err, "Error generating OpenSSL keypair")
		}
		template = helpers.CreateCertTemplate()
	}

	return errors.Wrap(b.BuildBundles(template, privkey, false), "Error building bundles")
}

func mergeMoMs(mixWS string, mixVer, lastVer int) error {
	upstreamMoM, err := swupd.ParseManifestFile(filepath.Join(mixWS, "Manifest.MoM"))
	if err != nil {
		return err
	}

	mixerMoM, err := swupd.ParseManifestFile(
		filepath.Join(mixWS, fmt.Sprintf("update/www/%d/Manifest.MoM", mixVer)))
	if err != nil {
		return err
	}

	// add mixerMoM filecount minus os-core
	upstreamMoM.Header.FileCount += mixerMoM.Header.FileCount - 1
	// need to set previous version, without this mixer version is the currently built version
	if lastVer == 0 {
		upstreamMoM.Header.Previous = upstreamMoM.Header.Version
	} else {
		upstreamMoM.Header.Previous = uint32(mixVer - 10)
	}
	// format is 1 until auto-format-bump support
	upstreamMoM.Header.Format = 1
	// system now on mixVer instead of upstream version
	upstreamMoM.Header.Version = uint32(mixVer)
	// remove os-core entry from upstreamMoM, we will replace with ours
	excludeName(upstreamMoM, "os-core")
	for i := range mixerMoM.Files {
		if mixerMoM.Files[i].Name == "os-core-update-index" {
			continue
		}
		mixerMoM.Files[i].Rename = swupd.MixManifest
		upstreamMoM.Files = append(upstreamMoM.Files, mixerMoM.Files[i])
	}

	return upstreamMoM.WriteManifestFile("Manifest.MoM")
}

func buildMix(prepNeeded bool) error {
	var err error
	lastVer := getLastVersion()
	mixFlagFile := filepath.Join(mixWS, ".valid-mix")
	ver, err := getCurrentVersion()
	if err != nil {
		_ = os.Remove(mixFlagFile)
		return err
	}
	mixVer := ver * 1000
	oldMix := filepath.Join(mixWS, fmt.Sprintf("update/www/%d", mixVer-10))
	b, err := builder.NewFromConfig(filepath.Join(mixWS, "builder.conf"))
	if err != nil {
		_ = os.Remove(mixFlagFile)
		return err
	}
	b.NumBundleWorkers = runtime.NumCPU()
	b.NumFullfileWorkers = runtime.NumCPU()

	err = os.Chdir(mixWS)
	if err != nil {
		_ = os.Remove(mixFlagFile)
		return err
	}

	if prepNeeded {
		rpms, err := helpers.ListVisibleFiles(b.Config.Mixer.LocalRPMDir)
		if err != nil {
			_ = os.Remove(mixFlagFile)
			return err
		}

		err = b.AddRPMList(rpms)
		if err != nil {
			_ = os.Remove(mixFlagFile)
			return err
		}
	}

	if lastVer != 0 {
		// older version mix exists, make the mix clean (pre-merge) before building
		_ = os.Rename(filepath.Join(oldMix, "Manifest.MoM"),
			filepath.Join(oldMix, "FullManifest.MoM"))
		_ = os.Rename(filepath.Join(oldMix, fmt.Sprintf("Manifest.MoM.%d", mixVer-10)),
			filepath.Join(oldMix, "Manifest.MoM"))
	}
	err = buildBundles(b)
	if err != nil {
		_ = os.Remove(mixFlagFile)
		return err
	}
	err = b.BuildUpdate("", 0, "", false, true, false)
	if err != nil {
		_ = os.Remove(mixFlagFile)
		return err
	}
	if lastVer != 0 {
		_ = os.Rename(filepath.Join(oldMix, "Manifest.MoM"),
			filepath.Join(oldMix, fmt.Sprintf("Manifest.MoM.%d", mixVer-10)))
		_ = os.Rename(filepath.Join(oldMix, "FullManifest.MoM"),
			filepath.Join(oldMix, "Manifest.MoM"))
	}

	upstreamMoM := fmt.Sprintf("https://download.clearlinux.org/update/%d/Manifest.MoM", ver)
	err = helpers.Download("Manifest.MoM", upstreamMoM)
	if err != nil {
		_ = os.Remove(mixFlagFile)
		return err
	}
	err = helpers.Download("Manifest.MoM.sig", upstreamMoM+".sig")
	if err != nil {
		_ = os.Remove(mixFlagFile)
		return err
	}

	cert := "/usr/share/ca-certs/Swupd_Root.pem"

	err = helpers.RunCommandSilent("openssl", "smime", "-verify", "-in", "Manifest.MoM.sig",
		"-inform", "der", "-content", "Manifest.MoM", "-CAfile", cert)
	if err != nil {
		_ = os.Remove(mixFlagFile)
		return err
	}
	fmt.Println("* Verified upstream Manifest.MoM")

	// merge upstream MoM with mixer MoM
	err = mergeMoMs(mixWS, mixVer, ver)
	if err != nil {
		_ = os.Remove(mixFlagFile)
		return err
	}

	mixDir := filepath.Join(mixWS, fmt.Sprintf("update/www/%d", mixVer))
	err = helpers.RunCommandSilent("openssl", "smime", "-sign", "-binary", "-in",
		filepath.Join(mixDir, "Manifest.MoM"),
		"-signer", filepath.Join(mixWS, "Swupd_Root.pem"),
		"-inkey", filepath.Join(mixWS, "private.pem"),
		"-outform", "DER",
		"-out", filepath.Join(mixDir, "Manifest.MoM.sig"))
	if err != nil {
		_ = os.Remove(mixFlagFile)
		return err
	}

	err = os.Rename(filepath.Join(mixDir, "Manifest.MoM"),
		filepath.Join(mixDir, fmt.Sprintf("Manifest.MoM.%d", mixVer)))
	if err != nil {
		_ = os.Remove(mixFlagFile)
		return err
	}
	err = os.Rename("Manifest.MoM", filepath.Join(mixDir, "Manifest.MoM"))
	if err != nil {
		_ = os.Remove(mixFlagFile)
		return err
	}
	err = helpers.RunCommandSilent("tar", "-C", mixDir, "-cvf", "Manifest.MoM.tar", "Manifest.MoM")
	if err != nil {
		_ = os.Remove(mixFlagFile)
		return err
	}

	// write a file that says this mix is ready to be consumed
	_, err = os.OpenFile(mixFlagFile, os.O_RDONLY|os.O_CREATE, 0666)
	if err != nil {
		return err
	}
	return os.Remove("Manifest.MoM.sig")
}
