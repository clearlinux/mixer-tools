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

package builder

import (
	"archive/tar"
	"bufio"
	"bytes"
	"crypto/rsa"
	"crypto/x509"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	textTemplate "text/template" // "template" conflicts with crypto/x509

	"github.com/clearlinux/mixer-tools/helpers"
	"github.com/clearlinux/mixer-tools/swupd"
	"github.com/pkg/errors"
)

// Version of Mixer. Also used by the Makefile for releases.
const Version = "4.1.2"

// UseNewSwupdServer controls whether to use the new implementation of
// swupd-server (package swupd) when possible. This is an experimental feature.
var UseNewSwupdServer = false

// UseNewChrootBuilder controls whether to use the new implementation of
// building the bundles. This is an experimental feature.
// We don't build chroots any more but leaving this as UseNewChrootBuilder since
// it will be removed soon.
var UseNewChrootBuilder = false

// Offline controls whether mixer attempts to automatically cache upstream
// bundles. In offline mode, all necessary bundles must exist in local-bundles.
var Offline = false

// A Builder contains all configurable fields required to perform a full mix
// operation, and is used to encapsulate life time data.
type Builder struct {
	BuildScript string
	BuildConf   string

	BundleDir         string
	Cert              string
	Format            string
	LocalBundleDir    string
	MixVer            string
	MixVerFile        string
	MixBundlesFile    string
	LocalPackagesFile string
	RepoDir           string
	RPMDir            string
	StateDir          string
	UpstreamURL       string
	UpstreamURLFile   string
	UpstreamVer       string
	UpstreamVerFile   string
	VersionDir        string
	DNFConf           string

	Signing int
	Bump    int

	NumFullfileWorkers int
	NumDeltaWorkers    int
	NumBundleWorkers   int

	// Parsed versions.
	MixVerUint32      uint32
	UpstreamVerUint32 uint32
}

var localPackages = make(map[string]bool)
var upstreamPackages = make(map[string]bool)

// New will return a new instance of Builder with some predetermined sane
// default values.
func New() *Builder {
	return &Builder{
		BuildScript:       "bundle-chroot-builder.py",
		UpstreamURLFile:   "upstreamurl",
		UpstreamVerFile:   "upstreamversion",
		MixBundlesFile:    "mixbundles",
		LocalPackagesFile: "local-packages",
		MixVerFile:        "mixversion",

		Signing: 1,
		Bump:    0,
	}
}

// NewFromConfig creates a new Builder with the given Configuration.
func NewFromConfig(conf string) (*Builder, error) {
	b := New()
	if err := b.LoadBuilderConf(conf); err != nil {
		return nil, err
	}
	if err := b.ReadBuilderConf(); err != nil {
		return nil, err
	}
	if err := b.ReadVersions(); err != nil {
		return nil, err
	}
	return b, nil
}

// CreateDefaultConfig creates a default builder.conf using the active
// directory as base path for the variables values.
func (b *Builder) CreateDefaultConfig(localrpms bool) error {
	pwd, err := os.Getwd()
	if err != nil {
		return err
	}

	builderconf := filepath.Join(pwd, "builder.conf")

	err = helpers.CopyFileNoOverwrite(builderconf, "/usr/share/defaults/bundle-chroot-builder/builder.conf")
	if os.IsExist(err) {
		// builder.conf already exists. Skip creation.
		return nil
	} else if err != nil {
		return err
	}

	fmt.Println("Creating new builder.conf configuration file...")

	raw, err := ioutil.ReadFile(builderconf)
	if err != nil {
		return err
	}

	// Patch all default path prefixes to PWD
	data := strings.Replace(string(raw), "/home/clr/mix", pwd, -1)

	// Add [Mixer] section
	data += "\n[Mixer]\n"
	data += "LOCAL_BUNDLE_DIR=" + filepath.Join(pwd, "local-bundles") + "\n"

	if localrpms {
		data += "LOCAL_RPM_DIR=" + filepath.Join(pwd, "local-rpms") + "\n"
		data += "LOCAL_REPO_DIR=" + filepath.Join(pwd, "local-yum") + "\n"
	}

	if err = ioutil.WriteFile(builderconf, []byte(data), 0666); err != nil {
		return err
	}
	return nil
}

// initDirs creates the directories mixer uses
func (b *Builder) initDirs() error {
	// Create folder to store local rpms if defined but doesn't already exist
	if b.RPMDir != "" {
		if err := os.MkdirAll(b.RPMDir, 0777); err != nil {
			return errors.Wrap(err, "Failed to create local rpms directory")
		}
	}

	// Create folder for local dnf repo if defined but doesn't already exist
	if b.RepoDir != "" {
		if err := os.MkdirAll(b.RepoDir, 0777); err != nil {
			return errors.Wrap(err, "Failed to create local rpms directory")
		}
	}

	// Create folder for local bundle files
	if err := os.MkdirAll(b.LocalBundleDir, 0777); err != nil {
		return errors.Wrap(err, "Failed to create local bundles directory")
	}

	return nil
}

// Get latest CLR version
func (b *Builder) getLatestUpstreamVersion() (string, error) {
	return b.DownloadFileFromUpstream("/latest")
}

// DownloadFileFromUpstream will download a file from the Upstream URL
// joined with the passed subpath. It will trim spaces from the result.
func (b *Builder) DownloadFileFromUpstream(subpath string) (string, error) {
	// Build the URL
	end, err := url.Parse(subpath)
	if err != nil {
		return "", err
	}
	base, err := url.Parse(b.UpstreamURL)
	if err != nil {
		return "", err
	}

	resolved := base.ResolveReference(end).String()
	// Fetch the version and parse it
	resp, err := http.Get(resolved)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("got status %q when downloading: %s", resp.Status, resolved)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(body)), nil
}

const mixDirGitIgnore = `upstream-bundles/
mix-bundles/`

// InitMix will initialise a new swupd-client consumable "mix" with the given
// based Clear Linux version and specified mix version.
func (b *Builder) InitMix(upstreamVer string, mixVer string, allLocal bool, allUpstream bool, upstreamURL string, git bool) error {
	// Set up local dirs
	if err := b.initDirs(); err != nil {
		return err
	}

	// Set up mix metadata
	// Deprecate '.clearurl' --> 'upstreamurl'
	if _, err := os.Stat(filepath.Join(b.VersionDir, ".clearurl")); err == nil {
		b.UpstreamURLFile = ".clearurl"
		fmt.Println("Warning: '.clearurl' has been deprecated. Please rename file to 'upstreamurl'")
	}
	if err := ioutil.WriteFile(filepath.Join(b.VersionDir, b.UpstreamURLFile), []byte(upstreamURL), 0644); err != nil {
		return err
	}
	b.UpstreamURL = upstreamURL

	if upstreamVer == "latest" {
		ver, err := b.getLatestUpstreamVersion()
		if err != nil {
			return errors.Wrap(err, "Failed to retrieve latest published upstream version")
		}
		upstreamVer = ver
	}

	fmt.Printf("Initializing mix version %s from upstream version %s\n", mixVer, upstreamVer)

	// Deprecate '.clearversion' --> 'upstreamversion'
	if _, err := os.Stat(filepath.Join(b.VersionDir, ".clearversion")); err == nil {
		b.UpstreamVerFile = ".clearversion"
		fmt.Println("Warning: '.clearversion' has been deprecated. Please rename file to 'upstreamversion'")
	}
	if err := ioutil.WriteFile(filepath.Join(b.VersionDir, b.UpstreamVerFile), []byte(upstreamVer), 0644); err != nil {
		return err
	}
	b.UpstreamVer = upstreamVer

	// Deprecate '.mixversion' --> 'mixversion'
	if _, err := os.Stat(filepath.Join(b.VersionDir, ".mixversion")); err == nil {
		b.MixVerFile = ".mixversion"
		fmt.Println("Warning: '.mixversion' has been deprecated. Please rename file to 'mixversion'")
	}
	if err := ioutil.WriteFile(filepath.Join(b.VersionDir, b.MixVerFile), []byte(mixVer), 0644); err != nil {
		return err
	}
	b.MixVer = mixVer

	// Parse strings into valid version numbers.
	var err error
	b.MixVerUint32, err = parseUint32(b.MixVer)
	if err != nil {
		return errors.Wrapf(err, "Couldn't parse mix version")
	}
	b.UpstreamVerUint32, err = parseUint32(b.UpstreamVer)
	if err != nil {
		return errors.Wrapf(err, "Couldn't parse upstream version")
	}

	// Initialize the Mix Bundles List
	if _, err := os.Stat(filepath.Join(b.VersionDir, b.MixBundlesFile)); os.IsNotExist(err) {
		// Add default bundles (or all)
		defaultBundles := []string{"os-core", "os-core-update", "bootloader", "kernel-native"}
		if err := b.AddBundles(defaultBundles, allLocal, allUpstream, false); err != nil {
			return err
		}
	}

	// Get upstream bundles
	if err := b.getUpstreamBundles(upstreamVer, true); err != nil {
		return err
	}

	if git {
		if err := ioutil.WriteFile(".gitignore", []byte(mixDirGitIgnore), 0644); err != nil {
			return errors.Wrap(err, "Failed to create .gitignore file")
		}

		// Init repo and add initial commit
		if err := helpers.Git("init"); err != nil {
			return err
		}
		if err := helpers.Git("add", "."); err != nil {
			return err
		}
		commitMsg := fmt.Sprintf("Initial mix version %s from upstream version %s", b.MixVer, b.UpstreamVer)
		if err := helpers.Git("commit", "-m", commitMsg); err != nil {
			return err
		}
	}

	return nil
}

