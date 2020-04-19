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
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"

	"github.com/clearlinux/mixer-tools/helpers"
	"github.com/clearlinux/mixer-tools/log"

	"github.com/go-ini/ini"
	"github.com/pkg/errors"
)

type dnfRepoConf struct {
	RepoName, RepoURL, RepoPriority string
}

type repoInfo struct {
	cacheDirs map[string]bool
	urlScheme string
	url       string
}

const dnfConfRepoTemplate = `
[{{.RepoName}}]
name={{.RepoName}}
failovermethod=priority
baseurl={{.RepoURL}}
enabled=1
gpgcheck=0
priority={{.RepoPriority}}
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
			return err
		}

		if err = t.Execute(f, conf); err != nil {
			return errors.Wrapf(err, "Failed to write to dnf file: %s", b.Config.Builder.DNFConf)
		}
	}

	if b.Config.Mixer.LocalRepoDir != "" {
		localRepo := path.Join(b.Config.Mixer.LocalRepoDir, "repodata")
		if _, err := os.Stat(localRepo); os.IsNotExist(err) {
			if err = b.createLocalRepo(); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
	}
	return nil
}

// AddRepo adds and enables a repo configuration named <name> pointing at
// URL <url> with priority <priority>. It calls b.NewDNFConfIfNeeded() to
// create the DNF config if it does not exist and performs a check to see
// if the repo passed has already been configured.
func (b *Builder) AddRepo(name, url, priority string) error {
	repo := dnfRepoConf{
		RepoName:     name,
		RepoURL:      url,
		RepoPriority: priority,
	}

	if err := b.NewDNFConfIfNeeded(); err != nil {
		return err
	}

	f, err := os.OpenFile(b.Config.Builder.DNFConf, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
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
		return err
	}

	if err = t.Execute(f, repo); err != nil {
		return errors.Wrapf(err, "Failed to write to dnf file: %s", b.Config.Builder.DNFConf)
	}

	return nil
}

// setRepoVal sets a key/value pair for the specified repo in the DNF config
func (b *Builder) setRepoVal(repo, key, val string) error {
	if err := b.NewDNFConfIfNeeded(); err != nil {
		return err
	}

	DNFConf, err := ini.Load(b.Config.Builder.DNFConf)
	if err != nil {
		return err
	}

	s, err := DNFConf.GetSection(repo)
	if err != nil {
		// Create new repo when setting baseurl for non-existent repo
		if key == "baseurl" {
			return b.AddRepo(repo, val, "1")
		}
		return err
	}

	k, err := s.GetKey(key)
	if err != nil {
		// Create new key section when it doesn't exist
		if k, err = DNFConf.Section(repo).NewKey(key, val); err != nil {
			return err
		}
	}

	k.SetValue(val)
	return DNFConf.SaveTo(b.Config.Builder.DNFConf)
}

// SetURLRepo sets the URL for the repo <repo> to <url>. If <repo> does not exist it is
// created.
func (b *Builder) SetURLRepo(repo, url string) error {
	return b.setRepoVal(repo, "baseurl", url)
}

// SetExcludesRepo sets the excludes for the repo <repo> to [pkgs...]
func (b *Builder) SetExcludesRepo(repo, pkgs string) error {
	return b.setRepoVal(repo, "excludepkgs", pkgs)
}

// SetPriorityRepo sets the priority for the repo
func (b *Builder) SetPriorityRepo(repo, priority string) error {
	p, err := strconv.Atoi(priority)
	if err != nil {
		return err
	}
	if p < 1 || p > 99 {
		return errors.Errorf("repo priority %d must be between 1 and 99", p)
	}
	return b.setRepoVal(repo, "priority", priority)
}

// WriteRepoURLOverrides writes a copy of the DNF conf file
// with overridden baseurl values for the specified repos to
// a tmp config file.
func (b *Builder) WriteRepoURLOverrides(tmpConf *os.File, repoOverrideURLs map[string]string) (map[string]string, error) {
	if err := b.NewDNFConfIfNeeded(); err != nil {
		return nil, err
	}

	DNFConf, err := ini.Load(b.Config.Builder.DNFConf)
	if err != nil {
		return nil, err
	}

	repoURLs := make(map[string]string)
	for repo, r := range b.repos {
		repoURLs[repo] = r.url
	}

	for repo, url := range repoOverrideURLs {
		s, err := DNFConf.GetSection(repo)
		if err != nil {
			// No existing repo, add new section for repo/baseurl pair
			s, err = DNFConf.NewSection(repo)
			if err != nil {
				return nil, err
			}

			repoSettings := map[string]string{
				"failovermethod": "priority",
				"priority":       "1",
				"enabled":        "1",
			}
			repoSettings["name"] = repo
			repoSettings["baseurl"] = url

			for key, val := range repoSettings {
				if _, err = s.NewKey(key, val); err != nil {
					return nil, err
				}
			}
		} else {
			// Override the baseurl for existing repo
			k, err := s.GetKey("baseurl")
			if err != nil {
				return nil, err
			}
			k.SetValue(url)
		}
		repoURLs[repo] = url
	}

	if _, err = DNFConf.WriteTo(tmpConf); err != nil {
		return nil, err
	}

	return repoURLs, nil
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
		log.Debug(log.Mixer, err.Error())
		return errors.Errorf("unable to remove repo %s, does not exist.", name)
	}

	DNFConf.DeleteSection(name)

	err = DNFConf.SaveTo(b.Config.Builder.DNFConf)
	if err != nil {
		return err
	}

	log.Info(log.Mixer, "Removing repo %s", name)
	return nil
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

	b.repos = make(map[string]repoInfo)

	for _, s := range DNFConf.Sections() {
		var repo repoInfo
		name := s.Name()
		if name == "" {
			continue
		}

		u := s.Key("baseurl").Value()
		if u == "" {
			continue
		}

		pURL, err := url.Parse(u)
		if err != nil {
			return err
		}
		if pURL.Scheme == "" {
			pURL.Scheme = "file"
		}

		priority := s.Key("priority").Value()
		if priority == "" {
			// When the priority field is omitted, the DNF default is 99.
			priority = "99"
		}

		log.Info(log.Mixer, "%s\t%s\t%s", name, priority, pURL.String())

		repo.urlScheme = pURL.Scheme
		repo.url = pURL.String()
		repo.cacheDirs = make(map[string]bool)
		if pURL.Scheme == "file" {
			repo.cacheDirs[pURL.String()] = true
		} else {
			repo.cacheDirs[dnfDownloadDir] = true
		}

		b.repos[name] = repo
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
			log.Info(log.Mixer, "Removing %s because source RPMs are not supported in mixes.", rpm)
			if err := os.RemoveAll(filepath.Join(b.Config.Mixer.LocalRPMDir, rpm)); err != nil {
				return errors.Wrapf(err, "Failed to remove %s, your mix will not generate properly with source RPMs included.", rpm)
			}
		}
		log.Info(log.Mixer, "Hardlinking %s to repodir", rpm)
		if err := os.Link(localPath, repoPath); err != nil {
			// Fallback to copying the file if hardlink fails.
			err = helpers.CopyFile(repoPath, localPath)
			if err != nil {
				return err
			}
		}
	}
	return b.createLocalRepo()
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

func (b *Builder) createLocalRepo() error {
	if _, err := os.Stat(b.Config.Mixer.LocalRepoDir); err != nil {
		return err
	}
	cmd := exec.Command("createrepo_c", ".")
	var out bytes.Buffer
	var errBuf bytes.Buffer

	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	cmd.Dir = b.Config.Mixer.LocalRepoDir

	err := cmd.Run()
	if err != nil {
		log.Error(log.CreateRepo, errBuf.String()+err.Error())
		log.Debug(log.CreateRepo, out.String())
		return fmt.Errorf("createrepo_c failed for directory %s", b.Config.Mixer.LocalRepoDir)
	}
	log.Verbose(log.CreateRepo, out.String())
	return err
}
