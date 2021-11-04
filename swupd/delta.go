// Copyright 2018 Intel Corporation
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
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"strings"

	"github.com/clearlinux/mixer-tools/helpers"
	"github.com/clearlinux/mixer-tools/log"
	"github.com/pkg/errors"
)

const (
	// From swupd-server's include/swupd.h:
	//
	//     Approximately the smallest size of a pair of input files which differ by a
	//     single bit that bsdiff can produce a more compact deltafile. Files smaller
	//     than this are always marked as different. See the magic 200 value in the
	//     bsdiff/src/diff.c code.
	//
	minimumSizeToMakeDeltaInBytes = 200
)

// Delta represents a delta file between two other files. If Error is present, it
// indicates that the delta couldn't be created.
type Delta struct {
	Path  string
	Error error
	from  *File
	to    *File
}

// CreateDeltasForManifest creates all delta files between the previous and current version of the
// supplied manifest. Returns a list of deltas (which contains information about
// individual delta errors). Returns error (and no deltas) if it can't assemble the delta
// list. If number of workers is zero or less, 1 worker is used.
func CreateDeltasForManifest(manifest, statedir string, from, to uint32, numWorkers int) ([]Delta, error) {
	var c config

	c, err := getConfig(statedir)
	if err != nil {
		return nil, err
	}

	var oldManifest *Manifest
	var newManifest *Manifest

	if oldManifest, err = ParseManifestFile(filepath.Join(c.outputDir, fmt.Sprintf("%d", from), manifest)); err != nil {
		return nil, err
	}
	if newManifest, err = ParseManifestFile(filepath.Join(c.outputDir, fmt.Sprintf("%d", to), manifest)); err != nil {
		return nil, err
	}

	return createDeltasFromManifests(&c, oldManifest, newManifest, numWorkers)
}

func createDeltasFromManifests(c *config, oldManifest, newManifest *Manifest, numWorkers int) ([]Delta, error) {
	deltas, err := findDeltas(c, oldManifest, newManifest)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to create deltas list %s", newManifest.Name)
	}

	if len(deltas) == 0 {
		return []Delta{}, nil
	}

	if numWorkers < 1 {
		numWorkers = 1
	}
	var deltaQueue = make(chan *Delta)
	var wg sync.WaitGroup
	wg.Add(numWorkers)

	// Delta creation takes a lot of memory, so create a limited amount of goroutines.
	for i := 0; i < numWorkers; i++ {
		go func() {
			defer wg.Done()
			for delta := range deltaQueue {
				delta.Error = createFileDelta(c, delta)
			}
		}()
	}

	// Send jobs to the queue for delta goroutines to pick up.
	for i := range deltas {
		deltaQueue <- &deltas[i]
	}

	// Send message that no more jobs are being sent
	close(deltaQueue)
	wg.Wait()

	return deltas, nil
}

// CreateManifestDeltas creates the delta manifest files for manifests in the from and to version of the
// referenced MoMs. Returns a list of deltas containing information on errors encountered during the
// delta generation process or an error (and no deltas list) if it can't create the deltas.
func CreateManifestDeltas(statedir string, fromManifest, toManifest *Manifest, numWorkers int) ([]Delta, error) {
	var c config
	var err error
	c, err = getConfig(statedir)
	if err != nil {
		return nil, err
	}

	toManifest.sortFilesName()
	fromManifest.sortFilesName()

	var deltas []Delta
	i := 0
	j := 0
	toVersion := toManifest.Header.Version
	for i < len(fromManifest.Files) && j < len(toManifest.Files) {
		f1 := fromManifest.Files[i]
		f2 := toManifest.Files[j]
		// Only create deltas if the bundle updated in the current version
		if f2.Version != toVersion {
			i++
			j++
			continue
		}
		if f1.Name == f2.Name {
			// Manifests wouldn't have the same name but not the same type
			if f1.Type != TypeManifest {
				i++
				j++
				continue
			}
			// Don't create deltas for manifests that aren't changed in the current version
			if f1.Version == f2.Version {
				i++
				j++
				continue
			}
			// Manifests aren't deleted so don't need to check status
			deltas = append(deltas, Delta{
				Path: filepath.Join(statedir, "www", fmt.Sprintf("%d", toManifest.Header.Version),
					fmt.Sprintf("Manifest-%s-delta-from-%d", f1.Name, f1.Version)),
				from: f1,
				to:   f2,
			})
			i++
			j++
		} else if f1.Name < f2.Name {
			i++
		} else {
			j++
		}
	}

	if len(deltas) == 0 {
		return []Delta{}, nil
	}

	if numWorkers < 1 {
		numWorkers = 1
	}
	var deltaQueue = make(chan *Delta)
	var wg sync.WaitGroup
	wg.Add(numWorkers)

	// Delta creation takes a lot of memory, so create a limited amount of goroutines.
	for i := 0; i < numWorkers; i++ {
		go func() {
			defer wg.Done()
			for delta := range deltaQueue {
				delta.Error = createManifestDelta(&c, delta)
			}
		}()
	}

	// Send jobs to the queue for delta goroutines to pick up.
	for i := range deltas {
		deltaQueue <- &deltas[i]
	}

	// Send message that no more jobs are being sent
	close(deltaQueue)
	wg.Wait()

	return deltas, nil
}