// LoadBuilderConf will read the builder configuration from the command line if
// it was provided, otherwise it will fall back to reading the configuration from
// the local builder.conf file.
func (b *Builder) LoadBuilderConf(builderconf string) error {
	local, err := os.Getwd()
	if err != nil {
		return err
	}

	// If builderconf is set via cmd line, use that one
	if len(builderconf) > 0 {
		b.BuildConf = builderconf
		return nil
	}

	// Check if there's a local builder.conf if one wasn't supplied
	localpath := filepath.Join(local, "builder.conf")
	if _, err := os.Stat(localpath); err == nil {
		b.BuildConf = localpath
	} else {
		return errors.Wrap(err, "Cannot find any builder.conf to use")
	}

	return nil
}

// ReadBuilderConf will populate the configuration data from the builder
// configuration file, which is mandatory information for performing a mix.
func (b *Builder) ReadBuilderConf() error {
	lines, err := helpers.ReadFileAndSplit(b.BuildConf)
	if err != nil {
		return errors.Wrap(err, "Failed to read buildconf")
	}

	// Map the builder values to the regex here to make it easier to assign
	fields := []struct {
		re       string
		dest     *string
		required bool
	}{
		{`^BUNDLE_DIR\s*=\s*`, &b.BundleDir, true}, //Note: Can be removed once UseNewChrootBuilder is obsolete
		{`^CERT\s*=\s*`, &b.Cert, true},
		{`^CLEARVER\s*=\s*`, &b.UpstreamVer, false},
		{`^FORMAT\s*=\s*`, &b.Format, true},
		{`^LOCAL_BUNDLE_DIR\s*=\s*`, &b.LocalBundleDir, false},
		{`^MIXVER\s*=\s*`, &b.MixVer, false},
		{`^LOCAL_REPO_DIR\s*=\s*`, &b.RepoDir, false},
		{`^LOCAL_RPM_DIR\s*=\s*`, &b.RPMDir, false},
		{`^SERVER_STATE_DIR\s*=\s*`, &b.StateDir, true},
		{`^VERSIONS_PATH\s*=\s*`, &b.VersionDir, true},
		{`^YUM_CONF\s*=\s*`, &b.DNFConf, true},
	}

	for _, h := range fields {
		r := regexp.MustCompile(h.re)
		// Look for Environment variables in the config file
		re := regexp.MustCompile(`\$\{?([[:word:]]+)\}?`)
		for _, i := range lines {
			if m := r.FindIndex([]byte(i)); m != nil {
				// We want the variable without the $ or {} for lookup checking
				matches := re.FindAllStringSubmatch(i[m[1]:], -1)
				for _, s := range matches {
					if _, ok := os.LookupEnv(s[1]); !ok {
						return errors.Errorf("buildconf contains an undefined environment variable: %s", s[1])
					}
				}

				// Replace valid Environment Variables
				*h.dest = os.ExpandEnv(i[m[1]:])
			}
		}

		if h.required && *h.dest == "" {
			missing := h.re
			re := regexp.MustCompile(`([[:word:]]+)\\s\*=`)
			if matches := re.FindStringSubmatch(h.re); matches != nil {
				missing = matches[1]
			}

			return errors.Errorf("buildconf missing entry for variable: %s", missing)
		}
	}

	if b.LocalBundleDir == "" {
		pwd, err := os.Getwd()
		if err != nil {
			return err
		}
		b.LocalBundleDir = filepath.Join(pwd, "local-bundles")
		fmt.Printf("WARNING: LOCAL_BUNDLE_DIR not found in builder.conf. Falling back to %q.\n", b.LocalBundleDir)
		fmt.Println("Please set this value to the location you want local bundle definition files to be stored.")
	}

	return nil
}

// ReadVersions will initialise the mix versions (mix and clearlinux) from
// the configuration files in the version directory.
func (b *Builder) ReadVersions() error {
	// Deprecate '.mixversion' --> 'mixversion'
	if _, err := os.Stat(filepath.Join(b.VersionDir, ".mixversion")); err == nil {
		b.MixVerFile = ".mixversion"
		fmt.Println("Warning: '.mixversion' has been deprecated. Please rename file to 'mixversion'")
	}
	ver, err := ioutil.ReadFile(filepath.Join(b.VersionDir, b.MixVerFile))
	if err != nil {
		return err
	}
	b.MixVer = strings.TrimSpace(string(ver))
	b.MixVer = strings.Replace(b.MixVer, "\n", "", -1)

	// Deprecate '.clearversion' --> 'upstreamversion'
	if _, err = os.Stat(filepath.Join(b.VersionDir, ".clearversion")); err == nil {
		b.UpstreamVerFile = ".clearversion"
		fmt.Println("Warning: '.clearversion' has been deprecated. Please rename file to 'upstreamversion'")
	}
	ver, err = ioutil.ReadFile(filepath.Join(b.VersionDir, b.UpstreamVerFile))
	if err != nil {
		return err
	}
	b.UpstreamVer = strings.TrimSpace(string(ver))
	b.UpstreamVer = strings.Replace(b.UpstreamVer, "\n", "", -1)

	// Deprecate '.clearversion' --> 'upstreamurl'
	if _, err = os.Stat(filepath.Join(b.VersionDir, ".clearurl")); err == nil {
		b.UpstreamURLFile = ".clearurl"
		fmt.Println("Warning: '.clearurl' has been deprecated. Please rename file to 'upstreamurl'")
	}
	ver, err = ioutil.ReadFile(filepath.Join(b.VersionDir, b.UpstreamURLFile))
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: %s/%s does not exist, run mixer init to generate\n", b.VersionDir, b.UpstreamURLFile)
		b.UpstreamURL = ""
	} else {
		b.UpstreamURL = strings.TrimSpace(string(ver))
		b.UpstreamURL = strings.Replace(b.UpstreamURL, "\n", "", -1)
	}

	// Parse strings into valid version numbers.
	b.MixVerUint32, err = parseUint32(b.MixVer)
	if err != nil {
		return errors.Wrapf(err, "Couldn't parse mix version")
	}
	b.UpstreamVerUint32, err = parseUint32(b.UpstreamVer)
	if err != nil {
		return errors.Wrapf(err, "Couldn't parse upstream version")
	}

	return nil
}

// SignManifestMoM will sign the Manifest.MoM file in in place based on the Mix
// version read from builder.conf.
func (b *Builder) SignManifestMoM() error {
	mom := filepath.Join(b.StateDir, "www", b.MixVer, "Manifest.MoM")
	sig := mom + ".sig"

	// Call openssl because signing and pkcs7 stuff is not well supported in Go yet.
	cmd := exec.Command("openssl", "smime", "-sign", "-binary", "-in", mom,
		"-signer", b.Cert, "-inkey", filepath.Dir(b.Cert)+"/private.pem",
		"-outform", "DER", "-out", sig)

	// Capture the output as it is useful in case of errors.
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to sign Manifest.MoM:\n%s", out.String())
	}
	return nil
}

const (
	upstreamBundlesBaseDir   = "upstream-bundles"
	upstreamBundlesVerDirFmt = "clr-bundles-%s"
	upstreamBundlesBundleDir = "bundles"
)

func getUpstreamBundlesVerDir(ver string) string {
	return fmt.Sprintf(upstreamBundlesVerDirFmt, ver)
}
func getUpstreamBundlesPath(ver string) string {
	return filepath.Join(upstreamBundlesBaseDir, fmt.Sprintf(upstreamBundlesVerDirFmt, ver), upstreamBundlesBundleDir)
}

func (b *Builder) getLocalPackagesPath() string {
	return filepath.Join(b.VersionDir, b.LocalPackagesFile)
}

