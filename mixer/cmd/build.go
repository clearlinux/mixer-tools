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
	"path/filepath"
	"runtime"
	"strconv"

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

	numFullfileWorkers int
	numDeltaWorkers    int
	numBundleWorkers   int
}

var buildFlags buildCmdFlags

func setWorkers(b *builder.Builder) {
	var workers int
	workers = buildFlags.numFullfileWorkers
	if workers < 1 {
		workers = runtime.NumCPU()
	}
	b.NumFullfileWorkers = workers
	workers = buildFlags.numDeltaWorkers
	if workers < 1 {
		workers = runtime.NumCPU()
	}
	b.NumDeltaWorkers = workers
	workers = buildFlags.numBundleWorkers
	if workers < 1 {
		workers = runtime.NumCPU()
	}
	b.NumBundleWorkers = workers
}

// buildCmd represents the base build command when called without any subcommands
var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build various pieces of OS content",
}

func buildBundles(builder *builder.Builder, signflag bool) error {
	// Create the signing and validation key/cert
	if _, err := os.Stat(builder.Config.Builder.Cert); os.IsNotExist(err) {
		fmt.Println("Generating certificate for signature validation...")
		privkey, err := helpers.CreateKeyPair()
		if err != nil {
			return errors.Wrap(err, "Error generating OpenSSL keypair")
		}
		template := helpers.CreateCertTemplate()

		err = builder.BuildBundles(template, privkey, signflag)
		if err != nil {
			return errors.Wrap(err, "Error building bundles")
		}
	} else {
		err := builder.BuildBundles(nil, nil, true)
		if err != nil {
			return errors.Wrap(err, "Error building bundles")
		}
	}
	return nil
}

var buildBundlesCmd = &cobra.Command{
	Use:     "bundles",
	Aliases: []string{"chroots"},
	Short:   "Build the bundles for your mix",
	Long:    `Build the bundles for your mix`,
	Run: func(cmd *cobra.Command, args []string) {
		b, err := builder.NewFromConfig(config)
		if err != nil {
			fail(err)
		}
		setWorkers(b)
		err = buildBundles(b, buildFlags.noSigning)
		if err != nil {
			fail(err)
		}
	},
}

