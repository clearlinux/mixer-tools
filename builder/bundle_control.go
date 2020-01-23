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
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/clearlinux/mixer-tools/helpers"
	"github.com/pkg/errors"
)

const (
	upstreamBundlesBaseDir   = "upstream-bundles"
	upstreamBundlesVerDirFmt = "clr-bundles-%s"
	upstreamBundlesBundleDir = "bundles"
)

func getUpstreamBundlesVerDir(ver string) string {
	return fmt.Sprintf(upstreamBundlesVerDirFmt, ver)
}

func (b *Builder) getUpstreamBundlesPath() string {
	return filepath.Join(b.Config.Builder.VersionPath, upstreamBundlesBaseDir,
		fmt.Sprintf(upstreamBundlesVerDirFmt, b.UpstreamVer), upstreamBundlesBundleDir)
}

func (b *Builder) getLocalPackagesPath() string {
	return filepath.Join(b.Config.Builder.VersionPath, b.LocalPackagesFile)
}

func (b *Builder) getUpstreamPackagesPath() string {
	return filepath.Join(b.Config.Builder.VersionPath, upstreamBundlesBaseDir, getUpstreamBundlesVerDir(b.UpstreamVer), "packages")
}

func (b *Builder) getUpstreamBundles() error {
	if Offline {
		return nil
	}

	bundleDir := b.getUpstreamBundlesPath()

	// Return if upstream bundle dir for current version already exists
	if _, err := os.Stat(bundleDir); err == nil {
		return nil
	}

	// Make the folder to store upstream bundles if does not exist
	if err := os.MkdirAll(upstreamBundlesBaseDir, 0777); err != nil {
		return errors.Wrap(err, "Failed to create upstream-bundles dir.")
	}

	// Download the upstream bundles
	tmpTarFile := filepath.Join(upstreamBundlesBaseDir, b.UpstreamVer+".tar.gz")
	URL := b.Config.Swupd.UpstreamBundlesURL + b.UpstreamVer + ".tar.gz"
	fmt.Printf("Fetching upstream bundles from %s\n", URL)
	if err := helpers.DownloadFile(URL, tmpTarFile); err != nil {
		return errors.Wrapf(err, "Failed to download bundles for upstream version %s", b.UpstreamVer)
	}

	if err := helpers.UnpackFile(tmpTarFile, upstreamBundlesBaseDir); err != nil {
		err = errors.Wrapf(err, "Error unpacking bundles for upstream version %s\n%s left for debuging", b.UpstreamVer, tmpTarFile)

		// Clean up upstream bundle dir, since unpack failed
		path := filepath.Join(upstreamBundlesBaseDir, getUpstreamBundlesVerDir(b.UpstreamVer))
		if cErr := os.RemoveAll(path); cErr != nil {
			err = errors.Wrapf(err, "Error cleaning up upstream bundle dir: %s", path)
		}
		return err
	}

	return errors.Wrapf(os.Remove(tmpTarFile), "Failed to remove temp bundle archive: %s", tmpTarFile)
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
	path := filepath.Join(b.Config.Mixer.LocalBundleDir, bundle)
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
	path = filepath.Join(b.getUpstreamBundlesPath(), bundle)
	if _, err = os.Stat(path); err == nil {
		return path, nil
	}

	// Check upstream-packages
	path = b.getUpstreamPackagesPath()
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
	if strings.HasPrefix(path, b.Config.Mixer.LocalBundleDir) {
		// the path must be longer than the localbundledir by at least
		// 2 so a bundle name follows the localbundledir prefix after the
		// slash (/)
		return len(path)-len(b.Config.Mixer.LocalBundleDir) >= 2
	}
	return b.isLocalPackagePath(path)
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
	return filepath.Base(path) == b.LocalPackagesFile
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

	return bundle, nil
}

