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

package cmd

import (
	"github.com/clearlinux/mixer-tools/config"
	"github.com/spf13/cobra"
)

// Top level config command ('mixer config')
var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Perform config related actions",
}

var configValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Parse a config file and print its properties",
	Long: `Parse a builder config file and display its properties. Properties containing
environment variables will be expanded`,
	Run: func(cmd *cobra.Command, args []string) {
		var err error
		if configFile, err = config.GetConfigPath(configFile); err != nil {
			// Print error, but don't print command usage
			fail(err)
			return
		}

		var mc config.MixConfig
		if err := mc.LoadConfig(configFile); err != nil {
			fail(err)
			return
		}

		if err := mc.Print(); err != nil {
			fail(err)
			return
		}

	},
}

var configConvertCmd = &cobra.Command{
	Use:   "convert",
	Short: "Converts an old config file to the new TOML format",
	Long: `Convert an old config file to the new TOML format. The command will generate
a backup file of the old config and will replace it with the converted one. Environment
variables will not be expanded and the values will not be validated`,
	Run: func(cmd *cobra.Command, args []string) {
		var err error
		if configFile, err = config.GetConfigPath(configFile); err != nil {
			fail(err)
			return
		}

		var mc config.MixConfig
		if err := mc.Convert(configFile); err != nil {
			fail(err)
			return
		}

	},
}

// List of all config commands
var configCmds = []*cobra.Command{
	configValidateCmd,
	configConvertCmd,
}

func init() {
	for _, cmd := range configCmds {
		configCmd.AddCommand(cmd)
		cmd.Flags().StringVarP(&configFile, "config", "c", "", "Builder config to use")
	}

	RootCmd.AddCommand(configCmd)
}