// buildOldFormatCmd is used to build the final version in the current format (the +10)
// and ready the mix state for the new (+20) version to be built by a (possibly) newer mixer
var buildOldFormatCmd = &cobra.Command{
	Use:   "format-old",
	Short: "Build the last version in the current format for the format bump",
	Long:  `Build the last version in the current format for the format bump`,
	Run: func(cmd *cobra.Command, args []string) {
		b, err := builder.NewFromConfig(config)
		if err != nil {
			fail(err)
		}

		// Update the mixversion just in case the user did not pass --increment
		// This must be the +20 to write the new format data files even though we
		// will build a +10 from the same content
		for i := 0; i < 2; i++ {
			if err = b.UpdateMixVer(); err != nil {
				failf("Couldn't update Mix Version")
			}
			// Must re-read the builder or UpdateMixVer() will be a noop each iteration
			if err = b.ReadVersions(); err != nil {
				fail(err)
			}
		}

		// Set format to format+1 so that the format file inserted into the
		// update content is the new one
		if err = b.UpdateFormatVersion(buildFlags.format); err != nil {
			fail(err)
		}
		setWorkers(b)

		fmt.Println(" Backing up full groups.ini")
		// Back up groups.ini in case we have deprecated bundles to delete
		if err = b.CopyFullGroupsINI(); err != nil {
			fail(err)
		}
		// Fill this in w/Update bundle definitions
		// if err := UpdateBudlesForFormatBump(); err != nil {...}

		// Build the +20 (first build in new format) bundles
		if err = buildBundles(b, buildFlags.noSigning); err != nil {
			fail(err)
		}

		fmt.Println("Backing up trimmed groups.ini for future build")
		// Copy the new trimmed groups.ini to use for the +20 build which will
		// not have manifest created for fully deleted bundles
		if err = b.CopyTrimmedGroupsINI(); err != nil {
			fail(err)
		}

		// Copy +20 chroots to +10 so we can build last formatN build with the
		// same content
		prevVersion, err := strconv.Atoi(b.MixVer)
		if err != nil {
			fail(err)
		}
		prevVersion -= 10
		source := filepath.Join(b.Config.Builder.ServerStateDir, "image", b.MixVer)
		dest := filepath.Join(b.Config.Builder.ServerStateDir, "image", strconv.Itoa(prevVersion))
		fmt.Println(" Copying +20 chroots to +10 chroots")
		if err = helpers.RunCommandSilent("cp", "-al", source, dest); err != nil {
			failf("Failed to copy +20 chroots to +10: %s\n", err)
		}

		// Copy the old groups.ini file back which contains ALL original bundle names
		// to account for any removed bundles in this build when creating manifests
		fmt.Println(" Copying full groups.ini back to working directory")
		if err = b.RevertFullGroupsINI(); err != nil {
			fail(err)
		}

		// Set the format back to the previous format version before building the +10 update
		prevFormat, err := strconv.Atoi(buildFlags.format)
		if err != nil {
			fail(err)
		}
		prevFormat--
		if err = b.UpdateFormatVersion(strconv.Itoa(prevFormat)); err != nil {
			fail(err)
		}
		// Set mixversion to the +10 since we have used +20 up to this point
		if err = b.DecrementMixVer(); err != nil {
			fail(err)
		}
		// Re-read builder.conf after updating format/mixver
		if err = b.ReadVersions(); err != nil {
			fail(err)
		}
		setWorkers(b)

		// Build the update content for the +10 build
		err = b.BuildUpdate(buildFlags.prefix, buildFlags.minVersion, buildFlags.format, buildFlags.noSigning, !buildFlags.noPublish, buildFlags.keepChroot)
		if err != nil {
			failf("Couldn't build update: %s", err)
		}
		// Copy the trimmed groups.ini back for the +20 build to use
		if err = b.RevertTrimmedGroupsINI(); err != nil {
			fail(err)
		}
		// Update the format back to the formatN+1 version for +20 to use
		if err = b.UpdateFormatVersion(buildFlags.format); err != nil {
			fail(err)
		}
		// Set the previous format latest file to the +10 build
		formatDir := filepath.Join(b.Config.Builder.ServerStateDir, "www", "version", "format"+strconv.Itoa(prevFormat))
		err = ioutil.WriteFile(filepath.Join(formatDir, "latest"), []byte(b.MixVer), 0644)
		if err != nil {
			failf("Couldn't update the latest version")
		}
		// Incremement regardless of flag because +20 must be the next version
		if err = b.UpdateMixVer(); err != nil {
			failf("Couldn't update Mix Version")
		}
	},
}

