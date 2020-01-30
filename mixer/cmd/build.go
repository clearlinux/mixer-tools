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
	"github.com/clearlinux/mixer-tools/helpers"
	"github.com/pkg/errors"

	"github.com/spf13/cobra"
)

const bumpMarker = "format-bump"

// Sets the default number of RPM download retries
const retriesDefault = 3

type buildCmdFlags struct {
	format          string
	newFormat       string
	increment       bool
	minVersion      int
	noSigning       bool
	downloadRetries int
	noPublish       bool
	template        string
	skipFullfiles   bool
	skipPacks       bool
	to              int
	from            int
	tableWidth      int
	toRepoURLs      *map[string]string
	fromRepoURLs    *map[string]string
	skipFormatCheck bool

	numFullfileWorkers int
	numDeltaWorkers    int
	numBundleWorkers   int
}

var buildFlags buildCmdFlags

func setWorkers(b *builder.Builder) {
	workers := buildFlags.numFullfileWorkers
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

	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Since this function overwrites root's PersistentPreRun, call it manually
		root := cmd
		for root.Parent() != nil {
			root = root.Parent()
		}
		if err := root.PersistentPreRunE(cmd, args); err != nil {
			return err
		}

		b, err := builder.NewFromConfig(configFile)
		if err != nil {
			return err
		}

		// Skip format bump check for format-bump commands
		if hasMarker(cmd, bumpMarker) {
			return nil
		}

		return checkFormatBump(b)
	},
}

func buildBundles(builder *builder.Builder, signflag bool, downloadRetries int) error {
	if downloadRetries < 0 {
		return errors.New("Please supply value >= 0 for --retries")
	}
	// Create the signing and validation key/cert
	if _, err := os.Stat(builder.Config.Builder.Cert); os.IsNotExist(err) {
		fmt.Println("Generating certificate for signature validation...")
		privkey, err := helpers.CreateKeyPair()
		if err != nil {
			return errors.Wrap(err, "Error generating OpenSSL keypair")
		}
		template := helpers.CreateCertTemplate()

		err = builder.BuildBundles(template, privkey, signflag, downloadRetries)
		if err != nil {
			return errors.Wrap(err, "Error building bundles")
		}
	} else {
		err := builder.BuildBundles(nil, nil, true, downloadRetries)
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
		if err := checkRoot(); err != nil {
			fail(err)
		}

		b, err := builder.NewFromConfig(configFile)
		if err != nil {
			fail(err)
		}
		setWorkers(b)
		err = buildBundles(b, buildFlags.noSigning, buildFlags.downloadRetries)
		if err != nil {
			fail(err)
		}
	},
}

var buildUpstreamFormatCmd = &cobra.Command{
	Use:    "upstream-format",
	Short:  "Use to create the necessary builds to cross an upstream format",
	Long:   `Use to create the necessary builds to cross an upstream format`,
	Hidden: true,
	Run: func(cmd *cobra.Command, args []string) {
		if err := checkRoot(); err != nil {
			fail(err)
		}

		b, err := builder.NewFromConfig(configFile)
		if err != nil {
			fail(err)
		}

		// check if --new-format flag is set, if not set it to prevFormat +1
		if buildFlags.newFormat == "" {
			newFormat, err := strconv.Atoi(b.State.Mix.Format)
			if err != nil {
				fail(err)
			}
			newFormat = newFormat + 1
			buildFlags.newFormat = strconv.Itoa(newFormat)
		}

		_, err = strconv.Atoi(buildFlags.newFormat)
		if err != nil {
			fail(errors.New("Please supply a valid format version with --new-format"))
		}

		// Don't print any more warnings about being behind formats when we loop
		silent := true
		bumpNeeded := true

		for bumpNeeded {
			cmdStr := fmt.Sprintf("mixer build format-bump old --new-format %s  --retries %d", buildFlags.newFormat, buildFlags.downloadRetries)
			cmdToRun := strings.Split(cmdStr, " ")
			if err = helpers.RunCommand(cmdToRun[0], cmdToRun[1:]...); err != nil {
				fail(err)
			}

			// Set the upstream version to the previous format's latest version
			b.UpstreamVer = strconv.FormatUint(uint64(b.UpstreamVerUint32), 10)
			vFile := filepath.Join(b.Config.Builder.VersionPath, b.UpstreamVerFile)
			if err = ioutil.WriteFile(vFile, []byte(b.UpstreamVer), 0644); err != nil {
				fail(err)
			}
			cmdStr = fmt.Sprintf("mixer build format-bump new --new-format %s", buildFlags.newFormat)
			cmdToRun = strings.Split(cmdStr, " ")
			if err = helpers.RunCommand(cmdToRun[0], cmdToRun[1:]...); err != nil {
				fail(err)
			}
			// Set the upstream version back to what the user originally tried to build
			if err = b.UnstageMixFromBump(); err != nil {
				fail(err)
			}
			bumpNeeded, err = b.CheckBumpNeeded(silent)
			if err != nil {
				fail(err)
			}
		}
		newFormatVer, err := strconv.Atoi(b.MixVer)
		if err != nil {
			failf("Couldn't get new format version")
		}
		newFormatVer += 10
		if err = b.UpdateMixVer(newFormatVer); err != nil {
			failf("Couldn't update Mix Version")
		}
	},
}

