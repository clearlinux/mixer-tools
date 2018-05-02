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
	"errors"
	"fmt"

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
	Short: "Add repo <name> at <url>",
	Long:  `Add the repo at <url> as a repo from which to pull RPMs for building bundles`,
	Run:   runAddRepo,
}

var repoCmds = []*cobra.Command{
	addRepoCmd,
}

func init() {
	for _, cmd := range repoCmds {
		repoCmd.AddCommand(cmd)
		cmd.Flags().StringVarP(&config, "config", "c", "", "Builder config to use")
	}

	RootCmd.AddCommand(repoCmd)
}

func runAddRepo(cmd *cobra.Command, args []string) {
	if len(args) != 2 {
		fail(errors.New("add requires 2 arguments: <repo-name> <repo-url>"))
	}
	b, err := builder.NewFromConfig(config)
	if err != nil {
		fail(err)
	}

	err = b.AddRepo(args)
	if err != nil {
		fail(err)
	}
	fmt.Printf("Added %s repo at %s url.\n", args[0], args[1])
}
