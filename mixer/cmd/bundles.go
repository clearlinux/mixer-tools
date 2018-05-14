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
	Use:   "add <bundle> [<bundle>...]",
	Short: "Add local or upstream bundles to your mix",
	Long: `Adds local or upstream bundles to your mix by modifying the Mix Bundle List
(stored in the 'mixbundles' file). The Mix Bundle List is parsed, the new
bundles are confirmed to exist and are added, duplicates are removed, and the
resultant list is written back out in sorted order.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !bundleAddFlags.allLocal && !bundleAddFlags.allUpstream {
			if len(args) == 0 {
				return errors.New("bundle add requires at least 1 argument if neither --all-local nor --all-upstream are passed")
			}
		}

		b, err := builder.NewFromConfig(configFile)
		if err != nil {
			fail(err)
		}

		err = b.AddBundles(args, bundleAddFlags.allLocal, bundleAddFlags.allUpstream, bundleAddFlags.git)
		if err != nil {
			fail(err)
		}

		return nil
	},
}

// Bundle remove command ('mixer bundle remove')
type bundleRemoveCmdFlags struct {
	mix   bool
	local bool
	git   bool
}

var bundleRemoveFlags bundleRemoveCmdFlags

var bundleRemoveCmd = &cobra.Command{
	Use:   "remove <bundle> [<bundle>...]",
	Short: "Remove bundles from your mix",
	Long: `Removes bundles from your mix by modifying the Mix Bundle List
(stored in the 'mixbundles' file). The Mix Bundle List is parsed, the bundles
are removed, and the resultant list is written back out in sorted order. If
bundles do not exist in the mix, they are skipped.

Passing '--local' will also remove the corresponding bundle definition file from
local-bundles, if it exists. Please note that this is an irrevocable step.

'--mix' defaults to true. Passing '--mix=false' will prevent the bundle from
being removed from your Mix Bundle List. This is useful when used in conjunction
with '--local' to *only* remove a bundle from local-bundles. If the bundle being
removed is an edited version from upstream, the bundle will remain in your mix
and now reference the original upstream version. If the bundle was custom, and
no upstream alternative exists, a warning will be returned.`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		b, err := builder.NewFromConfig(configFile)
		if err != nil {
			fail(err)
		}

		err = b.RemoveBundles(args, bundleRemoveFlags.mix, bundleRemoveFlags.local, bundleRemoveFlags.git)
		if err != nil {
			fail(err)
		}
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

		b, err := builder.NewFromConfig(configFile)
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
	Use:   "edit <bundle> [<bundle>...]",
	Short: "Edit local and upstream bundles, or create new bundles",
	Long: `Edits local and upstream bundle definition files. This command will locate the
bundle (looking first in local-bundles, then in upstream-bundles), and launch
an editor to edit it. If the bundle is only found upstream, the bundle file will
first be copied to your local-bundles directory for editing. If the bundle is
not found anywhere, a blank template is created with the correct name. When the
editor closes, the bundle file is then parsed for validity.

The editor is configured via environment variables. VISUAL takes precedence to
EDITOR. If neither are set, the tool defaults to nano. If nano is not installed,
the tool will skip editing, and act as if '--suppress-editor' had been passed.

Passing '--suppress-editor' will suppress launching the editor, and will thus
only copy the bundle file to local-bundles (if it is only found upstream), or
create the blank template (if it was not found anywhere). This can be useful if
you want to add a bundle to local-bundles, but wish to edit it at a later time.

Passing '--add' will also add the bundle(s) to your mix. Please note that
bundles are added after all bundles are edited, and thus will not be added if
any errors are encountered earlier on.`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		b, err := builder.NewFromConfig(configFile)
		if err != nil {
			fail(err)
		}

		err = b.EditBundles(args, bundleEditFlags.copyOnly, bundleEditFlags.add, bundleEditFlags.git)
		if err != nil {
			fail(err)
		}
	},
}

// Bundle validate command ('mixer bundle validate')
type bundleValidateCmdFlags struct {
	allLocal bool
	strict   bool
}

var bundleValidateFlags bundleValidateCmdFlags

var bundleValidateCmd = &cobra.Command{
	Use:   "validate <bundle> [<bundle>...]",
	Short: "Validate local bundle definition files",
	Long: `Checks bundle definition files for validity. Only local bundle files are
checked; upstream bundles are trusted as valid. Valid bundles yield no output.
Any invalid bundles will yield a non-zero return code.

Basic validation includes checking syntax and structure, and that the bundle has
a valid name. Commands like 'mixer bundle edit' run basic validation
automatically.

An optional '--strict' flag allows you to additionally check that the bundle 
header fields are parsable and non-empty, and that the header 'Title' is itself
valid and matches the bundle filename.

Passing '--all-local' will run validation on all bundles in local-bundles.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !bundleValidateFlags.allLocal && len(args) == 0 {
			return errors.New("bundle validate requires at least 1 argument if --all-local is not passed")
		}

		b, err := builder.NewFromConfig(configFile)
		if err != nil {
			fail(err)
		}

		var lvl builder.ValidationLevel
		if bundleValidateFlags.strict {
			lvl = builder.StrictValidation
		} else {
			lvl = builder.BasicValidation
		}

		if bundleValidateFlags.allLocal {
			err = b.ValidateLocalBundles(lvl)
		} else {
			err = b.ValidateBundles(args, lvl)
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
	bundleRemoveCmd,
	bundleListCmd,
	bundleEditCmd,
	bundleValidateCmd,
}

func init() {
	for _, cmd := range bundlesCmds {
		bundleCmd.AddCommand(cmd)
		cmd.Flags().StringVarP(&configFile, "config", "c", "", "Builder config to use")
	}

	RootCmd.AddCommand(bundleCmd)
	bundleAddCmd.Flags().BoolVar(&bundleAddFlags.allLocal, "all-local", false, "Add all local bundles; takes precedence over bundle list")
	bundleAddCmd.Flags().BoolVar(&bundleAddFlags.allUpstream, "all-upstream", false, "Add all upstream bundles; takes precedence over bundle list")
	bundleAddCmd.Flags().BoolVar(&bundleAddFlags.git, "git", false, "Automatically apply new git commit")

	bundleRemoveCmd.Flags().BoolVar(&bundleRemoveFlags.mix, "mix", true, "Remove bundle from Mix Bundle List")
	bundleRemoveCmd.Flags().BoolVar(&bundleRemoveFlags.local, "local", false, "Also remove bundle file from local-bundles (irrevocable)")
	bundleRemoveCmd.Flags().BoolVar(&bundleRemoveFlags.git, "git", false, "Automatically apply new git commit")

	bundleListCmd.Flags().BoolVar(&bundleListFlags.tree, "tree", false, "Pretty-print the list as a tree.")

	bundleEditCmd.Flags().BoolVar(&bundleEditFlags.copyOnly, "suppress-editor", false, "Suppress launching editor (only copy to local-bundles or create template)")
	bundleEditCmd.Flags().BoolVar(&bundleEditFlags.add, "add", false, "Add the bundle(s) to your mix")
	bundleEditCmd.Flags().BoolVar(&bundleEditFlags.git, "git", false, "Automatically apply new git commit")

	bundleValidateCmd.Flags().BoolVar(&bundleValidateFlags.allLocal, "all-local", false, "Validate all local bundles")
	bundleValidateCmd.Flags().BoolVar(&bundleValidateFlags.strict, "strict", false, "Strict validation (see usage)")
}