var buildFormatBumpCmd = &cobra.Command{
	Use:    "format-bump",
	Short:  "Used to create a downstream format bump",
	Long:   `Used to create a downstream format bump`,
	Hidden: true,
	Run: func(cmd *cobra.Command, args []string) {
		if err := checkRoot(); err != nil {
			fail(err)
		}

		if buildFlags.newFormat == "" {
			fail(errors.New("Please supply the next format version with --new-format"))
		}
		_, err := strconv.Atoi(buildFlags.newFormat)
		if err != nil {
			fail(errors.New("Please supply a valid format version with --new-format"))
		}

		cmdStr := fmt.Sprintf("mixer build format-bump old --new-format %s --retries %d", buildFlags.newFormat, buildFlags.downloadRetries)
		cmdToRun := strings.Split(cmdStr, " ")
		if output, err := helpers.RunCommandOutputEnv(cmdToRun[0], cmdToRun[1:], []string{}); err != nil {
			failf("%s: %s", output, err)
		}
		cmdStr = fmt.Sprintf("mixer build format-bump new --new-format %s", buildFlags.newFormat)
		cmdToRun = strings.Split(cmdStr, " ")
		if output, err := helpers.RunCommandOutputEnv(cmdToRun[0], cmdToRun[1:], []string{}); err != nil {
			failf("%s: %s", output, err)
		}
	},
}

