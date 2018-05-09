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
	"os"
	"path/filepath"
	"runtime"

	"github.com/clearlinux/mixer-tools/builder"
	"github.com/clearlinux/mixer-tools/helpers"

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
	err := addPackage(args[0], args[1], packageAddFlags.build)
	if err != nil {
		fail(err)
	}
	fmt.Printf("Added %s package to %s bundle.\n", args[0], args[1])
}

func addPackage(pkg, bundle string, build bool) error {
	var err error

	ver, err := getCurrentVersion()
	if err != nil {
		return err
	}
	mixVer := ver * 1000
	err = setUpMixDirIfNeeded(bundle, ver, mixVer)
	if err != nil {
		return err
	}

	err = os.Chdir(mixWS)
	if err != nil {
		return err
	}

	b, err := builder.NewFromConfig(filepath.Join(mixWS, "builder.conf"))
	if err != nil {
		return err
	}
	err = b.InitMix(fmt.Sprintf("%d", ver), fmt.Sprintf("%d", mixVer),
		false, false, "https://download.clearlinux.org", false)
	if err != nil {
		return err
	}
	b.NumBundleWorkers = runtime.NumCPU()
	b.NumFullfileWorkers = runtime.NumCPU()

	err = b.EditBundles([]string{bundle}, true, true, false)
	if err != nil {
		return err
	}

	err = appendToFile(filepath.Join(mixWS, "local-bundles", bundle), fmt.Sprintf("%s\n", pkg))
	if err != nil {
		return err
	}

	rpms, err := helpers.ListVisibleFiles(b.Config.Mixer.LocalRPMDir)
	if err != nil {
		return err
	}

	err = b.AddRPMList(rpms)
	if err != nil {
		return err
	}

	if build {
		return buildMix(false)
	}

	return nil
}