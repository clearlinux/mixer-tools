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

	mixerCmd "github.com/clearlinux/mixer-tools/mixer/cmd"
	"github.com/spf13/cobra"
)

// CompletionCmd represents the base command for mixer-completion
var CompletionCmd = &cobra.Command{
	Use:   "mixer-completion",
	Short: "Generate autocomplete files for mixer",
	Long: `Generates autocomplete files for mixer. If no arguments are passed, or if
'bash' is passed, a bash completion file is written to /etc/bash_completion.d/.
If 'zsh' is passed, a zsh completion file is written to
/usr/share/zsh/site-functions/.

These files allow you to type, for example:
  mixer bun[TAB] -> mixer bundle a[TAB] -> mixer bundle add

Note that this command must be run as root.`,
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

		var err error
		switch shell {
		case "bash":
			if err = os.MkdirAll("/usr/share/bash-completion/completions", 0755); err != nil {
				return err
			}
			err = mixerCmd.RootCmd.GenBashCompletionFile("/usr/share/bash-completion/completions/mixer")
		case "zsh":
			if err = os.MkdirAll("/usr/share/zsh/site-functions", 0755); err != nil {
				return err
			}
			err = mixerCmd.RootCmd.GenZshCompletionFile("/usr/share/zsh/site-functions/_mixer")
		}

		return err
	},
}