// This is the last build in the original format. At this point add ONLY the
// content relevant to the format bump to the mash to be used. Relevant content
// should be the only change.
//
// mixer will create manifests and update content based on the format it is
// building for. The format is set in the mixer.state file.
var buildFormatOldCmd = &cobra.Command{
	Use:   "old",
	Short: "Build the +10 version in the old format for the format bump",
	Long:  `Build the +10 version in the old format for the format bump`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := checkRoot(); err != nil {
			fail(err)
		}

		if buildFlags.newFormat == "" {
			fail(errors.New("Please supply the next format version with --new-format"))
		}

		_, err := strconv.Atoi(buildFlags.newFormat)
		if err != nil {
			fail(errors.New("Please supply a valid format version with --new-format"))
		}

		b, err := builder.NewFromConfig(configFile)
		if err != nil {
			fail(err)
		}

		setWorkers(b)

		lastVer, err := b.GetLastBuildVersion()
		if err != nil {
			fail(err)
		}

		// The PREVIOUS_MIX_VERSION value in mixer.state replaced the LAST_VER file for
		// setting the previous version in the manifest header field. The PREVIOUS_MIX_VERSION
		// value is set to the LAST_VER during format bumps to maintain consistent behavior.
		if err = b.UpdatePreviousMixVersion(lastVer); err != nil {
			fail(err)
		}

		originalVer, err := strconv.Atoi(lastVer)
		if err != nil {
			fail(err)
		}

		oldFormatVer := originalVer + 10
		newFormatVer := originalVer + 20
		// Update mixer to build version +20, to build the bundles with +20 inside not +10
		if err = b.UpdateMixVer(newFormatVer); err != nil {
			failf("Couldn't update Mix Version")
		}

		// Change the format internally just for building bundles so the right
		// /usr/share/defaults/swupd/format is inserted; preserve old format
		oldFormat := b.State.Mix.Format
		b.State.Mix.Format = buildFlags.newFormat

		// Build bundles normally. At this point the bundles to be deleted should still
		// be part of the mixbundles list and the groups.ini
		if err = buildBundles(b, buildFlags.noSigning, buildFlags.downloadRetries); err != nil {
			fail(err)
		}

		// Remove deleted bundles and replace with empty dirs for update to mark as deleted
		if err = b.ModifyBundles(b.ReplaceInfoBundles); err != nil {
			fail(err)
		}

		// Link the +20 bundles to the +10 so we are building the update with the same
		// underlying content. The only things that might change are the manifests
		// (potentially the pack and full-file formats as well, though this is very
		// rare).
		if err = b.UpdateMixVer(oldFormatVer); err != nil {
			failf("Couldn't update Mix Version")
		}
		source := filepath.Join(b.Config.Builder.ServerStateDir, "image", strconv.Itoa(newFormatVer))
		dest := filepath.Join(b.Config.Builder.ServerStateDir, "image", strconv.Itoa(oldFormatVer))
		fmt.Println(" Copying +20 bundles to +10 bundles")
		if err = helpers.RunCommandSilent("cp", "-al", source, dest); err != nil {
			failf("Failed to copy +10 bundles to +20: %s\n", err)
		}

		// Set the format back to old for the actual build update
		b.State.Mix.Format = oldFormat

		// Build the update content for the +10 build
		params := builder.UpdateParameters{
			MinVersion:    buildFlags.minVersion,
			Format:        b.State.Mix.Format,
			Publish:       !buildFlags.noPublish,
			SkipSigning:   buildFlags.noSigning,
			SkipFullfiles: buildFlags.skipFullfiles,
			SkipPacks:     buildFlags.skipPacks,
		}
		if err = b.BuildUpdate(params); err != nil {
			failf("Couldn't build update: %s", err)
		}

		// Write the +0 version to LAST_VER so that we reference in both +10 and +20 manifests as the 'previous:'
		if err = ioutil.WriteFile(filepath.Join(b.Config.Builder.ServerStateDir, "image", "LAST_VER"), []byte(lastVer), 0644); err != nil {
			failf("Couldn't update LAST_VER file: %s", err)
		}

		// PREVIOUS_MIX_VERSION is set to the LAST_VER for format bumps to maintain
		// consistent format bump behavior.
		if err = b.UpdatePreviousMixVersion(lastVer); err != nil {
			fail(err)
		}
	},
}

// This is the first build in the new format. The content is the same as the +10
// but the manifests might be created differently if a new manifest template is
// defined for the new format.
var buildFormatNewCmd = &cobra.Command{
	Use:   "new",
	Short: "Build the +20 version in the new format for the format bump",
	Long:  `Build the +20 version in the new format for the format bump`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := checkRoot(); err != nil {
			fail(err)
		}

		if buildFlags.newFormat == "" {
			fail(errors.New("Please supply the next format version with --new-format"))
		}

		_, err := strconv.Atoi(buildFlags.newFormat)
		if err != nil {
			fail(errors.New("Please supply a valid format version with --new-format"))
		}

		b, err := builder.NewFromConfig(configFile)
		if err != nil {
			fail(err)
		}

		// Set mixversion to +20 build now, we only need to build update since we
		// already have the bundles created
		ver, err := strconv.Atoi(b.MixVer)
		if err != nil {
			fail(err)
		}
		if err = b.UpdateMixVer(ver + 10); err != nil {
			failf("Couldn't update Mix Version")
		}

		// Set format to format+1 for future builds
		if err = b.UpdateFormatVersion(buildFlags.newFormat); err != nil {
			fail(err)
		}

		setWorkers(b)

		if err = b.ModifyBundles(b.RemoveBundlesGroupINI); err != nil {
			fail(err)
		}

		minver, err := strconv.Atoi(b.MixVer)
		if err != nil {
			fail(err)
		}

		// Build the +20 update so we don't have to switch tooling in between
		params := builder.UpdateParameters{
			MinVersion:    minver,
			Format:        buildFlags.newFormat,
			Publish:       !buildFlags.noPublish,
			SkipSigning:   buildFlags.noSigning,
			SkipFullfiles: buildFlags.skipFullfiles,
			SkipPacks:     buildFlags.skipPacks,
		}
		err = b.BuildUpdate(params)
		if err != nil {
			failf("Couldn't build update: %s", err)
		}

		// The PREVIOUS_MIX_VERSION value in mixer.state replaced the LAST_VER file for
		// setting the previous version in the manifest header field. The PREVIOUS_MIX_VERSION
		// value is set to the LAST_VER during format bumps to maintain consistent behavior.
		lastVer, err := b.GetLastBuildVersion()
		if err != nil {
			fail(err)
		}
		if err = b.UpdatePreviousMixVersion(lastVer); err != nil {
			fail(err)
		}
	},
}

var buildUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Build the update content for your mix",
	Long:  `Build the update content for your mix`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := checkRoot(); err != nil {
			fail(err)
		}

		b, err := builder.NewFromConfig(configFile)
		if err != nil {
			fail(err)
		}
		setWorkers(b)
		params := builder.UpdateParameters{
			MinVersion:    buildFlags.minVersion,
			Format:        buildFlags.format,
			Publish:       !buildFlags.noPublish,
			SkipSigning:   buildFlags.noSigning,
			SkipFullfiles: buildFlags.skipFullfiles,
			SkipPacks:     buildFlags.skipPacks,
		}
		err = b.BuildUpdate(params)
		if err != nil {
			failf("Couldn't build update: %s", err)
		}

		if buildFlags.increment {
			if err = b.UpdatePreviousMixVersion(b.MixVer); err != nil {
				fail(err)
			}
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
		if err := checkRoot(); err != nil {
			fail(err)
		}

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
		err = buildBundles(b, buildFlags.noSigning, buildFlags.downloadRetries)
		if err != nil {
			failf("Couldn't build bundles: %s", err)
		}
		params := builder.UpdateParameters{
			MinVersion:    buildFlags.minVersion,
			Format:        buildFlags.format,
			Publish:       !buildFlags.noPublish,
			SkipSigning:   buildFlags.noSigning,
			SkipFullfiles: buildFlags.skipFullfiles,
			SkipPacks:     buildFlags.skipPacks,
		}
		err = b.BuildUpdate(params)
		if err != nil {
			failf("Couldn't build update: %s", err)
		}

		if buildFlags.increment {
			if err = b.UpdatePreviousMixVersion(b.MixVer); err != nil {
				fail(err)
			}
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

var buildValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate manifests correctly generated between two versions",
	Long:  `Validate manifests correctly generated between two versions`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := checkRoot(); err != nil {
			fail(err)
		}

		b, err := builder.NewFromConfig(configFile)
		if err != nil {
			fail(err)
		}
		setWorkers(b)

		tableWidth := buildFlags.tableWidth
		if tableWidth == 0 {
			if tableWidth, err = builder.TerminalWidth(); err != nil {
				fmt.Fprintf(os.Stderr, "Cannot determine default MCA statistics table width, disabling")
				tableWidth = -1
			}
		}
		if tableWidth >= 0 && tableWidth < builder.MinMcaTableWidth {
			fmt.Fprintf(os.Stderr, "MCA statistics table width less than minimum: %d, disabling\n", builder.MinMcaTableWidth)
			tableWidth = -1
		}

		err = b.CheckManifestCorrectness(buildFlags.from, buildFlags.to, buildFlags.downloadRetries, tableWidth, *buildFlags.fromRepoURLs, *buildFlags.toRepoURLs)
		if err != nil {
			fail(err)
		}
	},
}

