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
	"github.com/clearlinux/mixer-tools/builder"
	"github.com/clearlinux/mixer-tools/helpers"

	"github.com/spf13/cobra"
)

var addRPMCmd = &cobra.Command{
	Use:   "add-rpms",
	Short: "Add RPMs to local dnf repository",
	Long:  `Add RPMS from the configured LOCAL_RPM_DIR to local dnf repository`,
	Run:   runAddRPM,
}

var rpmCmds = []*cobra.Command{
	addRPMCmd,
}

func init() {
	for _, cmd := range rpmCmds {
		RootCmd.AddCommand(cmd)
		cmd.Flags().StringVarP(&configFile, "config", "c", "", "Builder config to use")
	}

	externalDeps[addRPMCmd] = []string{
		"createrepo_c",
		"hardlink",
	}
}

func runAddRPM(cmd *cobra.Command, args []string) {
	b, err := builder.NewFromConfig(configFile)
	if err != nil {
		fail(err)
	}
	if b.Config.Mixer.LocalRPMDir == "" {
		failf("LOCAL_RPM_DIR not set in configuration")
	}
	rpms, err := helpers.ListVisibleFiles(b.Config.Mixer.LocalRPMDir)
	if err != nil {
		failf("cannot read LOCAL_RPM_DIR: %s", err)
	}
	err = b.AddRPMList(rpms)
	if err != nil {
		fail(err)
	}
}
