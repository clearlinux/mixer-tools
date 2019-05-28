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

package builder

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/clearlinux/mixer-tools/helpers"
	"github.com/go-ini/ini"
	"github.com/pkg/errors"
)

type dnfRepoConf struct {
	RepoName, RepoURL string
}

const dnfConfRepoTemplate = `
[{{.RepoName}}]
name={{.RepoName}}
failovermethod=priority
baseurl={{.RepoURL}}
enabled=1
gpgcheck=0
priority=1
`

// If Base == true, template will include the [main] and [clear] sections.
// If Local == true, template will include the [local] section.
type dnfConf struct {
	UpstreamURL, RepoDir string
	Base, Local          bool
}

const dnfConfTemplate = `{{if .Base}}[main]
cachedir=/var/cache/yum/clear/
keepcache=0
debuglevel=2
logfile=/var/log/yum.log
exactarch=1
obsoletes=1
gpgcheck=0
plugins=0
installonly_limit=3
reposdir=/root/mash

[clear]
name=Clear
failovermethod=priority
baseurl={{.UpstreamURL}}/releases/$releasever/clear/x86_64/os/
enabled=1
gpgcheck=0
timeout=45
{{end}}{{if .Local}}
[local]
name=Local
failovermethod=priority
baseurl=file://{{.RepoDir}}
enabled=1
gpgcheck=0
priority=1
{{end}}`

