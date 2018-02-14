package builder

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	validBundleNameRegex   = regexp.MustCompile(`^[A-Za-z0-9-_]+$`)
	validPackageNameRegex  = regexp.MustCompile(`^[A-Za-z0-9-_+.]+$`)
	bundleHeaderFieldRegex = regexp.MustCompile(`^# \[([A-Z]+)\]:\s*(.*)$`)
)

type bundleHeader struct {
	Title        string
	Description  string
	Status       string
	Capabilities string
	Maintainer   string
}

type bundle struct {
	Name     string
	Filename string
	Header   bundleHeader

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

// ValidationLevel represents a specific validation level
type ValidationLevel int

// Enum of available validation levels
const (
	BasicValidation ValidationLevel = iota
	StrictValidation
)

func validateBundleFile(filename string, lvl ValidationLevel) error {
	var errText string

	name := filepath.Base(filename)
	if err := validateBundleName(name); err != nil {
		errText = err.Error() + fmt.Sprintf(" derived from file %s\n", filename)
	}

	b, err := parseBundleFile(filename)
	if err != nil {
		errText += err.Error()
		return errors.New(errText)
	}

	if name != b.Header.Title {
		errText += fmt.Sprintf("Bundle name %q and bundle header Title %q do not match\n", name, b.Header.Title)
	}

	err = validateBundle(b, lvl)
	if err != nil {
		errText += err.Error() + "\n"
	}

	if errText != "" {
		return errors.New(strings.TrimSuffix(errText, "\n"))
	}

	return nil
}

func validateBundleName(name string) error {
	if !validBundleNameRegex.MatchString(name) || name == "MoM" || name == "full" {
		return fmt.Errorf("Invalid bundle name %q", name)
	}

	return nil
}

func validateBundleFileName(name string, b *bundle) error {
	var errText string

	if err := validateBundleName(name); err != nil {
		errText = err.Error() + fmt.Sprintf(" derived from file %s\n", name)
	}

	if name != b.Header.Title {
		errText += fmt.Sprintf("Bundle name %q and bundle header Title %q do not match\n", name, b.Header.Title)
	}

	if errText != "" {
		return errors.New(strings.TrimSuffix(errText, "\n"))
	}

	return nil
}

func validateBundle(b *bundle, lvl ValidationLevel) error {
	var errText string

	// Basic validation
	if err := validateBundleName(b.Header.Title); err != nil {
		errText = err.Error() + " in bundle header Title"
	}
	if lvl == BasicValidation {
		if errText != "" {
			return errors.New(strings.TrimSuffix(errText, "\n"))
		}
		return nil
	}

	// Strict validation
	if b.Header.Description == "" {
		errText += "Empty Description in bundle header\n"
	}
	if b.Header.Maintainer == "" {
		errText += "Empty Maintainer in bundle header\n"
	}
	if b.Header.Status == "" {
		errText += "Empty Status in bundle header\n"
	}
	if b.Header.Capabilities == "" {
		errText += "Empty Capabilities in bundle header\n"
	}
	if errText != "" {
		return errors.New(strings.TrimSuffix(errText, "\n"))
	}

	return nil
}

// parseBundleFile parses a bundle file identified by full filepath, and returns
// a bundle object representation of that bundle.
// Note: the Allpackages field of the bundles are left blank, as they cannot
// be calculated in isolation.
func parseBundleFile(filename string) (*bundle, error) {
	name := filepath.Base(filename)

	contents, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	bundle, err := parseBundle(contents)
	if err != nil {
		return nil, fmt.Errorf("Couldn't parse bundle file %s: %s", filename, err)
	}

	bundle.Name = name
	bundle.Filename = filename

	return bundle, nil
}

// parseBundle parses the bytes of a bundle file, ignoring comments and
// processing "include()" directives the same way that m4 works.
func parseBundle(contents []byte) (*bundle, error) {
	scanner := bufio.NewScanner(bytes.NewReader(contents))

	var b bundle
	var includes, packages []string

	line := 0
	for scanner.Scan() {
		line++
		text := scanner.Text()
		comment := strings.Index(text, "#")
		if comment > -1 {
			if matches := bundleHeaderFieldRegex.FindStringSubmatch(text); len(matches) > 2 {
				key := matches[1]
				value := strings.TrimSpace(matches[2])
				switch key {
				case "TITLE":
					b.Header.Title = value
				case "DESCRIPTION":
					b.Header.Description = value
				case "STATUS":
					b.Header.Status = value
				case "CAPABILITIES":
					b.Header.Capabilities = value
				case "MAINTAINER":
					b.Header.Maintainer = value
				}
				continue
			}
			text = text[:comment]
		}
		text = strings.TrimSpace(text)
		if len(text) == 0 {
			continue
		}
		if strings.HasPrefix(text, "include(") {
			if !strings.HasSuffix(text, ")") {
				return nil, fmt.Errorf("Missing end parenthesis in line %d: %q", line, text)
			}
			text = text[8 : len(text)-1]
			if !validBundleNameRegex.MatchString(text) {
				return nil, fmt.Errorf("Invalid bundle name %q in line %d", text, line)
			}
			includes = append(includes, text)
		} else {
			if !validPackageNameRegex.MatchString(text) {
				return nil, fmt.Errorf("Invalid package name %q in line %d", text, line)
			}
			packages = append(packages, text)
		}
	}

	if scanner.Err() != nil {
		return nil, scanner.Err()
	}

	b.DirectIncludes = includes
	b.DirectPackages = packages

	return &b, nil
}