var buildImageCmd = &cobra.Command{
	Use:   "image",
	Short: "Build an image from the mix content",
	Long:  `Build an image from the mix content`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := checkRoot(); err != nil {
			fail(err)
		}

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

var buildDeltaManifestsCmd = &cobra.Command{
	Use:   "delta-manifests",
	Short: "Build delta manifests used to optimize update between versions",
	Long: `Build delta manifests used to optimize update between versions

When a swupd client updates content, it uses manifest files to get file
metadata. If the current versions manifests already exists on the system
and the delta manifest files are available on the server, the client will
attempt to apply the delta manifests to create the new version manifests
rather than downloading the full manifest directly. If a bundle hasn't changed
between two versions, no delta manifest needs to be generated.

To generate the delta manifests to optimize update from VER to the current mix
version use

    mixer build delta-manifests --from VER

Alternatively, to generate delta manifests for a set of NUM previous versions,
each one to the current mix version, instead of --from use

    mixer build delta-manifests --previous-versions NUM

To change the target version (by default the current version), use the
flag --to. The target version must be larger than the --from version.

`,
	RunE: runBuildDeltaManifests,
}

var buildDeltaPacksFlags struct {
	previousVersions uint32
	from             uint32
	to               uint32
	report           bool
}

var buildDeltaManifestsFlags struct {
	previousVersions uint32
	from             uint32
	to               uint32
}

func runBuildDeltaPacks(cmd *cobra.Command, args []string) error {
	if err := checkRoot(); err != nil {
		fail(err)
	}

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

func runBuildDeltaManifests(cmd *cobra.Command, args []string) error {
	if err := checkRoot(); err != nil {
		fail(err)
	}

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
		err = b.BuildDeltaManifests(buildDeltaManifestsFlags.from, buildDeltaManifestsFlags.to)
	} else {
		err = b.BuildDeltaManifestsPreviousVersions(buildDeltaManifestsFlags.previousVersions, buildDeltaManifestsFlags.to)
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
	cmd.Flags().BoolVar(&buildFlags.noPublish, "no-publish", false, "Do not update the latest version after update")
	cmd.Flags().BoolVar(&buildFlags.skipFullfiles, "skip-fullfiles", false, "Do not generate fullfiles")
	cmd.Flags().BoolVar(&buildFlags.skipPacks, "skip-packs", false, "Do not generate zero packs")

	var unusedStringFlag string
	cmd.Flags().StringVar(&unusedStringFlag, "prefix", "", "Supply prefix for where the swupd binaries live")
	_ = cmd.Flags().MarkHidden("prefix")
	_ = cmd.Flags().MarkDeprecated("prefix", "this flag is ignored by the update builder")
	var unusedBoolFlag bool
	cmd.Flags().BoolVar(&unusedBoolFlag, "keep-chroots", false, "Keep individual chroots created and not just consolidated 'full'")
	_ = cmd.Flags().MarkHidden("keep-chroots")
	_ = cmd.Flags().MarkDeprecated("keep-chroots", "this flag is ignored by the update builder")
}

var buildCmds = []*cobra.Command{
	buildBundlesCmd,
	buildUpdateCmd,
	buildAllCmd,
	buildValidateCmd,
	buildDeltaPacksCmd,
	buildDeltaManifestsCmd,
	buildFormatBumpCmd,
	buildUpstreamFormatCmd,
	buildImageCmd,
}

var bumpCmds = []*cobra.Command{
	buildFormatNewCmd,
	buildFormatOldCmd,
}

func init() {
	for _, cmd := range buildCmds {
		buildCmd.AddCommand(cmd)
	}

	for _, cmd := range bumpCmds {
		addMarker(cmd, bumpMarker)
		buildFormatBumpCmd.AddCommand(cmd)
	}

	addMarker(buildUpstreamFormatCmd, bumpMarker)

	buildFormatBumpCmd.Flags().StringVar(&buildFlags.newFormat, "new-format", "", "Supply the next format version to build mixes in")
	buildFormatOldCmd.Flags().StringVar(&buildFlags.newFormat, "new-format", "", "Supply the next format version to build mixes in")
	buildFormatNewCmd.Flags().StringVar(&buildFlags.newFormat, "new-format", "", "Supply the next format version to build mixes in")
	buildUpstreamFormatCmd.Flags().StringVar(&buildFlags.newFormat, "new-format", "", "Supply the next format version to build mixes in")

	buildCmd.PersistentFlags().IntVar(&buildFlags.numFullfileWorkers, "fullfile-workers", 0, "Number of parallel workers when creating fullfiles, 0 means number of CPUs")
	buildCmd.PersistentFlags().IntVar(&buildFlags.numDeltaWorkers, "delta-workers", 0, "Number of parallel workers when creating deltas, 0 means number of CPUs")
	buildCmd.PersistentFlags().IntVar(&buildFlags.numBundleWorkers, "bundle-workers", 0, "Number of parallel workers when building bundles, 0 means number of CPUs")
	buildCmd.PersistentFlags().IntVar(&buildFlags.downloadRetries, "retries", retriesDefault, "Number of retry attempts to download RPMs")
	buildCmd.PersistentFlags().BoolVar(&buildFlags.skipFormatCheck, "skip-format-check", false, "Skip format bump check")

	RootCmd.AddCommand(buildCmd)

	unusedBoolFlag := false

	buildAllCmd.Flags().BoolVar(&unusedBoolFlag, "clean", false, "")
	_ = buildAllCmd.Flags().MarkHidden("clean")
	_ = buildAllCmd.Flags().MarkDeprecated("clean", "The workspace is always cleaned when building bundles, this flag is no longer used")

	buildBundlesCmd.Flags().BoolVar(&unusedBoolFlag, "clean", false, "")
	_ = buildBundlesCmd.Flags().MarkHidden("clean")
	_ = buildBundlesCmd.Flags().MarkDeprecated("clean", "The workspace is always cleaned when building bundles, this flag is no longer used")
	buildBundlesCmd.Flags().BoolVar(&buildFlags.noSigning, "no-signing", false, "Do not generate a certificate to sign the Manifest.MoM")

	buildBundlesCmd.Flags().BoolVar(&unusedBoolFlag, "new-chroots", false, "")
	_ = buildBundlesCmd.Flags().MarkHidden("new-chroots")
	_ = buildBundlesCmd.Flags().MarkDeprecated("new-chroots", "new functionality is now the standard behavior, this flag is obsolete and no longer used")

	buildValidateCmd.Flags().IntVar(&buildFlags.to, "to", 0, "Compare manifests targeting a specific version")
	buildValidateCmd.Flags().IntVar(&buildFlags.from, "from", 0, "Compare manifests from a specific version")
	buildValidateCmd.Flags().IntVar(&buildFlags.tableWidth, "table-width", 0, "Max width of package statistics table, defaults to terminal width and disabled by negative numbers")
	buildFlags.toRepoURLs = buildValidateCmd.Flags().StringToString("to-repo-url", nil, "Overrides the baseurl value for the provided repo in the DNF config file for the `to` version: <repo>=<URL>")
	buildFlags.fromRepoURLs = buildValidateCmd.Flags().StringToString("from-repo-url", nil, "Overrides the baseurl value for the provided repo in the DNF config file for the `from` version: <repo>=<URL>")

	buildImageCmd.Flags().StringVar(&buildFlags.format, "format", "", "Supply the format used for the Mix")
	buildImageCmd.Flags().StringVar(&buildFlags.template, "template", "", "Path to template file to use")

	buildDeltaPacksCmd.Flags().Uint32Var(&buildDeltaPacksFlags.from, "from", 0, "Generate packs from a specific version")
	buildDeltaPacksCmd.Flags().Uint32Var(&buildDeltaPacksFlags.previousVersions, "previous-versions", 0, "Generate packs for multiple previous versions")
	buildDeltaPacksCmd.Flags().Uint32Var(&buildDeltaPacksFlags.to, "to", 0, "Generate packs targeting a specific version")
	buildDeltaPacksCmd.Flags().BoolVar(&buildDeltaPacksFlags.report, "report", false, "Report reason each file in to manifest was packed or not")

	buildDeltaManifestsCmd.Flags().Uint32Var(&buildDeltaManifestsFlags.from, "from", 0, "Generate delta manifests from a specific version")
	buildDeltaManifestsCmd.Flags().Uint32Var(&buildDeltaManifestsFlags.previousVersions, "previous-versions", 0, "Generate delta manifests for multiple previous versions")
	buildDeltaManifestsCmd.Flags().Uint32Var(&buildDeltaManifestsFlags.to, "to", 0, "Generate delta manifests targeting a specific version")

	setUpdateFlags(buildUpdateCmd)
	setUpdateFlags(buildAllCmd)
	setUpdateFlags(buildFormatNewCmd)
	setUpdateFlags(buildFormatOldCmd)

	externalDeps[buildBundlesCmd] = []string{
		"rpm",
		"dnf",
		"rpm2archive",
		"tar",
	}
	externalDeps[buildUpdateCmd] = []string{
		"openssl",
		"xz",
	}
	externalDeps[buildImageCmd] = []string{
		"clr-installer",
	}
	externalDeps[buildAllCmd] = append(
		externalDeps[buildBundlesCmd],
		append(externalDeps[buildUpdateCmd],
			externalDeps[buildImageCmd]...)...)
}

func checkFormatBump(b *builder.Builder) error {
	// Skip check if offline
	if builder.Offline || buildFlags.skipFormatCheck {
		return nil
	}

	if bumpNeeded, err := b.CheckBumpNeeded(false); err != nil {
		return err
	} else if bumpNeeded {
		return errors.New("ERROR: Format bump required")
	}

	return nil
}
