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
	"log"
	"os"
	"os/exec"
	"runtime/pprof"
	"sort"
	"strings"

	"github.com/clearlinux/mixer-tools/builder"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var configFile string

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:  "mixer",
	Long: `Mixer is a tool used to compose OS update content and images.`,

	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// CPU Profiling
		if rootCmdFlags.cpuProfile != "" {
			f, err := os.Create(rootCmdFlags.cpuProfile)
			if err != nil {
				failf("couldn't create file for CPU profile: %s", err)
			}
			err = pprof.StartCPUProfile(f)
			if err != nil {
				failf("couldn't start profiling: %s", err)
			}
		}

		// Run dependency check for --check flag
		if cmd.Parent() == nil { // This is RootCmd.
			if rootCmdFlags.version {
				fmt.Printf("Mixer %s\n", builder.Version)
				os.Exit(0)
			}
			if rootCmdFlags.check {
				ok := checkAllDeps()
				if !ok {
					return errors.New("ERROR: Missing Dependency")
				}
			}

			return nil
		}

		return checkCmdDeps(cmd)
	},

	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		if rootCmdFlags.cpuProfile != "" {
			pprof.StopCPUProfile()
		}
	},

	Run: func(cmd *cobra.Command, args []string) {
		// Use cmd here to print exactly like other prints of Usage (that might be
		// configurable).
		cmd.Print(cmd.UsageString())
	},
}

var rootCmdFlags = struct {
	version    bool
	check      bool
	cpuProfile string
}{}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := RootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	RootCmd.PersistentFlags().StringVar(&rootCmdFlags.cpuProfile, "cpu-profile", "", "write CPU profile to a file")
	_ = RootCmd.PersistentFlags().MarkHidden("cpu-profile")

	// TODO: Remove this once we migrate to new implementation.
	unusedBoolFlag := false
	RootCmd.PersistentFlags().BoolVar(&unusedBoolFlag, "new-swupd", false, "")
	_ = RootCmd.PersistentFlags().MarkHidden("new-swupd")
	_ = RootCmd.PersistentFlags().MarkDeprecated("new-swupd", "new functionality is now the standard behavior, this flag is obsolete and no longer used")

	// TODO: Remove this once we drop the old config format
	RootCmd.PersistentFlags().BoolVar(&unusedBoolFlag, "new-config", true, "")
	_ = RootCmd.PersistentFlags().MarkHidden("new-config")
	_ = RootCmd.PersistentFlags().MarkDeprecated("new-config", "The config file is now automatically converted and the new format is always used")

	RootCmd.PersistentFlags().BoolVar(&unusedBoolFlag, "native", false, "")
	_ = RootCmd.PersistentFlags().MarkHidden("native")
	_ = RootCmd.PersistentFlags().MarkDeprecated("native", "commands are always run natively on the host")

	RootCmd.PersistentFlags().BoolVar(&builder.Offline, "offline", false, "Skip caching upstream-bundles; work entirely with local-bundles")
	RootCmd.PersistentFlags().StringVarP(&configFile, "config", "c", "", "Supply a specific builder.conf to use for mixing")

	RootCmd.Flags().BoolVar(&rootCmdFlags.version, "version", false, "Print version information and quit")
	RootCmd.Flags().BoolVar(&rootCmdFlags.check, "check", false, "Check all dependencies needed by mixer and quit")
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

func addMarker(cmd *cobra.Command, marker string) {
	if cmd.Annotations == nil {
		cmd.Annotations = make(map[string]string)
	}
	cmd.Annotations[marker] = ""
}

func hasMarker(cmd *cobra.Command, marker string) bool {
	_, ok := cmd.Annotations[marker]
	return ok
}

func fail(err error) {
	if rootCmdFlags.cpuProfile != "" {
		pprof.StopCPUProfile()
	}
	log.Printf("ERROR: %s\n", err)
	os.Exit(1)
}

func failf(format string, a ...interface{}) {
	log.Printf(fmt.Sprintf("ERROR: %s\n", format), a...)
	os.Exit(1)
}
