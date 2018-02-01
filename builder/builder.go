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
	"bytes"
	"crypto/rsa"
	"crypto/x509"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/clearlinux/mixer-tools/helpers"
	"github.com/clearlinux/mixer-tools/swupd"
	"github.com/pkg/errors"
)

// Version of Mixer. Also used by the Makefile for releases.
const Version = "3.2.1"

// UseNewSwupdServer controls whether to use the new implementation of
// swupd-server (package swupd) when possible. This is an experimental feature.
var UseNewSwupdServer = false

// UseNewChrootBuilder controls whether to use the new implementation of
// building the chroots. This is an experimental feature.
var UseNewChrootBuilder = false

// A Builder contains all configurable fields required to perform a full mix
// operation, and is used to encapsulate life time data.
type Builder struct {
	Buildscript string
	Buildconf   string

	Bundledir   string
	Cert        string
	Clearver    string
	Format      string
	Mixver      string
	Repodir     string
	RPMdir      string
	Statedir    string
	Versiondir  string
	Yumconf     string
	Yumtemplate string
	Upstreamurl string

	Signing int
	Bump    int
}

// New will return a new instance of Builder with some predetermined sane
// default values.
func New() *Builder {
	return &Builder{
		Buildscript: "bundle-chroot-builder.py",
		Yumtemplate: "/usr/share/defaults/mixer/yum.conf.in",

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

	data := strings.Replace(string(raw), "/home/clr/mix", pwd, -1)
	if localrpms {
		data += "\n[Mixer]\n"
		data += "RPMDIR=" + pwd + "/rpms\n"
		data += "REPODIR=" + pwd + "/local\n"
	}

	if err = ioutil.WriteFile(builderconf, []byte(data), 0666); err != nil {
		return err
	}
	return nil
}

// createRpmDirs creates the RPM directories
func (b *Builder) createRpmDirs() error {
	// Make the folder to store rpms if they don't already exist
	if b.RPMdir != "" {
		if err := os.MkdirAll(b.RPMdir, 0777); err != nil {
			return err
		}
	}

	if b.Repodir != "" {
		if err := os.MkdirAll(b.Repodir, 0777); err != nil {
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
		b.Buildconf = builderconf
		return nil
	}

	// Check if there's a local builder.conf if one wasn't supplied
	localpath := local + "/builder.conf"
	if _, err := os.Stat(localpath); err == nil {
		b.Buildconf = localpath
	} else {
		return errors.Wrap(err, "Cannot find any builder.conf to use")
	}

	return nil
}

// ReadBuilderConf will populate the configuration data from the builder
// configuration file, which is mandatory information for performing a mix.
func (b *Builder) ReadBuilderConf() error {
	lines, err := helpers.ReadFileAndSplit(b.Buildconf)
	if err != nil {
		return errors.Wrap(err, "Failed to read buildconf")
	}

	// Map the builder values to the regex here to make it easier to assign
	fields := []struct {
		re       string
		dest     *string
		required bool
	}{
		{`^BUNDLE_DIR\s*=\s*`, &b.Bundledir, true},
		{`^CERT\s*=\s*`, &b.Cert, true},
		{`^CLEARVER\s*=\s*`, &b.Clearver, false},
		{`^FORMAT\s*=\s*`, &b.Format, true},
		{`^MIXVER\s*=\s*`, &b.Mixver, false},
		{`^REPODIR\s*=\s*`, &b.Repodir, false},
		{`^RPMDIR\s*=\s*`, &b.RPMdir, false},
		{`^SERVER_STATE_DIR\s*=\s*`, &b.Statedir, true},
		{`^VERSIONS_PATH\s*=\s*`, &b.Versiondir, true},
		{`^YUM_CONF\s*=\s*`, &b.Yumconf, true},
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

	return nil
}

// ReadVersions will initialise the mix versions (mix and clearlinux) from
// the configuration files in the version directory.
func (b *Builder) ReadVersions() error {
	ver, err := ioutil.ReadFile(b.Versiondir + "/.mixversion")
	if err != nil {
		return err
	}
	b.Mixver = strings.TrimSpace(string(ver))
	b.Mixver = strings.Replace(b.Mixver, "\n", "", -1)

	ver, err = ioutil.ReadFile(b.Versiondir + "/.clearversion")
	if err != nil {
		return err
	}
	b.Clearver = strings.TrimSpace(string(ver))
	b.Clearver = strings.Replace(b.Clearver, "\n", "", -1)

	ver, err = ioutil.ReadFile(b.Versiondir + "/.clearurl")
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: %s/.clearurl does not exist, run mixer init-mix to generate\n", b.Versiondir)
		b.Upstreamurl = ""
	} else {
		b.Upstreamurl = strings.TrimSpace(string(ver))
		b.Upstreamurl = strings.Replace(b.Upstreamurl, "\n", "", -1)
	}

	return nil
}

// SignManifestMoM will sign the Manifest.MoM file in in place based on the Mix
// version read from builder.conf.
func (b *Builder) SignManifestMoM() error {
	mom := filepath.Join(b.Statedir, "www", b.Mixver, "Manifest.MoM")
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
	upstreamBundlesBaseDir   = ".mixer/upstream-bundles/"
	upstreamBundlesVerDirFmt = "clr-bundles-%s"
	upstreamBundlesBundleDir = "bundles"
)

func getUpstreamBundlesVerDir(ver string) string {
	return fmt.Sprintf(upstreamBundlesVerDirFmt, ver)
}
func getUpstreamBundlesPath(ver string) string {
	return filepath.Join(upstreamBundlesBaseDir, fmt.Sprintf(upstreamBundlesVerDirFmt, ver), upstreamBundlesBundleDir)
}

func (b *Builder) getUpstreamBundles(ver string, prune bool) error {
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

// AddBundles will copy the specified upstream bundles from the configured
// upstream version to the mix-bundles directory. In error event, the int value
// returned will contain the number of bundles added thus far.
// bundles: array slice of bundle names
// force: override bundle in mix-dir when present
// all: include all CLR bundles. Overrides bundles.
// git: automatically git commit with bundles added
func (b *Builder) AddBundles(bundles []string, force bool, allbundles bool, git bool) (int, error) {
	var bundleAddCount int

	// Check if mix bundles dir exists
	if _, err := os.Stat(b.Bundledir); os.IsNotExist(err) {
		return bundleAddCount, errors.New("Mix bundles directory does not exist. Run mixer init-mix")
	}

	clrbundledir := getUpstreamBundlesPath(b.Clearver)

	// Fetch upstream bundles if needed
	if err := b.getUpstreamBundles(b.Clearver, true); err != nil {
		return bundleAddCount, err
	}

	// Add all bundles to list if --all passed
	if allbundles {
		files, err := ioutil.ReadDir(clrbundledir)
		if err != nil {
			return bundleAddCount, err
		}

		bundles = make([]string, len(files))

		for i, file := range files {
			bundles[i] = file.Name()
		}
	}

	var includes []string
	for _, bundle := range bundles {
		// Check if bundle exists in clrbundledir
		if _, err := os.Stat(filepath.Join(clrbundledir, bundle)); os.IsNotExist(err) {
			return bundleAddCount, errors.Errorf("Bundle %s does not exist in CLR version %s", bundle, b.Clearver)
		}
		// Check if bundle exists in mix bundles dir
		if _, err := os.Stat(filepath.Join(b.Bundledir, bundle)); os.IsNotExist(err) || force {
			if !allbundles {
				var ib []string
				// Parse bundle to get all includes
				if ib, err = helpers.GetIncludedBundles(filepath.Join(clrbundledir, bundle)); err != nil {
					return bundleAddCount, errors.Errorf("Cannot parse bundle %s from CLR version %s", bundle, b.Clearver)
				} else if len(ib) > 0 {
					includes = append(includes, ib...)
				}
			}

			fmt.Printf("Adding bundle %q\n", bundle)
			if err = helpers.CopyFile(filepath.Join(b.Bundledir, bundle), filepath.Join(clrbundledir, bundle)); err != nil {
				return bundleAddCount, err
			}
			bundleAddCount++
		} else {
			fmt.Printf("Warning: bundle %q already exists; skipping.\n", bundle)
		}
	}
	// Recurse on included bundles
	if len(includes) > 0 {
		ib, err := b.AddBundles(includes, force, false, false)
		if err != nil {
			return bundleAddCount, err
		}
		bundleAddCount += ib
	}

	if git && bundleAddCount > 0 {
		// Save current dir so we can get back to it
		fmt.Println("Adding git commit")
		if err := helpers.Git("-C", b.Bundledir, "add", "."); err != nil {
			return bundleAddCount, err
		}
		commitMsg := fmt.Sprintf("Added bundles from Clear Version %s\n\nBundles added: %v", b.Clearver, bundles)
		if err := helpers.Git("-C", b.Bundledir, "commit", "-q", "-m", commitMsg); err != nil {
			return bundleAddCount, err
		}
	}
	return bundleAddCount, nil
}

// InitMix will initialise a new swupd-client consumable "mix" with the given
// based Clear Linux version and specified mix version.
func (b *Builder) InitMix(clearver string, mixver string, all bool, upstreamurl string) error {
	// Set up metadata
	err := ioutil.WriteFile(b.Versiondir+"/.clearurl", []byte(upstreamurl), 0644)
	if err != nil {
		return err
	}
	b.Upstreamurl = upstreamurl

	err = ioutil.WriteFile(b.Versiondir+"/.clearversion", []byte(clearver), 0644)
	if err != nil {
		return err
	}
	b.Clearver = clearver

	err = ioutil.WriteFile(b.Versiondir+"/.mixversion", []byte(mixver), 0644)
	if err != nil {
		return err
	}
	b.Mixver = mixver

	// Get upstream bundles
	if err := b.getUpstreamBundles(clearver, true); err != nil {
		return err
	}

	// Set up mix-bundles dir
	if _, err := os.Stat(b.Bundledir); os.IsNotExist(err) {
		if err = os.MkdirAll(b.Bundledir, 0777); err != nil {
			return errors.Wrap(err, "Failed to create mix-bundles dir")
		}

		// Add default bundles (or all)
		// Note: os-core-update already includes os-core, and kernel-native already includes bootloader
		defaultBundles := []string{"os-core-update", "kernel-native"}
		if _, err := b.AddBundles(defaultBundles, false, all, false); err != nil {
			return err
		}

		// Init the mix-bundles git repo
		if err := helpers.Git("-C", b.Bundledir, "init"); err != nil {
			return err
		}
		if err := helpers.Git("-C", b.Bundledir, "add", "."); err != nil {
			return err
		}
		commitMsg := fmt.Sprintf("Initial mix version %s from upstream version %s", b.Mixver, b.Clearver)
		if err := helpers.Git("-C", b.Bundledir, "commit", "-m", commitMsg); err != nil {
			return err
		}
	}

	// Set up rpms and local dirs
	if err := b.createRpmDirs(); err != nil {
		return errors.Wrap(err, "Failed to create local RPM dirs")
	}

	return nil
}

// UpdateMixVer automatically bumps the mixversion file +10 to prepare for the next build
// without requiring user intervention. This makes the flow slightly more automatable.
func (b *Builder) UpdateMixVer() error {
	mixver, _ := strconv.Atoi(b.Mixver)
	return ioutil.WriteFile(b.Versiondir+"/.mixversion", []byte(strconv.Itoa(mixver+10)), 0644)
}

// BuildChroots will attempt to construct the chroots required by populating roots
// using the m4 bundle configurations in conjunction with the YUM configuration file,
// installing all required named packages into the roots.
func (b *Builder) BuildChroots(template *x509.Certificate, privkey *rsa.PrivateKey, packager string, signflag bool) error {
	// Generate the yum config file if it does not exist.
	// This takes the template and adds the relevant local rpm repo path if needed
	fmt.Println("Building chroots..")
	if _, err := os.Stat(b.Yumconf); os.IsNotExist(err) {
		outfile, err := os.Create(b.Yumconf)
		if err != nil {
			helpers.PrintError(err)
			panic(err)
		}
		defer func() {
			_ = outfile.Close()
		}()
		if b.Repodir == "" {
			cmd := exec.Command("m4", "-D", "UPSTREAM_URL="+b.Upstreamurl, b.Yumtemplate)
			cmd.Stdout = outfile
			if err = cmd.Run(); err != nil {
				helpers.PrintError(err)
				return err
			}
		} else {
			cmd := exec.Command("m4", "-D", "MIXER_REPO",
				"-D", "MIXER_REPOPATH="+b.Repodir,
				"-D", "UPSTREAM_URL="+b.Upstreamurl,
				b.Yumtemplate)
			cmd.Stdout = outfile
			if err = cmd.Run(); err != nil {
				helpers.PrintError(err)
				return err
			}
		}
		if err != nil {
			helpers.PrintError(err)
			return err
		}
	}

	// If MIXVER already exists, wipe it so it's a fresh build
	if _, err := os.Stat(b.Statedir + "/image/" + b.Mixver); err == nil {
		fmt.Printf("Wiping away previous version %s...\n", b.Mixver)
		err = os.RemoveAll(b.Statedir + "/www/" + b.Mixver)
		if err != nil {
			return err
		}
		err = os.RemoveAll(b.Statedir + "/image/" + b.Mixver)
		if err != nil {
			return err
		}
	}

	if UseNewChrootBuilder {
		// TODO: Fix this output when we take bundles from multiple places.
		fmt.Printf("Reading bundles from: %s\n", b.Bundledir)
		files, err := ioutil.ReadDir(b.Bundledir)
		if err != nil {
			return err
		}
		var set bundleSet
		for _, file := range files {
			if file.IsDir() || strings.HasPrefix(file.Name(), ".") {
				continue
			}
			var bundle *bundle
			bundle, err = parseBundleFile(filepath.Join(b.Bundledir, file.Name()))
			if err != nil {
				return err
			}
			set[bundle.Name] = bundle
		}

		if err = validateAndFillBundleSet(set); err != nil {
			return err
		}

		// TODO: Merge the rest of this function into buildBundleChroots (or vice-versa).
		err = b.buildBundleChroots(set, packager)
		if err != nil {
			return err
		}

		// TODO: Move this logic to code that uses this.
		// If LAST_VER don't exists, it means this is the first chroot we build,
		// so initialize it to version "0".
		lastVerPath := filepath.Join(b.Statedir, "image", "LAST_VER")
		if _, err = os.Stat(lastVerPath); os.IsNotExist(err) {
			err = ioutil.WriteFile(lastVerPath, []byte("0\n"), 0644)
			if err != nil {
				return err
			}
		}

	} else {
		if packager != "" {
			fmt.Println("WARNING: Ignoring flag --packager, just works with --new-chroots.")
		}
		// If this is a mix, we need to build with the Clear version, but publish the mix version
		chrootcmd := exec.Command(b.Buildscript, "-c", b.Buildconf, "-m", b.Mixver, b.Clearver)
		chrootcmd.Stdout = os.Stdout
		chrootcmd.Stderr = os.Stderr
		err := chrootcmd.Run()
		if err != nil {
			return err
		}
	}

	// Generate the certificate needed for signing verification if it does not exist and insert it into the chroot
	if signflag == false && template != nil {
		err := helpers.GenerateCertificate(b.Cert, template, template, &privkey.PublicKey, privkey)
		if err != nil {
			return err
		}
	}

	// Only copy the certificate into the mix if it exists
	if _, err := os.Stat(b.Cert); err == nil {
		certdir := b.Statedir + "/image/" + b.Mixver + "/os-core-update/usr/share/clear/update-ca"
		err = os.MkdirAll(certdir, 0755)
		if err != nil {
			helpers.PrintError(err)
			return err
		}
		chrootcert := certdir + "/Swupd_Root.pem"
		fmt.Println("Copying Certificate into chroot...")
		err = helpers.CopyFile(chrootcert, b.Cert)
		if err != nil {
			helpers.PrintError(err)
			return err
		}
	}

	// TODO: Remove all the files-* entries since they're now copied into the noship dir
	// do code stuff here

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

	// TODO: move this to parsing configuration time.
	var mixVerUint uint32
	mixVerUint, err = parseUint32(b.Mixver)
	if err != nil {
		return errors.Wrapf(err, "couldn't parse mix version")
	}

	// Ensure the format dir exists.
	formatDir := filepath.Join(b.Statedir, "www", "version", "format"+b.Format)
	err = os.MkdirAll(formatDir, 0777)
	if err != nil {
		return errors.Wrapf(err, "couldn't create the format directory")
	}

	timer := &stopWatch{w: os.Stdout}
	defer timer.WriteSummary(os.Stdout)

	if UseNewSwupdServer {
		err = b.buildUpdateWithNewSwupd(timer, mixVerUint, uint32(minVersion), formatUint, skipSigning)
	} else {
		err = b.buildUpdateWithOldSwupd(timer, prefixflag, minVersion, skipSigning)
	}
	if err != nil {
		return err
	}

	timer.Start("MINIMIZE STORED CHROOTS")
	// Clean up the bundle chroots as only the full chroot is needed from this point on.
	if !keepChroots {
		var files []os.FileInfo
		files, err = ioutil.ReadDir(b.Bundledir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Ignored error when cleaning bundle chroots: %s", err)
		}
		basedir := filepath.Join(b.Statedir, "image", b.Mixver)
		for _, f := range files {
			if f.Name() == "full" {
				continue
			}
			err = os.RemoveAll(filepath.Join(basedir, f.Name()))
			if err != nil {
				fmt.Fprintf(os.Stderr, "Ignored error when cleaning bundle chroots: %s", err)
			}
		}
	}
	// Hardlink the duplicate files. This helps when keeping the bundle chroots.
	hardlinkcmd := exec.Command("hardlink", "-f", b.Statedir+"/image/"+b.Mixver+"/")
	hardlinkcmd.Stdout = os.Stdout
	hardlinkcmd.Stderr = os.Stderr
	err = hardlinkcmd.Run()
	if err != nil {
		return errors.Wrapf(err, "couldn't perform hardlink step")
	}
	timer.Stop()

	// Save upstream information.
	if b.Upstreamurl != "" {
		fmt.Printf("Saving the upstream URL: %s\n", b.Upstreamurl)
		upstreamURLFile := filepath.Join(b.Statedir, "www", b.Mixver, "/upstreamurl")
		err = ioutil.WriteFile(upstreamURLFile, []byte(b.Upstreamurl), 0644)
		if err != nil {
			return errors.Wrapf(err, "couldn't write upstreamurl file")
		}
		fmt.Printf("Saving the upstream version: %s\n", b.Clearver)
		upstreamVerFile := filepath.Join(b.Statedir, "www", b.Mixver, "upstreamver")
		err = ioutil.WriteFile(upstreamVerFile, []byte(b.Clearver), 0644)
		if err != nil {
			return errors.Wrapf(err, "couldn't write upstreamver file")
		}
	}

	// Publish. Update the latest version both in the format (used by update itself) and in LAST_VER
	// (used to create newer manifests).
	if !publish {
		return nil
	}

	fmt.Printf("Setting latest version to %s\n", b.Mixver)

	err = ioutil.WriteFile(filepath.Join(formatDir, "latest"), []byte(b.Mixver), 0644)
	if err != nil {
		return errors.Wrapf(err, "couldn't update the latest version")
	}
	err = ioutil.WriteFile(filepath.Join(b.Statedir, "image", "LAST_VER"), []byte(b.Mixver), 0644)
	if err != nil {
		return errors.Wrapf(err, "couldn't update the latest version")
	}

	return nil
}

func (b *Builder) buildUpdateWithNewSwupd(timer *stopWatch, mixVersion uint32, minVersion uint32, format uint32, skipSigning bool) error {
	var err error

	err = writeMetaFiles(filepath.Join(b.Statedir, "www", b.Mixver), b.Format, Version)
	if err != nil {
		return errors.Wrapf(err, "failed to write update metadata files")
	}
	timer.Start("CREATE MANIFESTS")
	mom, err := swupd.CreateManifests(mixVersion, minVersion, uint(format), b.Statedir)
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

	outputDir := filepath.Join(b.Statedir, "www")
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
	fullfilesDir := filepath.Join(outputDir, b.Mixver, "files")
	fullChrootDir := filepath.Join(b.Statedir, "image", b.Mixver, "full")
	// TODO: CreateFullfiles should return us feedback on what was
	// done so we can report here.
	err = swupd.CreateFullfiles(mom.FullManifest, fullChrootDir, fullfilesDir)
	if err != nil {
		return err
	}
	timer.Stop()

	timer.Start("CREATE ZERO PACKS")
	chrootDir := filepath.Join(b.Statedir, "image")
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
		info, err = swupd.CreatePack(name, 0, version, outputDir, chrootDir)
		if err != nil {
			return errors.Wrapf(err, "couldn't make pack for bundle %s", name)
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
	updatecmd := exec.Command(prefixflag+"swupd_create_update", "-S", b.Statedir, "--minversion", strconv.Itoa(minVersion), "-F", b.Format, "--osversion", b.Mixver)
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
	fullfilecmd := exec.Command(prefixflag+"swupd_make_fullfiles", "-S", b.Statedir, b.Mixver)
	fullfilecmd.Stdout = os.Stdout
	fullfilecmd.Stderr = os.Stderr
	err = fullfilecmd.Run()
	if err != nil {
		return errors.Wrapf(err, "couldn't create fullfiles")
	}
	timer.Stop()

	// Create zero packs.
	timer.Start("CREATE ZERO PACKS")
	zeropackArgs := []string{"--to", b.Mixver, "-S", b.Statedir}
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

	content := "file://" + b.Statedir + "/www"
	imagecmd := exec.Command("ister.py", "-S", tempStage, "-t", template, "-V", content, "-C", content, "-f", format, "-s", b.Cert)
	imagecmd.Stdout = os.Stdout
	imagecmd.Stderr = os.Stderr

	return imagecmd.Run()
}

// AddRPMList copies rpms into the repodir and calls createrepo_c on it to
// generate a yum-consumable repository for the chroot builder to use.
func (b *Builder) AddRPMList(rpms []os.FileInfo) error {
	if b.Repodir == "" {
		return errors.Errorf("REPODIR not set in configuration")
	}
	err := os.MkdirAll(b.Repodir, 0755)
	if err != nil {
		return errors.Wrapf(err, "couldn't create REPODIR")
	}
	for _, rpm := range rpms {
		localPath := filepath.Join(b.RPMdir, rpm.Name())
		if err := checkRPM(localPath); err != nil {
			return err
		}
		// Skip RPM already in repo.
		repoPath := filepath.Join(b.Repodir, rpm.Name())
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
	cmd.Dir = b.Repodir

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
func (b *Builder) BuildDeltaPacks(from, to uint32) error {
	var err error

	// TODO: Configuration parsing should handle this validation/conversion.
	var mixVersion uint32
	mixVersion, err = parseUint32(b.Mixver)
	if err != nil {
		return errors.Wrapf(err, "couldn't parse mix version to use as target")
	}

	if to == 0 {
		to = mixVersion
	} else {
		if to > mixVersion {
			return errors.Errorf("--to version must be at most the latest mix version (%d)", mixVersion)
		}
	}
	if from >= to {
		return errors.Errorf("the --from version must be smaller than the --to version")
	}

	outputDir := filepath.Join(b.Statedir, "www")
	toManifest, err := swupd.ParseManifestFile(filepath.Join(outputDir, fmt.Sprint(to), "Manifest.MoM"))
	if err != nil {
		return errors.Wrapf(err, "couldn't find manifest of target version")
	}

	fromManifest, err := swupd.ParseManifestFile(filepath.Join(outputDir, fmt.Sprint(from), "Manifest.MoM"))
	if err != nil {
		return errors.Wrapf(err, "couldn't find manifest of from version")
	}

	chrootDir := filepath.Join(b.Statedir, "image")
	return createDeltaPacks(fromManifest, toManifest, outputDir, chrootDir)
}

// BuildDeltaPacksPreviousVersions builds packs to version from up to
// prev versions. It walks the Manifest "previous" field to find those from versions.
func (b *Builder) BuildDeltaPacksPreviousVersions(prev, to uint32) error {
	var err error

	// TODO: Configuration parsing should handle this validation/conversion.
	var mixVersion uint32
	mixVersion, err = parseUint32(b.Mixver)
	if err != nil {
		return errors.Wrapf(err, "couldn't parse mix version to use as target")
	}

	if to == 0 {
		to = mixVersion
	} else {
		if to > mixVersion {
			return errors.Errorf("--to version must be at most the latest mix version (%d)", mixVersion)
		}
	}

	outputDir := filepath.Join(b.Statedir, "www")
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
			return errors.Wrapf(err, "couldn't find manifest of previous version %d", cur)
		}
		previousManifests = append(previousManifests, m)
		cur = m.Header.Previous
	}

	fmt.Printf("Found %d previous versions\n", len(previousManifests))

	chrootDir := filepath.Join(b.Statedir, "image")
	for _, fromManifest := range previousManifests {
		fmt.Println()
		err = createDeltaPacks(fromManifest, toManifest, outputDir, chrootDir)
		if err != nil {
			return err
		}
	}
	return nil
}

func createDeltaPacks(from *swupd.Manifest, to *swupd.Manifest, outputDir, chrootDir string) error {
	fmt.Printf("Creating delta packs from %d to %d\n", from.Header.Version, to.Header.Version)
	bundlesToPack, err := swupd.FindBundlesToPack(from, to)
	if err != nil {
		return err
	}
	for _, b := range bundlesToPack {
		packPath := filepath.Join(outputDir, fmt.Sprint(b.ToVersion), swupd.GetPackFilename(b.Name, b.FromVersion))
		_, err = os.Lstat(packPath)
		if err == nil {
			fmt.Printf("  Delta pack already exists for %s from %d to %d\n", b.Name, b.FromVersion, b.ToVersion)
			continue
		}
		if !os.IsNotExist(err) {
			return errors.Wrapf(err, "couldn't access existing pack file %s", packPath)
		}
		fmt.Printf("  Creating delta pack for bundle %s from %d to %d\n", b.Name, b.FromVersion, b.ToVersion)
		info, err := swupd.CreatePack(b.Name, b.FromVersion, b.ToVersion, outputDir, chrootDir)
		if err != nil {
			return err
		}

		if len(info.Warnings) > 0 {
			for _, w := range info.Warnings {
				fmt.Printf("    WARNING: %s\n", w)
			}
			fmt.Println()
		}
		fmt.Printf("    Fullfiles in pack: %d\n", info.FullfileCount)
		fmt.Printf("    Deltas in pack: %d\n", info.DeltaCount)
	}
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
