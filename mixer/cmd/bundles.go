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
	"strings"

	"github.com/clearlinux/mixer-tools/builder"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

type bundleCmdFlags struct {
	all   bool
	force bool
	git   bool
}

var bundleFlags bundleCmdFlags

var bundleCmd = &cobra.Command{
	Use:   "bundle",
	Short: "Perform various actions on bundles",
}

var addBundlesCmd = &cobra.Command{
	Use:   "add [bundle(s)]",
	Short: "Add clr-bundles to your mix",
	Long:  `Add clr-bundles to your mix`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var bundles []string
		if bundleFlags.all == false {
			if len(args) == 0 {
				return errors.New("bundle add requires at least 1 argument if --all is not passed")
			}
			bundles = strings.Split(args[0], ",")
		}
		b := builder.NewFromConfig(config)
		numadded, err := b.AddBundles(bundles, bundleFlags.force, bundleFlags.all, bundleFlags.git)
		fmt.Println(numadded, " bundles were added")
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

	addBundlesCmd.Flags().BoolVar(&bundleFlags.force, "force", false, "Override bundles that already exist")
	addBundlesCmd.Flags().BoolVar(&bundleFlags.all, "all", false, "Add all bundles from CLR; takes precedence over -bundles")
	addBundlesCmd.Flags().BoolVar(&bundleFlags.git, "git", false, "Automatically apply new git commit")
}
