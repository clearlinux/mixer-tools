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
	"runtime/pprof"
	"sort"
	"strconv"
	"strings"

	"github.com/clearlinux/mixer-tools/builder"
	"github.com/clearlinux/mixer-tools/config"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var configFile string

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:  "mixer",
	Long: `Mixer is a tool used to compose OS update content and images.`,

	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
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
		// Both --version and --check should work regardless of the regular
		// check for external programs.
		if cmd.Parent() == nil { // This is RootCmd.
			if rootCmdFlags.version {
				fmt.Printf("Mixer %s\n", builder.Version)
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

		// Init needs to be handled differently because there is no config yet
		if cmdContains(cmd, "init") {
			return checkCmdDeps(cmd)
		}

		b, err := builder.NewFromConfig(configFile)
		if err != nil {
			fail(err)
		}

		// If running natively, check for format missmatch and warn
		if builder.Native {
			hostFormat, upstreamFormat, err := b.GetHostAndUpstreamFormats()
			if err != nil {
				fail(err)
			}

			if hostFormat == "" {
				fmt.Println("Warning: Unable to determine host format. Running natively may fail.")
			} else if hostFormat != upstreamFormat {
				fmt.Println("Warning: The host format and mix upstream format do not match.",
					"Mixer may be incompatible with this format; running natively may fail.")
			}
		}

		// For non-bump build commands, check if building across a format
		// If so: inform, stage, and exit.
		// If not: run command in container and cancel pre-run
		if !cmdContains(cmd, "format-bump") && !cmdContains(cmd, "upstream-format") && cmdContains(cmd, "build") {
			if bumpNeeded, err := b.CheckBumpNeeded(); err != nil {
				return err
			} else if bumpNeeded {
				cancelRun(cmd)
				return nil
			}

			// If Native==false
			if !builder.Native {
				if err := b.RunCommandInContainer(reconstructCommand(cmd, args)); err != nil {
					fail(err)
				}
				// Cancel native run and return
				cancelRun(cmd)
				return nil
			}
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

type initCmdFlags struct {
	allLocal    bool
	allUpstream bool
	noDefaults  bool
	clearVer    string
	mixver      int
	localRPMs   bool
	upstreamURL string
	git         bool
}

var initFlags initCmdFlags

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize the mixer and workspace",
	Long:  `Initialize the mixer and workspace`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := strconv.Atoi(initFlags.clearVer); err != nil {
			if initFlags.clearVer != "latest" {
				// Note: output matches Cobra's default pflag error syntax, as
				// if initFlags.clearVer were an int all along.
				return errors.Errorf("invalid argument \"%s\" for \"--clear-version\" flag: %s", initFlags.clearVer, err)
			}
		}

		b := builder.New()
		if configFile == "" {
			// Create default config if necessary
			if err := b.Config.CreateDefaultConfig(initFlags.localRPMs); err != nil {
				fail(err)
			}
		}

		if err := b.Config.LoadConfig(configFile); err != nil {
			fail(err)
		}
		err := b.InitMix(initFlags.clearVer, strconv.Itoa(initFlags.mixver), initFlags.allLocal, initFlags.allUpstream, initFlags.noDefaults, initFlags.upstreamURL, initFlags.git)
		if err != nil {
			fail(err)
		}

		return nil
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
	RootCmd.PersistentFlags().StringVar(&rootCmdFlags.cpuProfile, "cpu-profile", "", "write CPU profile to a file")
	_ = RootCmd.PersistentFlags().MarkHidden("cpu-profile")

	// TODO: Remove this once we migrate to new implementation.
	unusedBoolFlag := false
	RootCmd.PersistentFlags().BoolVar(&unusedBoolFlag, "new-swupd", false, "")
	_ = RootCmd.Flags().MarkHidden("new-swupd")
	_ = RootCmd.Flags().MarkDeprecated("new-swupd", "new functionality is now the standard behavior, this flag is obsolete and no longer used")

	// TODO: Remove this once we drop the old config format
	RootCmd.PersistentFlags().BoolVar(&config.UseNewConfig, "new-config", false, "EXPERIMENTAL: use the new TOML config format")

	RootCmd.PersistentFlags().BoolVar(&builder.Native, "native", true, "Run mixer command on native host instead of in a container")
	RootCmd.PersistentFlags().BoolVar(&builder.Offline, "offline", false, "Skip caching upstream-bundles; work entirely with local-bundles")

	RootCmd.AddCommand(initCmd)
	RootCmd.Flags().BoolVar(&rootCmdFlags.version, "version", false, "Print version information and quit")
	RootCmd.Flags().BoolVar(&rootCmdFlags.check, "check", false, "Check all dependencies needed by mixer and quit")

	initCmd.Flags().BoolVar(&initFlags.allLocal, "all-local", false, "Initialize mix with all local bundles automatically included")
	initCmd.Flags().BoolVar(&initFlags.allUpstream, "all-upstream", false, "Initialize mix with all upstream bundles automatically included")
	initCmd.Flags().BoolVar(&initFlags.noDefaults, "no-default-bundles", false, "Skip adding default bundles to the mix")
	initCmd.Flags().StringVar(&initFlags.clearVer, "clear-version", "latest", "Supply the Clear version to compose the mix from")
	initCmd.Flags().StringVar(&initFlags.clearVer, "upstream-version", "latest", "Alias to --clear-version")
	initCmd.Flags().IntVar(&initFlags.mixver, "mix-version", 10, "Supply the Mix version to build")
	initCmd.Flags().BoolVar(&initFlags.localRPMs, "local-rpms", false, "Create and configure local RPMs directories")
	initCmd.Flags().StringVar(&configFile, "config", "", "Supply a specific builder.conf to use for mixing")
	initCmd.Flags().StringVar(&initFlags.upstreamURL, "upstream-url", "https://download.clearlinux.org", "Supply an upstream URL to use for mixing")
	initCmd.Flags().BoolVar(&initFlags.git, "git", false, "Track mixer's internal work dir with git")

	externalDeps[initCmd] = []string{
		"git",
	}
}

func cancelRun(cmd *cobra.Command) {
	cmd.RunE = nil
	cmd.Run = func(cmd *cobra.Command, args []string) {} // No-op
}

// cmdContains returns true if cmd or any of its parents are named name
func cmdContains(cmd *cobra.Command, name string) bool {
	if cmd.Name() == name {
		return true
	}
	if cmd.HasParent() {
		return cmdContains(cmd.Parent(), name)
	}
	return false
}

func reconstructCommand(cmd *cobra.Command, args []string) []string {
	command := []string{cmd.Name()}

	// Loop back up parents, prepending command name
	for p := cmd.Parent(); p != nil; p = p.Parent() {
		command = append([]string{p.Name()}, command...)
	}
	// For each flag that was set, append its name and value
	cmd.Flags().Visit(func(flag *pflag.Flag) {
		if flag.Name == "native" {
			return
		}
		command = append(command, "--"+flag.Name+"="+flag.Value.String())
	})
	// Append args
	command = append(command, args...)

	return command
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
	if rootCmdFlags.cpuProfile != "" {
		pprof.StopCPUProfile()
	}
	fmt.Fprintf(os.Stderr, "ERROR: %s\n", err)
	os.Exit(1)
}

func failf(format string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, fmt.Sprintf("ERROR: %s\n", format), a...)
	os.Exit(1)
}
