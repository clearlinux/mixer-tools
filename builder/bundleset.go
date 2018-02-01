package builder

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	validBundleNameRegex  = regexp.MustCompile(`^[A-Za-z0-9-_]+$`)
	validPackageNameRegex = regexp.MustCompile(`^[A-Za-z0-9-_+.]+$`)
)

type bundle struct {
	Name     string
	Filename string

	DirectIncludes []string
	DirectPackages []string
	AllPackages    []string
}

type bundleSet map[string]*bundle

// validateAndFillBundleSet will validate a bundleSet and fill in the Allpackage
// fields in each bundle. Specifically, it will validate that a bundleSet meets
// the following constraints:
// 1) Completeness. For each bundle in the set, every bundle included by that
//    bundle is also in the set.
// 2) Cycle-Free. The set contains no bundle include cycles.
func validateAndFillBundleSet(bundles bundleSet) error {
	// Sort the bundles so that all includes appear before a bundle, then
	// calculate AllPackages for each bundle. Cycles and missing bundles are
	// identified as part of sorting the bundles.
	sortedBundles, err := sortBundles(bundles)
	if err != nil {
		return err
	}
	for _, b := range sortedBundles {
		var allPackages []string
		allPackages = append(allPackages, b.DirectPackages...)
		for _, include := range b.DirectIncludes {
			allPackages = append(allPackages, bundles[include].AllPackages...)
		}
		// Remove redundant packages.
		sort.Strings(allPackages)
		for i, p := range allPackages {
			if i != 0 && allPackages[i-1] == p {
				continue
			}
			b.AllPackages = append(b.AllPackages, p)
		}
	}

	return nil
}

// sortBundles sorts the bundles in a bundleSet to produce a slice of bundles
// such that all includes for a bundle appear before that bundle. sortBundles
// also detects include cycle errors and missing includes as a byproduct of
// sorting.
func sortBundles(bundles bundleSet) ([]*bundle, error) {
	// Traverse all the bundles, recursing to mark its included bundles as
	// visited before mark the bundle itself as visited. VISITING state is used
	// to identify cycles.
	type state int
	const (
		NotVisited state = iota
		Visiting
		Visited
	)
	mark := make(map[*bundle]state, len(bundles))
	sorted := make([]*bundle, 0, len(bundles))
	visiting := make([]string, 0, len(bundles)) // Used to produce nice error messages.

	var visit func(b *bundle) error
	visit = func(b *bundle) error {
		switch mark[b] {
		case Visiting:
			return fmt.Errorf("cycle found in bundles: %s -> %s", strings.Join(visiting, " -> "), b.Name)
		case NotVisited:
			mark[b] = Visiting
			visiting = append(visiting, b.Name)
			for _, inc := range b.DirectIncludes {
				bundle, exists := bundles[inc]
				if !exists {
					return fmt.Errorf("bundle %q includes bundle %q which is not available", b.Name, inc)
				}
				err := visit(bundle)
				if err != nil {
					return err
				}
			}
			visiting = visiting[:len(visiting)-1]
			mark[b] = Visited
			sorted = append(sorted, b)
		}
		return nil
	}

	for _, b := range bundles {
		err := visit(b)
		if err != nil {
			return nil, err
		}
	}

	return sorted, nil

}

// parseBundleFile parses a bundle file identified by full filepath, and returns
// a bundle object representation of that bundle.
// Note: the Allpackages field of the bundles are left blank, as they cannot
// be calculated in isolation.
func parseBundleFile(filename string) (*bundle, error) {
	name := filepath.Base(filename)
	if !validBundleNameRegex.MatchString(name) || name == "MoM" || name == "full" {
		return nil, fmt.Errorf("invalid bundle name %q derived from file %s", name, filename)
	}

	contents, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	includes, packages, err := parseBundle(contents)
	if err != nil {
		return nil, fmt.Errorf("couldn't parse bundle file %s: %s", filename, err)
	}

	b := &bundle{
		Name:           name,
		DirectIncludes: includes,
		DirectPackages: packages,
		Filename:       filename,
	}
	return b, nil
}

// parseBundle parses the bytes of a bundle file, ignoring comments and
// processing "include()" directives the same way that m4 works.
func parseBundle(contents []byte) (includes []string, packages []string, err error) {
	scanner := bufio.NewScanner(bytes.NewReader(contents))

	line := 0
	for scanner.Scan() {
		line++
		text := scanner.Text()
		comment := strings.Index(text, "#")
		if comment > -1 {
			text = text[:comment]
		}
		text = strings.TrimSpace(text)
		if len(text) == 0 {
			continue
		}
		if strings.HasPrefix(text, "include(") {
			if !strings.HasSuffix(text, ")") {
				return nil, nil, fmt.Errorf("missing end parenthesis in line %d: %q", line, text)
			}
			text = text[8 : len(text)-1]
			if !validBundleNameRegex.MatchString(text) {
				return nil, nil, fmt.Errorf("invalid bundle name %q in line %d", text, line)
			}
			includes = append(includes, text)
		} else {
			if !validPackageNameRegex.MatchString(text) {
				return nil, nil, fmt.Errorf("invalid package name %q in line %d", text, line)
			}
			packages = append(packages, text)
		}
	}

	if scanner.Err() != nil {
		return nil, nil, scanner.Err()
	}

	return includes, packages, nil
}
