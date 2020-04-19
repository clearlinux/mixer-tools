// Copyright 2017 Intel Corporation
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

package swupd

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/clearlinux/mixer-tools/log"
)

// UpdateInfo contains the meta information for the current update
type UpdateInfo struct {
	oldFormat  uint
	format     uint
	previous   uint32
	minVersion uint32
	version    uint32
	bundles    []string
	timeStamp  time.Time
}

func initBuildEnv(c config) error {
	tmpDir := filepath.Join(c.stateDir, "temp")
	// remove old directory
	if err := os.RemoveAll(tmpDir); err != nil {
		return err
	}

	// create new one
	return os.Mkdir(tmpDir, os.ModePerm)
}

func getOldManifest(path string) (*Manifest, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return &Manifest{}, nil
	}
	return ParseManifestFile(path)
}

func initBundles(ui UpdateInfo, c config, numWorkers int) ([]*Manifest, error) {
	var wg sync.WaitGroup
	if numWorkers <= 0 {
		numWorkers = runtime.NumCPU()
	}
	if numWorkers > len(ui.bundles) {
		numWorkers = len(ui.bundles)
	}
	wg.Add(numWorkers)

	bundleChan := make(chan string)
	errorChan := make(chan error, numWorkers)
	defer close(errorChan)

	mux := &sync.Mutex{}
	tmpManifests := []*Manifest{}
	log.Info(log.Mixer, "Generating initial manifests...")
	bundleWorker := func() {
		defer wg.Done()
		var err error
		for bundleName := range bundleChan {
			bundle := &Manifest{
				Header: ManifestHeader{
					Format:    ui.format,
					Version:   ui.version,
					Previous:  ui.previous,
					TimeStamp: ui.timeStamp,
				},
				Name: bundleName,
			}

			if bundleName == "full" {
				// full manifest needs to be processed differently by reading
				// the files directly from the full chroot. No bundle-info
				// file exists for full.
				mux.Lock()
				tmpManifests = append(tmpManifests, bundle)
				mux.Unlock()
				continue
			} else {
				log.Info(log.Mixer, "  %s", bundleName)
				biPath := filepath.Join(c.imageBase, fmt.Sprint(ui.version), bundle.Name+"-info")
				if _, err = os.Stat(biPath); os.IsNotExist(err) {
					err = syncToFull(ui.version, bundle.Name, c.imageBase)
					if err != nil {
						errorChan <- err
						return
					}
				}

				err = bundle.GetBundleInfo(c.stateDir, biPath)
				if err != nil {
					errorChan <- err
					return
				}
			}

			mux.Lock()
			tmpManifests = append(tmpManifests, bundle)
			mux.Unlock()
		}
	}

	for i := 0; i < numWorkers; i++ {
		go bundleWorker()
	}

	var err error
	for _, bn := range ui.bundles {
		select {
		case bundleChan <- bn:
		case err = <-errorChan:
			// If there is an error,we break
			// immediately and return error
			break
		}
		if err != nil {
			break
		}
	}

	close(bundleChan)
	wg.Wait()

	if err != nil {
		return nil, err
	}

	if len(errorChan) > 0 {
		return nil, <-errorChan
	}

	return tmpManifests, err
}
func readIncludes(tmpManifests []*Manifest, numWorkers int) (*Manifest, error) {
	var wg sync.WaitGroup
	if numWorkers <= 0 {
		numWorkers = runtime.NumCPU()
	}
	if numWorkers > len(tmpManifests) {
		numWorkers = len(tmpManifests)
	}
	wg.Add(numWorkers)

	manifestChan := make(chan *Manifest)
	errorChan := make(chan error, numWorkers)
	defer close(errorChan)
	var newFull *Manifest
	// read includes for subtraction processing
	log.Info(log.Mixer, "Reading bundle includes...")
	bundleWorker := func() {
		defer wg.Done()
		var err error

		for manifest := range manifestChan {
			if manifest.Name == "full" {
				newFull = manifest
				continue
			}
			if manifest.Name != "os-core" {
				// read in bundle includes
				if err = manifest.ReadIncludesFromBundleInfo(tmpManifests); err != nil {
					errorChan <- err
					return
				}
			}
		}
	}
	for i := 0; i < numWorkers; i++ {
		go bundleWorker()
	}

	var err error
	for _, bn := range tmpManifests {
		select {
		case manifestChan <- bn:
		case err = <-errorChan:
			// If there is an error,we break
			// immediately and return error
			break
		}
		if err != nil {
			break
		}
	}

	close(manifestChan)
	wg.Wait()

	if err != nil {
		return nil, err
	}

	if len(errorChan) > 0 {
		return nil, <-errorChan
	}

	return newFull, err
}