// NewDNFConfIfNeeded creates a new DNF configuration file if it does not already exist
func (b *Builder) NewDNFConfIfNeeded() error {
	conf := dnfConf{
		UpstreamURL: b.UpstreamURL,
		RepoDir:     b.Config.Mixer.LocalRepoDir,
	}

	if _, err := os.Stat(b.Config.Builder.DNFConf); os.IsNotExist(err) {
		conf.Base = true
		if b.Config.Mixer.LocalRepoDir != "" {
			conf.Local = true
		}
	} else if err == nil && b.Config.Mixer.LocalRepoDir != "" {
		// check if conf file contains local section
		raw, err := ioutil.ReadFile(b.Config.Builder.DNFConf)
		if err != nil {
			return err
		}
		if !strings.Contains(string(raw), "[local]") {
			conf.Local = true
		}
	}

	if conf.Base || conf.Local {
		f, err := os.OpenFile(b.Config.Builder.DNFConf, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		defer func() {
			_ = f.Close()
		}()

		t, err := template.New("dnfConfTemplate").Parse(dnfConfTemplate)
		if err != nil {
			log.Println(err)
			return err
		}

		if err = t.Execute(f, conf); err != nil {
			return errors.Wrapf(err, "Failed to write to dnf file: %s", b.Config.Builder.DNFConf)
		}
	}
	return nil
}

// AddRepo adds and enables a repo configuration named <name> pointing at
// URL <url>. It calls b.NewDNFConfIfNeeded() to create the DNF config if it
// does not exist and performs a check to see if the repo passed has already
// been configured.
func (b *Builder) AddRepo(name, url string) error {
	repo := dnfRepoConf{
		RepoName: name,
		RepoURL:  url,
	}

	if err := b.NewDNFConfIfNeeded(); err != nil {
		return err
	}

	f, err := os.OpenFile(b.Config.Builder.DNFConf, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		log.Println(err)
		return err
	}

	DNFConf, err := ini.Load(b.Config.Builder.DNFConf)
	if err != nil {
		return err
	}

	_, err = DNFConf.GetSection(repo.RepoName)
	if err == nil {
		return fmt.Errorf("repo %s already exists in %s, not adding duplicate",
			repo.RepoName,
			b.Config.Builder.DNFConf)
	}

	t, err := template.New("dnfConfRepoTemplate").Parse(dnfConfRepoTemplate)
	if err != nil {
		log.Println(err)
		return err
	}

	if err = t.Execute(f, repo); err != nil {
		return errors.Wrapf(err, "Failed to write to dnf file: %s", b.Config.Builder.DNFConf)
	}

	return nil
}

// SetURLRepo sets the URL for the repo <name> to <url>. If <name> does not exist it is
// created.
func (b *Builder) SetURLRepo(name, url string) error {
	if err := b.NewDNFConfIfNeeded(); err != nil {
		return err
	}

	DNFConf, err := ini.Load(b.Config.Builder.DNFConf)
	if err != nil {
		return err
	}

	s, err := DNFConf.GetSection(name)
	if err != nil {
		// the section doesn't exist, just add a new one
		return b.AddRepo(name, url)
	}

	k, err := s.GetKey("baseurl")
	if err != nil {
		return err
	}

	k.SetValue(url)
	return DNFConf.SaveTo(b.Config.Builder.DNFConf)
}

// SetRepoVal sets a value for the provided repo and key
func (b *Builder) SetRepoVal(reponame, key, val string) error {
	if err := b.NewDNFConfIfNeeded(); err != nil {
		return err
	}

	DNFConf, err := ini.Load(b.Config.Builder.DNFConf)
	if err != nil {
		return err
	}

	s, err := DNFConf.GetSection(reponame)
	if err != nil {
		return err
	}

	k, err := s.GetKey(key)
	if err != nil {
		if k, err = DNFConf.Section(reponame).NewKey(key, val); err != nil {
			return err
		}
	}

	k.SetValue(val)
	return DNFConf.SaveTo(b.Config.Builder.DNFConf)
}

// SetExcludesRepo sets the ecludes for the repo <name> to [pkgs...]
func (b *Builder) SetExcludesRepo(reponame, pkgs string) error {
	if err := b.NewDNFConfIfNeeded(); err != nil {
		return err
	}

	DNFConf, err := ini.Load(b.Config.Builder.DNFConf)
	if err != nil {
		return err
	}

	s, err := DNFConf.GetSection(reponame)
	if err != nil {
		return err
	}

	k, err := s.GetKey("excludepkgs")
	if err != nil {
		if k, err = DNFConf.Section(reponame).NewKey("excludepkgs", pkgs); err != nil {
			return err
		}
	}

	k.SetValue(pkgs)
	return DNFConf.SaveTo(b.Config.Builder.DNFConf)
}

// RemoveRepo removes a configured repo <name> if it exists in the DNF configuration.
// This will fail if a DNF conf has not yet been generated.
func (b *Builder) RemoveRepo(name string) error {
	if _, err := os.Stat(b.Config.Builder.DNFConf); os.IsNotExist(err) {
		return err
	}

	DNFConf, err := ini.Load(b.Config.Builder.DNFConf)
	if err != nil {
		return err
	}

	_, err = DNFConf.GetSection(name)
	if err != nil {
		fmt.Printf("Repo %s does not exist.\n", name)
	}

	DNFConf.DeleteSection(name)
	return DNFConf.SaveTo(b.Config.Builder.DNFConf)
}

// ListRepos lists all configured repositories in the DNF configuration file.
// This will fail if a DNF conf has not yet been generated.
func (b *Builder) ListRepos() error {
	if _, err := os.Stat(b.Config.Builder.DNFConf); os.IsNotExist(err) {
		return errors.Wrap(err, "unable to find DNF configuration, try initializing workspace")
	}

	DNFConf, err := ini.Load(b.Config.Builder.DNFConf)
	if err != nil {
		return errors.Wrap(err, "unable to load DNF configration, try initializing workspace")
	}

	for _, s := range DNFConf.Sections() {
		name := s.Name()
		if name == "" {
			continue
		}

		url := s.Key("baseurl").Value()
		if url == "" {
			continue
		}

		fmt.Printf("%s\t%s\n", name, url)
	}
	return nil
}

// AddRPMList copies rpms into the repodir and calls createrepo_c on it to
// generate a dnf-consumable repository for the bundle builder to use.
func (b *Builder) AddRPMList(rpms []string) error {
	if b.Config.Mixer.LocalRepoDir == "" {
		return errors.Errorf("LOCAL_REPO_DIR not set in configuration")
	}
	err := os.MkdirAll(b.Config.Mixer.LocalRepoDir, 0755)
	if err != nil {
		return errors.Wrapf(err, "couldn't create LOCAL_REPO_DIR")
	}
	for _, rpm := range rpms {
		localPath := filepath.Join(b.Config.Mixer.LocalRPMDir, rpm)
		if err := checkRPM(localPath); err != nil {
			return err
		}
		// Skip RPM already in repo.
		repoPath := filepath.Join(b.Config.Mixer.LocalRepoDir, rpm)
		if _, err := os.Stat(repoPath); err == nil {
			continue
		}
		// Remove source RPMs because they should not be added to mixes
		if strings.HasSuffix(rpm, ".src.rpm") {
			fmt.Printf("Removing %s because source RPMs are not supported in mixes.\n", rpm)
			if err := os.RemoveAll(filepath.Join(b.Config.Mixer.LocalRPMDir, rpm)); err != nil {
				return errors.Wrapf(err, "Failed to remove %s, your mix will not generate properly with source RPMs included.", rpm)
			}
		}
		fmt.Printf("Hardlinking %s to repodir\n", rpm)
		if err := os.Link(localPath, repoPath); err != nil {
			// Fallback to copying the file if hardlink fails.
			err = helpers.CopyFile(repoPath, localPath)
			if err != nil {
				return err
			}
		}
	}

	cmd := exec.Command("createrepo_c", ".")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = b.Config.Mixer.LocalRepoDir

	return cmd.Run()
}

// checkRPM returns nil if path contains a valid RPM file.
func checkRPM(path string) error {
	output, err := exec.Command("file", path).Output()
	if err != nil {
		return errors.Wrapf(err, "couldn't check if %s is a RPM", path)
	}
	if !bytes.Contains(output, []byte("RPM v")) {
		output = bytes.TrimSpace(output)
		return errors.Errorf("file is not a RPM: %s", string(output))
	}
	return nil
}