func (b *Builder) getUpstreamBundles(ver string, prune bool) error {
	if Offline {
		return nil
	}

	// Make the folder to store upstream bundles if does not exist
	if err := os.MkdirAll(upstreamBundlesBaseDir, 0777); err != nil {
		return errors.Wrap(err, "Failed to create upstream-bundles dir.")
	}

	bundleDir := getUpstreamBundlesPath(ver)

	// Clear out other bundle dirs if needed
	if prune {
		files, err := ioutil.ReadDir(upstreamBundlesBaseDir)
		if err != nil {
			return errors.Wrap(err, "Failed to read bundles for pruning.")
		}
		curver := getUpstreamBundlesVerDir(ver)
		for _, file := range files {
			// Skip the current version cache, but delete all others
			if file.Name() != curver {
				if err = os.RemoveAll(filepath.Join(upstreamBundlesBaseDir, file.Name())); err != nil {
					return errors.Wrapf(err, "Failed to remove %s while pruning bundles", file.Name())
				}
			}
		}
	}

	// Check if bundles already exist
	if _, err := os.Stat(bundleDir); err == nil {
		return nil
	}

	tmptarfile := filepath.Join(upstreamBundlesBaseDir, ver+".tar.gz")
	URL := "https://github.com/clearlinux/clr-bundles/archive/" + ver + ".tar.gz"
	if err := helpers.Download(tmptarfile, URL); err != nil {
		return errors.Wrapf(err, "Failed to download bundles for upstream version %s", ver)
	}

	if err := helpers.UnpackFile(tmptarfile, upstreamBundlesBaseDir); err != nil {
		err = errors.Wrapf(err, "Error unpacking bundles for upstream version %s\n%s left for debuging", ver, tmptarfile)

		// Clean up upstream bundle dir, since unpack failed
		path := filepath.Join(upstreamBundlesBaseDir, getUpstreamBundlesVerDir(ver))
		if rerr := os.RemoveAll(path); rerr != nil {
			err = errors.Wrapf(err, "Error cleaning up upstream bundle dir: %s", path)
		}
		return err
	}

	return errors.Wrapf(os.Remove(tmptarfile), "Failed to remove temp bundle archive: %s", tmptarfile)
}

func setPackagesList(source *map[string]bool, filename string) error {
	var err error
	if len(*source) > 0 {
		return nil
	}
	if _, err = os.Stat(filename); os.IsNotExist(err) {
		return nil
	}

	*source, err = parsePackageBundleFile(filename)
	return err
}

// getBundlePath returns the path to the bundle definition file for a given
// bundle name, or error if it cannot be found. Looks in the following order:
// local-bundles/
// local-packages
// upstream-bundles/clr-bundles-<ver>/bundles/
// upstream-bundles/clr-bundles-<ver>/packages
func (b *Builder) getBundlePath(bundle string) (string, error) {
	// Check local-bundles
	path := filepath.Join(b.LocalBundleDir, bundle)
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}

	// Check local-packages
	path = b.getLocalPackagesPath()
	err := setPackagesList(&localPackages, path)
	if err != nil {
		return "", err
	}

	if _, ok := localPackages[bundle]; ok {
		return path, nil
	}

	// Check upstream-bundles
	path = filepath.Join(getUpstreamBundlesPath(b.UpstreamVer), bundle)
	if _, err = os.Stat(path); err == nil {
		return path, nil
	}

	// Check upstream-packages
	path = filepath.Join(upstreamBundlesBaseDir, getUpstreamBundlesVerDir(b.UpstreamVer), "packages")
	err = setPackagesList(&upstreamPackages, path)
	if err != nil {
		return "", err
	}

	if _, ok := upstreamPackages[bundle]; ok {
		return path, nil
	}

	return "", errors.Errorf("Cannot find bundle %q in local or upstream bundles", bundle)
}

// isLocalBundle checks to see if a bundle filepath is a local bundle definition or package file
func (b *Builder) isLocalBundle(path string) bool {
	return strings.HasPrefix(path, b.LocalBundleDir) || b.isLocalPackagePath(path)
}

func getBundleSetKeys(set bundleSet) []string {
	keys := make([]string, len(set))
	i := 0
	for k := range set {
		keys[i] = k
		i++
	}
	return keys
}

func getBundleSetKeysSorted(set bundleSet) []string {
	keys := getBundleSetKeys(set)
	sort.Strings(keys)
	return keys
}

// isLocalPackagePath checks if path is a local-packages definition file
func (b *Builder) isLocalPackagePath(path string) bool {
	return strings.HasSuffix(path, b.LocalPackagesFile)
}

// isUpstreamPackagePath checks if path is an upstream packages definition file
func isUpstreamPackagePath(path string) bool {
	return strings.HasSuffix(path, "/packages")
}

// isPathToPackageFile checks if the path is a local or upstream package definition file
func (b *Builder) isPathToPackageFile(path string) bool {
	return b.isLocalPackagePath(path) || isUpstreamPackagePath(path)
}

func (b *Builder) getBundleFromName(name string) (*bundle, error) {
	var bundle *bundle
	path, err := b.getBundlePath(name)
	if err != nil {
		return nil, err
	}

	if b.isPathToPackageFile(path) {
		return newBundleFromPackage(name, path)
	}

	bundle, err = parseBundleFile(path)
	if err != nil {
		return nil, err
	}
	if err = validateBundleFilename(path); err != nil {
		return nil, err
	}

	return bundle, nil
}

// getMixBundlesListAsSet reads in the Mix Bundles List file and returns the
// resultant set of unique bundle objects. If the mix bundles file does not
// exist or is empty, an empty set is returned.
func (b *Builder) getMixBundlesListAsSet() (bundleSet, error) {
	set := make(bundleSet)

	bundles, err := helpers.ReadFileAndSplit(filepath.Join(b.VersionDir, b.MixBundlesFile))
	if os.IsNotExist(err) {
		return set, nil
	} else if err != nil {
		return nil, errors.Wrap(err, "Failed to read in Mix Bundle List")
	}

	for _, bName := range bundles {
		bName = strings.TrimSpace(bName)
		if bName == "" {
			continue
		}

		bundle, err := b.getBundleFromName(bName)
		if err != nil {
			return nil, err
		}
		set[bName] = bundle
	}
	return set, nil
}

// getDirBundlesListAsSet reads the files in a directory and returns the
// resultant set of unique bundle objects. If the directory is empty, an empty
// set is returned.
func (b *Builder) getDirBundlesListAsSet(dir string) (bundleSet, error) {
	set := make(bundleSet)

	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to read bundles dir: %s", dir)
	}

	for _, file := range files {
		bundle, err := b.getBundleFromName(file.Name())
		if err != nil {
			return nil, err
		}
		set[file.Name()] = bundle
	}
	return set, nil
}

// writeMixBundleList writes the contents of a bundle set out to the Mix Bundles
// List file. Values will be in sorted order.
func (b *Builder) writeMixBundleList(set bundleSet) error {
	data := []byte(strings.Join(getBundleSetKeysSorted(set), "\n") + "\n")
	if err := ioutil.WriteFile(filepath.Join(b.VersionDir, b.MixBundlesFile), data, 0644); err != nil {
		return errors.Wrap(err, "Failed to write out Mix Bundle List")
	}
	return nil
}

// getFullBundleSet takes a set of bundle names to traverse, and returns a full
// set of recursively-parsed bundle objects.
func (b *Builder) getFullBundleSet(bundles bundleSet) (bundleSet, error) {
	set := make(bundleSet)

	// recurseBundleSet adds a list of bundles to a bundle set,
	// recursively adding any bundles included by those in the list.
	var recurseBundleSet func(bundles []string) error
	recurseBundleSet = func(bundles []string) error {
		for _, bName := range bundles {
			if _, exists := set[bName]; !exists {
				bundle, err := b.getBundleFromName(bName)
				if err != nil {
					return err
				}
				set[bName] = bundle

				if len(bundle.DirectIncludes) > 0 {
					err := recurseBundleSet(bundle.DirectIncludes)
					if err != nil {
						return err
					}
				}
			}
		}
		return nil
	}

	err := recurseBundleSet(getBundleSetKeys(bundles))
	if err != nil {
		return nil, err
	}

	return set, nil
}

func populateSetFromPackages(source *map[string]bool, dest bundleSet, filename string) error {
	var err error
	err = setPackagesList(source, filename)
	if err != nil {
		return errors.Wrapf(err, "Failed to read packages file: %s", filename)
	}
	for k := range *source {
		if _, ok := dest[k]; ok {
			fmt.Printf("Bundle %q already in mix; skipping\n", k)
			continue
		}
		dest[k], err = newBundleFromPackage(k, filename)
		if err != nil {
			return errors.Wrapf(err, "Failed to add %q bundle to mix", k)
		}
	}
	return nil
}

// getFullMixBundleSet returns the full set of mix bundle objects. It is a
// convenience function that is equivalent to calling getFullBundleSet on the
// results of getMixBundlesListAsSet.
func (b *Builder) getFullMixBundleSet() (bundleSet, error) {
	bundles, err := b.getMixBundlesListAsSet()
	if err != nil {
		return nil, err
	}
	set, err := b.getFullBundleSet(bundles)
	if err != nil {
		return nil, err
	}
	return set, nil
}

