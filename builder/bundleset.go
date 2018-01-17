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

type bundleSet struct {
	Bundles map[string]*bundle
}

// newBundleSet will take a list of files and create a bundle struct from the list of
// files, doing all the required validation and processing.
//
// It currently implements parsing of the bundle files, ignoring comments and processing
// "include()" directives the same way that m4 works.
func newBundleSet(bundleFiles []string) (*bundleSet, error) {
	bundles := make(map[string]*bundle, len(bundleFiles))

	// First parse each bundle file in isolation.
	for _, filename := range bundleFiles {
		name := filepath.Base(filename)
		b, exists := bundles[name]
		if exists {
			return nil, fmt.Errorf("bundle %q defined twice: in %s and %s", name, b.Filename, filename)
		}
		if !validBundleNameRegex.MatchString(name) {
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

		b = &bundle{
			Name:           name,
			DirectIncludes: includes,
			DirectPackages: packages,
			Filename:       filename,
		}
		bundles[name] = b
	}

	// Then check if there are missing includes.
	for _, b := range bundles {
		for _, include := range b.DirectIncludes {
			_, exists := bundles[include]
			if !exists {
				return nil, fmt.Errorf("bundle %q includes bundle %q which is not available", b.Name, include)
			}
		}
	}

	// And finally sort the bundles so that all includes appear before a bundle, then
	// calculate AllPackages for each bundle. Cycles are identified as part of sorting
	// the bundles.
	sortedBundles, err := sortBundles(bundles)
	if err != nil {
		return nil, err
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

	return &bundleSet{Bundles: bundles}, nil
}

func sortBundles(bundles map[string]*bundle) ([]*bundle, error) {
	// Traverse all the bundles, recursing to mark its included bundles as visited before mark
	// the bundle itself as visited. VISITING state is used to identify cycles.
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
				err := visit(bundles[inc])
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
