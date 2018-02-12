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

// Bundle Edit command ('mixer bundle edit')
type bundleEditCmdFlags struct {
	copyOnly bool
	add      bool
	git      bool
}

var bundleEditFlags bundleEditCmdFlags

var bundleEditCmd = &cobra.Command{
	Use:   "edit [bundle(s)]",
	Short: "Edit local and upstream bundles",
	Long: `Edit local and upstream bundle definition files. This command will locate the
bundle (looking first in local-bundles, then in upstream-bundles), and launch
an editor to edit it. If the bundle is only found upstream, the bundle file will
first be copied to your local-bundles directory for editing. When the editor
closes, the bundle file is then parsed for validity.

The editor is configured via environment variables. VISUAL takes precedence to
EDITOR. If neither are set, the tool defaults to nano. If nano is not installed,
the tool will skip editing, and act as if '--copy-only' had been passed.

Passing '--copy-only' will suppress launching the editor, and will thus only
copy the bundle file to local-bundles (if it is only found upstream). This can
be useful if you want to add a bundle to local-bundles, but wish to edit it at a
later time.

Passing '--add' will also add the bundle(s) to your mix. Please note that
bundles are added after all bundles are edited, and thus will not be added if
any errors are encountered earlier on.`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		b, err := builder.NewFromConfig(config)
		if err != nil {
			fail(err)
		}

		err = b.EditBundles(args, bundleEditFlags.copyOnly, bundleEditFlags.add, bundleEditFlags.git)
		if err != nil {
			fail(err)
		}
	},
}

// Bundle Edit command ('mixer bundle edit')
type bundleCreateCmdFlags struct {
	createOnly bool
	add        bool
	git        bool
}

var bundleCreateFlags bundleCreateCmdFlags

var bundleCreateCmd = &cobra.Command{
	Use:   "create [bundle(s)]",
	Short: "Create custom local bundles",
	Long: `Create custom local bundle definition files. This command will generate an
empty bundle definition file template with the appropriate name in your
local-bundles directory, and launch an editor to edit it. When the editor
closes, the bundle file is then parsed for validity.

The editor is configured via environment variables. VISUAL takes precedence to
EDITOR. If neither are set, the tool defaults to nano. If nano is not installed,
the tool will skip editing, and act as if '--create-only' had been passed.

Passing '--create-only' will suppress launching the editor, and will thus only
create the empty bundle template in local-bundles. This can be useful if you
want to create a new bundle, but wish to edit it at a later time.

Passing '--add' will also add the bundle(s) to your mix. Please note that
bundles are added after all bundles are created, and thus will not be added if
any errors are encountered earlier on.`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		b, err := builder.NewFromConfig(config)
		if err != nil {
			fail(err)
		}

		err = b.CreateBundles(args, bundleCreateFlags.createOnly, bundleCreateFlags.add, bundleCreateFlags.git)
		if err != nil {
			fail(err)
		}
	},
}

// List of all bundle commands
var bundlesCmds = []*cobra.Command{
	bundleAddCmd,
	bundleListCmd,
	bundleEditCmd,
	bundleCreateCmd,
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

	bundleEditCmd.Flags().BoolVar(&bundleEditFlags.copyOnly, "copy-only", false, "Suppress launching editor (only copy to local-bundles if upstream)")
	bundleEditCmd.Flags().BoolVar(&bundleEditFlags.add, "add", false, "Add the bundle(s) to your mix")
	bundleEditCmd.Flags().BoolVar(&bundleEditFlags.git, "git", false, "Automatically apply new git commit")

	bundleCreateCmd.Flags().BoolVar(&bundleCreateFlags.createOnly, "create-only", false, "Suppress launching editor (only create empty template in local-bundles)")
	bundleCreateCmd.Flags().BoolVar(&bundleCreateFlags.add, "add", false, "Add the bundle(s) to your mix")
	bundleCreateCmd.Flags().BoolVar(&bundleCreateFlags.git, "git", false, "Automatically apply new git commit")
}
