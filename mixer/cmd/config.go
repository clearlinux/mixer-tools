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
	"fmt"

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
		var mc config.MixConfig
		if err := mc.Load(configFile); err != nil {
			fail(err)
		}

		if err := mc.Print(); err != nil {
			fail(err)
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
		/* If no state file exists, it must be created first to ensure the FORMAT value
		is transferred from old configs before conversion */
		var ms config.MixState
		if err := ms.Load(""); err != nil {
			fail(err)
		}

		var mc config.MixConfig
		if err := mc.Convert(configFile); err != nil {
			fail(err)
		}

	},
}

var configSetCmd = &cobra.Command{
	Use:   "set <property> <value>",
	Short: "Set a property in the config file to a given value",
	Long: `This command will parse the provided property in the format 'Section.Property',
	assign the provided value and update the config file. The command will only validate
	the existence of the provided property, but will not validate the value provided.`,
	Args: cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		// Look for the value in mixer.state first
		var ms config.MixState
		err := ms.Load("")
		if err == nil {
			err = config.SetProperty(&ms, args[0], args[1])
			if err == nil {
				return
			}
		}

		// Look for the value in builder.conf
		var mc config.MixConfig
		if err := mc.Load(configFile); err != nil {
			fail(err)
		}

		if err := config.SetProperty(&mc, args[0], args[1]); err != nil {
			fail(err)
		}

	},
}

var configGetCmd = &cobra.Command{
	Use:   "get <property>",
	Short: "Get a property in the config file",
	Long: `This command will parse the provided property in the format 'Section.Property'
	and return its value. The value returned is the value used by mixer, which will be
	either the value set in the config file or the default value if the property has not
	been defined in the file.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		var value string

		// Look for the value in mixer.state first
		var ms config.MixState
		err := ms.Load("")
		if err == nil {
			value, err = config.GetProperty(&ms, args[0])
			if err == nil {
				fmt.Println(value)
				return
			}
		}

		// Look for the value in builder.conf
		var mc config.MixConfig
		if err = mc.Load(configFile); err != nil {
			fail(err)
		}

		value, err = config.GetProperty(&mc, args[0])
		if err != nil {
			fail(err)
		}

		fmt.Println(value)
	},
}

// List of all config commands
var configCmds = []*cobra.Command{
	configValidateCmd,
	configConvertCmd,
	configSetCmd,
	configGetCmd,
}

func init() {
	for _, cmd := range configCmds {
		configCmd.AddCommand(cmd)
	}

	RootCmd.AddCommand(configCmd)
}
