package cmd

import (
	"strconv"

	"github.com/clearlinux/mixer-tools/builder"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

type initCmdFlags struct {
	allLocal    bool
	allUpstream bool
	noDefaults  bool
	clearVer    string
	mixver      int
	upstreamURL string
	git         bool
	format      string
}

var initFlags initCmdFlags

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize the mixer workspace",
	Long:  `Initialize the mixer workspace`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := strconv.Atoi(initFlags.clearVer); err != nil {
			if initFlags.clearVer != "latest" {
				// Note: output matches Cobra's default pflag error syntax, as
				// if initFlags.clearVer were an int all along.
				return errors.Errorf("invalid argument \"%s\" for \"--clear-version\" flag: %s", initFlags.clearVer, err)
			}
		}

		b := builder.New()
		if configFile == "" {
			// Create default config if necessary
			if err := b.Config.CreateDefaultConfig(); err != nil {
				fail(err)
			}
		}

		if err := b.Config.LoadConfig(configFile); err != nil {
			fail(err)
		}

		b.State.LoadDefaults()
		if initFlags.format != "" {
			b.State.Mix.Format = initFlags.format
		}
		if err := b.State.Save(); err != nil {
			fail(err)
		}

		err := b.InitMix(initFlags.clearVer, strconv.Itoa(initFlags.mixver), initFlags.allLocal, initFlags.allUpstream, initFlags.noDefaults, initFlags.upstreamURL, initFlags.git)
		if err != nil {
			fail(err)
		}

		return nil
	},
}

func init() {
	RootCmd.AddCommand(initCmd)

	// Deprecated Init flags
	unusedBoolFlag := false
	initCmd.Flags().BoolVar(&unusedBoolFlag, "local-rpms", false, "")
	_ = initCmd.Flags().MarkHidden("local-rpms")
	_ = initCmd.Flags().MarkDeprecated("local-rpms", "Local rpm folders are now always created by default")

	initCmd.Flags().BoolVar(&initFlags.allLocal, "all-local", false, "Initialize mix with all local bundles automatically included")
	initCmd.Flags().BoolVar(&initFlags.allUpstream, "all-upstream", false, "Initialize mix with all upstream bundles automatically included")
	initCmd.Flags().BoolVar(&initFlags.noDefaults, "no-default-bundles", false, "Skip adding default bundles to the mix")
	initCmd.Flags().StringVar(&initFlags.clearVer, "clear-version", "latest", "Supply the Clear version to compose the mix from")
	initCmd.Flags().StringVar(&initFlags.clearVer, "upstream-version", "latest", "Alias to --clear-version")
	initCmd.Flags().IntVar(&initFlags.mixver, "mix-version", 10, "Supply the Mix version to build")
	initCmd.Flags().StringVar(&initFlags.upstreamURL, "upstream-url", "https://download.clearlinux.org", "Supply an upstream URL to use for mixing")
	initCmd.Flags().BoolVar(&initFlags.git, "git", false, "Track mixer's internal work dir with git")
	initCmd.Flags().StringVar(&initFlags.format, "format", "", "Supply the format version for the mix")

	externalDeps[initCmd] = []string{
		"git",
	}
}
