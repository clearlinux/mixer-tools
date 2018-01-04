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
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"github.com/clearlinux/mixer-tools/builder"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

// Version of Mixer. Also used by the Makefile for releases.
const Version = "3.2.1"

var config string

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:  "mixer",
	Long: `Mixer is a tool used to compose OS update content and images.`,

	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Both --version and --check should work regardless of the regular
		// check for external programs.
		if cmd.Parent() == nil { // This is RootCmd.
			if rootCmdFlags.version {
				fmt.Printf("Mixer %s\n", Version)
				os.Exit(0)
			}
			if rootCmdFlags.check {
				ok := checkAllDeps()
				if !ok {
					os.Exit(1)
				}
				os.Exit(0)
			}
		}
		return checkCmdDeps(cmd)
	},

	Run: func(cmd *cobra.Command, args []string) {
		// Use cmd here to print exactly like other prints of Usage (that might be
		// configurable).
		cmd.Print(cmd.UsageString())
	},
}

var rootCmdFlags = struct {
	version bool
	check   bool
}{}

type initCmdFlags struct {
	all         bool
	clearver    int
	mixver      int
	upstreamurl string
}

var initFlags initCmdFlags

var initCmd = &cobra.Command{
	Use:   "init-mix",
	Short: "Initialize the mixer and workspace",
	Long:  `Initialize the mixer and workspace`,
	Run: func(cmd *cobra.Command, args []string) {
		b := builder.New()
		b.LoadBuilderConf(config)
		b.ReadBuilderConf()
		err := b.InitMix(strconv.Itoa(initFlags.clearver), strconv.Itoa(initFlags.mixver), initFlags.all, initFlags.upstreamurl)
		if err != nil {
			fail(err)
		}
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := RootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	RootCmd.AddCommand(initCmd)
	RootCmd.Flags().BoolVar(&rootCmdFlags.version, "version", false, "Print version information and quit")
	RootCmd.Flags().BoolVar(&rootCmdFlags.check, "check", false, "Check all dependencies needed by mixer and quit")

	initCmd.Flags().BoolVar(&initFlags.all, "all", false, "Create a mix with all Clear bundles included")
	initCmd.Flags().IntVar(&initFlags.clearver, "clear-version", 1, "Supply the Clear version to compose the mix from")
	initCmd.Flags().IntVar(&initFlags.mixver, "mix-version", 0, "Supply the Mix version to build")
	initCmd.Flags().StringVar(&config, "config", "", "Supply a specific builder.conf to use for mixing")
	initCmd.Flags().StringVar(&initFlags.upstreamurl, "upstream-url", "https://download.clearlinux.org", "Supply an upstream URL to use for mixing")

	externalDeps[initCmd] = []string{
		"git",
	}
}

// externalDeps let commands keep track of their external program dependencies. Those will be
// verified when the command is executed, just make sure it is filled at initialization.
var externalDeps = make(map[*cobra.Command][]string)

func checkCmdDeps(cmd *cobra.Command) error {
	var deps []string
	for ; cmd != nil; cmd = cmd.Parent() {
		deps = append(deps, externalDeps[cmd]...)
	}
	sort.Strings(deps)

	var missing []string
	for i, dep := range deps {
		if i > 0 && deps[i] == deps[i-1] {
			// Skip duplicate.
			continue
		}
		_, err := exec.LookPath(dep)
		if err != nil {
			missing = append(missing, dep)
		}
	}
	if len(missing) > 0 {
		return errors.Errorf("missing following external programs: %s", strings.Join(missing, ", "))
	}
	return nil
}

func checkAllDeps() bool {
	var allDeps []string
	for _, deps := range externalDeps {
		allDeps = append(allDeps, deps...)
	}
	sort.Strings(allDeps)

	var max int
	for _, dep := range allDeps {
		if len(dep) > max {
			max = len(dep)
		}
	}

	fmt.Println("Programs used by Mixer commands:")
	ok := true
	for i, dep := range allDeps {
		if i > 0 && allDeps[i] == allDeps[i-1] {
			// Skip duplicate.
			continue
		}
		_, err := exec.LookPath(dep)
		if err != nil {
			fmt.Printf("  %-*s not found\n", max, dep)
			ok = false
		} else {
			fmt.Printf("  %-*s ok\n", max, dep)
		}
	}
	return ok
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "ERROR: %s\n", err)
	os.Exit(1)
}

func failf(format string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, fmt.Sprintf("ERROR: %s\n", format), a...)
	os.Exit(1)
}
