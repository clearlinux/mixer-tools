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
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"

	"github.com/clearlinux/mixer-tools/log"
	"github.com/clearlinux/mixer-tools/swupd"
	"github.com/pkg/errors"
)

func createDeltaPacks(fromMoM *swupd.Manifest, toMoM *swupd.Manifest, printReport bool, outputDir, bundleDir string, numWorkers int) error {
	timer := &stopWatch{w: os.Stdout}
	defer timer.WriteSummary(os.Stdout)
	timer.Start("CREATE DELTA PACKS")
	log.Info(log.Mixer, "Using %d workers", numWorkers)

	log.Info(log.Mixer, "Creating delta packs from %d to %d", fromMoM.Header.Version, toMoM.Header.Version)
	bundlesToPack, err := swupd.FindBundlesToPack(fromMoM, toMoM)
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
			log.Info(log.Mixer, "  Delta pack already exists for %s from %d to %d", b.Name, b.FromVersion, b.ToVersion)
			// Remove so the goroutines don't try to make deltas for these
			delete(bundlesToPack, name)
			continue
		}
		if !os.IsNotExist(err) {
			return errors.Wrapf(err, "couldn't access existing pack file %s", packPath)
		}
	}

	if numWorkers < 1 {
		numWorkers = runtime.NumCPU()
	}

	var bundleQueue = make(chan *swupd.BundleToPack)
	var wg sync.WaitGroup
	wg.Add(numWorkers)

	// Delta creation takes a lot of memory, so create a limited amount of goroutines.
	for i := 0; i < numWorkers; i++ {
		go func() {
			defer wg.Done()
			for b := range bundleQueue {
				log.Info(log.Mixer, "  Creating delta pack for bundle %q from %d to %d", b.Name, b.FromVersion, b.ToVersion)
				info, err := swupd.CreatePack(b.Name, b.FromVersion, b.ToVersion, outputDir, bundleDir)
				if err != nil {
					log.Error(log.Mixer, "Pack %q from %d to %d FAILED to be created: %s", b.Name, b.FromVersion, b.ToVersion, err.Error())
					// Do not exit on errors, we have logging for all other failures and deltas are optional
					continue
				}

				if len(info.Warnings) > 0 {
					for _, w := range info.Warnings {
						log.Warning(log.Mixer, "    Bundle %s: %s", b.Name, w)
					}
				}
				report := fmt.Sprintf("  Finished delta pack for bundle %q from %d to %d\n", b.Name, b.FromVersion, b.ToVersion)
				if printReport {
					max := 0
					for _, e := range info.Entries {
						if len(e.File.Name) > max {
							max = len(e.File.Name)
						}
					}
					report += "    Pack report:\n"
					for _, e := range info.Entries {
						report += fmt.Sprintf("      %-*s %s (%s)\n", max, e.File.Name, e.State, e.Reason)
					}
					report += "\n"
				}
				report += fmt.Sprintf("    Fullfiles in pack: %d\n", info.FullfileCount)
				report += fmt.Sprintf("    Deltas in pack: %d\n", info.DeltaCount)
				log.Info(log.Mixer, report)
			}
		}()
	}
	// Send jobs to the queue for delta goroutines to pick up.
	for _, bundle := range bundlesToPack {
		bundleQueue <- bundle
	}
	// Send message that no more jobs are being sent
	close(bundleQueue)
	wg.Wait()

	timer.Stop()
	return nil
}
