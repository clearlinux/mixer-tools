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
	"strings"

	"github.com/clearlinux/mixer-tools/builder"
	"github.com/clearlinux/mixer-tools/config"
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
		b, err := builder.NewFromConfig(configFile)
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

var buildUpstreamFormatCmd = &cobra.Command{
	Use:   "upstream-format",
	Short: "Use to create the necessary builds to cross an upstream format",
	Long:  `Use to create the necessary builds to cross an upstream format`,
	Run: func(cmd *cobra.Command, args []string) {
		b, err := builder.NewFromConfig(configFile)
		if err != nil {
			fail(err)
		}
		cmdToRun := strings.Split("mixer build format-bump new", " ")
		if config.UseNewConfig {
			cmdToRun = append(cmdToRun, "--new-config")
		}
		if err := b.RunCommandInContainer(cmdToRun); err != nil {
			fail(err)
		}

		// Set the upstream version to the previous format's latest version
		b.UpstreamVerUint32 -= 10
		b.UpstreamVer = strconv.FormatUint(uint64(b.UpstreamVerUint32), 10)
		vFile := filepath.Join(b.Config.Builder.VersionPath, b.UpstreamVerFile)
		if err := ioutil.WriteFile(vFile, []byte(b.UpstreamVer), 0644); err != nil {
			fail(err)
		}
		cmdToRun = strings.Split("mixer build format-bump old", " ")
		if config.UseNewConfig {
			cmdToRun = append(cmdToRun, "--new-config")
		}
		if err := b.RunCommandInContainer(cmdToRun); err != nil {
			fail(err)
		}
		// Set the upstream version back to what the user originally tried to build
		if err := b.UnstageMixFromBump(); err != nil {
			fail(err)
		}
	},
}

var buildFormatBumpCmd = &cobra.Command{
	Use:   "format-bump",
	Short: "Used to create a downstream format bump",
	Long:  `Used to create a downstream format bump`,
	Run: func(cmd *cobra.Command, args []string) {
		b, err := builder.NewFromConfig(configFile)
		if err != nil {
			fail(err)
		}
		cmdToRun := strings.Split("mixer build format-bump new", " ")
		if config.UseNewConfig {
			cmdToRun = append(cmdToRun, "--new-config")
		}
		if err := b.RunCommandInContainer(cmdToRun); err != nil {
			fail(err)
		}
		cmdToRun = strings.Split("mixer build format-bump old", " ")
		if config.UseNewConfig {
			cmdToRun = append(cmdToRun, "--new-config")
		}
		if err := b.RunCommandInContainer(cmdToRun); err != nil {
			fail(err)
		}
	},
}

