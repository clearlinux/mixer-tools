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
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/clearlinux/mixer-tools/builder"
	"github.com/clearlinux/mixer-tools/helpers"
	"github.com/clearlinux/mixer-tools/swupd"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

// Default number of RPM download retries
const retriesDefault = 3

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
	return errors.Wrap(b.BuildBundles(template, privkey, false, retriesDefault), "Error building bundles")
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
	for i := range mixerMoM.Files {
		if mixerMoM.Files[i].Name == "os-core-update-index" {
			continue
		}
		// remove overlapping bundle names including os-core
		excludeName(upstreamMoM, mixerMoM.Files[i].Name)
		mixerMoM.Files[i].Misc = swupd.MiscMixManifest
		upstreamMoM.Files = append(upstreamMoM.Files, mixerMoM.Files[i])
	}

	return upstreamMoM.WriteManifestFile("Manifest.MoM")
}

func incrementMixVerIfNeeded(mixVer int, mixFlagFile string) int {
	out, err := ioutil.ReadFile(mixFlagFile)
	if err != nil {
		return mixVer
	}

	ver, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return mixVer
	}

	return ver + 10
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
	mixVer := incrementMixVerIfNeeded(ver*1000, mixFlagFile)

	err = os.Chdir(mixWS)
	if err != nil {
		_ = os.Remove(mixFlagFile)
		return err
	}

	b, err := builder.NewFromConfig(filepath.Join(mixWS, "builder.conf"))
	if err != nil {
		_ = os.Remove(mixFlagFile)
		return err
	}
	b.NumBundleWorkers = runtime.NumCPU()
	b.NumFullfileWorkers = runtime.NumCPU()
	b.UpstreamVer = fmt.Sprint(ver)
	b.MixVer = fmt.Sprint(mixVer)
	b.MixVerUint32 = uint32(mixVer)

	// Mixin does not use or update the PREVIOUS_MIX_VERSION field in
	// mixer.state. The previous mix version is overridden by the last
	// version to maintain consistent behavior for Mixin manifest
	// generation.
	b.State.Mix.PreviousMixVer = strconv.Itoa(lastVer)

	if prepNeeded {
		var rpms []string
		rpms, err = helpers.ListVisibleFiles(b.Config.Mixer.LocalRPMDir)
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

	var oldMix string
	if lastVer != 0 {
		oldMix = filepath.Join(mixWS, "update/www", fmt.Sprint(lastVer))
		// older version mix exists, make the mix clean (pre-merge)
		// before building
		_ = os.Rename(filepath.Join(oldMix, "Manifest.MoM"),
			filepath.Join(oldMix, "FullManifest.MoM"))
		_ = os.Rename(filepath.Join(oldMix, fmt.Sprintf("Manifest.MoM.%d", lastVer)),
			filepath.Join(oldMix, "Manifest.MoM"))
	}
	err = buildBundles(b)
	if err != nil {
		_ = os.Remove(mixFlagFile)
		return err
	}
	err = b.BuildUpdate(builder.UpdateParameters{Publish: true})
	if err != nil {
		_ = os.Remove(mixFlagFile)
		return err
	}
	if oldMix != "" {
		_ = os.Rename(filepath.Join(oldMix, "Manifest.MoM"),
			filepath.Join(oldMix, fmt.Sprintf("Manifest.MoM.%d", lastVer)))
		_ = os.Rename(filepath.Join(oldMix, "FullManifest.MoM"),
			filepath.Join(oldMix, "Manifest.MoM"))
	}

	upstreamMoM := fmt.Sprintf("https://cdn.download.clearlinux.org/update/%d/Manifest.MoM", ver)
	err = helpers.DownloadFile(upstreamMoM, "Manifest.MoM")
	if err != nil {
		_ = os.Remove(mixFlagFile)
		return err
	}
	err = helpers.DownloadFile(upstreamMoM+".sig", "Manifest.MoM.sig")
	if err != nil {
		_ = os.Remove(mixFlagFile)
		return err
	}

	cert := "/usr/share/ca-certs/Swupd_Root.pem"

	err = helpers.RunCommandSilent("openssl", "smime", "-verify", "-in", "Manifest.MoM.sig",
		"-inform", "der", "-content", "Manifest.MoM", "-purpose", "any", "-CAfile", cert)
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
	err = ioutil.WriteFile(mixFlagFile, []byte(fmt.Sprint(mixVer)), 0666)
	if err != nil {
		return err
	}
	return os.Remove("Manifest.MoM.sig")
}
