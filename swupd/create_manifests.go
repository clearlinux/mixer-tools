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
	"errors"
	"fmt"
	//"io/ioutil"
	"os"
	"path/filepath"
	"time"
)

var StateDir = "/var/lib/swupd"
var MinVersion = false

type Update struct {
	oldFormat   uint
	format      uint
	lastVersion uint32
	version     uint32
	bundles     []string
	timeStamp   time.Time
}

func initBuildEnv() error {
	tmpDir := filepath.Join(StateDir, "temp")
	// remove old directory
	if err := os.RemoveAll(tmpDir); err != nil {
		return err
	}

	// create new one
	return os.Mkdir(tmpDir, os.ModePerm)
}

// initBuildDirs creates the following directory structure
// StateDir/
//    image/
//        <version>/
//            <bundle1>/
//            ...
//    LAST_VER (file containing "0\n" for now)
func initBuildDirs(version uint32, groups []string) error {
	// what is this???
	//lastVer := []byte{'0', '\n'}
	//if err := ioutil.WriteFile(filepath.Join(imageBase, "LAST_VER"), lastVer, 0755); err != nil {
	//	return err
	//}

	verDir := filepath.Join(imageBase, fmt.Sprint(version))
	for _, bundle := range groups {
		if err := os.MkdirAll(filepath.Join(verDir, bundle), 0755); err != nil {
			return err
		}
	}

	return nil
}

func processBundles(update Update) ([]*Manifest, error) {
	newManifests := []*Manifest{}
	for _, bundle := range update.bundles {
		oldM := Manifest{}
		oldMPath := filepath.Join(outputDir, fmt.Sprint(update.lastVersion), "Manifest."+bundle)
		if err := oldM.ReadManifestFromFile(oldMPath); err != nil {
			// old manifest may not exist, continue
			//return newManifests, err
		}

		newM := Manifest{
			Header: ManifestHeader{
				Format:    update.format,
				Version:   update.version,
				Previous:  update.lastVersion,
				TimeStamp: update.timeStamp,
			},
			Name: bundle,
		}

		newMChroot := filepath.Join(imageBase, fmt.Sprint(update.version), newM.Name)
		if err := newM.addFilesFromChroot(newMChroot); err != nil {
			return newManifests, err
		}

		if update.oldFormat == update.format {
			newM.addDeleted(&oldM)
		}

		changedIncludes := compareIncludes(&newM, &oldM)
		changedFiles := newM.linkPeersAndChange(&oldM)
		added := newM.filesAdded(&oldM)
		deleted := newM.newDeleted(&oldM)
		if changedFiles == 0 && added == 0 && deleted == 0 && !changedIncludes {
			newM.Header.Version = oldM.Header.Version
			continue
		}

		// apply heuristics
		newM.applyHeuristics()

		// detect type changes
		// fail out here if a type change is detected since this is not yet supported in client
		if newM.hasTypeChanges() {
			return newManifests, errors.New("type changes not yet supported")
		}

		newManifests = append(newManifests, &newM)
	}

	for _, bundle := range newManifests {
		if bundle.Name != "os-core" && bundle.Name != "full" {
			// read in bundle includes
			if err := bundle.readIncludes(newManifests); err != nil {
				return newManifests, err
			}

			// subtract manifests
			bundle.subtractManifests(bundle)
		}
	}

	for _, bundle := range newManifests {
		// sort manifest by version (then by filename)
		// this must be done after subtractManifests has been done for all manifests
		// because subtractManifests sorts the file lists by filename alone
		bundle.sortFilesVersionName()
	}

	return newManifests, nil
}

func CreateManifests(version uint32, minVersion bool, format uint, statedir string) error {
	if statedir != "" {
		StateDir = statedir
	}

	if minVersion {
		MinVersion = true
	}

	var err error
	if err = initBuildEnv(); err != nil {
		return err
	}

	if err = readServerINI(filepath.Join(StateDir, "server.ini")); err != nil {
		return err
	}

	var groups []string
	if groups, err = readGroupsINI(filepath.Join(StateDir, "groups.ini")); err != nil {
		return err
	}

	groups = append(groups, "full")

	var lastVersion uint32
	lastVersion, err = readLastVerFile(filepath.Join(imageBase, "LAST_VER"))
	if err != nil {
		return err
	}

	if err = initBuildDirs(version, groups); err != nil {
		return err
	}

	oldFullManifest := Manifest{}
	oldFullManifestPath := filepath.Join(StateDir, fmt.Sprint(lastVersion), "Manifest.full")
	if err = oldFullManifest.ReadManifestFromFile(oldFullManifestPath); err != nil {
		// might not exist, so the empty manifest is fine
	}

	timeStamp := time.Now()
	newFullManifest := Manifest{
		Header: ManifestHeader{
			Format:    format,
			Version:   version,
			Previous:  lastVersion,
			TimeStamp: timeStamp,
		},
		Name: "full",
	}
	if err = createNewFullChroot(version, groups); err != nil {
		return err
	}

	newFullChroot := filepath.Join(imageBase, fmt.Sprint(version), "full")
	if err = newFullManifest.addFilesFromChroot(newFullChroot); err != nil {
		return err
	}

	oldMoM := Manifest{}
	oldMoMPath := filepath.Join(StateDir, fmt.Sprint(lastVersion), "Manifest.MoM")
	if err = oldMoM.ReadManifestFromFile(oldMoMPath); err != nil {
		// nothing here yet either
	}

	newMoM := Manifest{
		Name: "MoM",
		Header: ManifestHeader{
			Format:    format,
			Version:   version,
			Previous:  lastVersion,
			TimeStamp: timeStamp,
		},
	}

	oldFormat := oldMoM.Header.Format

	// PROCESS BUNDLES
	var newManifests []*Manifest
	update := Update{
		oldFormat:   oldFormat,
		format:      format,
		lastVersion: lastVersion,
		version:     version,
		bundles:     groups,
		timeStamp:   timeStamp,
	}
	if newManifests, err = processBundles(update); err != nil {
		return err
	}

	verOutput := filepath.Join(outputDir, fmt.Sprint(version))
	if err = os.MkdirAll(verOutput, 0755); err != nil {
		return err
	}

	for _, bMan := range newManifests {
		manPath := filepath.Join(verOutput, "Manifest."+bMan.Name)
		if err = bMan.WriteManifestFile(manPath); err != nil {
			return err
		}

		var fi os.FileInfo
		if fi, err = os.Lstat(manPath); err != nil {
			return err
		}

		if newMoM.createManifestRecord(verOutput, manPath, fi, version); err != nil {
			return err
		}
	}

	for _, m := range oldMoM.Files {
		if match := m.findFileNameInSlice(newMoM.Files); match != nil {
			newMoM.Files = append(newMoM.Files, match)
		}
	}

	newMoM.sortFilesVersionName()

	// write MoM
	if err = newMoM.WriteManifestFile(filepath.Join(verOutput, "Manifest.MoM")); err != nil {
		return err
	}

	// TODO: populate and write full
	// TODO: manifest tars?

	return nil
}
