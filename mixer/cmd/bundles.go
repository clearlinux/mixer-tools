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

type bundleCmdFlags struct {
	alllocal    bool
	allupstream bool
	git         bool
}

var bundleFlags bundleCmdFlags

var bundleCmd = &cobra.Command{
	Use:   "bundle",
	Short: "Perform various actions on bundles",
}

var addBundlesCmd = &cobra.Command{
	Use:   "add [bundle(s)]",
	Short: "Add local or upstream bundles to your mix",
	Long:  `Add local or upstream bundles to your mix`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var bundles []string
		if bundleFlags.alllocal == false && bundleFlags.allupstream == false {
			if len(args) == 0 {
				return errors.New("bundle add requires at least 1 argument neither --all-local nor --all-upstream are passed")
			}
			bundles = strings.Split(args[0], ",")
		}
		b, err := builder.NewFromConfig(config)
		if err != nil {
			fail(err)
		}
		err = b.AddBundles(bundles, bundleFlags.alllocal, bundleFlags.allupstream, bundleFlags.git)
		if err != nil {
			fail(err)
		}
		return nil
	},
}

var bundlesCmds = []*cobra.Command{
	addBundlesCmd,
	// Leaving this in place because more are coming soon
}

func init() {
	for _, cmd := range bundlesCmds {
		bundleCmd.AddCommand(cmd)
		cmd.Flags().StringVarP(&config, "config", "c", "", "Builder config to use")
	}

	RootCmd.AddCommand(bundleCmd)

	addBundlesCmd.Flags().BoolVar(&bundleFlags.alllocal, "all-local", false, "Add all local bundles; takes precedence over bundle list")
	addBundlesCmd.Flags().BoolVar(&bundleFlags.allupstream, "all-upstream", false, "Add all upstream bundles; takes precedence over bundle list")
	addBundlesCmd.Flags().BoolVar(&bundleFlags.git, "git", false, "Automatically apply new git commit")
}
