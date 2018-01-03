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
	"strconv"

	"github.com/clearlinux/mixer-tools/builder"

	"github.com/spf13/cobra"
)

// Version of Mixer. Also used by the Makefile for releases.
const Version = "3.2.1"

var config string

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:  "mixer",
	Long: `Mixer is a tool used to compose OS update content and images.`,
	Run: func(cmd *cobra.Command, args []string) {
		if rootCmdFlags.version {
			fmt.Printf("Mixer %s\n", Version)
			os.Exit(0)
		}
		// Use cmd here to print exactly like other prints of Usage (that might be
		// configurable).
		cmd.Print(cmd.UsageString())
	},
}

var rootCmdFlags = struct {
	version bool
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

func checkDeps() error {
	deps := []string{
		"createrepo_c",
		"git",
		"hardlink",
		"m4",
		"openssl",
		"parallel",
		"rpm",
		"yum",
	}
	for _, dep := range deps {
		if _, err := exec.LookPath(dep); err != nil {
			return fmt.Errorf("failed to find program %q: %v", dep, err)
		}
	}
	return nil
}

func init() {
	if err := checkDeps(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}

	RootCmd.AddCommand(initCmd)
	RootCmd.Flags().BoolVar(&rootCmdFlags.version, "version", false, "Print version information and quit")

	initCmd.Flags().BoolVar(&initFlags.all, "all", false, "Create a mix with all Clear bundles included")
	initCmd.Flags().IntVar(&initFlags.clearver, "clear-version", 1, "Supply the Clear version to compose the mix from")
	initCmd.Flags().IntVar(&initFlags.mixver, "mix-version", 0, "Supply the Mix version to build")
	initCmd.Flags().StringVar(&config, "config", "", "Supply a specific builder.conf to use for mixing")
	initCmd.Flags().StringVar(&initFlags.upstreamurl, "upstream-url", "https://download.clearlinux.org", "Supply an upstream URL to use for mixing")
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "ERROR: %s\n", err)
	os.Exit(1)
}

func failf(format string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, fmt.Sprintf("ERROR: %s\n", format), a...)
	os.Exit(1)
}
