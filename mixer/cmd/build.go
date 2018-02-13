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

package cmd

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/clearlinux/mixer-tools/builder"
	"github.com/clearlinux/mixer-tools/helpers"
	"github.com/pkg/errors"

	"github.com/spf13/cobra"
)

type buildCmdFlags struct {
	format     string
	increment  bool
	minVersion int
	noSigning  bool
	prefix     string
	noPublish  bool
	keepChroot bool
	template   string
}

var buildFlags buildCmdFlags

// buildCmd represents the base build command when called without any subcommands
var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build various pieces of OS content",
}

func buildChroots(builder *builder.Builder, signflag bool) error {
	// Create the signing and validation key/cert
	if _, err := os.Stat(builder.Cert); os.IsNotExist(err) {
		fmt.Println("Generating certificate for signature validation...")
		privkey, err := helpers.CreateKeyPair()
		if err != nil {
			return errors.Wrap(err, "Error generating OpenSSL keypair")
		}
		template := helpers.CreateCertTemplate()

		err = builder.BuildChroots(template, privkey, signflag)
		if err != nil {
			return errors.Wrap(err, "Error building chroots")
		}
	} else {
		err := builder.BuildChroots(nil, nil, true)
		if err != nil {
			return errors.Wrap(err, "Error building chroots")
		}
	}
	return nil
}

var buildChrootsCmd = &cobra.Command{
	Use:   "chroots",
	Short: "Build the chroots for your mix",
	Long:  `Build the chroots for your mix`,
	Run: func(cmd *cobra.Command, args []string) {
		b, err := builder.NewFromConfig(config)
		if err != nil {
			fail(err)
		}
		err = buildChroots(b, buildFlags.noSigning)
		if err != nil {
			fail(err)
		}
	},
}

var buildUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Build the update content for your mix",
	Long:  `Build the update content for your mix`,
	Run: func(cmd *cobra.Command, args []string) {
		b, err := builder.NewFromConfig(config)
		if err != nil {
			fail(err)
		}
		err = b.BuildUpdate(buildFlags.prefix, buildFlags.minVersion, buildFlags.format, buildFlags.noSigning, !buildFlags.noPublish, buildFlags.keepChroot)
		if err != nil {
			failf("couldn't build update: %s", err)
		}

		if buildFlags.increment {
			if err = b.UpdateMixVer(); err != nil {
				failf("Couldn't update Mix Version")
			}
		}
	},
}

var buildAllCmd = &cobra.Command{
	Use:   "all",
	Short: "Build all content for mix with default options",
	Long:  `Build all content for mix with default options`,
	Run: func(cmd *cobra.Command, args []string) {
		b, err := builder.NewFromConfig(config)
		if err != nil {
			fail(err)
		}
		rpms, err := ioutil.ReadDir(b.RPMDir)
		if err == nil {
			err = b.AddRPMList(rpms)
			if err != nil {
				failf("couldn't add the RPMs: %s", err)
			}
		}
		err = buildChroots(b, buildFlags.noSigning)
		if err != nil {
			failf("couldn't build chroots: %s", err)
		}
		err = b.BuildUpdate(buildFlags.prefix, buildFlags.minVersion, buildFlags.format, buildFlags.noSigning, !buildFlags.noPublish, buildFlags.keepChroot)
		if err != nil {
			failf("couldn't build update: %s", err)
		}
		err = b.UpdateMixVer()
		if err != nil {
			failf("Couldn't update Mix Version")
		}
	},
}

var buildImageCmd = &cobra.Command{
	Use:   "image",
	Short: "Build an image from the mix content",
	Long:  `Build an image from the mix content`,
	Run: func(cmd *cobra.Command, args []string) {
		b, err := builder.NewFromConfig(config)
		if err != nil {
			fail(err)
		}
		err = b.BuildImage(buildFlags.format, buildFlags.template)
		if err != nil {
			failf("couldn't build image: %s", err)
		}
	},
}

var buildDeltaPacksCmd = &cobra.Command{
	Use:   "delta-packs",
	Short: "Build packs used to optimize update between versions",
	Long: `Build packs used to optimize update between versions

EXPERIMENTAL: this command only works with --new-swupd. For the
current swupd-server implementation use mixer-pack-maker.sh program.

When a swupd client updates a bundle, it looks for a pack file from
its current version to the new version. If not available, the client
will download the individual files necessary for the update. If a
bundle haven't changed between two versions, no pack need to be
generated.

To generate the packs to optimize update from VER to the current mix
version use

    mixer build delta-packs --from VER

Alternatively, to generate packs for a set of NUM previous versions,
each one to the current mix version, instead of --from use

    mixer build delta-packs --previous-versions NUM

To change the target version (by default the current version), use the
flag --to. The target version must be larger than the --from version.

`,
	RunE: runBuildDeltaPacks,
}

