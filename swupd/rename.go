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
	"sort"
	"strings"
)

func renameDetection(manifest *Manifest, added []*File, removed []*File, c config) error {
	if len(added) == 0 || len(removed) == 0 {
		return nil // nothing to rename
	}
	added = trimRenamed(added) // Make copies of input slices, tidy up whilst we are here
	removed = trimRenamed(removed)
	if err := fixupStatFields(removed, manifest, &c); err != nil {
		return err
	}
	if err := fixupStatFields(added, manifest, &c); err != nil {
		return err
	}
	// Handle pure renames first, don't need to worry about size. Should we skip zero size?
	// just add call to trimSmall if so
	// Sort by Hash. No need to have tiebreaker on name as the Manifest links by hash
	// so 2 identical files being renamed, e.g. python2.6/foo and python3.7/bar being
	// renamed to python2.7/foo and python3.7/bar have no concept which is the source for the
	// python2.7/foo. This decision might need to change if the way renames are shown in
	// the manifest changes.
	sort.Slice(added, func(i, j int) bool {
		return added[i].Hash < added[j].Hash
	})
	sort.Slice(removed, func(i, j int) bool {
		return removed[i].Hash < removed[j].Hash
	})
	for ax, rx := 0, 0; ax < len(added) && rx < len(removed); {
		af := added[ax]
		rf := removed[rx]
		switch {
		case af.Hash < rf.Hash:
			ax++
		case af.Hash > rf.Hash:
			rx++
		default: // Equal hash, so link
			linkRenamePair(af, rf)
			ax++
			rx++
		}
	}
	// Link things with the same names skipping digits
	// First remove small files (not worth sending diff) and files which have
	// exact match
	added, err := trimSmall(trimRenamed(added), minimumSizeToMakeDeltaInBytes) // Make it explicit we are doing two steps
	if err != nil {
		return err
	}
	removed, err = trimSmall(trimRenamed(removed), minimumSizeToMakeDeltaInBytes) // TODO. make it one pass.
	if err != nil {
		return err
	}
	//generate the pairs of *File and short name
	pa := makePairedNames(added)
	pr := makePairedNames(removed)
	// Merge where short names match
	for ax, rx := 0, 0; ax < len(pa) && rx < len(pr); {
		af := pa[ax]
		rf := pr[rx]
		switch {
		case af.partialName < rf.partialName:
			ax++
		case af.partialName > rf.partialName:
			rx++
		default: // Equal truncated name
			linkRenamePair(af.f, rf.f)
			ax++
			rx++
		}
	}
	return nil
}

// linkRenamePair links two files together
func linkRenamePair(renameTo, renameFrom *File) {
	renameTo.DeltaPeer = renameFrom
	renameFrom.DeltaPeer = renameTo
	renameTo.Misc = MiscRename
	renameFrom.Misc = MiscRename
}

// trimRenamed returns an slice which has had files with DeltaPeers purged
func trimRenamed(a []*File) []*File {
	r := make([]*File, 0, len(a))
	for _, f := range a {
		// Do not worry about ghosted files here, assume that they will
		// exist on the target system, and so can be used as the source for
		// renames. The classic ghosted files are the OS kernels and modules,
		// and it is helpful to be able to ship just the differences for them.
		// Worst case is that the files do not exist on the target system, in which
		// case the swupd-client will fall back to doing a full download.
		//
		// Renames are only supported for TypeFile right now so skip if the file is
		// not the right type.
		//
		// Unfortunately deleted files have lost their association with a file type
		// so we have to leave all deleted files in the list
		// TODO: refactor to leave file type in the deleted file records. There is no
		// reason to clear them.
		if f.DeltaPeer == nil && (!f.Present() || f.Type == TypeFile) {
			r = append(r, f)
		}
	}
	return r
}

// trimsmall returns an slice which has had files with small files removed
func trimSmall(a []*File, minsize int64) ([]*File, error) {
	r := make([]*File, 0, len(a))
	for _, f := range a {
		if f.Info == nil {
			err := fmt.Errorf("Missing f.Info for " + f.Name)
			return r, err
		}
		if f.Info.Size() > minsize {
			r = append(r, f)
		}
	}
	return r, nil
}

// pairednames holds a *File and another "name", which is used for various
// alterations of the filename for matching purposes. In particular removing
// digits so a new release of a shared library is matched to previous ones
type pairedNames struct {
	f           *File
	partialName string // Filename with digits removed
}

// stripVers is used with strings.Map to remove digits and '.' from filename
// because these characters are commonly used in versioned paths
func stripVers(r rune) rune {
	// strings.Map removes negative values
	switch {
	case r >= '0' && r <= '9':
		return -1
	case r == '.':
		return -1
	}
	return r
}

// makePairedNames returns `a` sorted by name, disregarding digits.
// The secondary sort key is the original name. This is mainly intended for
// cases like with python we have the same filename under both python3.6 and python2.7
// it would look weird to rename the old python2.7/file1 to python3.6/file2 and at
// the same time rename the old python3.6/file1 to python2.7/file2.
func makePairedNames(list []*File) []pairedNames {
	pairs := make([]pairedNames, len(list))
	for i, f := range list {
		pairs[i].f = f
		pairs[i].partialName = strings.Map(stripVers, f.Name)
	}
	sort.Slice(pairs, func(a, b int) bool {
		if pairs[a].partialName == pairs[b].partialName { // Same stripped name, sort on original name
			// mainly for python2.x vs 3.x
			return pairs[a].f.Name < pairs[b].f.Name
		}
		return pairs[a].partialName < pairs[b].partialName
	})
	return pairs
}

// fixupstatfields adds the missing stat fields
// construct the path to the old chroot by Joining the c.imageBase
// file.Previous, bundle name, and file.Name fields
// Note this is horrible
func fixupStatFields(needed []*File, m *Manifest, c *config) error {
	var bundleChroot string
	for i := range needed {
		if needed[i].Info != nil {
			continue
		}
		bundleChroot = filepath.Join(c.imageBase, fmt.Sprint(needed[i].Version), "full")
		path := filepath.Join(bundleChroot, needed[i].Name)
		fi, err := os.Lstat(path)
		if err != nil {
			return err
		}
		needed[i].Info = fi
	}
	return nil
}
