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

package cmd

import (
	"strconv"

	"github.com/clearlinux/mixer-tools/builder"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var versionsCmd = &cobra.Command{
	Use:   "versions",
	Short: "Manage mix and upstream versions",
	Long: `Manage mix and upstream versions. By itself the command
will print the current version of mix and upstream, and also report on
the latest version of upstream available.

Use 'mixer versions update' to increment the mix version and
optionally the upstream version.
`,
	Run: runVersions,
}

var versionsUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update mix and optionally upstream versions",
	Long: `Increment the mix version to generate a new release. To
also update the upstream version used, pass either --upstream-version NN
or --upstream-version latest.

This command will not update to an upstream version of a different
format ("format bumps"). At the moment this needs to be handled
manually.

By default the mix version is incremented by 10, following Clear Linux
conventions to leave room for some intermediate versions if
necessary. The increment can be configured with the --increment flag.
`,
	Run: runVersionsUpdate,
}

var versionsUpdateFlags struct {
	mixVersion      uint32
	upstreamVersion string // Accepts "latest".
	increment       uint32
}

func init() {
	versionsCmd.AddCommand(versionsUpdateCmd)
	RootCmd.AddCommand(versionsCmd)

	versionsUpdateCmd.Flags().StringVarP(&configFile, "config", "c", "", "Builder config to use")
	versionsUpdateCmd.Flags().Uint32Var(&versionsUpdateFlags.mixVersion, "mix-version", 0, "Set a specific mix version")
	versionsUpdateCmd.Flags().StringVar(&versionsUpdateFlags.upstreamVersion, "upstream-version", "", "Next upstream version (either version number or 'latest')")
	versionsUpdateCmd.Flags().StringVar(&versionsUpdateFlags.upstreamVersion, "clear-version", "", "Alias to --upstream-version")
	versionsUpdateCmd.Flags().Uint32Var(&versionsUpdateFlags.increment, "increment", 10, "Amount to increment current mix version")
}

func runVersions(cmd *cobra.Command, args []string) {
	b, err := builder.NewFromConfig(configFile)
	if err != nil {
		fail(err)
	}
	err = b.PrintVersions()
	if err != nil {
		fail(err)
	}
}

func runVersionsUpdate(cmd *cobra.Command, args []string) {
	b, err := builder.NewFromConfig(configFile)
	if err != nil {
		fail(err)
	}

	var nextMix uint32
	if versionsUpdateFlags.mixVersion > 0 {
		nextMix = versionsUpdateFlags.mixVersion
	} else {
		nextMix = b.MixVerUint32 + versionsUpdateFlags.increment
	}

	var nextUpstream uint32
	switch versionsUpdateFlags.upstreamVersion {
	case "":
		nextUpstream = b.UpstreamVerUint32
	case "latest":
		nextUpstream = 0
	default:
		nextUpstream, err = parseUint32(versionsUpdateFlags.upstreamVersion)
		if err != nil {
			fail(err)
		}
	}

	err = b.UpdateVersions(nextMix, nextUpstream)
	if err != nil {
		fail(err)
	}
}

func parseUint32(s string) (uint32, error) {
	parsed, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return 0, errors.Wrapf(err, "error parsing value %q", s)
	}
	return uint32(parsed), nil
}