// buildOldFormatCmd is used to build the final version in the current format (the +10)
// and ready the mix state for the new (+20) version to be built by a (possibly) newer mixer
var buildFormatNewCmd = &cobra.Command{
	Use:   "new",
	Short: "Build the +20 version in the new format for the format bump",
	Long:  `Build the +20 version in the new format for the format bump`,
	Run: func(cmd *cobra.Command, args []string) {
		b, err := builder.NewFromConfig(configFile)
		if err != nil {
			fail(err)
		}

		lastVer, err := b.GetLastBuildVersion()
		if err != nil {
			fail(err)
		}
		ver, err := strconv.Atoi(lastVer)
		if err != nil {
			fail(err)
		}
		if err = b.UpdateMixVer(ver + 20); err != nil {
			failf("Couldn't update Mix Version")
		}

		// Set format to format+1 so that the format file inserted into the
		// update content is the new one
		newFormat, err := strconv.Atoi(b.Config.Swupd.Format)
		if err != nil {
			fail(err)
		}
		newFormat++
		if err = b.UpdateFormatVersion(strconv.Itoa(newFormat)); err != nil {
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

		// TODO: Inject any extra files (certs, etc) here
		// if err := AddBundleExtrasFiles(); err != nil {...}

		ver, err = strconv.Atoi(b.MixVer)
		if err != nil {
			fail(err)
		}

		// Build the +20 update so we don't have to switch tooling in between
		err = b.BuildUpdate(buildFlags.prefix, ver, strconv.Itoa(newFormat), buildFlags.noSigning, !buildFlags.noPublish, buildFlags.keepChroot)
		if err != nil {
			failf("Couldn't build update: %s", err)
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
		prevFormat, err := strconv.Atoi(b.Config.Swupd.Format)
		if err != nil {
			fail(err)
		}
		prevFormat--
		if err = b.UpdateFormatVersion(strconv.Itoa(prevFormat)); err != nil {
			fail(err)
		}
		// Set mixversion to the +10 since we have used +20 up to this point
		if err = b.UpdateMixVer(prevVersion); err != nil {
			fail(err)
		}
	},
}

var buildFormatOldCmd = &cobra.Command{
	Use:   "old",
	Short: "Build the +10 version in the old format for the format bump",
	Long:  `Build the +10 version in the old format for the format bump`,
	Run: func(cmd *cobra.Command, args []string) {
		b, err := builder.NewFromConfig(configFile)
		if err != nil {
			fail(err)
		}
		setWorkers(b)
		ver, err := strconv.Atoi(b.MixVer)
		if err != nil {
			fail(err)
		}
		// Build the update content for the +10 build
		err = b.BuildUpdate(buildFlags.prefix, ver, b.Config.Swupd.Format, buildFlags.noSigning, !buildFlags.noPublish, buildFlags.keepChroot)
		if err != nil {
			failf("Couldn't build update: %s", err)
		}

		if err = b.UpdateMixVer(ver + 20); err != nil {
			failf("Couldn't update Mix Version")
		}
		// Update the update/image/LAST_VER to the +20 build, since we built the +10 out of order
		if err := ioutil.WriteFile(filepath.Join(b.Config.Builder.ServerStateDir, "image/LAST_VER"), []byte(strconv.Itoa(ver+10)), 0644); err != nil {
			fail(err)
		}
	},
}

var buildUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Build the update content for your mix",
	Long:  `Build the update content for your mix`,
	Run: func(cmd *cobra.Command, args []string) {
		b, err := builder.NewFromConfig(configFile)
		if err != nil {
			fail(err)
		}
		setWorkers(b)
		err = b.BuildUpdate(buildFlags.prefix, buildFlags.minVersion, buildFlags.format, buildFlags.noSigning, !buildFlags.noPublish, buildFlags.keepChroot)
		if err != nil {
			failf("Couldn't build update: %s", err)
		}

		if buildFlags.increment {
			ver, err := strconv.Atoi(b.MixVer)
			if err != nil {
				fail(err)
			}
			if err = b.UpdateMixVer(ver + 10); err != nil {
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
		b, err := builder.NewFromConfig(configFile)
		if err != nil {
			fail(err)
		}
		setWorkers(b)
		rpms, err := helpers.ListVisibleFiles(b.Config.Mixer.LocalRPMDir)
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

		ver, err := strconv.Atoi(b.MixVer)
		if err != nil {
			fail(err)
		}
		if err = b.UpdateMixVer(ver + 10); err != nil {
			failf("Couldn't update Mix Version")
		}
	},
}

var buildImageCmd = &cobra.Command{
	Use:   "image",
	Short: "Build an image from the mix content",
	Long:  `Build an image from the mix content`,
	Run: func(cmd *cobra.Command, args []string) {
		b, err := builder.NewFromConfig(configFile)
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
	fromChanged := cmd.Flags().Changed("from")
	prevChanged := cmd.Flags().Changed("previous-versions")
	if fromChanged == prevChanged {
		return errors.Errorf("either --from or --previous-versions must be set, but not both")
	}

	b, err := builder.NewFromConfig(configFile)
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
	buildUpstreamFormatCmd,
	buildFormatBumpCmd,
}

func init() {
	for _, cmd := range buildCmds {
		buildCmd.AddCommand(cmd)
		cmd.Flags().StringVarP(&configFile, "config", "c", "", "Builder config to use")
	}

	buildFormatBumpCmd.AddCommand(buildFormatNewCmd)
	buildFormatBumpCmd.AddCommand(buildFormatOldCmd)

	buildCmd.PersistentFlags().IntVar(&buildFlags.numFullfileWorkers, "fullfile-workers", 0, "Number of parallel workers when creating fullfiles, 0 means number of CPUs")
	buildCmd.PersistentFlags().IntVar(&buildFlags.numDeltaWorkers, "delta-workers", 0, "Number of parallel workers when creating deltas, 0 means number of CPUs")
	buildCmd.PersistentFlags().IntVar(&buildFlags.numBundleWorkers, "bundle-workers", 0, "Number of parallel workers when building bundles, 0 means number of CPUs")
	RootCmd.AddCommand(buildCmd)

	buildBundlesCmd.Flags().BoolVar(&buildFlags.noSigning, "no-signing", false, "Do not generate a certificate to sign the Manifest.MoM")
	unusedBoolFlag := false
	buildBundlesCmd.Flags().BoolVar(&unusedBoolFlag, "new-chroots", false, "")
	_ = buildBundlesCmd.Flags().MarkHidden("new-chroots")
	_ = buildBundlesCmd.Flags().MarkDeprecated("new-chroots", "new functionality is now the standard behavior, this flag is obsolete and no longer used")

	buildImageCmd.Flags().StringVar(&buildFlags.format, "format", "", "Supply the format used for the Mix")
	buildImageCmd.Flags().StringVar(&buildFlags.template, "template", "", "Path to template file to use")

	buildDeltaPacksCmd.Flags().Uint32Var(&buildDeltaPacksFlags.from, "from", 0, "Generate packs from a specific version")
	buildDeltaPacksCmd.Flags().Uint32Var(&buildDeltaPacksFlags.previousVersions, "previous-versions", 0, "Generate packs for multiple previous versions")
	buildDeltaPacksCmd.Flags().Uint32Var(&buildDeltaPacksFlags.to, "to", 0, "Generate packs targeting a specific version")
	buildDeltaPacksCmd.Flags().BoolVar(&buildDeltaPacksFlags.report, "report", false, "Report reason each file in to manifest was packed or not")

	setUpdateFlags(buildUpdateCmd)
	setUpdateFlags(buildAllCmd)
	setUpdateFlags(buildFormatNewCmd)
	setUpdateFlags(buildFormatOldCmd)

	externalDeps[buildBundlesCmd] = []string{
		"rpm",
		"dnf",
	}
	externalDeps[buildUpdateCmd] = []string{
		"hardlink",
		"openssl",
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
