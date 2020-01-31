package builder

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/clearlinux/mixer-tools/swupd"
)

var (
	validBundleNameRegex   = regexp.MustCompile(`^[A-Za-z0-9-_]+$`)
	validPackageNameRegex  = regexp.MustCompile(`^[A-Za-z0-9-_+.]+$`)
	bundleHeaderFieldRegex = regexp.MustCompile(`^# \[([A-Z]+)\]:\s*(.*)$`)
)

type bundle struct {
	Name             string
	Filename         string
	Header           swupd.BundleHeader
	DirectIncludes   []string
	OptionalIncludes []string
	DirectPackages   map[string]bool
	AllPackages      map[string]bool

	Files map[string]bool

	/* hidden property, not to be included in file usr/share/clear/allbundles */
	AllRpms        map[string]packageMetadata `json:"-"`
	ContentChroots map[string]bool            `json:"-"`
}

type bundleSet map[string]*bundle

// validateAndFillBundleSet will validate a bundleSet and fill in the Allpackage
// fields in each bundle. Specifically, it will validate that a bundleSet meets
// the following constraints:
// 1) Completeness. For each bundle in the set, every bundle included by that
//    bundle is also in the set.
// 2) Cycle-Free. The set contains no bundle include cycles.
func validateAndFillBundleSet(bundles bundleSet) error {
	// Sort the bundles so that all includes and optional (also-add) includes appear
	// before a bundle, then calculate AllPackages for each bundle.
	// Cycles and missing bundles are identified as part of sorting the bundles.
	sortedBundles, err := sortBundles(bundles)
	if err != nil {
		return err
	}
	for _, b := range sortedBundles {
		b.AllPackages = make(map[string]bool)
		for k, v := range b.DirectPackages {
			b.AllPackages[k] = v
		}
		for _, include := range b.DirectIncludes {
			for k, v := range bundles[include].AllPackages {
				b.AllPackages[k] = v
			}
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
			for _, opt := range b.OptionalIncludes {
				_, exists := bundles[opt]
				if !exists {
					return fmt.Errorf("bundle %q includes optional (also-add) bundle %q which is not available", b.Name, opt)
				}
				// we don't need to visit optional bundles, they will not be used to determine
				// the contents of the manifest that includes them and since they are an
				// independent bundles, they will be validated on their own in a separate step
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

	// Basic Validation
	if err := validateBundleFilename(filename); err != nil {
		errText = err.Error() + "\n"
	}

	b, err := parseBundleFile(filename)
	if err != nil {
		errText += err.Error() + "\n"
	}

	if lvl == BasicValidation {
		if errText != "" {
			return errors.New(strings.TrimSuffix(errText, "\n"))
		}
		return nil
	}

	// Strict Validation
	err = validateBundle(b)
	if err != nil {
		errText += err.Error() + "\n"
	}

	name := filepath.Base(filename)
	if name != b.Header.Title {
		errText += fmt.Sprintf("Bundle name %q and bundle header Title %q do not match\n", name, b.Header.Title)
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

func validateBundleFilename(filename string) error {
	name := filepath.Base(filename)
	if err := validateBundleName(name); err != nil {
		return fmt.Errorf("Invalid bundle name %q derived from file %q", name, filename)
	}

	return nil
}

func validateBundle(b *bundle) error {
	var errText string

	if err := validateBundleName(b.Header.Title); err != nil {
		errText = fmt.Sprintf("Invalid bundle name %q in bundle header Title\n", b.Header.Title)
	}
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

// parsePackageBundleFile parses a file at filename containing a flat list of package
// names to be converted to one-package bundles (pundles). Returns a map of package
// names found in the file.
func parsePackageBundleFile(filename string) (map[string]bool, error) {
	contents, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	return parsePackageBundle(contents), nil
}

func parsePackageBundle(contents []byte) map[string]bool {
	packages := make(map[string]bool)
	scanner := bufio.NewScanner(bytes.NewReader(contents))

	for scanner.Scan() {
		p := scanner.Text()

		// Drop comments.
		comment := strings.Index(p, "#")
		if comment > -1 {
			p = p[:comment]
		}

		p = strings.TrimSpace(p)

		// Skip empty lines.
		if len(p) == 0 {
			continue
		}

		packages[p] = true
	}

	return packages
}

// newBundleFromPackage creates a new bundle from a package name p
func newBundleFromPackage(p, path string) (*bundle, error) {
	if !validPackageNameRegex.MatchString(p) {
		return nil, fmt.Errorf("Invalid package name %q", p)
	}

	if _, ok := localPackages[p]; !ok {
		if _, ok = upstreamPackages[p]; !ok {
			return nil, fmt.Errorf("no package for %q", p)
		}
	}

	b := bundle{
		Name:           p,
		Filename:       path,
		DirectPackages: map[string]bool{p: true, "filesystem": true},
	}

	return &b, nil
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
	var duplicate bool
	var includes, packages, optional []string

	b.ContentChroots = make(map[string]bool)

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
		} else if strings.HasPrefix(text, "also-add(") {
			if !strings.HasSuffix(text, ")") {
				return nil, fmt.Errorf("Missing end parenthesis in line %d: %q", line, text)
			}
			text = text[9 : len(text)-1]
			if !validBundleNameRegex.MatchString(text) {
				return nil, fmt.Errorf("Invalid bundle name %q in line %d", text, line)
			}
			optional = append(optional, text)
		} else if strings.HasPrefix(text, "content(") {
			if !strings.HasSuffix(text, ")") {
				return nil, fmt.Errorf("Missing end parenthesis in line %d: %q", line, text)
			}
			text = text[8 : len(text)-1]

			// Remove unnecessary trailing slashes
			for len(text) > 1 && strings.HasSuffix(text, "/") {
				text = strings.TrimSuffix(text, "/")
			}

			if _, err := os.Stat(text); err != nil {
				return nil, fmt.Errorf("Invalid content path %q in line %d", text, line)
			}
			b.ContentChroots[text] = true
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

	// if a bundle was added to both includes and optional
	// remove it from the optional
	for _, optionalBundle := range optional {
		duplicate = false
		for _, includedBundle := range includes {
			if optionalBundle == includedBundle {
				duplicate = true
				break
			}
		}
		if !duplicate {
			b.OptionalIncludes = append(b.OptionalIncludes, optionalBundle)
		}
	}
	b.DirectIncludes = includes
	b.DirectPackages = make(map[string]bool)
	for _, p := range packages {
		b.DirectPackages[p] = true
	}

	return &b, nil
}

func isEmptyBundle(bundle *bundle) bool {
	if len(bundle.DirectIncludes) == 0 &&
		len(bundle.OptionalIncludes) == 0 &&
		len(bundle.DirectPackages) == 0 &&
		len(bundle.AllPackages) == 0 &&
		len(bundle.Files) == 0 {
		return true
	}
	return false
}
