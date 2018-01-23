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
	allLocal    bool
	allUpstream bool
	git         bool
}

var bundleAddFlags bundleAddCmdFlags

var bundleAddCmd = &cobra.Command{
	Use:   "add [bundle(s)]",
	Short: "Add local or upstream bundles to your mix",
	Long: `Adds local or upstream bundles to your mix by modifying the Mix Bundle List
(stored in the 'mixbundles' file). The Mix Bundle List is parsed, the new
bundles are confirmed to exist and are added, duplicates are removed, and the
resultant list is written back out in sorted order.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var bundles []string
		if !bundleAddFlags.allLocal && !bundleAddFlags.allUpstream {
			if len(args) == 0 {
				return errors.New("bundle add requires at least 1 argument if neither --all-local nor --all-upstream are passed")
			}
			bundles = strings.Split(args[0], ",")
		}
		b, err := builder.NewFromConfig(config)
		if err != nil {
			fail(err)
		}
		err = b.AddBundles(bundles, bundleAddFlags.allLocal, bundleAddFlags.allUpstream, bundleAddFlags.git)
		if err != nil {
			fail(err)
		}
		return nil
	},
}

// Bundle list command ('mixer bundle list')
type bundleListCmdFlags struct {
	tree bool
}

var bundleListFlags bundleListCmdFlags

var bundleListCmd = &cobra.Command{
	Use:   "list [mix|local|upstream]",
	Short: "List bundles",
	Long: `List either:
  mix       The bundles in the mix, recursively following includes (DEFAULT)
  local     The available local bundles
  upstream  The available upstream bundles`,
	Args:      cobra.OnlyValidArgs,
	ValidArgs: []string{"mix", "local", "upstream"},
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 1 {
			return errors.New("bundle list takes at most one argument")
		}

		b, err := builder.NewFromConfig(config)
		if err != nil {
			fail(err)
		}

		var listType string
		if len(args) > 0 {
			listType = args[0]
		} else {
			listType = "mix"
		}

		switch listType {
		case "upstream":
			err = b.ListBundles(builder.UpstreamList, bundleListFlags.tree)
		case "local":
			err = b.ListBundles(builder.LocalList, bundleListFlags.tree)
		default:
			err = b.ListBundles(builder.MixList, bundleListFlags.tree)
		}
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
	bundleAddCmd.Flags().BoolVar(&bundleAddFlags.allLocal, "all-local", false, "Add all local bundles; takes precedence over bundle list")
	bundleAddCmd.Flags().BoolVar(&bundleAddFlags.allUpstream, "all-upstream", false, "Add all upstream bundles; takes precedence over bundle list")
	bundleAddCmd.Flags().BoolVar(&bundleAddFlags.git, "git", false, "Automatically apply new git commit")

	bundleListCmd.Flags().BoolVar(&bundleListFlags.tree, "tree", false, "Pretty-print the list as a tree.")
}