// deltaTooLarge returns true if the delta file is larger than or equal in size
// to the compressed fullfile. This is not a critical check so any failures in
// the process just cause a false return.
func deltaTooLarge(c *config, delta *Delta, newPath string) bool {
	dInfo, err := os.Stat(delta.Path)
	if err != nil {
		return false
	}
	deltaSize := dInfo.Size()
	fHash, err := GetHashForFile(newPath)
	if err != nil {
		return false
	}
	fCompressed := filepath.Join(c.outputDir,
		fmt.Sprint(delta.to.Version),
		"files",
		fHash+".tar")
	fcInfo, err := os.Stat(fCompressed)
	if err != nil {
		return false
	}
	fcSize := fcInfo.Size()
	return deltaSize >= fcSize
}

func createFileDelta(c *config, delta *Delta) error {
	oldPath := filepath.Join(c.imageBase, fmt.Sprint(delta.from.Version), "full", delta.from.Name)
	newPath := filepath.Join(c.imageBase, fmt.Sprint(delta.to.Version), "full", delta.to.Name)

	return createDelta(c, oldPath, newPath, delta)
}

func createManifestDelta(c *config, delta *Delta) error {
	oldPath := filepath.Join(c.stateDir, "www", fmt.Sprint(delta.from.Version), "Manifest."+delta.from.Name)
	newPath := filepath.Join(c.stateDir, "www", fmt.Sprint(delta.to.Version), "Manifest."+delta.to.Name)

	return createDelta(c, oldPath, newPath, delta)
}

func createDelta(c *config, oldPath, newPath string, delta *Delta) error {
	if _, err := os.Stat(delta.Path); err == nil {
		// Skip existing deltas. Not verifying since client is resilient about that.
		return nil
	}

	// Set timeout to 8 minutes (480 seconds) for bsdiff.
	// The majority of all delta creations take significantly less than 8
	// minutes; the deltas that take longer usually indicate that old/new
	// files are large or very difficult to diff.
	if err := helpers.RunCommandTimeout(log.BsDiff, 480, "bsdiff", oldPath, newPath, delta.Path); err != nil {
		_ = os.Remove(delta.Path)
		if exitErr, ok := errors.Cause(err).(*exec.ExitError); ok {
			// bsdiff returns 1 that stands for "FULLDL", i.e. it decided that
			// a delta is not worth. Give a better error message for that case.
			if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				if status.ExitStatus() == 1 {
					err = fmt.Errorf("bsdiff returned FULLDL, not using delta %s (%d-%s) -> %s (%d-%s)", delta.from.Name, delta.from.Version, delta.from.Hash, delta.to.Name, delta.to.Version, delta.to.Hash)
					log.Debug(log.BsDiff, err.Error())
					return err
				}
			}
		}
		errStr := fmt.Sprintf("Failed to create delta for %s (%d-%s) -> %s (%d-%s)", delta.from.Name, delta.from.Version, delta.from.Hash, delta.to.Name, delta.to.Version, delta.to.Hash)
		err = errors.Wrap(err, errStr)
		log.Debug(log.BsDiff, err.Error())
		return err
	}

	// Check that delta is smaller than compressed full file
	if deltaTooLarge(c, delta, newPath) {
		_ = os.Remove(delta.Path)
		errStr := fmt.Sprintf("Delta file larger than compressed full file %s (%d-%s) -> %s", delta.to.Name, delta.to.Version, delta.to.Hash, newPath)
		log.Debug(log.BsDiff, errStr)
		return errors.New(errStr)
	}

	// Check that the delta actually applies correctly.
	testPath := delta.Path + ".testnewfile"
	if err := helpers.RunCommandSilent(log.BsDiff, "bspatch", oldPath, testPath, delta.Path); err != nil {
		_ = os.Remove(delta.Path)
		err = errors.Wrapf(err, "Failed to apply delta %s", delta.Path)
		log.Debug(log.BsPatch, err.Error())
		return err
	}
	defer func() {
		_ = os.Remove(testPath)
	}()

	testHash, err := Hashcalc(testPath)
	if err != nil {
		_ = os.Remove(delta.Path)
		err = errors.Wrap(err, "Failed to calculate hash for test file created applying delta")
		log.Debug(log.BsDiff, err.Error())
		return err
	}
	if testHash != delta.to.Hash {
		_ = os.Remove(delta.Path)
		err = errors.Errorf("Delta mismatch: %s -> %s via delta: %s", oldPath, newPath, delta.Path)
		log.Debug(log.BsDiff, err.Error())
		return err
	}
	return nil
}

func findDeltas(c *config, oldManifest, newManifest *Manifest) ([]Delta, error) {
	oldManifest.sortFilesName()
	newManifest.sortFilesName()

	err := linkDeltaPeersForPack(c, oldManifest, newManifest)
	if err != nil {
		return nil, err
	}

	deltaCount := 0
	for _, nf := range newManifest.Files {
		if nf.DeltaPeer != nil {
			deltaCount++
		}
	}

	deltas := make([]Delta, 0, deltaCount)

	// Use set to remove completely equal delta entries. These happen when two files that look
	// the same, change content in next version (but still look the same).
	seen := make(map[string]bool)

	for _, nf := range newManifest.Files {
		if nf.DeltaPeer == nil {
			continue
		}

		from := nf.DeltaPeer
		to := nf
		dir := filepath.Join(c.outputDir, fmt.Sprint(to.Version), "delta")
		name := fmt.Sprintf("%d-%d-%s-%s", from.Version, to.Version, from.Hash, to.Hash)
		path := filepath.Join(dir, name)

		if seen[path] {
			continue
		}

		if strings.Contains(to.Name, "/usr/bin/") {
			continue
		}
		if strings.Contains(to.Name, "/usr/lib64/") {
			continue
		}

		seen[path] = true
		deltas = append(deltas, Delta{
			Path: path,
			from: from,
			to:   to,
		})
	}

	return deltas, nil
}
