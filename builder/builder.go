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
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/clearlinux/mixer-tools/config"
	"github.com/clearlinux/mixer-tools/helpers"
	"github.com/clearlinux/mixer-tools/swupd"
	"github.com/pkg/errors"
)

// Version of Mixer. This is provided by ldflags in Makefile during compilation
var Version = ""

// Native controls whether mixer runs the command on the native machine or in a
// container.
var Native = true

// Offline controls whether mixer attempts to automatically cache upstream
// bundles. In offline mode, all necessary bundles must exist in local-bundles.
var Offline = false

// A Builder contains all configurable fields required to perform a full mix
// operation, and is used to encapsulate life time data.
type Builder struct {
	Config config.MixConfig
	State  config.MixState

	MixVer            string
	MixVerFile        string
	MixBundlesFile    string
	LocalPackagesFile string
	UpstreamURL       string
	UpstreamURLFile   string
	UpstreamVer       string
	UpstreamVerFile   string

	Signing int
	Bump    int

	NumFullfileWorkers int
	NumDeltaWorkers    int
	NumBundleWorkers   int

	// Parsed versions.
	MixVerUint32      uint32
	UpstreamVerUint32 uint32
}

// UpdateParameters contains the configuration parameters for building an update
type UpdateParameters struct {
	// Minimum version used to generate delta packs
	MinVersion int
	// Format version used in this update
	Format string
	// Update latest format version and image version files to current mix
	Publish bool
	// Skip signing Manifest.MoM
	SkipSigning bool
	// Skip fullfiles generation
	SkipFullfiles bool
	// Skip zero packs generation
	SkipPacks bool
}

var localPackages = make(map[string]bool)
var upstreamPackages = make(map[string]bool)

