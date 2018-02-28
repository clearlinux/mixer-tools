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
	"errors"
	"os"
	"path/filepath"

	mixerCmd "github.com/clearlinux/mixer-tools/mixer/cmd"
	"github.com/spf13/cobra"
)

type completionCmdFlags struct {
	path string
}

var completionFlags completionCmdFlags

// CompletionCmd represents the base command for mixer-completion
var CompletionCmd = &cobra.Command{
	Use:   "mixer-completion",
	Short: "Generate autocomplete files for mixer",
	Long: `Generates autocomplete files for mixer. If no arguments are passed, or if
'bash' is passed, a bash completion file is written to 
/usr/share/bash-completion/completions/mixer. If 'zsh' is passed, a zsh
completion file is written to /usr/share/zsh/site-functions/_mixer. These
locations can be overridden by passing '--path', with a full path to the chosen
destination file.

These files allow you to type, for example:
  mixer bun[TAB] -> mixer bundle a[TAB] -> mixer bundle add

Note that this command must be run as root if using the default file locations.`,
	Args:      cobra.OnlyValidArgs,
	ValidArgs: []string{"bash", "zsh"},
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 1 {
			return errors.New("mixer-completion takes at most one argument")
		}

		var shell string
		if len(args) > 0 {
			shell = args[0]
		} else {
			shell = "bash"
		}

		var path = completionFlags.path
		var err error
		switch shell {
		case "bash":
			if path == "" {
				path = "/usr/share/bash-completion/completions/mixer"
			}
			if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				return err
			}
			err = mixerCmd.RootCmd.GenBashCompletionFile(path)
		case "zsh":
			if path == "" {
				path = "/usr/share/zsh/site-functions/_mixer"
			}
			if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				return err
			}
			err = mixerCmd.RootCmd.GenZshCompletionFile(path)
		}

		return err
	},
}

func init() {
	CompletionCmd.Flags().StringVar(&completionFlags.path, "path", "", "Completion file destination path override")
}