var buildNewFormatCmd = &cobra.Command{
	Use:   "format-new",
	Short: "Build the first version in the new format for the format bump",
	Long:  `Build the first version in the new format for the format bump`,
	Run: func(cmd *cobra.Command, args []string) {
		b, err := builder.NewFromConfig(config)
		if err != nil {
			fail(err)
		}
		setWorkers(b)
		ver, err := strconv.Atoi(b.MixVer)
		if err != nil {
			fail(err)
		}
		err = b.BuildUpdate(buildFlags.prefix, ver, buildFlags.format, buildFlags.noSigning, !buildFlags.noPublish, buildFlags.keepChroot)
		if err != nil {
			failf("Couldn't build update: %s", err)
		}

		if buildFlags.increment {
			if err = b.UpdateMixVer(); err != nil {
				failf("Couldn't update Mix Version")
			}
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
		setWorkers(b)
		err = b.BuildUpdate(buildFlags.prefix, buildFlags.minVersion, buildFlags.format, buildFlags.noSigning, !buildFlags.noPublish, buildFlags.keepChroot)
		if err != nil {
			failf("Couldn't build update: %s", err)
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
		setWorkers(b)
		rpms, err := ioutil.ReadDir(b.Config.Mixer.LocalRPMDir)
		if err == nil {
			err = b.AddRPMList(rpms)
			if err != nil {
				failf("Couldn't add the RPMs: %s", err)
			}
		}
		err = buildBundles(b, buildFlags.noSigning)
		if err != nil {
			failf("Couldn't build bundles: %s", err)
		}
		err = b.BuildUpdate(buildFlags.prefix, buildFlags.minVersion, buildFlags.format, buildFlags.noSigning, !buildFlags.noPublish, buildFlags.keepChroot)
		if err != nil {
			failf("Couldn't build update: %s", err)
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
		setWorkers(b)
		err = b.BuildImage(buildFlags.format, buildFlags.template)
		if err != nil {
			failf("Couldn't build image: %s", err)
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
	report           bool
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
	setWorkers(b)
	if fromChanged {
		err = b.BuildDeltaPacks(buildDeltaPacksFlags.from, buildDeltaPacksFlags.to, buildDeltaPacksFlags.report)
	} else {
		err = b.BuildDeltaPacksPreviousVersions(buildDeltaPacksFlags.previousVersions, buildDeltaPacksFlags.to, buildDeltaPacksFlags.report)
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
	buildBundlesCmd,
	buildUpdateCmd,
	buildAllCmd,
	buildImageCmd,
	buildDeltaPacksCmd,
	buildOldFormatCmd,
	buildNewFormatCmd,
}

func init() {
	for _, cmd := range buildCmds {
		buildCmd.AddCommand(cmd)
		cmd.Flags().StringVarP(&config, "config", "c", "", "Builder config to use")
	}

	buildCmd.PersistentFlags().IntVar(&buildFlags.numFullfileWorkers, "fullfile-workers", 0, "Number of parallel workers when creating fullfiles, 0 means number of CPUs")
	buildCmd.PersistentFlags().IntVar(&buildFlags.numDeltaWorkers, "delta-workers", 0, "Number of parallel workers when creating deltas, 0 means number of CPUs")
	buildCmd.PersistentFlags().IntVar(&buildFlags.numBundleWorkers, "bundle-workers", 0, "Number of parallel workers when building bundles, 0 means number of CPUs")
	RootCmd.AddCommand(buildCmd)

	buildBundlesCmd.Flags().BoolVar(&buildFlags.noSigning, "no-signing", false, "Do not generate a certificate to sign the Manifest.MoM")
	buildBundlesCmd.Flags().BoolVar(&builder.UseNewChrootBuilder, "new-chroots", false, "EXPERIMENTAL: Use new implementation of build chroots")

	buildImageCmd.Flags().StringVar(&buildFlags.format, "format", "", "Supply the format used for the Mix")
	buildImageCmd.Flags().StringVar(&buildFlags.template, "template", "", "Path to template file to use")

	buildDeltaPacksCmd.Flags().Uint32Var(&buildDeltaPacksFlags.from, "from", 0, "Generate packs from a specific version")
	buildDeltaPacksCmd.Flags().Uint32Var(&buildDeltaPacksFlags.previousVersions, "previous-versions", 0, "Generate packs for multiple previous versions")
	buildDeltaPacksCmd.Flags().Uint32Var(&buildDeltaPacksFlags.to, "to", 0, "Generate packs targeting a specific version")
	buildDeltaPacksCmd.Flags().BoolVar(&buildDeltaPacksFlags.report, "report", false, "Report reason each file in to manifest was packed or not")

	setUpdateFlags(buildUpdateCmd)
	setUpdateFlags(buildAllCmd)
	setUpdateFlags(buildOldFormatCmd)
	setUpdateFlags(buildNewFormatCmd)
	buildOldFormatCmd.Flags().BoolVar(&builder.UseNewChrootBuilder, "new-chroots", false, "EXPERIMENTAL: Use new implementation of build chroots")

	externalDeps[buildBundlesCmd] = []string{
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
		externalDeps[buildBundlesCmd],
		append(externalDeps[buildUpdateCmd],
			externalDeps[buildImageCmd]...)...)
}
