// Copyright © 2019 Intel Corporation
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

package cmd

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/clearlinux/mixer-tools/swupd"

	"github.com/clearlinux/mixer-tools/builder"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var resetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Revert mix to a previous version",
	Long: `Revert the mix to a previous version.
Reverts a mix state to the end of the last build 
or to the end of a given version build if one is provided. 
By default, the value of PREVIOUS_MIX_VERSION 
in mixer.state will be used to define the last build. 
This command can be used to roll back the mixer state 
in case of a build failure or in case the user 
wants to roll back to a previous version.`,
	RunE: runReset,
}

type resetFlags struct {
	toVersion int32
	clean     bool
}

var resetCmdFlags resetFlags

func init() {
	RootCmd.AddCommand(resetCmd)

	resetCmd.Flags().Int32Var(&resetCmdFlags.toVersion, "to", -1, "Reset to a specific mix version, default = PREVIOUS_MIX_VERSION")
	resetCmd.Flags().BoolVar(&resetCmdFlags.clean, "clean", false, "Deletes all files with versions bigger than the one provided")
}

func runReset(cmd *cobra.Command, args []string) error {
	if err := checkRoot(); err != nil {
		fail(err)
	}

	b, err := builder.NewFromConfig(configFile)
	if err != nil {
		return err
	}

	// if toVersion provided by the user, replace the mixer version
	if resetCmdFlags.toVersion >= 0 {
		b.MixVer = strconv.Itoa(int(resetCmdFlags.toVersion))
		b.MixVerUint32 = uint32(resetCmdFlags.toVersion)
	} else {
		// assuming mixer.state file has the correct info
		b.MixVer = b.State.Mix.PreviousMixVer
		b.MixVerUint32, err = parseUint32(b.State.Mix.PreviousMixVer)
		if err != nil {
			return err
		}
		fmt.Println("Reseting to default PREVIOUS_MIX_VERSION ", b.State.Mix.PreviousMixVer)
	}

	if b.MixVer == "0" {
		b.State.LoadDefaults(b.Config)
		b.State.Mix.PreviousMixVer = "0"
		if err := b.State.Save(); err != nil {
			fail(err)
		}

		if err := ioutil.WriteFile(filepath.Join(b.Config.Builder.VersionPath, b.MixVerFile), []byte("10"), 0644); err != nil {
			return err
		}

		if !resetCmdFlags.clean {
			fmt.Println("Reset completed.")
			return nil
		}
		err := os.RemoveAll(b.Config.Builder.ServerStateDir)
		if err != nil {
			log.Println(err)
			return err
		}
		fmt.Println("Reset completed.")
		return nil
	}

	// find the new previous version by reading the Manifest.Mom file for the new current version
	momFile := filepath.Join(b.Config.Builder.ServerStateDir, "www", b.MixVer, "Manifest.MoM")
	mom, err := swupd.ParseManifestFile(momFile)
	if err != nil {
		return err
	}
	b.State.Mix.PreviousMixVer = strconv.Itoa(int(mom.Header.Previous))
	currentMixFormatInt := mom.Header.Format

	// Make sure FORMAT in mixer.state has the same value as update/www/<previousMixVer>/format
	var format string
	if mom.Header.Previous != 0 {
		format, err = b.GetFormatForVersion(b.State.Mix.PreviousMixVer)
		if err != nil {
			return err
		}
	} else {
		format = strconv.Itoa(int(currentMixFormatInt))
	}
	b.State.Mix.Format = strings.TrimSpace(format)

	// Change upstreamVersion file content to the new current version upstreamver
	var lastStableMixUpstreamVersion string
	lastStableMixUpstreamVersion, err = b.GetLocalUpstreamVersion(b.MixVer)
	if err != nil {
		return err
	}
	filename := filepath.Join(b.Config.Builder.ServerStateDir, "www", b.MixVer, "upstreamver")
	if strings.TrimSpace(lastStableMixUpstreamVersion) != b.UpstreamVer {
		// Set the upstream version to the previous format's latest version
		b.UpstreamVer = strings.TrimSpace(lastStableMixUpstreamVersion)
		b.UpstreamVerUint32, err = parseUint32(b.UpstreamVer)
		if err != nil {
			return errors.Wrapf(err, "Couldn't parse upstream version")
		}
		vFile := filepath.Join(b.Config.Builder.VersionPath, b.UpstreamVerFile)
		if err = ioutil.WriteFile(vFile, []byte(b.UpstreamVer), 0644); err != nil {
			return err
		}
	}

	// Change upstreamURL file content to the new current version upstreamurl
	var lastStableMixUpstreamURL []byte
	filename = filepath.Join(b.Config.Builder.ServerStateDir, "www", b.MixVer, "upstreamurl")
	if lastStableMixUpstreamURL, err = ioutil.ReadFile(filename); err != nil {
		return err
	}
	if strings.TrimSpace(string(lastStableMixUpstreamURL)) != b.UpstreamURL {
		// Set the upstream version to the previous format's latest version
		b.UpstreamURL = strings.TrimSpace(string(lastStableMixUpstreamURL))
		vFile := filepath.Join(b.Config.Builder.VersionPath, b.UpstreamURLFile)
		if err = ioutil.WriteFile(vFile, []byte(b.UpstreamURL), 0644); err != nil {
			return err
		}
	}

	// Change mixVersion to point to new mixer version
	if err := ioutil.WriteFile(filepath.Join(b.Config.Builder.VersionPath, b.MixVerFile), []byte(b.MixVer), 0644); err != nil {
		return err
	}

	// Make sure update/image/LAST_VER points to new mixer version
	if err = ioutil.WriteFile(filepath.Join(b.Config.Builder.ServerStateDir, "image", "LAST_VER"), []byte(b.MixVer), 0644); err != nil {
		return fmt.Errorf("couldn't update LAST_VER file: %s", err)
	}

	// Make sure update/image/format#/latest points to new mixer version
	newFormat := "format" + strconv.Itoa(int(currentMixFormatInt))
	if err = ioutil.WriteFile(filepath.Join(b.Config.Builder.ServerStateDir, "/www/version/", newFormat, "latest"), []byte(b.MixVer), 0644); err != nil {
		return fmt.Errorf("couldn't update latest file: %s", err)
	}
	//update the sig file for the latest

	// Make sure update/image/latest_version points to new mixer version
	if err = ioutil.WriteFile(filepath.Join(b.Config.Builder.ServerStateDir, "/www/version/", "latest_version"), []byte(b.MixVer), 0644); err != nil {
		return fmt.Errorf("couldn't update latest_version file: %s", err)
	}
	//update the sig file for the latest_version

	// update the mixer.state file
	err = b.State.Save()
	if err != nil {
		return fmt.Errorf("couldn't update mixer.state file: %s", err)
	}

	// if clean flag not set
	if !resetCmdFlags.clean {
		fmt.Println("Reset completed.")
		return nil
	}

	// Remove any folder inside update/image for versions above mixver
	files, err := ioutil.ReadDir(b.Config.Builder.ServerStateDir + "/image")
	if err != nil {
		log.Println(err)
	}

	for _, f := range files {
		if f.Name() == "LAST_VER" {
			continue
		}
		dirNameInt, err := parseUint32(f.Name())
		if err != nil {
			log.Println(err)
			continue
		}
		if dirNameInt > b.MixVerUint32 {
			err := os.RemoveAll(b.Config.Builder.ServerStateDir + "/image/" + f.Name())
			if err != nil {
				log.Println(err)
				continue
			}
		}
	}

	// Remove any folder inside update/www for versions above mixver
	files, err = ioutil.ReadDir(b.Config.Builder.ServerStateDir + "/www")
	if err != nil {
		log.Println(err)
	}

	for _, f := range files {
		if f.Name() == "version" {
			continue
		}
		dirNameInt, err := parseUint32(f.Name())
		if err != nil {
			log.Println(err)
			continue
		}
		if dirNameInt > b.MixVerUint32 {
			err := os.RemoveAll(b.Config.Builder.ServerStateDir + "/www/" + f.Name())
			if err != nil {
				log.Println(err)
			}
		}
	}

	// Remove any folder inside update/www/version/ for version above mixver
	files, err = ioutil.ReadDir(b.Config.Builder.ServerStateDir + "/www/version")
	if err != nil {
		log.Println(err)
	}
	// iterate over all the formats folder
	for _, f := range files {
		// read the dir name and extract the format
		if f.IsDir() {
			dirName := strings.SplitAfter(f.Name(), "format")
			if len(dirName) >= 2 {
				dirNameInt, err := parseUint32(dirName[1])
				if err != nil {
					log.Println(err)
					continue
				}
				if dirNameInt > b.MixVerUint32 {
					err := os.RemoveAll(b.Config.Builder.ServerStateDir + "/www/version/" + f.Name())
					if err != nil {
						log.Println(err)
					}
				}
			}
		}
	}
	fmt.Println("Reset completed.")
	return nil
}