var buildDeltaPacksFlags struct {
	previousVersions uint32
	from             uint32
	to               uint32
}

func runBuildDeltaPacks(cmd *cobra.Command, args []string) error {
	if !builder.UseNewSwupdServer {
		// TODO: Depending on how long we are going to live with both implementations, it
		// might be worth making this command call the script when not using new swupd-server.
		failf("build delta packs is only available with --new-swupd\nUse mixer-pack-maker.sh instead.")
	}

	fromChanged := cmd.Flags().Changed("from")
	prevChanged := cmd.Flags().Changed("previous-versions")
	if fromChanged == prevChanged {
		return errors.Errorf("either --from or --previous-versions must be set, but not both")
	}

	b, err := builder.NewFromConfig(config)
	if err != nil {
		fail(err)
	}
	if fromChanged {
		err = b.BuildDeltaPacks(buildDeltaPacksFlags.from, buildDeltaPacksFlags.to)
	} else {
		err = b.BuildDeltaPacksPreviousVersions(buildDeltaPacksFlags.previousVersions, buildDeltaPacksFlags.to)
	}
	if err != nil {
		fail(err)
	}
	return nil
}

func setUpdateFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&buildFlags.format, "format", "", "Supply format to use")
	cmd.Flags().BoolVar(&buildFlags.increment, "increment", false, "Automatically increment the mixversion post build")
	cmd.Flags().IntVar(&buildFlags.minVersion, "min-version", 0, "Supply minversion to build update with")
	cmd.Flags().BoolVar(&buildFlags.noSigning, "no-signing", false, "Do not generate a certificate and do not sign the Manifest.MoM")
	cmd.Flags().StringVar(&buildFlags.prefix, "prefix", "", "Supply prefix for where the swupd binaries live")
	cmd.Flags().BoolVar(&buildFlags.noPublish, "no-publish", false, "Do not update the latest version after update")
	cmd.Flags().BoolVar(&buildFlags.keepChroot, "keep-chroots", false, "Keep individual chroots created and not just consolidated 'full'")
}

var buildCmds = []*cobra.Command{
	buildChrootsCmd,
	buildUpdateCmd,
	buildAllCmd,
	buildImageCmd,
	buildDeltaPacksCmd,
}

func init() {
	for _, cmd := range buildCmds {
		buildCmd.AddCommand(cmd)
		cmd.Flags().StringVarP(&config, "config", "c", "", "Builder config to use")
	}

	RootCmd.AddCommand(buildCmd)

	buildChrootsCmd.Flags().BoolVar(&buildFlags.noSigning, "no-signing", false, "Do not generate a certificate to sign the Manifest.MoM")
	buildChrootsCmd.Flags().BoolVar(&builder.UseNewChrootBuilder, "new-chroots", false, "EXPERIMENTAL: Use new implementation of build chroots")

	buildImageCmd.Flags().StringVar(&buildFlags.format, "format", "", "Supply the format used for the Mix")
	buildImageCmd.Flags().StringVar(&buildFlags.template, "template", "", "Path to template file to use")

	buildDeltaPacksCmd.Flags().Uint32Var(&buildDeltaPacksFlags.from, "from", 0, "Generate packs from a specific version")
	buildDeltaPacksCmd.Flags().Uint32Var(&buildDeltaPacksFlags.previousVersions, "previous-versions", 0, "Generate packs for multiple previous versions")
	buildDeltaPacksCmd.Flags().Uint32Var(&buildDeltaPacksFlags.to, "to", 0, "Generate packs targeting a specific version")

	setUpdateFlags(buildUpdateCmd)
	setUpdateFlags(buildAllCmd)

	externalDeps[buildChrootsCmd] = []string{
		"m4",
		"rpm",
		"dnf",
	}
	externalDeps[buildUpdateCmd] = []string{
		"hardlink",
		"mixer-pack-maker.sh",
		"openssl",
		"parallel", // Used by pack-maker.
		"xz",
	}
	externalDeps[buildImageCmd] = []string{
		"ister.py",
	}
	externalDeps[buildAllCmd] = append(
		externalDeps[buildChrootsCmd],
		append(externalDeps[buildUpdateCmd],
			externalDeps[buildImageCmd]...)...)
}