// AddBundles adds the specified bundles to the Mix Bundles List. Values are
// verified as valid, and duplicate values are removed. The resulting Mix
// Bundles List will be in sorted order.
func (b *Builder) AddBundles(bundles []string, allLocal bool, allUpstream bool, git bool) error {
	// Fetch upstream bundle files if needed
	if err := b.getUpstreamBundles(b.UpstreamVer, true); err != nil {
		return err
	}

	// Read in current mix bundles list
	set, err := b.getMixBundlesListAsSet()
	if err != nil {
		return err
	}

	// Add the ones passed in to the set
	for _, bName := range bundles {
		if _, exists := set[bName]; exists {
			fmt.Printf("Bundle %q already in mix; skipping\n", bName)
			continue
		}

		bundle, err := b.getBundleFromName(bName)
		if err != nil {
			return err
		}
		if b.isLocalBundle(bundle.Filename) {
			fmt.Printf("Adding bundle %q from local bundles\n", bName)
		} else {
			fmt.Printf("Adding bundle %q from upstream bundles\n", bName)
		}
		set[bName] = bundle
	}

	// Add all local bundles to the bundles
	if allLocal {
		localSet, err := b.getDirBundlesListAsSet(b.LocalBundleDir)
		if err != nil {
			return errors.Wrapf(err, "Failed to read local bundles dir: %s", b.LocalBundleDir)
		}
		// handle packages defined in local-packages, if it exists
		err = populateSetFromPackages(&localPackages, localSet, b.getLocalPackagesPath())
		if err != nil {
			return err
		}

		for _, bundle := range localSet {
			if _, exists := set[bundle.Name]; exists {
				fmt.Printf("Bundle %q already in mix; skipping\n", bundle.Name)
				continue
			}

			set[bundle.Name] = bundle
			fmt.Printf("Adding bundle %q from local bundles\n", bundle.Name)
		}
	}

	// Add all upstream bundles to the bundles
	if allUpstream {
		upstreamBundleDir := getUpstreamBundlesPath(b.UpstreamVer)
		upstreamSet, err := b.getDirBundlesListAsSet(upstreamBundleDir)
		if err != nil {
			return errors.Wrapf(err, "Failed to read upstream bundles dir: %s", upstreamBundleDir)
		}
		// handle packages defined in upstream packages file, if it exists
		upstreamPackagesFile := filepath.Join(upstreamBundlesBaseDir, getUpstreamBundlesVerDir(b.UpstreamVer), "packages")
		err = populateSetFromPackages(&upstreamPackages, upstreamSet, upstreamPackagesFile)
		if err != nil {
			return err
		}

		for _, bundle := range upstreamSet {
			if _, exists := set[bundle.Name]; exists {
				fmt.Printf("Bundle %q already in mix; skipping\n", bundle.Name)
				continue
			}

			set[bundle.Name] = bundle
			fmt.Printf("Adding bundle %q from upstream bundles\n", bundle.Name)
		}
	}

	// Write final mix bundle list back to file
	if err := b.writeMixBundleList(set); err != nil {
		return err
	}

	if git {
		fmt.Println("Adding git commit")
		if err := helpers.Git("add", "."); err != nil {
			return err
		}
		commitMsg := fmt.Sprintf("Added bundles from local-bundles or upstream version %s\n\nBundles added: %v", b.UpstreamVer, bundles)
		if err := helpers.Git("commit", "-q", "-m", commitMsg); err != nil {
			return err
		}
	}
	return nil
}

// RemoveBundles removes a list of bundles from the Mix Bundles List. If a
// bundle is not present, it is skipped. If 'local' is passed, the corresponding
// bundle file is removed from local-bundles. Note that this is an irreversible
// step. The Mix Bundles List is validated when read in, and the resulting Mix
// Bundles List will be in sorted order.
func (b *Builder) RemoveBundles(bundles []string, mix bool, local bool, git bool) error {
	// Fetch upstream bundle files if needed
	if err := b.getUpstreamBundles(b.UpstreamVer, true); err != nil {
		return err
	}

	// Read in current mix bundles list
	set, err := b.getMixBundlesListAsSet()
	if err != nil {
		return err
	}

	// Remove the ones passed in from the set
	for _, bundle := range bundles {
		_, inMix := set[bundle]

		if local {
			if _, err := os.Stat(filepath.Join(b.LocalBundleDir, bundle)); err == nil {
				fmt.Printf("Removing bundle file for %q from local-bundles\n", bundle)
				if err := os.Remove(filepath.Join(b.LocalBundleDir, bundle)); err != nil {
					return errors.Wrapf(err, "Cannot remove bundle file for %q from local-bundles", bundle)
				}

				if !mix && inMix {
					// Check if bundle is still available upstream
					if _, err := b.getBundlePath(bundle); err != nil {
						fmt.Printf("Warning: Invalid bundle left in mix: %q\n", bundle)
					} else {
						fmt.Printf("Mix bundle %q now points to upstream\n", bundle)
					}
				}
			} else {
				fmt.Printf("Bundle %q not found in local-bundles; skipping\n", bundle)
			}
		}

		if mix {
			if inMix {
				fmt.Printf("Removing bundle %q from mix\n", bundle)
				delete(set, bundle)
			} else {
				fmt.Printf("Bundle %q not found in mix; skipping\n", bundle)
			}
		}
	}

	// Write final mix bundle list back to file, only if the Mix Bundle List was edited
	if mix {
		if err := b.writeMixBundleList(set); err != nil {
			return err
		}
	}

	if git {
		fmt.Println("Adding git commit")
		if err := helpers.Git("add", "."); err != nil {
			return err
		}
		commitMsg := fmt.Sprintf("Bundles removed: %v", bundles)
		if err := helpers.Git("commit", "-q", "-m", commitMsg); err != nil {
			return err
		}
	}

	return nil
}

const (
	// Using the Unicode "Box Drawing" group
	treeNil = "    "
	treeBar = "│   "
	treeMid = "├── "
	treeEnd = "└── "
)

func (b *Builder) buildTreePrintValue(bundle *bundle, level int, levelEnded []bool) string {
	// Set up the value for this bundle
	value := bundle.Name
	if b.isLocalBundle(bundle.Filename) {
		value += " (local)"
	} else {
		value += " (upstream)"
	}

	if level == 0 {
		return value
	}

	var buff bytes.Buffer
	// Add continuation bars if earlier levels have not ended
	for i := 0; i < level-1; i++ {
		if levelEnded[i] {
			buff.WriteString(treeNil)
		} else {
			buff.WriteString(treeBar)
		}
	}

	// Add a mid bar or an end bar
	if levelEnded[level-1] {
		buff.WriteString(treeEnd)
	} else {
		buff.WriteString(treeMid)
	}

	// Add the actual value
	buff.WriteString(value)

	return buff.String()
}

func (b *Builder) bundleTreePrint(set bundleSet, bundle string, level int, levelEnded []bool) {
	fmt.Println(b.buildTreePrintValue(set[bundle], level, levelEnded))

	levelEnded = append(levelEnded, false)
	last := len(set[bundle].DirectIncludes) - 1
	for i, include := range set[bundle].DirectIncludes {
		levelEnded[level] = i == last
		b.bundleTreePrint(set, include, level+1, levelEnded)
	}
}

type listType int

// Enum of available list types
const (
	MixList      listType = iota // List bundles in the mix (with includes)
	LocalList                    // List bundles available locally
	UpstreamList                 // List bundles available upstream
)