// New will return a new instance of Builder with some predetermined sane
// default values.
func New() *Builder {
	return &Builder{
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
	if err := b.Config.LoadDefaults(); err != nil {
		return nil, err
	}
	if err := b.Config.LoadConfig(conf); err != nil {
		return nil, err
	}
	if err := b.State.Load(b.Config); err != nil {
		return nil, err
	}
	if err := b.ReadVersions(); err != nil {
		return nil, err
	}
	return b, nil
}

// InitMix will initialise a new swupd-client consumable "mix" with the given
// based Clear Linux version and specified mix version.
func (b *Builder) InitMix(upstreamVer string, mixVer string, allLocal bool, allUpstream bool, noDefaults bool, upstreamURL string, git bool) error {
	// Set up local dirs
	if err := b.initDirs(); err != nil {
		return err
	}

	// Set up mix metadata
	// Deprecate '.clearurl' --> 'upstreamurl'
	if _, err := os.Stat(filepath.Join(b.Config.Builder.VersionPath, ".clearurl")); err == nil {
		b.UpstreamURLFile = ".clearurl"
		log.Println("Warning: '.clearurl' has been deprecated. Please rename file to 'upstreamurl'")
	}
	if err := ioutil.WriteFile(filepath.Join(b.Config.Builder.VersionPath, b.UpstreamURLFile), []byte(upstreamURL), 0644); err != nil {
		return err
	}
	b.UpstreamURL = upstreamURL

	if upstreamVer == "latest" {
		ver, err := b.getLatestUpstreamVersion()
		if err != nil {
			return err
		}
		upstreamVer = ver
	}

	fmt.Printf("Initializing mix version %s from upstream version %s\n", mixVer, upstreamVer)

	// Deprecate '.clearversion' --> 'upstreamversion'
	if _, err := os.Stat(filepath.Join(b.Config.Builder.VersionPath, ".clearversion")); err == nil {
		b.UpstreamVerFile = ".clearversion"
		log.Println("Warning: '.clearversion' has been deprecated. Please rename file to 'upstreamversion'")
	}
	if err := ioutil.WriteFile(filepath.Join(b.Config.Builder.VersionPath, b.UpstreamVerFile), []byte(upstreamVer), 0644); err != nil {
		return err
	}
	b.UpstreamVer = upstreamVer

	// Deprecate '.mixversion' --> 'mixversion'
	if _, err := os.Stat(filepath.Join(b.Config.Builder.VersionPath, ".mixversion")); err == nil {
		b.MixVerFile = ".mixversion"
		log.Println("Warning: '.mixversion' has been deprecated. Please rename file to 'mixversion'")
	}
	if err := ioutil.WriteFile(filepath.Join(b.Config.Builder.VersionPath, b.MixVerFile), []byte(mixVer), 0644); err != nil {
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

	// When running in offline mode, there is no upstream to get the default bundles from,
	// so the mix must be created without the default bundles.
	if Offline && !noDefaults {
		fmt.Println("Running in offline mode. Forcing --no-default-bundles")
		noDefaults = true
	}

	// Initialize the Mix Bundles List
	var bundles []string
	if !noDefaults {
		bundles = []string{"os-core", "os-core-update", "bootloader", "kernel-native"}
	}
	if err := b.AddBundles(bundles, allLocal, allUpstream, false); err != nil {
		return err
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

	// Create the DNF conf early in case we want to edit before building a first mix
	if err := b.NewDNFConfIfNeeded(); err != nil {
		return err
	}

	return nil
}

// BuildBundles will attempt to construct the bundles required by generating a
// DNF configuration file, then resolving all files for each bundle using dnf
// resolve and no-op installs. One full chroot is created from this step with
// the file contents of all bundles.
func (b *Builder) BuildBundles(template *x509.Certificate, privkey *rsa.PrivateKey, signflag, clean bool, downloadRetries int) error {
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
	if err := b.NewDNFConfIfNeeded(); err != nil {
		return err
	}

	// If ServerStateDir does not exist, create it with ownership matching its
	// closest existing ancestor directory
	if _, err := os.Stat(b.Config.Builder.ServerStateDir); os.IsNotExist(err) {
		uid, gid, err := getClosestAncestorOwner(b.Config.Builder.ServerStateDir)
		if err != nil {
			return err
		}
		if err = os.MkdirAll(b.Config.Builder.ServerStateDir, 0755); err != nil {
			return errors.Wrapf(err, "Failed to create server state dir: %q", b.Config.Builder.ServerStateDir)
		}
		if err = os.Chown(b.Config.Builder.ServerStateDir, uid, gid); err != nil {
			return errors.Wrapf(err, "Failed to set ownership of dir: %q", b.Config.Builder.ServerStateDir)
		}
	}

	if _, err := os.Stat(b.Config.Builder.ServerStateDir + "/image/" + b.MixVer); err == nil && clean {
		fmt.Printf("* Wiping away previous version %s...\n", b.MixVer)
		err = os.RemoveAll(b.Config.Builder.ServerStateDir + "/www/" + b.MixVer)
		if err != nil {
			return err
		}
		err = os.RemoveAll(b.Config.Builder.ServerStateDir + "/image/" + b.MixVer)
		if err != nil {
			return err
		}
	}

	// Generate the certificate needed for signing verification if it does not exist
	if signflag == false && template != nil {
		err := helpers.GenerateCertificate(b.Config.Builder.Cert, template, template, &privkey.PublicKey, privkey)
		if err != nil {
			return err
		}
	}

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
	err = b.buildBundles(set, downloadRetries)
	if err != nil {
		return err
	}

	// TODO: Move this logic to code that uses this.
	// If LAST_VER don't exists, it means this is the first bundle we build,
	// so initialize it to version "0".
	lastVerPath := filepath.Join(b.Config.Builder.ServerStateDir, "image", "LAST_VER")
	if _, err = os.Stat(lastVerPath); os.IsNotExist(err) {
		err = ioutil.WriteFile(lastVerPath, []byte("0\n"), 0644)
		if err != nil {
			return err
		}
	}

	timer.Stop()

	return nil
}

// BuildUpdate will produce an update consumable by the swupd client
func (b *Builder) BuildUpdate(params UpdateParameters) error {
	var err error

	if params.MinVersion < 0 || params.MinVersion > math.MaxUint32 {
		return errors.Errorf("minVersion %d is out of range", params.MinVersion)
	}

	if params.Format != "" {
		b.State.Mix.Format = params.Format
	}

	// Ensure the format dir exists.
	formatDir := filepath.Join(b.Config.Builder.ServerStateDir, "www", "version", "format"+b.State.Mix.Format)
	err = os.MkdirAll(formatDir, 0777)
	if err != nil {
		return errors.Wrapf(err, "couldn't create the format directory")
	}

	timer := &stopWatch{w: os.Stdout}
	defer timer.WriteSummary(os.Stdout)

	err = b.buildUpdateContent(params, timer)
	if err != nil {
		return err
	}

	// Save upstream information.
	if b.UpstreamURL != "" {
		fmt.Printf("Saving the upstream URL: %s\n", b.UpstreamURL)
		upstreamURLFile := filepath.Join(b.Config.Builder.ServerStateDir, "www", b.MixVer, "/upstreamurl")
		err = ioutil.WriteFile(upstreamURLFile, []byte(b.UpstreamURL), 0644)
		if err != nil {
			return errors.Wrapf(err, "couldn't write upstreamurl file")
		}
		fmt.Printf("Saving the upstream version: %s\n", b.UpstreamVer)
		upstreamVerFile := filepath.Join(b.Config.Builder.ServerStateDir, "www", b.MixVer, "upstreamver")
		err = ioutil.WriteFile(upstreamVerFile, []byte(b.UpstreamVer), 0644)
		if err != nil {
			return errors.Wrapf(err, "couldn't write upstreamver file")
		}
	}

	// Publish. Update the latest version file in various locations.
	if !params.Publish {
		return nil
	}

	fmt.Printf("Setting latest version to %s\n", b.MixVer)
	latestVerFilePath := filepath.Join(b.Config.Builder.ServerStateDir, "www", "version", "latest_version")
	err = ioutil.WriteFile(latestVerFilePath, []byte(b.MixVer), 0644)
	if err != nil {
		return errors.Wrapf(err, "couldn't update the latest_version file")
	}

	// sign the latest_version file
	if !params.SkipSigning {
		fmt.Println("Signing latest_version file.")
		err = b.signFile(latestVerFilePath)
		if err != nil {
			return errors.Wrapf(err, "couldn't sign the latest_version file")
		}
	}
	err = ioutil.WriteFile(filepath.Join(formatDir, "latest"), []byte(b.MixVer), 0644)
	if err != nil {
		return errors.Wrapf(err, "couldn't update the latest version")
	}

	// sign the latest file in place based on the Mixed format
	// read from builder.conf.
	if !params.SkipSigning {
		fmt.Println("Signing latest file.")
		err = b.signFile(filepath.Join(formatDir, "latest"))
		if err != nil {
			return errors.Wrapf(err, "couldn't sign the latest file")
		}
	}

	err = ioutil.WriteFile(filepath.Join(b.Config.Builder.ServerStateDir, "image", "LAST_VER"), []byte(b.MixVer), 0644)
	if err != nil {
		return errors.Wrapf(err, "couldn't update the latest version")
	}

	return nil
}

const imageTemplate = "release-image-config.json"
const isterConfigDir = "/usr/share/defaults/ister"

// BuildImage will now proceed to build the full image with the previously
// validated configuration.
func (b *Builder) BuildImage(format string, template string) error {
	// If the user did not pass in a format, default to builder.conf
	if format == "" {
		format = b.State.Mix.Format
	}

	// If the user did not pass in a template, use the default template
	if template == "" {
		template = imageTemplate
		// If the default image template is not present in the mix workspace,
		// copy it from the default ister config directory and update the bundle list based on mix bundles
		templateFile := filepath.Join(b.Config.Builder.VersionPath, template)
		if _, err := os.Stat(templateFile); os.IsNotExist(err) {
			fmt.Printf("Warning: Image template %s not found\n", templateFile)
			configFile := filepath.Join(isterConfigDir, template)
			fmt.Printf("Copying image template from %s\n", configFile)
			if err = b.copyImageTemplate(configFile, templateFile); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
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

	content := "file://" + b.Config.Builder.ServerStateDir + "/www"
	imagecmd := exec.Command("ister.py", "-S", tempStage, "-t", template, "-V", content, "-C", content, "-f", format, "-s", b.Config.Builder.Cert)
	imagecmd.Stdout = os.Stdout
	imagecmd.Stderr = os.Stderr

	return imagecmd.Run()
}

// copyImageTemplate will copy the image template from the default ister config directory
// and update the image bundle list based on mix bundles.
// If there is an error updating the image bundle list, the default bundle list will be used.
func (b *Builder) copyImageTemplate(configFile, templateFile string) error {
	// Read ister template
	configValues, err := ioutil.ReadFile(configFile)
	if err != nil {
		return err
	}
	fmt.Printf("Updating image bundle list based on %s\n", b.MixBundlesFile)
	mixBundles, err := b.getMixBundlesListAsSet() // returns empty set with no error if mix bundles file is not present
	if err == nil && len(mixBundles) > 0 {        // check if set is not empty
		var bundles []string
		for _, bundle := range mixBundles {
			bundles = append(bundles, bundle.Name)
		}
		if err = updateImageBundles(&configValues, bundles); err != nil {
			return errors.Wrap(err, "Failed to copy image template. Invalid JSON format")
		}
	} else {
		fmt.Printf("Warning: Failed to read %s. Using default bundle list instead\n", b.MixBundlesFile)
	}

	return ioutil.WriteFile(templateFile, configValues, 0644)
}

// updateImageBundles will update the image bundle list based on the mix bundles.
func updateImageBundles(configValues *[]byte, bundles []string) error {
	var data map[string]interface{}
	err := json.Unmarshal(*configValues, &data)
	if err != nil {
		return err
	}
	data["Bundles"] = bundles
	*configValues, err = json.MarshalIndent(data, "", " ")
	return err
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
	if from == to {
		fmt.Println("the --from version matches the --to version, nothing to do")
		return nil
	} else if from > to {
		return errors.Errorf("the --from version must be smaller than the --to version")
	}

	outputDir := filepath.Join(b.Config.Builder.ServerStateDir, "www")
	toManifest, err := swupd.ParseManifestFile(filepath.Join(outputDir, fmt.Sprint(to), "Manifest.MoM"))
	if err != nil {
		return errors.Wrapf(err, "couldn't find manifest of target version")
	}

	fromManifest, err := swupd.ParseManifestFile(filepath.Join(outputDir, fmt.Sprint(from), "Manifest.MoM"))
	if err != nil {
		return errors.Wrapf(err, "couldn't find manifest of from version")
	}

	bundleDir := filepath.Join(b.Config.Builder.ServerStateDir, "image")
	fmt.Printf("Using %d workers\n", b.NumDeltaWorkers)
	// Create all deltas first
	bsdiffLog, logFile, err := swupd.CreateBsdiffLogger(b.Config.Builder.ServerStateDir)
	if err != nil {
		return err
	}
	defer func() {
		_ = logFile.Close()
	}()
	err = swupd.CreateAllDeltas(outputDir, int(fromManifest.Header.Version), int(toManifest.Header.Version), b.NumDeltaWorkers, bsdiffLog)
	if err != nil {
		return err
	}

	// Create packs filling in any missing deltas
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

	outputDir := filepath.Join(b.Config.Builder.ServerStateDir, "www")
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
			log.Printf("Warning: Could not find manifest for previous version %d, skipping...\n", cur)
			continue
		}
		// do not create delta-packs over format bumps since clients can't update
		// past the boundary anyways. Only check for inequality, if the format
		// goes down that should be checked elsewhere.
		if m.Header.Format != toManifest.Header.Format {
			log.Println("Warning: skipping delta-pack creation over format bump")
			continue
		}
		previousManifests = append(previousManifests, m)
		cur = m.Header.Previous
	}

	fmt.Printf("Found %d previous versions\n", len(previousManifests))
	if len(previousManifests) == 0 {
		return nil
	}
	bundleDir := filepath.Join(b.Config.Builder.ServerStateDir, "image")
	// Create all deltas for all previous versions first based on full manifests
	var versionQueue = make(chan *swupd.Manifest)
	mux := &sync.Mutex{}
	var wg sync.WaitGroup
	var deltaErrors []error
	versionWorkers := 1

	// If we have at least 2x the number of CPUs as versions, give each version
	// its own thread to build deltas in.
	if runtime.NumCPU() >= 2*len(previousManifests) {
		b.NumDeltaWorkers = int(math.Ceil(float64(runtime.NumCPU()) / float64(len(previousManifests))))
		versionWorkers = len(previousManifests)
	}
	wg.Add(versionWorkers)
	fmt.Printf("Using %d version threads and %d delta threads in each\n", versionWorkers, b.NumDeltaWorkers)

	bsdiffLog, logFile, err := swupd.CreateBsdiffLogger(b.Config.Builder.ServerStateDir)
	if err != nil {
		return err
	}
	defer func() {
		_ = logFile.Close()
	}()

	// If possible, run a thread for each version back so we don't get locked up
	// at the end of a version doing some large/slow delta pack in serial. This way
	// the large file(s) at the end of each version will run in parallel.
	for i := 0; i < versionWorkers; i++ {
		go func() {
			defer wg.Done()
			for fromManifest := range versionQueue {
				deltaErr := swupd.CreateAllDeltas(outputDir, int(fromManifest.Header.Version), int(toManifest.Header.Version), b.NumDeltaWorkers, bsdiffLog)
				if deltaErr != nil {
					mux.Lock()
					deltaErrors = append(deltaErrors, deltaErr)
					mux.Unlock()
				}
			}
		}()
	}

	// Send jobs to the queue for version goroutines to pick up.
	for i := range previousManifests {
		versionQueue <- previousManifests[i]
	}

	// Send message that no more jobs are being sent
	close(versionQueue)
	wg.Wait()

	for i := 0; i < len(deltaErrors); i++ {
		log.Printf("%s\n", deltaErrors[i])
	}

	// Simply pack all deltas up since they are now created
	for _, fromManifest := range previousManifests {
		fmt.Println()
		err = createDeltaPacks(fromManifest, toManifest, printReport, outputDir, bundleDir, b.NumDeltaWorkers)
		if err != nil {
			return err
		}
	}
	return nil
}

// BuildDeltaManifests between two versions of the mix.
func (b *Builder) BuildDeltaManifests(from, to uint32) error {
	var err error

	if to == 0 {
		to = b.MixVerUint32
	} else if to > b.MixVerUint32 {
		return errors.Errorf("--to version must be at most the latest mix version (%d)", b.MixVerUint32)
	}
	if from == to {
		fmt.Println("the --from version matches the --to version, nothing to do")
		return nil
	} else if from > to {
		return errors.Errorf("the --from version must be smaller than the --to version")
	}

	outputDir := filepath.Join(b.Config.Builder.ServerStateDir, "www")

	toManifest, err := swupd.ParseManifestFile(filepath.Join(outputDir, fmt.Sprint(to), "Manifest.MoM"))
	if err != nil {
		return errors.Wrapf(err, "couldn't find manifest of target version")
	}

	fromManifest, err := swupd.ParseManifestFile(filepath.Join(outputDir, fmt.Sprint(from), "Manifest.MoM"))
	if err != nil {
		return errors.Wrapf(err, "couldn't find manifest of from version")
	}

	fmt.Printf("Using %d workers\n", b.NumDeltaWorkers)
	fmt.Printf("Creating Manifest delta files from %d to %d\n", from, to)
	deltas, err := swupd.CreateManifestDeltas(b.Config.Builder.ServerStateDir, fromManifest, toManifest, b.NumDeltaWorkers)
	if err != nil {
		log.Printf("  %s\n", err)
	} else {
		created := 0
		for _, delta := range deltas {
			if delta.Error == nil {
				created++
			}
		}
		fmt.Printf("  Created %d Manifest delta files\n", created)
	}

	return nil
}

// BuildDeltaManifestsPreviousVersions builds manifests to version from up to
// prev versions. It walks the Manifest "previous" field to find those from versions.
func (b *Builder) BuildDeltaManifestsPreviousVersions(prev, to uint32) error {
	var err error

	if to == 0 {
		to = b.MixVerUint32
	} else if to > b.MixVerUint32 {
		return errors.Errorf("--to version must be at most the latest mix version (%d)", b.MixVerUint32)
	}

	outputDir := filepath.Join(b.Config.Builder.ServerStateDir, "www")
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
			log.Printf("Warning: Could not find manifest for previous version %d, skipping...\n", cur)
			continue
		}
		// do not create delta-manifests over format bumps since clients can't update
		// past the boundary anyways. Only check for inequality, if the format
		// goes down that should be checked elsewhere.
		if m.Header.Format != toManifest.Header.Format {
			log.Println("Warning: skipping delta-pack creation over format bump")
			break
		}
		previousManifests = append(previousManifests, m)
		cur = m.Header.Previous
	}

	fmt.Printf("Found %d previous versions\n", len(previousManifests))
	if len(previousManifests) == 0 {
		return nil
	}

	for _, i := range previousManifests {
		fmt.Printf("Creating Manifest delta files from %d to %d\n", i.Header.Version, to)
		deltas, deltaErr := swupd.CreateManifestDeltas(b.Config.Builder.ServerStateDir, i, toManifest, b.NumDeltaWorkers)
		if deltaErr != nil {
			log.Printf("  %s\n", err)
		} else {
			created := 0
			for _, delta := range deltas {
				if delta.Error == nil {
					created++
				}
			}
			fmt.Printf("  Created %d Manifest delta files\n", created)

		}
	}

	return nil
}
