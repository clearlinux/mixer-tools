package swupd

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/clearlinux/mixer-tools/helpers"
	"github.com/pkg/errors"
)

// CreateDeltas creates all delta files between the previous and current
// version of the supplied manifest, returning a list of files that
// delta creation failed for.
func CreateDeltas(manifest, statedir string, from, to uint32) ([]*File, error) {
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

	// Must be sorted before passing into linkPeersAndChange()
	oldManifest.sortFilesName()
	newManifest.sortFilesName()

	_, _, _ = newManifest.linkPeersAndChange(oldManifest, from)

	deltas, err := consolidateDeltaFiles(newManifest, from, c)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to create deltas list %s", manifest)
	}

	const maxRoutines = 3
	var deltaQueue = make(chan *File)
	var failedChan = make(chan *File)
	var failedList = make(chan []*File)
	var wg sync.WaitGroup
	wg.Add(maxRoutines)

	// Start a collector for the failure cases
	go func() {
		var failed []*File
		// Drains the failedChan appropriately
		for f := range failedChan {
			failed = append(failed, f)
		}
		failedList <- failed // This list is what we return in the end
	}()

	// Don't flood the system with goroutines, delta creation takes up an
	// incredibly amount of memory, so we cannot max out on goroutines
	for i := 0; i < maxRoutines; i++ {
		go func(statedir string) {
			defer wg.Done()

			// Take in jobs from the queue and try to make a delta
			for f := range deltaQueue {
				err := createDelta(c, f, statedir, f.DeltaPeer.Version, f.DeltaPeer.Hash)
				if err != nil {
					failedChan <- f
				}
			}
		}(statedir)
	}

	// Send jobs to the queue for delta goroutines to pick up
	for _, file := range deltas {
		deltaQueue <- file
	}

	// Send message that no more jobs are being sent
	close(deltaQueue)
	wg.Wait()

	// Once wait finishes we can signal the collector that no more jobs exist
	close(failedChan)

	return <-failedList, nil
}

func consolidateDeltaFiles(manifest *Manifest, from uint32, c config) ([]*File, error) {
	var files []*File

	if manifest == nil {
		return nil, nil
	}

	for _, file := range manifest.Files {
		if file.Version <= from {
			continue
		}
		if file.DeltaPeer == nil {
			continue
		}
		if file.Type != TypeFile ||
			file.DeltaPeer.Type != TypeFile {
			continue
		}

		deltadir := filepath.Join(c.outputDir, fmt.Sprintf("%d", file.Version), "delta")
		fromToString := fmt.Sprintf("%d-%d-%s-%s", file.DeltaPeer.Version, file.Version, file.DeltaPeer.Hash, file.Hash)

		deltaFile := filepath.Join(deltadir, fromToString)

		// Only add to list if the delta file does not already exist on the system
		if _, err := os.Stat(deltaFile); os.IsNotExist(err) {
			// Only create deltas for files bigger than 200 bytes
			fullname := filepath.Join(c.imageBase, fmt.Sprintf("%d", file.Version), "full", file.Name)
			file.Info, _ = os.Stat(fullname)
			if file.Info.Size() > 200 {
				files = append(files, file)
			}
		}

	}

	return files, nil
}

func createDelta(c config, file *File, statedir string, fromVersion uint32, fromHash Hashval) error {
	// File without peers cannot have deltas
	if file.DeltaPeer == nil {
		return nil
	}

	// We only support deltas between two regular files for now
	// TODO: Support directory -> file in the future, this is a complex case
	if file.Type != TypeFile || file.DeltaPeer.Type != TypeFile {
		return nil
	}

	newfile := filepath.Join(c.imageBase, fmt.Sprintf("%d", file.Version), "full", file.Name)
	original := filepath.Join(c.imageBase, fmt.Sprintf("%d", fromVersion), "full", file.Name)

	deltadir := filepath.Join(c.outputDir, fmt.Sprintf("%d", file.Version), "delta")
	fromToString := fmt.Sprintf("%d-%d-%s-%s", fromVersion, file.Version, fromHash, file.Hash)

	// Files to create and validate deltas with
	deltafile := filepath.Join(deltadir, fromToString)
	testnewfile := filepath.Join(deltadir, "."+fromToString+".testnewfile")

	// Shell out to bsdiff for now...false = don't print to screen
	if err := helpers.RunCommandSilent("bsdiff", original, newfile, deltafile); err != nil {
		_ = os.Remove(deltafile) // Might have returned FULLDL
		return errors.Wrapf(err, "Failed to create delta for %s -> %s", original, newfile)
	}

	// Check that the delta actually applies correctly
	if err := helpers.RunCommandSilent("bspatch", original, testnewfile, deltafile); err != nil {
		return errors.Wrapf(err, "Failed to apply delta %s", deltafile)
	}

	deltahash, err := Hashcalc(testnewfile)
	if err != nil {
		return errors.Wrap(err, "Failed to calculate hash for new file")
	}

	if !HashEquals(deltahash, file.Hash) {
		return errors.Wrapf(err, "Delta mismatch: %s -> %s via delta: %s", original, newfile, deltafile)
	}

	_ = os.Remove(testnewfile)

	// add rename code here

	return nil
}
