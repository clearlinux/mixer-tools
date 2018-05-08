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

package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var packageCmd = &cobra.Command{
	Use:   "package",
	Short: "Add packages from remote or local repositories",
	Long: `Add RPM packages from remote or local repositories
for use by mixer-client integration. This command does not perform any checks
to see if the added package exists in any of the configured repository. Therefore
if a user is adding a local package, they must also copy the corresponding RPM
into the local-rpms repository under /usr/share/mix.`,
}

// only addPackage exists for now, but leave 'add' as a subcommand to 'package
// to enable future 'remove' and 'list' commands.
var addPackageCmd = &cobra.Command{
	Use:   "add <package-name> <bundle-name> [options]",
	Short: "Add package <package-name> to the <bundle-name> bundle",
	Long: `Add the package <package-name> to the <bundle-name> bundle.
Optionally add the --build command to immediately update your mix with this new
package/bundle definition. Leave the --build flag off to add multiple packages
before building your mix.`,
	Args: cobra.ExactArgs(1),
	Run:  runAddPackage,
}

var packageCmds = []*cobra.Command{
	addPackageCmd,
}

type packageAddCmdFlags struct {
	build bool
}

var packageAddFlags packageAddCmdFlags

func init() {
	for _, cmd := range packageCmds {
		packageCmd.AddCommand(cmd)
		cmd.Flags().StringVarP(&config, "config", "c", "/usr/share/mix/builder.conf", "Builder config to use")
	}

	addPackageCmd.Flags().BoolVar(&packageAddFlags.build, "build", false, "Build mix update after adding package to bundle")

	RootCmd.AddCommand(packageCmd)
}

func runAddPackage(cmd *cobra.Command, args []string) {
	err := AddPackage(args[0], args[1], packageAddFlags.build)
	if err != nil {
		fail(err)
	}
	fmt.Printf("Added %s package to %s bundle.\n", args[0], args[1])
}