// getMixBundlesListAsSet reads in the Mix Bundles List file and returns the
// resultant set of unique bundle objects. If the mix bundles file does not
// exist or is empty, an empty set is returned.
func (b *Builder) getMixBundlesListAsSet() (bundleSet, error) {
	set := make(bundleSet)

	bundles, err := helpers.ReadFileAndSplit(filepath.Join(b.Config.Builder.VersionPath, b.MixBundlesFile))
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

	files, err := helpers.ListVisibleFiles(dir)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to read bundles dir: %s", dir)
	}

	for _, file := range files {
		bundle, err := b.getBundleFromName(file)
		if err != nil {
			return nil, err
		}
		set[file] = bundle
	}
	return set, nil
}

// writeMixBundleList writes the contents of a bundle set out to the Mix Bundles
// List file. Values will be in sorted order.
func (b *Builder) writeMixBundleList(set bundleSet) error {
	data := []byte(strings.Join(getBundleSetKeysSorted(set), "\n") + "\n")
	if err := ioutil.WriteFile(filepath.Join(b.Config.Builder.VersionPath, b.MixBundlesFile), data, 0644); err != nil {
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

				if len(bundle.OptionalIncludes) > 0 {
					err := recurseBundleSet(bundle.OptionalIncludes)
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
	err := setPackagesList(source, filename)
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
	// Add the included and optional included bundles to the mix
	for _, bundle := range set {
		err := b.AddBundles(bundle.DirectIncludes, false, false, false)
		if err != nil {
			return nil, err
		}
		err = b.AddBundles(bundle.OptionalIncludes, false, false, false)
		if err != nil {
			return nil, err
		}
	}
	return set, nil
}

// AddBundles adds the specified bundles to the Mix Bundles List. Values are
// verified as valid, and duplicate values are removed. The resulting Mix
// Bundles List will be in sorted order.
func (b *Builder) AddBundles(bundles []string, allLocal bool, allUpstream bool, git bool) error {
	// Fetch upstream bundle files if needed
	if err := b.getUpstreamBundles(); err != nil {
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
			log.Println("Warning: " + err.Error() + "; skipping")
			continue
		}
		if err = validateBundleName(bName); err != nil {
			log.Println("Warning: " + err.Error() + "; skipping")
			continue
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
		localSet, err := b.getDirBundlesListAsSet(b.Config.Mixer.LocalBundleDir)
		if err != nil {
			return errors.Wrapf(err, "Failed to read local bundles dir: %s", b.Config.Mixer.LocalBundleDir)
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
		upstreamBundleDir := b.getUpstreamBundlesPath()
		upstreamSet, err := b.getDirBundlesListAsSet(upstreamBundleDir)
		if err != nil {
			return errors.Wrapf(err, "Failed to read upstream bundles dir: %s", upstreamBundleDir)
		}
		// handle packages defined in upstream packages file, if it exists
		err = populateSetFromPackages(&upstreamPackages, upstreamSet, b.getUpstreamPackagesPath())
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
	if err := b.getUpstreamBundles(); err != nil {
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
			if _, err := os.Stat(filepath.Join(b.Config.Mixer.LocalBundleDir, bundle)); err == nil {
				fmt.Printf("Removing bundle file for %q from local-bundles\n", bundle)
				if err := os.Remove(filepath.Join(b.Config.Mixer.LocalBundleDir, bundle)); err != nil {
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
		if b.isLocalPackagePath(bundle.Filename) {
			value += " (local package)"
		} else {
			value += " (local bundle)"
		}
	} else {
		if isUpstreamPackagePath(bundle.Filename) {
			value += " (upstream package)"
		} else {
			value += " (upstream bundle)"
		}
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
	if err := b.getUpstreamBundles(); err != nil {
		return err
	}

	var bundles bundleSet

	// Get the bundle sets used for processing
	mixBundles, err := b.getMixBundlesListAsSet()
	if err != nil {
		return err
	}
	localBundles, err := b.getDirBundlesListAsSet(b.Config.Mixer.LocalBundleDir)
	if err != nil {
		return err
	}
	// handle packages defined in local-packages, if it exists
	err = populateSetFromPackages(&localPackages, localBundles, b.getLocalPackagesPath())
	if err != nil {
		return err
	}
	upstreamBundles, err := b.getDirBundlesListAsSet(b.getUpstreamBundlesPath())
	if err != nil {
		if !Offline {
			return err
		}
		upstreamBundles = make(bundleSet)
	}
	// handle packages defined in upstream packages file, if it exists
	err = populateSetFromPackages(&upstreamPackages, upstreamBundles, b.getUpstreamPackagesPath())
	if err != nil {
		return err
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
				if b.isLocalPackagePath(localBundles[bundle].Filename) {
					location = "(local package)"
				} else {
					location = "(local bundle)"
				}
			} else {
				if isUpstreamPackagePath(upstreamBundles[bundle].Filename) {
					location = "(upstream package)"
				} else {
					location = "(upstream bundle)"
				}
			}
			var included string
			if _, exists := bundles[bundle]; !exists {
				included = "(included)"
			}
			if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\n", bundle, location, included); err != nil {
				return err
			}
		}
	case LocalList:
		// Only print the top-level set
		sorted := getBundleSetKeysSorted(bundles)
		for _, bundle := range sorted {
			var mix string
			if _, exists := mixBundles[bundle]; exists {
				mix = "(in mix)"
			}
			var pkg string
			if b.isLocalPackagePath(localBundles[bundle].Filename) {
				pkg = "(package)"
			}
			var masking string
			if _, exists := upstreamBundles[bundle]; exists {
				masking = "(masking upstream)"
			}
			if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", bundle, pkg, mix, masking); err != nil {
				return err
			}
		}
	case UpstreamList:
		// Only print the top-level set
		sorted := getBundleSetKeysSorted(bundles)
		for _, bundle := range sorted {
			var mix string
			if _, exists := mixBundles[bundle]; exists {
				mix = "(in mix)"
			}
			var pkg string
			if isUpstreamPackagePath(upstreamBundles[bundle].Filename) {
				pkg = "(package)"
			}
			var masked string
			if _, exists := localBundles[bundle]; exists {
				masked = "(masked by local)"
			}
			if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", bundle, pkg, mix, masked); err != nil {
				return err
			}
		}
	}

	_ = tw.Flush()

	return nil
}

const bundleTemplateFormat = `# [TITLE]: %s
# [DESCRIPTION]: 
# [STATUS]: 
# [CAPABILITIES]:
# [MAINTAINER]: 
# 
# List packages one per line.
# includes have format:        include(bundle)
# also-adds have format:       also-add(bundle)
# content chroots have format: content(path) 
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

// CreateBundles copies a list of bundles from upstream-bundles to local-bundles
// if they are not already there or creates a blank template if they are new.
// 'add' will also add the bundles to the mix.
func (b *Builder) CreateBundles(bundles []string, add bool, git bool) error {
	// Fetch upstream bundle files if needed
	if err := b.getUpstreamBundles(); err != nil {
		return err
	}
	var err error
	for _, bundle := range bundles {
		path, _ := b.getBundlePath(bundle)
		if !b.isLocalBundle(path) {
			localPath := filepath.Join(b.Config.Mixer.LocalBundleDir, bundle)

			if path == "" {
				// Bundle not found upstream, so create new
				if err = createBundleFile(bundle, localPath); err != nil {
					return errors.Wrapf(err, "Failed to write bundle template for bundle %q", bundle)
				}
			} else {
				// Bundle found upstream, so copy over
				if err = helpers.CopyFile(localPath, path); err != nil {
					return err
				}
			}
		}
		fmt.Printf("Created bundle %q in %q\n", bundle, b.Config.Mixer.LocalBundleDir)
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
