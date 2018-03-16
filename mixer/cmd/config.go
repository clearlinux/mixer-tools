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
	"github.com/clearlinux/mixer-tools/builder"
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
		if config, err = builder.GetConfigPath(config); err != nil {
			// Print error, but don't print command usage
			fail(err)
			return
		}

		var mc builder.MixConfig
		if err := mc.LoadConfig(config); err != nil {
			fail(err)
			return
		}

		if err := mc.Print(); err != nil {
			fail(err)
			return
		}

	},
}

// List of all config commands
var configCmds = []*cobra.Command{
	configValidateCmd,
}

func init() {
	for _, cmd := range configCmds {
		configCmd.AddCommand(cmd)
		cmd.Flags().StringVarP(&config, "config", "c", "", "Builder config to use")
	}

	RootCmd.AddCommand(configCmd)
}
