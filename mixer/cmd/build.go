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
	"io/ioutil"
	"os"

	"github.com/clearlinux/mixer-tools/builder"
	"github.com/clearlinux/mixer-tools/helpers"
	"github.com/pkg/errors"

	"github.com/spf13/cobra"
)

type buildCmdFlags struct {
	format     string
	increment  bool
	minVersion int
	noSigning  bool
	prefix     string
	noPublish  bool
	keepChroot bool
	template   string
}

var buildFlags buildCmdFlags

// buildCmd represents the base build command when called without any subcommands
var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build various pieces of OS content",
}

func buildChroots(builder *builder.Builder, signflag bool) error {
	// Create the signing and validation key/cert
	if _, err := os.Stat(builder.Cert); os.IsNotExist(err) {
		fmt.Println("Generating certificate for signature validation...")
		privkey, err := helpers.CreateKeyPair()
		if err != nil {
			return errors.Wrap(err, "Error generating OpenSSL keypair")
		}
		template := helpers.CreateCertTemplate()

		err = builder.BuildChroots(template, privkey, signflag)
		if err != nil {
			return errors.Wrap(err, "Error building chroots")
		}
	} else {
		err := builder.BuildChroots(nil, nil, true)
		if err != nil {
			return errors.Wrap(err, "Error building chroots")
		}
	}
	return nil
}

var buildChrootsCmd = &cobra.Command{
	Use:   "chroots",
	Short: "Build the chroots for your mix",
	Long:  `Build the chroots for your mix`,
	RunE: func(cmd *cobra.Command, args []string) error {
		b := builder.NewFromConfig(config)
		return buildChroots(b, buildFlags.noSigning)
	},
}

var buildUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Build the update content for your mix",
	Long:  `Build the update content for your mix`,
	RunE: func(cmd *cobra.Command, args []string) error {
		b := builder.NewFromConfig(config)
		err := b.BuildUpdate(buildFlags.prefix, buildFlags.minVersion, buildFlags.format, buildFlags.noSigning, !buildFlags.noPublish, buildFlags.keepChroot)
		if err != nil {
			return errors.Wrap(err, "couldn't build update")
		}

		if buildFlags.increment {
			b.UpdateMixVer()
		}
		return nil
	},
}

var buildAllCmd = &cobra.Command{
	Use:   "all",
	Short: "Build all content for mix with default options",
	Long:  `Build all content for mix with default options`,
	RunE: func(cmd *cobra.Command, args []string) error {
		b := builder.NewFromConfig(config)
		rpms, err := ioutil.ReadDir(b.RPMdir)
		if err == nil {
			err = b.AddRPMList(rpms)
			if err != nil {
				return errors.Wrap(err, "couldn't add the RPMs")
			}
		}
		err = buildChroots(b, buildFlags.noSigning)
		if err != nil {
			return errors.Wrap(err, "Error building chroots")
		}
		err = b.BuildUpdate(buildFlags.prefix, buildFlags.minVersion, buildFlags.format, buildFlags.noSigning, !buildFlags.noPublish, buildFlags.keepChroot)
		if err != nil {
			return errors.Wrap(err, "Error building update")
		}

		b.UpdateMixVer()
		return nil
	},
}

var buildImageCmd = &cobra.Command{
	Use:   "image",
	Short: "Build an image from the mix content",
	Long:  `Build an image from the mix content`,
	RunE: func(cmd *cobra.Command, args []string) error {
		b := builder.NewFromConfig(config)
		err := b.BuildImage(buildFlags.format, buildFlags.template)
		if err != nil {
			return errors.Wrap(err, "Error building image")
		}
		return nil
	},
}

func setUpdateFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&buildFlags.format, "format", "", "Supply format to use")
	cmd.Flags().BoolVar(&buildFlags.increment, "increment", false, "Automatically increment the mixversion post build")
	cmd.Flags().IntVar(&buildFlags.minVersion, "min-version", 0, "Supply minversion to build update with")
	cmd.Flags().BoolVar(&buildFlags.noSigning, "no-signing", false, "Do not generate a certificate and do not sign the Manifest.MoM")
	cmd.Flags().StringVar(&buildFlags.prefix, "prefix", "", "Supply prefix for where the swupd binaries live")
	cmd.Flags().BoolVar(&buildFlags.noPublish, "no-publish", false, "Do not update the latest version after update")
	cmd.Flags().BoolVar(&buildFlags.keepChroot, "keep-chroots", false, "Keep individual chroots created and not just consolidated 'full'")
}

var buildCmds = []*cobra.Command{
	buildChrootsCmd,
	buildUpdateCmd,
	buildAllCmd,
	buildImageCmd,
}

func init() {
	for _, cmd := range buildCmds {
		buildCmd.AddCommand(cmd)
		cmd.Flags().StringVarP(&config, "config", "c", "", "Builder config to use")
	}

	RootCmd.AddCommand(buildCmd)

	buildChrootsCmd.Flags().BoolVar(&buildFlags.noSigning, "no-signing", false, "Do not generate a certificate to sign the Manifest.MoM")

	buildImageCmd.Flags().StringVar(&buildFlags.format, "format", "", "Supply the format used for the Mix")
	buildImageCmd.Flags().StringVar(&buildFlags.template, "template", "", "Path to template file to use")

	setUpdateFlags(buildUpdateCmd)
	setUpdateFlags(buildAllCmd)
}
