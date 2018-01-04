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
	"io/ioutil"

	"github.com/clearlinux/mixer-tools/builder"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var addRPMCmd = &cobra.Command{
	Use:   "add-rpms",
	Short: "Add rpms to local yum repository",
	Long:  `Add rpms to local yum repository`,
	RunE: func(cmd *cobra.Command, args []string) error {
		b := builder.NewFromConfig(config)
		rpms, err := ioutil.ReadDir(b.Rpmdir)
		if err != nil {
			return errors.Wrapf(err, "Error cannot read %s\n", b.Rpmdir)
		}
		// TODO return error to check from AddRPMList
		b.AddRPMList(rpms)
		return nil
	},
}

var rpmCmds = []*cobra.Command{
	addRPMCmd,
}

func init() {
	for _, cmd := range rpmCmds {
		RootCmd.AddCommand(cmd)
		cmd.Flags().StringVarP(&config, "config", "c", "", "Builder config to use")
	}

	externalDeps[addRPMCmd] = []string{
		"createrepo_c",
		"hardlink",
	}
}
