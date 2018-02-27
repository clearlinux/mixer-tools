package builder

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-ini/ini"
)

// MixConfig represents the config parameters found in the builder config file.
type MixConfig struct {
	// [Builder]
	BundleDir  string
	Cert       string
	StateDir   string
	VersionDir string
	YumConf    string

	// [swupd]
	Bundle     string
	ContentURL string
	Format     string
	VersionURL string

	// [Server]
	DebugInfoBanned string
	DebugInfoLib    string
	DebugInfoSrc    string

	// [Mixer]
	LocalBundleDir string
	RepoDir        string
	RPMDir         string
}

type configPair struct {
	key   string
	value *string
}

type configMapping struct {
	section string
	pairs   []configPair
}

func (config *MixConfig) getMapping() []configMapping {
	return []configMapping{
		{
			section: "Builder",
			pairs: []configPair{
				{key: "BUNDLE_DIR", value: &config.BundleDir},
				{"CERT", &config.Cert},
				{"SERVER_STATE_DIR", &config.StateDir},
				{"VERSIONS_PATH", &config.VersionDir},
				{"YUM_CONF", &config.YumConf},
			}},
		{
			section: "swupd",
			pairs: []configPair{
				{"BUNDLE", &config.Bundle},
				{"CONTENTURL", &config.ContentURL},
				{"FORMAT", &config.Format},
				{"VERSIONURL", &config.VersionURL},
			}},
		{
			section: "Server",
			pairs: []configPair{
				{"debuginfo_banned", &config.DebugInfoBanned},
				{"debuginfo_lib", &config.DebugInfoLib},
				{"debuginfo_src", &config.DebugInfoSrc},
			}},
		{
			section: "Mixer",
			pairs: []configPair{
				{"LOCAL_BUNDLE_DIR", &config.LocalBundleDir},
				{"LOCAL_REPO_DIR", &config.RepoDir},
				{"LOCAL_RPM_DIR", &config.RPMDir},
			}},
	}
}

func (config *MixConfig) mapToIni() (*ini.File, error) {
	cfg := ini.Empty()

	for _, entry := range config.getMapping() {
		section, err := cfg.NewSection(entry.section)
		if err != nil {
			return nil, err
		}

		for _, pair := range entry.pairs {
			if _, err = section.NewKey(pair.key, *pair.value); err != nil {
				return nil, err
			}
		}
	}

	return cfg, nil
}

func (config *MixConfig) mapFromIni(cfg *ini.File) {
	for _, entry := range config.getMapping() {
		section, err := cfg.GetSection(entry.section)
		if err != nil {
			continue
		}

		for _, pair := range entry.pairs {
			key, err := section.GetKey(pair.key)
			if err == nil {
				*pair.value = key.String()
			}
		}
	}
}

// LoadDefaults sets sane defaults for all the config values in MixCOnfig
func (config *MixConfig) LoadDefaults() error {
	pwd, err := os.Getwd()
	if err != nil {
		return nil
	}

	// [Builder]
	config.BundleDir = filepath.Join(pwd, "mix-bundles")
	config.Cert = filepath.Join(pwd, "Swupd_Root.pem")
	config.StateDir = filepath.Join(pwd, "update")
	config.VersionDir = pwd
	config.YumConf = filepath.Join(pwd, ".yum-mix.conf")

	// [Swupd]
	config.Bundle = "os-core-update"
	config.ContentURL = "<URL where the content will be hosted>"
	config.Format = "1"
	config.VersionURL = "<URL where the version of the mix will be hosted>"

	// [Server]
	config.DebugInfoBanned = "true"
	config.DebugInfoLib = "/usr/lib/debug"
	config.DebugInfoSrc = "/usr/src/debug"

	// [Mixer]
	config.LocalBundleDir = filepath.Join(pwd, "local-bundles")
	config.RPMDir = ""
	config.RepoDir = ""

	return nil
}

// CreateDefaultConfig creates a default builder.conf using the active
// directory as base path for the variables values.
func (config *MixConfig) CreateDefaultConfig(localrpms bool) error {
	if err := config.LoadDefaults(); err != nil {
		return err
	}

	pwd, err := os.Getwd()
	if err != nil {
		return err
	}

	builderconf := filepath.Join(pwd, "builder.conf")

	if localrpms {
		config.RPMDir = filepath.Join(pwd, "local-rpms")
		config.RepoDir = filepath.Join(pwd, "local-yum")
	}

	fmt.Println("Creating new builder.conf configuration file...")

	cfg, err := config.mapToIni()
	if err != nil {
		return err
	}

	return cfg.SaveTo(builderconf)
}

// LoadConfig loads a configuration file from a provided path or from local directory
// is none is provided
func (config *MixConfig) LoadConfig(filename string) error {
	cfg, err := ini.InsensitiveLoad(filename)
	if err != nil {
		return err
	}

	//Expand Environment Variables
	cfg.ValueMapper = os.ExpandEnv

	config.mapFromIni(cfg)

	return nil
}