func detectManifestChanges(oldMoM *Manifest, tmpManifests []*Manifest, numWorkers int, ui UpdateInfo, c config) ([]*Manifest, error) {
	log.Info(log.Mixer, "Detecting manifest changes...")

	newManifests := []*Manifest{}
	taskCh := make(chan *Manifest)
	var wg sync.WaitGroup
	mux := &sync.Mutex{}

	if numWorkers <= 0 {
		numWorkers = runtime.NumCPU()
	}
	if numWorkers > len(tmpManifests) {
		numWorkers = len(tmpManifests)
	}

	wg.Add(numWorkers)
	errorCh := make(chan error, numWorkers)

	defer close(errorCh)

	taskRunner := func() {
		defer wg.Done()
		for manifest := range taskCh {
			var tErr error
			// Check for changed includes, changed or added or deleted files
			// must be done after subtractManifests because the oldM is a subtracted
			// manifest
			ver := getManifestVerFromMoM(oldMoM, manifest)
			if ver == 0 {
				ver = ui.previous
			}

			oldMPath := filepath.Join(c.outputDir, fmt.Sprint(ver), "Manifest."+manifest.Name)
			oldM, err := getOldManifest(oldMPath)
			if err != nil {
				errorCh <- tErr
				return
			}
			changedIncludes := includesChanged(manifest, oldM)
			oldM.sortFilesName()
			changedFiles, added, deleted := manifest.linkPeersAndChange(oldM, ui.minVersion)
			// if nothing changed, skip
			if changedFiles == 0 && added == 0 && deleted == 0 && !changedIncludes {
				continue
			}

			// detect modifier flag for all files in the manifest
			// must happen after finding newDeleted files to catch ghosted files.
			manifest.applyHeuristics()
			// Assign final FileCount based on the files that made it this far
			manifest.Header.FileCount = uint32(len(manifest.Files))
			// If we made it this far, this bundle has a change and should be written
			mux.Lock()
			newManifests = append(newManifests, manifest)
			mux.Unlock()
		}
	}

	for i := 0; i < numWorkers; i++ {
		go taskRunner()
	}
	var err error
	for _, tmpMan := range tmpManifests {
		select {
		case taskCh <- tmpMan:
		case err = <-errorCh:
			// break as soon as there is a failure.
			break
		}
		if err != nil {
			break
		}
	}
	close(taskCh)
	wg.Wait()
	if err != nil {
		return nil, err
	}
	// Sending loop might finish before any goroutine could send an error back, so check for
	// error again after they are all done.
	if len(errorCh) > 0 {
		return nil, <-errorCh
	}
	return newManifests, nil
}

func processBundles(ui UpdateInfo, c config, numWorkers int, oldMoM *Manifest) ([]*Manifest, error) {
	var newFull *Manifest
	var err error
	// initialize bundles with with their info files
	tmpManifests, err := initBundles(ui, c, numWorkers)
	if err != nil {
		return nil, err
	}

	//read includes from bundleinfo files
	newFull, err = readIncludes(tmpManifests, numWorkers)
	if err != nil {
		return nil, err
	}
	// Add manifest file records. Important this is done after all includes
	// have been read so nested subtraction works.
	log.Info(log.Mixer, "Adding manifest file records...")
	if err = addAllManifestFiles(tmpManifests, ui, c, numWorkers); err != nil {
		return nil, err
	}

	// final loop detects changes, applies heuristics to files, and sorts the file lists
	newManifests, err := detectManifestChanges(oldMoM, tmpManifests, numWorkers, ui, c)
	if err != nil {
		return nil, err
	}
	// maximize full manifest while all the manifests are still sorted by name
	maximizeFull(newFull, newManifests)

	return newManifests, nil
}

func addUnchangedManifests(appendTo *Manifest, appendFrom *Manifest, bundles []string) {
	for _, f := range appendFrom.Files {
		if f.findFileNameInSlice(appendTo.Files) != nil {
			continue
		}

		if f.Name == IndexBundle {
			// this is generated new each time
			continue
		}

		for _, bundle := range bundles {
			if f.Name == bundle {
				appendTo.Files = append(appendTo.Files, f)
				break
			}
		}
	}
}

// writeBundleManifests writes all bundle manifests in newManifests,
// populates the MoM, and returns the full manifest for this update.
func (MoM *Manifest) writeBundleManifests(newManifests []*Manifest, out string) (*Manifest, error) {
	var newFull *Manifest
	var err error
	// write manifests then add them to the MoM
	for _, bMan := range newManifests {
		if bMan.Name == "full" {
			// record full manifest to return it from this function
			newFull = bMan
			continue
		}

		// this sets maximum contentsize to the incorrect maximum set in
		// swupd-client v3.15.3 if the manifest is in the format where the
		// bug was introduced.
		bMan.setMaxContentSizeForFormat()

		// sort by version then by filename, previously to this sort these bundles
		// were sorted by file name only to make processing easier
		bMan.sortFilesVersionName()
		manPath := filepath.Join(out, "Manifest."+bMan.Name)
		if err = bMan.WriteManifestFile(manPath); err != nil {
			return nil, err
		}

		// add bundle to Manifest.MoM
		if err = MoM.createManifestRecord(out, manPath, MoM.Header.Version, TypeManifest, bMan.BundleInfo.Header.Status); err != nil {
			return nil, err
		}
	}

	return newFull, nil
}

