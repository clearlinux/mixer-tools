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
	"net/url"
	"strings"

	"github.com/clearlinux/mixer-tools/builder"

	"github.com/spf13/cobra"
)

// Top level repo command ('mixer repo')
var repoCmd = &cobra.Command{
	Use:   "repo",
	Short: "Add, list, or remove RPM repositories for use by mixer",
}

var addRepoCmd = &cobra.Command{
	Use:   "add <name> <url>",
	Short: "Add repo to DNF conf file",
	Long:  `Add repo <name> with <url> to the DNF conf file used by mixer. The <url> must be an absolute path.`,
	Args:  cobra.ExactArgs(2),
	Run:   runAddRepo,
}

var removeRepoCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove repo from DNF conf file",
	Long:  `Remove repo <name> from the DNF conf file used by mixer.`,
	Args:  cobra.ExactArgs(1),
	Run:   runRemoveRepo,
}

var listReposCmd = &cobra.Command{
	Use:   "list",
	Short: "List all repos in DNF conf file",
	Long:  `List all the repos in the DNF conf file used by mixer.`,
	Run:   runListRepos,
}

var initRepoCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize DNF conf file with default repos",
	Long:  `Initialize the DNF conf file with the default 'Clear' and 'Local' repos enabled.`,
	Run:   runInitRepo,
}

var setURLRepoCmd = &cobra.Command{
	Use:   "set-url <name> <url>",
	Short: "Set URL of repo in DNF conf file",
	Long: `Set repo <name> with <url> to the DNF conf file used by mixer. The <url> must be an absolute path.
If repo <name> does not exist, the repo will be added to the conf file.`,
	Args: cobra.ExactArgs(2),
	Run:  runSetURLRepo,
}

var setExcludesRepoCmd = &cobra.Command{
	Use:   "exclude <repo> <pkg> [<pkg>...]",
	Short: "Exclude packages from specified repo",
	Long: `Exclude packages from a specified repo. These packages will be ignored
during build bundles, which allows the user to explicitly select which packages
to use when building bundles by excluding the unwanted ones. Globbing is supported.`,
	Args: cobra.MinimumNArgs(2),
	Run:  runExcludesRepo,
}

var repoCmds = []*cobra.Command{
	addRepoCmd,
	removeRepoCmd,
	listReposCmd,
	initRepoCmd,
	setURLRepoCmd,
	setExcludesRepoCmd,
}

func init() {
	for _, cmd := range repoCmds {
		repoCmd.AddCommand(cmd)
	}

	RootCmd.AddCommand(repoCmd)
}

func runExcludesRepo(cmd *cobra.Command, args []string) {
	b, err := builder.NewFromConfig(configFile)
	if err != nil {
		fail(err)
	}

	err = b.SetExcludesRepo(args[0], strings.Join(args[1:], " "))
	if err != nil {
		fail(err)
	}
	fmt.Printf("Excluded packages from repo %s:\n%s\n", args[0], strings.Join(args[1:], "\n"))
}

func runAddRepo(cmd *cobra.Command, args []string) {
	b, err := builder.NewFromConfig(configFile)
	if err != nil {
		fail(err)
	}
	u, err := url.Parse(args[1])
	if err != nil {
		fail(err)
	}
	if u.Scheme == "" {
		u.Scheme = "file"
	}
	err = b.AddRepo(args[0], u.String())
	if err != nil {
		fail(err)
	}
	fmt.Printf("Adding repo %s with url %s \n", args[0], u.String())
}

func runRemoveRepo(cmd *cobra.Command, args []string) {
	b, err := builder.NewFromConfig(configFile)
	if err != nil {
		fail(err)
	}

	err = b.RemoveRepo(args[0])
	if err != nil {
		fail(err)
	}
}

func runListRepos(cmd *cobra.Command, args []string) {
	b, err := builder.NewFromConfig(configFile)
	if err != nil {
		fail(err)
	}

	err = b.ListRepos()
	if err != nil {
		fail(err)
	}
}

func runInitRepo(cmd *cobra.Command, args []string) {
	b, err := builder.NewFromConfig(configFile)
	if err != nil {
		fail(err)
	}

	err = b.NewDNFConfIfNeeded()
	if err != nil {
		fail(err)
	}
}

func runSetURLRepo(cmd *cobra.Command, args []string) {
	b, err := builder.NewFromConfig(configFile)
	if err != nil {
		fail(err)
	}

	u, err := url.Parse(args[1])
	if err != nil {
		fail(err)
	}
	if u.Scheme == "" {
		u.Scheme = "file"
	}

	err = b.SetURLRepo(args[0], u.String())
	if err != nil {
		fail(err)
	}

	fmt.Printf("Setting repo %s with url %s\n", args[0], u.String())
}