// ListBundles prints out a bundle list in either a flat list or tree view
func (b *Builder) ListBundles(listType listType, tree bool) error {
	// Fetch upstream bundle files if needed
	if err := b.getUpstreamBundles(b.UpstreamVer, true); err != nil {
		return err
	}

	var bundles bundleSet

	// Get the bundle sets used for processing
	mixBundles, err := b.getMixBundlesListAsSet()
	if err != nil {
		return err
	}
	localBundles, err := b.getDirBundlesListAsSet(b.LocalBundleDir)
	if err != nil {
		return err
	}
	upstreamBundles, err := b.getDirBundlesListAsSet(getUpstreamBundlesPath(b.UpstreamVer))
	if err != nil {
		if !Offline {
			return err
		}
		upstreamBundles = make(bundleSet)
	}

	// Assign "top level" bundles
	switch listType {
	case MixList:
		bundles = mixBundles
	case LocalList:
		bundles = localBundles
	case UpstreamList:
		bundles = upstreamBundles
	}

	// Create the full, parsed set
	set, err := b.getFullBundleSet(bundles)
	if err != nil {
		return err
	}

	if tree {
		// Print the tree view
		sorted := getBundleSetKeysSorted(bundles)
		for _, bundle := range sorted {
			b.bundleTreePrint(set, bundle, 0, make([]bool, 0))
		}

		return nil
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', 0)

	// Print a flat list and return
	switch listType {
	case MixList:
		// Print the full, parsed set
		sorted := getBundleSetKeysSorted(set)
		for _, bundle := range sorted {
			var location string
			if _, exists := localBundles[bundle]; exists {
				location = "(local)"
			} else {
				location = "(upstream)"
			}
			var included string
			if _, exists := bundles[bundle]; !exists {
				included = "(included)"
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\n", bundle, location, included)
		}
	case LocalList:
		// Only print the top-level set
		sorted := getBundleSetKeysSorted(bundles)
		for _, bundle := range sorted {
			var mix string
			if _, exists := mixBundles[bundle]; exists {
				mix = "(in mix)"
			}
			var masking string
			if _, exists := upstreamBundles[bundle]; exists {
				masking = "(masking upstream)"
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\n", bundle, mix, masking)
		}
	case UpstreamList:
		// Only print the top-level set
		sorted := getBundleSetKeysSorted(bundles)
		for _, bundle := range sorted {
			var mix string
			if _, exists := mixBundles[bundle]; exists {
				mix = "(in mix)"
			}
			var masked string
			if _, exists := localBundles[bundle]; exists {
				masked = "(masked by local)"
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\n", bundle, mix, masked)
		}
	}

	_ = tw.Flush()

	return nil
}

func getEditorCmd() (string, error) {
	cmd := os.Getenv("VISUAL")
	if cmd != "" {
		return cmd, nil
	}

	cmd = os.Getenv("EDITOR")
	if cmd != "" {
		return cmd, nil
	}

	return exec.LookPath("nano")
}

// editBundleFile launches an editor command to edit the bundle defined by path.
// When the edit process ends, the bundle file is parsed for validity. If a
// parsing error is encountered, the user is asked how to proceed: retry, revert
// and retry, or skip.
func editBundleFile(editorCmd string, bundle string, path string) error {
	// Make backup
	backup := path + ".orig"
	if err := helpers.CopyFileNoOverwrite(backup, path); err != nil && !os.IsExist(err) {
		return errors.Wrapf(err, "Could not backup bundle %q file for editing", bundle)
	}

	reader := bufio.NewReader(os.Stdin)
	revert := false

editLoop:
	for {
		if revert {
			if err := helpers.CopyFile(path, backup); err != nil {
				return errors.Wrapf(err, "Could not restore original from backup for bundle %q", bundle)
			}
		}

		// Ignore return from command; parsing below is what will reveal errors
		_ = helpers.RunCommandInput(os.Stdin, editorCmd, path)

		err := validateBundleFile(path, BasicValidation)
		if err == nil {
			// Clean-up backup
			if err = os.Remove(backup); err != nil {
				return errors.Wrapf(err, "Error cleaning up backup for bundle %q", bundle)
			}
			break editLoop
		}

		fmt.Printf("Error parsing bundle %q: %s\n", bundle, err)
		for {
			// Ask the user if they want to retry, revert, or skip
			fmt.Print("Would you like to edit as-is, revert and edit, or skip [Edit/Revert/Skip]?: ")
			text, err := reader.ReadString('\n')
			if err != nil {
				return errors.Wrapf(err, "Error reading input")
			}
			text = strings.ToLower(text)
			text = strings.TrimSpace(text)
			switch text {
			case "e", "edit":
				revert = false
				continue editLoop
			case "r", "revert":
				revert = true
				continue editLoop
			case "s", "skip":
				fmt.Printf("Skipping bundle %q despite errors. Backup retained as %q\n", bundle, bundle+".orig")
				break editLoop
			default:
				fmt.Printf("Invalid input: %q", text)
			}
		}
	}

	return nil
}

const bundleTemplateFormat = `# [TITLE]: %s
# [DESCRIPTION]: 
# [STATUS]: 
# [CAPABILITIES]:
# [MAINTAINER]: 
# 
# List bundles one per line. Includes have format: include(bundle)
`

func createBundleFile(bundle string, path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_EXCL, 0666)
	if err != nil {
		return err
	}

	data := []byte(fmt.Sprintf(bundleTemplateFormat, bundle))
	_, err = f.Write(data)
	_ = f.Close()
	return err
}

// EditBundles copies a list of bundles from upstream-bundles to local-bundles
// (if they are not already there) or creates a blank template if they are new,
// and launches an editor to edit them. Passing true for 'suppressEditor' will
// suppress the launching of the editor (and just do the copy or create, if
// needed), and 'add' will also add the bundles to the mix.
func (b *Builder) EditBundles(bundles []string, suppressEditor bool, add bool, git bool) error {
	// Fetch upstream bundle files if needed
	if err := b.getUpstreamBundles(b.UpstreamVer, true); err != nil {
		return err
	}

	editorCmd, err := getEditorCmd()
	if err != nil {
		fmt.Println("Cannot find a valid editor (see usage for configuration). Copying to local-bundles only.")
		suppressEditor = true
	}

	for _, bundle := range bundles {
		path, _ := b.getBundlePath(bundle)
		if !b.isLocalBundle(path) {
			localPath := filepath.Join(b.LocalBundleDir, bundle)

			if path == "" {
				// Bunlde not found upstream, so create new
				if err = createBundleFile(bundle, localPath); err != nil {
					return errors.Wrapf(err, "Failed to write bundle template for bundle %q", bundle)
				}
			} else {
				// Bundle found upstream, so copy over
				if err = helpers.CopyFile(localPath, path); err != nil {
					return err
				}
			}

			path = localPath
		}

		if suppressEditor {
			continue
		}

		if err = editBundleFile(editorCmd, bundle, path); err != nil {
			return err
		}
	}

	if add {
		if err = b.AddBundles(bundles, false, false, false); err != nil {
			return err
		}
	}

	if git {
		fmt.Println("Adding git commit")
		if err := helpers.Git("add", "."); err != nil {
			return err
		}
		commitMsg := fmt.Sprintf("Edited bundles: %v", bundles)
		if err := helpers.Git("commit", "-q", "-m", commitMsg); err != nil {
			return err
		}
	}

	return nil
}

// ValidateLocalBundles runs bundle parsing validation on all local bundles.
func (b *Builder) ValidateLocalBundles(lvl ValidationLevel) error {
	files, err := ioutil.ReadDir(b.LocalBundleDir)
	if err != nil {
		return errors.Wrap(err, "Failed to read local-bundles")
	}

	bundles := make([]string, len(files))
	for i, file := range files {
		bundles[i] = file.Name()
	}

	return b.ValidateBundles(bundles, lvl)
}

// ValidateBundles runs bundle parsing validation on a list of local bundles. In
// addition to parsing errors, errors are generated if the bundle is not found
// in local-bundles.
func (b *Builder) ValidateBundles(bundles []string, lvl ValidationLevel) error {
	invalid := false
	for _, bundle := range bundles {
		path := filepath.Join(b.LocalBundleDir, bundle)

		if err := validateBundleFile(path, lvl); err != nil {
			invalid = true
			fmt.Printf("Invalid: %q:\n%s\n\n", bundle, err)
		}
	}

	if invalid {
		return errors.New("Invalid bundles found")
	}

	return nil
}

// UpdateMixVer automatically bumps the mixversion file +10 to prepare for the next build
// without requiring user intervention. This makes the flow slightly more automatable.
func (b *Builder) UpdateMixVer() error {
	// Deprecate '.mixversion' --> 'mixversion'
	if _, err := os.Stat(filepath.Join(b.VersionDir, ".mixversion")); err == nil {
		b.MixVerFile = ".mixversion"
		fmt.Println("Warning: '.mixversion' has been deprecated. Please rename file to 'mixversion'")
	}
	mixVer, _ := strconv.Atoi(b.MixVer)
	return ioutil.WriteFile(filepath.Join(b.VersionDir, b.MixVerFile), []byte(strconv.Itoa(mixVer+10)), 0644)
}

// createMixBundleDir generates the mix-bundles/ dir for chroot building. It will
// clear the dir if it exists, compute the full list of bundles for the mix, and
// copy the corresponding bundle files into mix-bundles/
// Note: this function is only needed for the old Bundle Chroot Builder, and can
// be removed once the UseNewChrootBuilder flag is gone.
func (b *Builder) createMixBundleDir() error {
	// Wipe out the existing bundle dir, if it exists
	if err := os.RemoveAll(b.BundleDir); err != nil {
		return errors.Errorf("Failed to clear out old dir: %s", b.BundleDir)
	}
	if err := os.MkdirAll(b.BundleDir, 0777); err != nil {
		return errors.Errorf("Failed to create dir: %s", b.BundleDir)
	}

	// Get the set of bundles to build
	set, err := b.getFullMixBundleSet()
	if err != nil {
		return err
	}

	// Validate set
	if err = validateAndFillBundleSet(set); err != nil {
		return err
	}

	for _, bundle := range set {
		if err = helpers.CopyFile(filepath.Join(b.BundleDir, bundle.Name), bundle.Filename); err != nil {
			return err
		}
	}

	return nil
}

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
{{end}}{{if .Local}}
[local]
name=Local
failovermethod=priority
baseurl=file://{{.RepoDir}}
enabled=1
gpgcheck=0
priority=1
{{end}}`

// BuildBundles will attempt to construct the bundles required by generating a
// DNF configuration file, then resolving all files for each bundle using dnf
// resolve and no-op installs. One full chroot is created from this step with
// the file contents of all bundles.
func (b *Builder) BuildBundles(template *x509.Certificate, privkey *rsa.PrivateKey, signflag bool) error {
	// Fetch upstream bundle files if needed
	if err := b.getUpstreamBundles(b.UpstreamVer, true); err != nil {
		return err
	}

	// Generate the dnf config file if it does not exist.
	// This takes the template and adds the relevant local rpm repo path if needed
	fmt.Println("Building bundles...")

	timer := &stopWatch{w: os.Stdout}
	defer timer.WriteSummary(os.Stdout)

	timer.Start("BUILD BUNDLES")
	conf := dnfConf{
		UpstreamURL: b.UpstreamURL,
		RepoDir:     b.RepoDir,
	}

	if _, err := os.Stat(b.DNFConf); os.IsNotExist(err) {
		conf.Base = true
		if b.RepoDir != "" {
			conf.Local = true
		}
	} else if err == nil && b.RepoDir != "" {
		// check if conf file contains local section
		raw, err := ioutil.ReadFile(b.DNFConf)
		if err != nil {
			return err
		}
		if !strings.Contains(string(raw), "[local]") {
			conf.Local = true
		}
	}

	if conf.Base || conf.Local {
		f, err := os.OpenFile(b.DNFConf, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		defer func() {
			_ = f.Close()
		}()

		t, err := textTemplate.New("dnfConfTemplate").Parse(dnfConfTemplate)
		if err != nil {
			helpers.PrintError(err)
			return err
		}

		if err = t.Execute(f, conf); err != nil {
			return errors.Wrapf(err, "Failed to write to dnf file: %s", b.DNFConf)
		}
	}

	// If MIXVER already exists, wipe it so it's a fresh build
	if _, err := os.Stat(b.StateDir + "/image/" + b.MixVer); err == nil {
		fmt.Printf("Wiping away previous version %s...\n", b.MixVer)
		err = os.RemoveAll(b.StateDir + "/www/" + b.MixVer)
		if err != nil {
			return err
		}
		err = os.RemoveAll(b.StateDir + "/image/" + b.MixVer)
		if err != nil {
			return err
		}
	}

	// Generate the certificate needed for signing verification if it does not exist
	if signflag == false && template != nil {
		err := helpers.GenerateCertificate(b.Cert, template, template, &privkey.PublicKey, privkey)
		if err != nil {
			return err
		}
	}

	if UseNewChrootBuilder {
		// Get the set of bundles to build
		set, err := b.getFullMixBundleSet()
		if err != nil {
			return err
		}

		// Validate set and compute AllPackages
		if err = validateAndFillBundleSet(set); err != nil {
			return err
		}

		// TODO: Merge the rest of this function into buildBundles (or vice-versa).
		err = b.buildBundles(set)
		if err != nil {
			return err
		}

		// TODO: Move this logic to code that uses this.
		// If LAST_VER don't exists, it means this is the first bundle we build,
		// so initialize it to version "0".
		lastVerPath := filepath.Join(b.StateDir, "image", "LAST_VER")
		if _, err = os.Stat(lastVerPath); os.IsNotExist(err) {
			err = ioutil.WriteFile(lastVerPath, []byte("0\n"), 0644)
			if err != nil {
				return err
			}
		}

	} else {
		// Generate the mix-bundles list
		if err := b.createMixBundleDir(); err != nil {
			return err
		}

		// If this is a mix, we need to build with the Clear version, but publish the mix version
		chrootcmd := exec.Command(b.BuildScript, "-c", b.BuildConf, "-m", b.MixVer, b.UpstreamVer)
		chrootcmd.Stdout = os.Stdout
		chrootcmd.Stderr = os.Stderr
		err := chrootcmd.Run()
		if err != nil {
			return err
		}

		// Only copy the certificate into the mix if it exists
		if _, err := os.Stat(b.Cert); err == nil {
			certdir := filepath.Join(b.StateDir, "image", b.MixVer, "os-core-update/usr/share/clear/update-ca")
			err = os.MkdirAll(certdir, 0755)
			if err != nil {
				helpers.PrintError(err)
				return err
			}
			chrootcert := filepath.Join(certdir, "/Swupd_Root.pem")
			fmt.Println("Copying Certificate into chroot...")
			err = helpers.CopyFile(chrootcert, b.Cert)
			if err != nil {
				helpers.PrintError(err)
				return err
			}
		}
	}

	// TODO: Remove all the files-* entries since they're now copied into the noship dir
	// do code stuff here

	timer.Stop()

	return nil
}

// BuildUpdate will produce an update consumable by the swupd client
func (b *Builder) BuildUpdate(prefixflag string, minVersion int, format string, skipSigning bool, publish bool, keepChroots bool) error {
	var err error

	if minVersion < 0 || minVersion > math.MaxUint32 {
		return errors.Errorf("minVersion %d is out of range", minVersion)
	}

	if format != "" {
		b.Format = format
	}
	// TODO: move this to parsing configuration / parameter time.
	// TODO: should this be uint64?
	var formatUint uint32
	formatUint, err = parseUint32(b.Format)
	if err != nil {
		return errors.Errorf("invalid format")
	}

	// Ensure the format dir exists.
	formatDir := filepath.Join(b.StateDir, "www", "version", "format"+b.Format)
	err = os.MkdirAll(formatDir, 0777)
	if err != nil {
		return errors.Wrapf(err, "couldn't create the format directory")
	}

	timer := &stopWatch{w: os.Stdout}
	defer timer.WriteSummary(os.Stdout)

	if UseNewSwupdServer {
		err = b.buildUpdateWithNewSwupd(timer, b.MixVerUint32, uint32(minVersion), formatUint, skipSigning)
		if err != nil {
			return err
		}
	} else {
		err = b.buildUpdateWithOldSwupd(timer, prefixflag, minVersion, skipSigning)
		if err != nil {
			return err
		}

		timer.Start("MINIMIZE STORED CHROOTS")
		// Clean up the bundle chroots as only the full chroot is needed from this point on.
		if !keepChroots {
			// Get the set of bundles for which chroots were built
			var set bundleSet
			set, err = b.getFullMixBundleSet()
			if err != nil {
				return errors.Wrap(err, "Ignored error when cleaning bundle chroots")
			}
			// Delete the bundle chroots
			basedir := filepath.Join(b.StateDir, "image", b.MixVer)
			for bundle := range set {
				err = os.RemoveAll(filepath.Join(basedir, bundle))
				if err != nil {
					return errors.Wrap(err, "Ignored error when cleaning bundle chroots")
				}
			}
		} else {
			// Hardlink the duplicate files ONLY when keeping the bundle chroots.
			hardlinkcmd := exec.Command("hardlink", "-f", filepath.Join(b.StateDir, "image", b.MixVer))
			hardlinkcmd.Stdout = os.Stdout
			hardlinkcmd.Stderr = os.Stderr
			err = hardlinkcmd.Run()
			if err != nil {
				return errors.Wrapf(err, "couldn't perform hardlink step")
			}
		}
		timer.Stop()
	}

	// Save upstream information.
	if b.UpstreamURL != "" {
		fmt.Printf("Saving the upstream URL: %s\n", b.UpstreamURL)
		upstreamURLFile := filepath.Join(b.StateDir, "www", b.MixVer, "/upstreamurl")
		err = ioutil.WriteFile(upstreamURLFile, []byte(b.UpstreamURL), 0644)
		if err != nil {
			return errors.Wrapf(err, "couldn't write upstreamurl file")
		}
		fmt.Printf("Saving the upstream version: %s\n", b.UpstreamVer)
		upstreamVerFile := filepath.Join(b.StateDir, "www", b.MixVer, "upstreamver")
		err = ioutil.WriteFile(upstreamVerFile, []byte(b.UpstreamVer), 0644)
		if err != nil {
			return errors.Wrapf(err, "couldn't write upstreamver file")
		}
	}

	// Publish. Update the latest version both in the format (used by update itself) and in LAST_VER
	// (used to create newer manifests).
	if !publish {
		return nil
	}

	fmt.Printf("Setting latest version to %s\n", b.MixVer)

	err = ioutil.WriteFile(filepath.Join(formatDir, "latest"), []byte(b.MixVer), 0644)
	if err != nil {
		return errors.Wrapf(err, "couldn't update the latest version")
	}
	err = ioutil.WriteFile(filepath.Join(b.StateDir, "image", "LAST_VER"), []byte(b.MixVer), 0644)
	if err != nil {
		return errors.Wrapf(err, "couldn't update the latest version")
	}

	return nil
}

func (b *Builder) buildUpdateWithNewSwupd(timer *stopWatch, mixVersion uint32, minVersion uint32, format uint32, skipSigning bool) error {
	var err error

	err = writeMetaFiles(filepath.Join(b.StateDir, "www", b.MixVer), b.Format, Version)
	if err != nil {
		return errors.Wrapf(err, "failed to write update metadata files")
	}
	timer.Start("CREATE MANIFESTS")
	mom, err := swupd.CreateManifests(mixVersion, minVersion, uint(format), b.StateDir)
	if err != nil {
		return errors.Wrapf(err, "failed to create update metadata")
	}
	fmt.Printf("MoM version %d\n", mom.Header.Version)
	for _, f := range mom.Files {
		fmt.Printf("- %-20s %d\n", f.Name, f.Version)
	}

	if !skipSigning {
		fmt.Println("Signing manifest.")
		err = b.SignManifestMoM()
		if err != nil {
			return err
		}
	}

	outputDir := filepath.Join(b.StateDir, "www")
	thisVersionDir := filepath.Join(outputDir, fmt.Sprint(mixVersion))
	fmt.Println("Compressing Manifest.MoM")
	momF := filepath.Join(thisVersionDir, "Manifest.MoM")
	if skipSigning {
		err = createCompressedArchive(momF+".tar", momF)
	} else {
		err = createCompressedArchive(momF+".tar", momF, momF+".sig")
	}
	if err != nil {
		return err
	}
	fmt.Println("Compressing bundle manifests")
	for _, bundle := range mom.UpdatedBundles {
		fmt.Printf("  %s\n", bundle.Name)
		f := filepath.Join(thisVersionDir, "Manifest."+bundle.Name)
		err = createCompressedArchive(f+".tar", f)
		if err != nil {
			return err
		}
	}
	// TODO: Create manifest tars for Manifest.MoM and the mom.UpdatedBundles.
	timer.Stop()

	timer.Start("CREATE FULLFILES")
	fmt.Printf("Using %d workers\n", b.NumFullfileWorkers)
	fullfilesDir := filepath.Join(outputDir, b.MixVer, "files")
	fullChrootDir := filepath.Join(b.StateDir, "image", b.MixVer, "full")
	info, err := swupd.CreateFullfiles(mom.FullManifest, fullChrootDir, fullfilesDir, b.NumFullfileWorkers)
	if err != nil {
		return err
	}
	// Print summary of fullfile generation.
	{
		total := info.Skipped + info.NotCompressed
		fmt.Printf("- Already created: %d\n", info.Skipped)
		fmt.Printf("- Not compressed:  %d\n", info.NotCompressed)
		fmt.Printf("- Compressed\n")
		for k, v := range info.CompressedCounts {
			total += v
			fmt.Printf("  - %-20s %d\n", k, v)
		}
		fmt.Printf("Total fullfiles: %d\n", total)
	}
	timer.Stop()

	timer.Start("CREATE ZERO PACKS")
	bundleDir := filepath.Join(b.StateDir, "image")
	for _, bundle := range mom.Files {
		// TODO: Evaluate if it's worth using goroutines.
		name := bundle.Name
		version := bundle.Version
		packPath := filepath.Join(outputDir, fmt.Sprint(version), swupd.GetPackFilename(name, 0))
		_, err = os.Lstat(packPath)
		if err == nil {
			fmt.Printf("Zero pack already exists for %s to version %d\n", name, version)
			continue
		}
		if !os.IsNotExist(err) {
			return errors.Wrapf(err, "couldn't access existing pack file %s", packPath)
		}

		fmt.Printf("Creating zero pack for %s to version %d\n", name, version)

		var info *swupd.PackInfo
		info, err = swupd.CreatePack(name, 0, version, outputDir, bundleDir, 0)
		if err != nil {
			return errors.Wrapf(err, "couldn't make pack for bundle %q", name)
		}
		if len(info.Warnings) > 0 {
			fmt.Println("Warnings during pack:")
			for _, w := range info.Warnings {
				fmt.Printf("  %s\n", w)
			}
			fmt.Println()
		}
		fmt.Printf("  Fullfiles in pack: %d\n", info.FullfileCount)
		fmt.Printf("  Deltas in pack: %d\n", info.DeltaCount)
	}
	timer.Stop()

	return nil
}

func (b *Builder) buildUpdateWithOldSwupd(timer *stopWatch, prefixflag string, minVersion int, skipSigning bool) error {
	var err error

	// Create update metadata for the mix.
	timer.Start("CREATE MANIFESTS")
	updatecmd := exec.Command(prefixflag+"swupd_create_update", "-S", b.StateDir, "--minversion", strconv.Itoa(minVersion), "-F", b.Format, "--osversion", b.MixVer)
	updatecmd.Stdout = os.Stdout
	updatecmd.Stderr = os.Stderr
	err = updatecmd.Run()
	if err != nil {
		return errors.Wrapf(err, "failed to create update metadata")
	}
	timer.Stop()

	// Sign the Manifest.MoM that was just created.
	if !skipSigning {
		err = b.SignManifestMoM()
		if err != nil {
			return err
		}
		fmt.Println("Signed Manifest.MoM")
	}

	// Create full files.
	timer.Start("CREATE FULLFILES")
	fullfilecmd := exec.Command(prefixflag+"swupd_make_fullfiles", "-S", b.StateDir, b.MixVer)
	fullfilecmd.Stdout = os.Stdout
	fullfilecmd.Stderr = os.Stderr
	err = fullfilecmd.Run()
	if err != nil {
		return errors.Wrapf(err, "couldn't create fullfiles")
	}
	timer.Stop()

	// Create zero packs.
	timer.Start("CREATE ZERO PACKS")
	zeropackArgs := []string{"--to", b.MixVer, "-S", b.StateDir}
	if prefixflag != "" {
		zeropackArgs = append(zeropackArgs, "--repodir", prefixflag)
	}
	zeropackcmd := exec.Command("mixer-pack-maker.sh", zeropackArgs...)
	zeropackcmd.Stdout = os.Stdout
	zeropackcmd.Stderr = os.Stderr
	err = zeropackcmd.Run()
	if err != nil {
		return errors.Wrapf(err, "couldn't create zero packs")
	}
	timer.Stop()

	return nil
}

// BuildImage will now proceed to build the full image with the previously
// validated configuration.
func (b *Builder) BuildImage(format string, template string) error {
	// If the user did not pass in a format, default to builder.conf
	if format == "" {
		format = b.Format
	}

	// If the user did not pass in a template, default to release-image-config.json
	if template == "" {
		template = "release-image-config.json"
	}

	// swupd (client) called by itser will need a temporary directory to act as its stage dir.
	wd, _ := os.Getwd()
	tempStage, err := ioutil.TempDir(wd, "ister-swupd-client-")
	if err != nil {
		return err
	}
	defer func() {
		_ = os.RemoveAll(tempStage)
	}()

	content := "file://" + b.StateDir + "/www"
	imagecmd := exec.Command("ister.py", "-S", tempStage, "-t", template, "-V", content, "-C", content, "-f", format, "-s", b.Cert)
	imagecmd.Stdout = os.Stdout
	imagecmd.Stderr = os.Stderr

	return imagecmd.Run()
}

// AddRPMList copies rpms into the repodir and calls createrepo_c on it to
// generate a dnf-consumable repository for the bundle builder to use.
func (b *Builder) AddRPMList(rpms []os.FileInfo) error {
	if b.RepoDir == "" {
		return errors.Errorf("LOCAL_REPO_DIR not set in configuration")
	}
	err := os.MkdirAll(b.RepoDir, 0755)
	if err != nil {
		return errors.Wrapf(err, "couldn't create LOCAL_REPO_DIR")
	}
	for _, rpm := range rpms {
		localPath := filepath.Join(b.RPMDir, rpm.Name())
		if err := checkRPM(localPath); err != nil {
			return err
		}
		// Skip RPM already in repo.
		repoPath := filepath.Join(b.RepoDir, rpm.Name())
		if _, err := os.Stat(repoPath); err == nil {
			continue
		}
		fmt.Printf("Hardlinking %s to repodir\n", rpm.Name())
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
	cmd.Dir = b.RepoDir

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

func parseUint32(s string) (uint32, error) {
	parsed, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return 0, errors.Wrapf(err, "error parsing value %q", s)
	}
	return uint32(parsed), nil
}

// createCompressedArchive will use tar and xz to create a compressed
// file. It does not stream the sources contents, doing all the work
// in memory before writing the destination file.
func createCompressedArchive(dst string, srcs ...string) error {
	err := createCompressedArchiveInternal(dst, srcs...)
	return errors.Wrapf(err, "couldn't create compressed archive %s", dst)
}

func createCompressedArchiveInternal(dst string, srcs ...string) error {
	archive := &bytes.Buffer{}
	xw, err := swupd.NewExternalWriter(archive, "xz")
	if err != nil {
		return err
	}

	err = archiveFiles(xw, srcs)
	if err != nil {
		_ = xw.Close()
		return err
	}

	err = xw.Close()
	if err != nil {
		return err
	}

	return ioutil.WriteFile(dst, archive.Bytes(), 0644)
}

func archiveFiles(w io.Writer, srcs []string) error {
	tw := tar.NewWriter(w)
	for _, src := range srcs {
		fi, err := os.Lstat(src)
		if err != nil {
			return err
		}
		if !fi.Mode().IsRegular() {
			return errors.Errorf("%s has unsupported type of file", src)
		}
		var hdr *tar.Header
		hdr, err = tar.FileInfoHeader(fi, "")
		if err != nil {
			return err
		}

		err = tw.WriteHeader(hdr)
		if err != nil {
			return err
		}
		var srcData []byte
		srcData, err = ioutil.ReadFile(src)
		if err != nil {
			return err
		}
		_, err = tw.Write(srcData)
		if err != nil {
			return err
		}
	}
	return tw.Close()
}

// BuildDeltaPacks between two versions of the mix.
func (b *Builder) BuildDeltaPacks(from, to uint32, printReport bool) error {
	var err error

	if to == 0 {
		to = b.MixVerUint32
	} else {
		if to > b.MixVerUint32 {
			return errors.Errorf("--to version must be at most the latest mix version (%d)", b.MixVerUint32)
		}
	}
	if from >= to {
		return errors.Errorf("the --from version must be smaller than the --to version")
	}

	outputDir := filepath.Join(b.StateDir, "www")
	toManifest, err := swupd.ParseManifestFile(filepath.Join(outputDir, fmt.Sprint(to), "Manifest.MoM"))
	if err != nil {
		return errors.Wrapf(err, "couldn't find manifest of target version")
	}

	fromManifest, err := swupd.ParseManifestFile(filepath.Join(outputDir, fmt.Sprint(from), "Manifest.MoM"))
	if err != nil {
		return errors.Wrapf(err, "couldn't find manifest of from version")
	}

	bundleDir := filepath.Join(b.StateDir, "image")
	fmt.Printf("Using %d workers\n", b.NumDeltaWorkers)
	return createDeltaPacks(fromManifest, toManifest, printReport, outputDir, bundleDir, b.NumDeltaWorkers)
}

// BuildDeltaPacksPreviousVersions builds packs to version from up to
// prev versions. It walks the Manifest "previous" field to find those from versions.
func (b *Builder) BuildDeltaPacksPreviousVersions(prev, to uint32, printReport bool) error {
	var err error

	if to == 0 {
		to = b.MixVerUint32
	} else {
		if to > b.MixVerUint32 {
			return errors.Errorf("--to version must be at most the latest mix version (%d)", b.MixVerUint32)
		}
	}

	outputDir := filepath.Join(b.StateDir, "www")
	toManifest, err := swupd.ParseManifestFile(filepath.Join(outputDir, fmt.Sprint(to), "Manifest.MoM"))
	if err != nil {
		return errors.Wrapf(err, "couldn't find manifest of target version")
	}

	var previousManifests []*swupd.Manifest
	cur := toManifest.Header.Previous
	for i := uint32(0); i < prev; i++ {
		if cur == 0 {
			break
		}
		var m *swupd.Manifest
		m, err = swupd.ParseManifestFile(filepath.Join(outputDir, fmt.Sprint(cur), "Manifest.MoM"))
		if err != nil {
			fmt.Fprintf(os.Stderr, "could not find manifest for previous version %d, skipping...\n", cur)
			continue
		}
		previousManifests = append(previousManifests, m)
		cur = m.Header.Previous
	}

	fmt.Printf("Using %d workers\n", b.NumDeltaWorkers)
	fmt.Printf("Found %d previous versions\n", len(previousManifests))

	bundleDir := filepath.Join(b.StateDir, "image")
	for _, fromManifest := range previousManifests {
		fmt.Println()
		err = createDeltaPacks(fromManifest, toManifest, printReport, outputDir, bundleDir, b.NumDeltaWorkers)
		if err != nil {
			return err
		}
	}
	return nil
}

func createDeltaPacks(from *swupd.Manifest, to *swupd.Manifest, printReport bool, outputDir, bundleDir string, numWorkers int) error {
	timer := &stopWatch{w: os.Stdout}
	defer timer.WriteSummary(os.Stdout)
	timer.Start("CREATE DELTA PACKS")

	fmt.Printf("Creating delta packs from %d to %d\n", from.Header.Version, to.Header.Version)
	bundlesToPack, err := swupd.FindBundlesToPack(from, to)
	if err != nil {
		return err
	}

	// Get an ordered output. This make easy to compare different runs.
	var orderedBundles []string
	for name := range bundlesToPack {
		orderedBundles = append(orderedBundles, name)
	}
	sort.Strings(orderedBundles)

	for _, name := range orderedBundles {
		b := bundlesToPack[name]
		packPath := filepath.Join(outputDir, fmt.Sprint(b.ToVersion), swupd.GetPackFilename(b.Name, b.FromVersion))
		_, err = os.Lstat(packPath)
		if err == nil {
			fmt.Printf("  Delta pack already exists for %s from %d to %d\n", b.Name, b.FromVersion, b.ToVersion)
			continue
		}
		if !os.IsNotExist(err) {
			return errors.Wrapf(err, "couldn't access existing pack file %s", packPath)
		}
		fmt.Printf("  Creating delta pack for bundle %q from %d to %d\n", b.Name, b.FromVersion, b.ToVersion)
		info, err := swupd.CreatePack(b.Name, b.FromVersion, b.ToVersion, outputDir, bundleDir, numWorkers)
		if err != nil {
			return err
		}

		if len(info.Warnings) > 0 {
			for _, w := range info.Warnings {
				fmt.Printf("    WARNING: %s\n", w)
			}
			fmt.Println()
		}
		if printReport {
			max := 0
			for _, e := range info.Entries {
				if len(e.File.Name) > max {
					max = len(e.File.Name)
				}
			}
			fmt.Println("    Pack report:")
			for _, e := range info.Entries {
				fmt.Printf("      %-*s %s (%s)\n", max, e.File.Name, e.State, e.Reason)
			}
			fmt.Println()
		}
		fmt.Printf("    Fullfiles in pack: %d\n", info.FullfileCount)
		fmt.Printf("    Deltas in pack: %d\n", info.DeltaCount)
	}

	timer.Stop()
	return nil
}

// writeMetaFiles writes mixer and format metadata to files
func writeMetaFiles(path, format, version string) error {
	err := os.MkdirAll(path, 0777)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(filepath.Join(path, "format"), []byte(format), 0644)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(filepath.Join(path, "mixer-src-version"), []byte(version), 0644)
}

func (b *Builder) getUpstreamFormatRange() (format string, first, latest uint32, err error) {
	format, err = b.DownloadFileFromUpstream(fmt.Sprintf("update/%d/format", b.UpstreamVerUint32))
	if err != nil {
		return "", 0, 0, errors.Wrap(err, "couldn't download information about upstream")
	}

	readUint32 := func(subpath string) (uint32, error) {
		str, rerr := b.DownloadFileFromUpstream(subpath)
		if rerr != nil {
			return 0, rerr
		}
		val, rerr := parseUint32(str)
		if rerr != nil {
			return 0, rerr
		}
		return val, nil
	}

	latest, err = readUint32(fmt.Sprintf("update/version/format%s/latest", format))
	if err != nil {
		return "", 0, 0, errors.Wrap(err, "couldn't read information about upstream")
	}

	// TODO: Clear Linux does produce the "first" files, but not sure mixes got
	// those. We should add those (or change this to walk previous format latest).
	first, err = readUint32(fmt.Sprintf("update/version/format%s/first", format))
	if err != nil {
		return "", 0, 0, errors.Wrap(err, "couldn't read information about upstream")
	}

	return format, first, latest, err
}

// PrintVersions prints the current mix and upstream versions, and the
// latest version of upstream.
func (b *Builder) PrintVersions() error {
	format, first, latest, err := b.getUpstreamFormatRange()
	if err != nil {
		return err
	}

	fmt.Printf(`
Current mix:               %d
Current upstream:          %d (format: %s)

First upstream in format:  %d
Latest upstream in format: %d
`, b.MixVerUint32, b.UpstreamVerUint32, format, first, latest)

	return nil
}

// UpdateVersions will validate then update both mix and upstream versions. If upstream
// version is 0, then the latest upstream version possible will be taken instead.
func (b *Builder) UpdateVersions(nextMix, nextUpstream uint32) error {
	format, first, latest, err := b.getUpstreamFormatRange()
	if err != nil {
		return err
	}

	if nextMix <= b.MixVerUint32 {
		return fmt.Errorf("invalid mix version to update (%d), need to be greater than current mix version (%d)", nextMix, b.MixVerUint32)
	}

	switch {
	case nextUpstream == 0:
		nextUpstream = latest

	case nextUpstream < first, nextUpstream > latest:
		return fmt.Errorf("invalid upstream version to update (%d) out of the format %s range: must be at least %d and at most %d", nextUpstream, format, first, latest)
	}

	// Verify the version exists by checking if its Manifest.MoM is around.
	_, err = b.DownloadFileFromUpstream(fmt.Sprintf("/update/%d/Manifest.MoM", nextUpstream))
	if err != nil {
		return errors.Wrapf(err, "invalid upstream version %d", nextUpstream)
	}

	fmt.Printf(`Current mix:      %d
Current upstream: %d (format: %s)

Updated mix:      %d
Updated upstream: %d (format: %s)
`, b.MixVerUint32, b.UpstreamVerUint32, format, nextMix, nextUpstream, format)

	mixVerContents := []byte(fmt.Sprintf("%d\n", nextMix))
	err = ioutil.WriteFile(filepath.Join(b.VersionDir, b.MixVerFile), mixVerContents, 0644)
	if err != nil {
		return errors.Wrap(err, "couldn't write updated mix version")
	}
	fmt.Printf("\nWrote %s.\n", b.MixVerFile)

	upstreamVerContents := []byte(fmt.Sprintf("%d\n", nextUpstream))
	err = ioutil.WriteFile(filepath.Join(b.VersionDir, b.UpstreamVerFile), upstreamVerContents, 0644)
	if err != nil {
		return errors.Wrap(err, "couldn't write updated upstream version")
	}
	fmt.Printf("Wrote %s.\n", b.UpstreamVerFile)

	return nil
}