// CreateManifests creates update manifests for changed and added bundles for <version>
func CreateManifests(version, previous, minVersion uint32, format uint, statedir string, numWorkers int) (*MoM, error) {
	var err error
	var c config

	if minVersion > version {
		return nil, fmt.Errorf("minVersion (%v), must be between 0 and %v (inclusive)",
			minVersion, version)
	}

	c, err = getConfig(statedir)
	if err != nil {
		log.Warning(log.Mixer, "Found server.ini, but was unable to read it. "+
			"Continuing with default configuration")
	}

	if err = initBuildEnv(c); err != nil {
		return nil, err
	}

	var groups []string
	if groups, err = readGroupsINI(filepath.Join(c.stateDir, "groups.ini")); err != nil {
		return nil, err
	}

	groups = append(groups, "full")

	timeStamp := time.Now()
	oldMoMPath := filepath.Join(c.outputDir, fmt.Sprint(previous), "Manifest.MoM")
	oldMoM, err := getOldManifest(oldMoMPath)
	if err != nil {
		return nil, err
	}
	oldFormat := oldMoM.Header.Format

	// PROCESS BUNDLES
	ui := UpdateInfo{
		oldFormat:  oldFormat,
		format:     format,
		previous:   previous,
		minVersion: minVersion,
		version:    version,
		bundles:    groups,
		timeStamp:  timeStamp,
	}
	var newManifests []*Manifest
	if newManifests, err = processBundles(ui, c, numWorkers, oldMoM); err != nil {
		return nil, err
	}

	verOutput := filepath.Join(c.outputDir, fmt.Sprint(version))
	if err = os.MkdirAll(verOutput, 0755); err != nil {
		return nil, err
	}

	// Bootstrap delta directory, so we can assume every version will have one.
	if err = os.MkdirAll(filepath.Join(verOutput, "delta"), 0755); err != nil {
		return nil, err
	}

	newMoM := Manifest{
		Name: "MoM",
		Header: ManifestHeader{
			Format:     format,
			Version:    version,
			MinVersion: minVersion,
			Previous:   previous,
			TimeStamp:  timeStamp,
		},
	}
	// if min-version wasn't explicitly set we need to carry the header forward
	// from the old MoM
	if newMoM.Header.MinVersion == 0 && oldMoM.Header.MinVersion > 0 {
		newMoM.Header.MinVersion = oldMoM.Header.MinVersion
	}

	log.Info(log.Mixer, "Writing manifest files...")
	newFull, err := newMoM.writeBundleManifests(newManifests, verOutput)
	if err != nil {
		return nil, err
	}

	// copy over unchanged manifests
	addUnchangedManifests(&newMoM, oldMoM, groups)

	var osIdx *Manifest
	if osIdx, err = writeIndexManifest(&c, &ui, oldMoM, newManifests); err != nil {
		return nil, err
	}

	if osIdx != nil {
		// read index manifest from the version directory specified in the manifest
		// itself as an index manifest may not have been created for this version.
		osIdxDir := filepath.Join(c.outputDir, fmt.Sprint(osIdx.Header.Version))
		osIdxPath := filepath.Join(osIdxDir, "Manifest."+osIdx.Name)
		if err = newMoM.createManifestRecord(osIdxDir, osIdxPath, osIdx.Header.Version, TypeManifest, osIdx.BundleInfo.Header.Status); err != nil {
			return nil, err
		}

		// track here as well so the manifest tar is made
		// but only if we made it new for this version
		if osIdx.Header.Version == newMoM.Header.Version {
			newManifests = append(newManifests, osIdx)
		}
	} else {
		// Carry old index if present
		for _, f := range oldMoM.Files {
			if f.Name == IndexBundle {
				newMoM.Files = append(newMoM.Files, f)
				break
			}
		}
	}

	// handle full manifest
	newFull.sortFilesVersionName()
	if err = newFull.WriteManifestFile(filepath.Join(verOutput, "Manifest.full")); err != nil {
		return nil, err
	}

	newMoM.Header.FileCount = uint32(len(newMoM.Files))
	newMoM.sortFilesVersionName()

	// write MoM
	if err = newMoM.WriteManifestFile(filepath.Join(verOutput, "Manifest.MoM")); err != nil {
		return nil, err
	}

	// Make the result MoM struct to return.
	result := &MoM{
		Manifest:       newMoM,
		UpdatedBundles: make([]*Manifest, 0, len(newManifests)),
	}

	result.FullManifest = newFull
	for _, b := range newManifests {
		if b.Name != "full" {
			result.UpdatedBundles = append(result.UpdatedBundles, b)
		}
	}
	return result, nil
}
