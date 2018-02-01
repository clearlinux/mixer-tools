package swupd

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/clearlinux/mixer-tools/helpers"
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

// CreateDeltas creates all delta files between the previous and current version of the
// supplied manifest. Returns a list of deltas (which contains information about
// individual delta errors). Returns error (and no deltas) if it can't assemble the delta
// list.
func CreateDeltas(manifest, statedir string, from, to uint32) ([]Delta, error) {
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

	return createDeltasFromManifests(&c, oldManifest, newManifest)
}

func createDeltasFromManifests(c *config, oldManifest, newManifest *Manifest) ([]Delta, error) {
	deltas, err := findDeltas(c, oldManifest, newManifest)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to create deltas list %s", newManifest.Name)
	}

	const maxRoutines = 3
	var deltaQueue = make(chan *Delta)
	var wg sync.WaitGroup
	wg.Add(maxRoutines)

	// Don't flood the system with goroutines, delta creation takes up an
	// incredibly amount of memory, so we cannot max out on goroutines.
	for i := 0; i < maxRoutines; i++ {
		go func() {
			defer wg.Done()
			for delta := range deltaQueue {
				delta.Error = createDelta(c, delta)
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

func createDelta(c *config, delta *Delta) error {
	oldPath := filepath.Join(c.imageBase, fmt.Sprint(delta.from.Version), "full", delta.from.Name)
	newPath := filepath.Join(c.imageBase, fmt.Sprint(delta.to.Version), "full", delta.to.Name)
	deltaDir := filepath.Join(c.outputDir, fmt.Sprint(delta.to.Version), "delta")
	deltaName := fmt.Sprintf("%d-%d-%s-%s", delta.from.Version, delta.to.Version, delta.from.Hash, delta.to.Hash)
	delta.Path = filepath.Join(deltaDir, deltaName)

	if err := helpers.RunCommandSilent("bsdiff", oldPath, newPath, delta.Path); err != nil {
		_ = os.Remove(delta.Path) // Might have returned FULLDL
		return errors.Wrapf(err, "Failed to create delta for %s -> %s", oldPath, newPath)
	}

	// Check that the delta actually applies correctly.
	testPath := filepath.Join(deltaDir, "."+deltaName+".testnewfile")
	if err := helpers.RunCommandSilent("bspatch", oldPath, testPath, delta.Path); err != nil {
		return errors.Wrapf(err, "Failed to apply delta %s", delta.Path)
	}
	defer func() {
		_ = os.Remove(testPath)
	}()

	testHash, err := Hashcalc(testPath)
	if err != nil {
		return errors.Wrap(err, "Failed to calculate hash for test file created applying delta")
	}
	if testHash != delta.to.Hash {
		return errors.Wrapf(err, "Delta mismatch: %s -> %s via delta: %s", oldPath, newPath, delta.Path)
	}

	return nil
}

func findDeltas(c *config, oldManifest, newManifest *Manifest) ([]Delta, error) {
	oldManifest.sortFilesName()
	newManifest.sortFilesName()

	err := linkDeltaPeers(c, oldManifest, newManifest)
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
	for _, nf := range newManifest.Files {
		if nf.DeltaPeer == nil {
			continue
		}
		deltas = append(deltas, Delta{
			from: nf.DeltaPeer,
			to:   nf,
		})
	}

	return deltas, nil
}
