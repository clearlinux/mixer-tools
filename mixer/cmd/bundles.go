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
	"strings"

	"github.com/clearlinux/mixer-tools/builder"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

// Top level bundle command ('mixer bundle')
var bundleCmd = &cobra.Command{
	Use:   "bundle",
	Short: "Perform various actions on bundles",
}

// Bundle add command ('mixer bundle add')
type bundleAddCmdFlags struct {
	alllocal    bool
	allupstream bool
	git         bool
}

var bundleAddFlags bundleAddCmdFlags

var bundleAddCmd = &cobra.Command{
	Use:   "add [bundle(s)]",
	Short: "Add local or upstream bundles to your mix",
	Long:  `Add local or upstream bundles to your mix`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var bundles []string
		if bundleAddFlags.alllocal == false && bundleAddFlags.allupstream == false {
			if len(args) == 0 {
				return errors.New("bundle add requires at least 1 argument neither --all-local nor --all-upstream are passed")
			}
			bundles = strings.Split(args[0], ",")
		}
		b, err := builder.NewFromConfig(config)
		if err != nil {
			fail(err)
		}
		err = b.AddBundles(bundles, bundleAddFlags.alllocal, bundleAddFlags.allupstream, bundleAddFlags.git)
		if err != nil {
			fail(err)
		}
		return nil
	},
}

// Bundle list command ('mixer bundle list')
type bundleListCmdFlags struct {
	full     bool
	local    bool
	upstream bool
}

var bundleListFlags bundleListCmdFlags

var bundleListCmd = &cobra.Command{
	Use:   "list",
	Short: "List the bundles in the mix",
	Long:  `List the bundles in the mix, local bundles, or upstream bundles`,
	RunE: func(cmd *cobra.Command, args []string) error {
		b, err := builder.NewFromConfig(config)
		if err != nil {
			fail(err)
		}
		err = b.ListBundles(bundleListFlags.full, bundleListFlags.local, bundleListFlags.upstream)
		if err != nil {
			fail(err)
		}
		return nil
	},
}

// List of all bundle commands
var bundlesCmds = []*cobra.Command{
	bundleAddCmd,
	bundleListCmd,
}

func init() {
	for _, cmd := range bundlesCmds {
		bundleCmd.AddCommand(cmd)
		cmd.Flags().StringVarP(&config, "config", "c", "", "Builder config to use")
	}

	RootCmd.AddCommand(bundleCmd)
	bundleAddCmd.Flags().BoolVar(&bundleAddFlags.alllocal, "all-local", false, "Add all local bundles; takes precedence over bundle list")
	bundleAddCmd.Flags().BoolVar(&bundleAddFlags.allupstream, "all-upstream", false, "Add all upstream bundles; takes precedence over bundle list")
	bundleAddCmd.Flags().BoolVar(&bundleAddFlags.git, "git", false, "Automatically apply new git commit")

	bundleListCmd.Flags().BoolVar(&bundleListFlags.full, "full", false, "List all bundles in the mix, recursively pulling in included bundles")
	bundleListCmd.Flags().BoolVar(&bundleListFlags.local, "local", false, "List all available local bundles")
	bundleListCmd.Flags().BoolVar(&bundleListFlags.upstream, "upstream", false, "List all available upstream bundles")
}
