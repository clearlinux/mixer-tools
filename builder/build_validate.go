package builder

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/clearlinux/mixer-tools/helpers"
	"github.com/clearlinux/mixer-tools/swupd"
	"github.com/olekukonko/tablewriter"
	"github.com/pkg/errors"
)

type bundleStatus int

// MinMcaTableWidth is the minimum width for MCA statistics table
const MinMcaTableWidth = 80

const (
	unchanged bundleStatus = iota
	added
	removed
	modified
)

// Contains all results from MCA diff and lists of added/deleted/modified bundles
type mcaDiffResults struct {
	bundleDiff []*mcaBundleDiff

	addList []string
	delList []string
	modList []string
}

// MCA diff results for a bundle
type mcaBundleDiff struct {
	name          string
	status        bundleStatus
	minversion    bool
	pkgFileCounts map[string]*bundlePkgStats

	pkgFileDiffs diffLists
	manFileDiffs diffLists
	pkgDiffs     diffLists
}

// File counters for a package
type bundlePkgStats struct {
	add int
	del int
	mod int
}

// Lists of added, deleted, and modified packages or files
type diffLists struct {
	addList []string
	delList []string
	modList []string
}

// mcaBundleInfo contains manifest and package metadata necessary to perform an
// MCA diff on a bundle.
type mcaBundleInfo struct {
	size uint64

	subPkgs     map[string]bool
	subPkgFiles map[string]*fileInfo
	manFiles    map[string]*swupd.File
}

// mcaBundlePkgInfo contains MCA package and resolved file lists for a bundle
// before subtraction.
type mcaBundlePkgInfo struct {
	name     string
	allPkgs  map[string]*pkgInfo
	allFiles map[string]bool
}

// pkgInfo contains package metadata
type pkgInfo struct {
	name string
	uri  string

	files []*fileInfo
}

// fileInfo contains file metadata
type fileInfo struct {
	name  string
	size  string
	hash  string
	modes string
	links string
	user  string
	group string

	hashLen int
	pkg     string
}

// CheckManifestCorrectness validates that the changes in manifest files between
// two versions aligns to the corresponding RPM changes. Any mismatched files
// between the manifests and RPMs will be printed as errors. When there are no
// errors, package and file statistics for each modified bundle will be displayed.
func (b *Builder) CheckManifestCorrectness(fromVer, toVer, downloadRetries, tableWidth int, fromRepoURLOverrides, toRepoURLOverrides map[string]string) error {
	if fromVer < 0 || toVer < 0 {
		return fmt.Errorf("Negative version not supported")
	}

	if fromVer >= toVer {
		return fmt.Errorf("From version must be less than to version")
	}

	// Suppress Stdout so that it doesn't clutter the results
	stdOut := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	defer func() {
		os.Stdout = stdOut
	}()

	// Load initial repo map
	if err := b.ListRepos(); err != nil {
		return err
	}

	// Get manifest file lists and subtracted RPM pkg/file lists
	fromInfo, err := b.mcaInfo(fromVer, downloadRetries, fromRepoURLOverrides)
	if err != nil {
		return err
	}
	toInfo, err := b.mcaInfo(toVer, downloadRetries, toRepoURLOverrides)
	if err != nil {
		return err
	}

	// Diff from version's manifest file list and subtracted RPM pkg/file lists
	// against the to version's lists
	results, err := diffMcaInfo(fromInfo, toInfo)
	if err != nil {
		return err
	}

	// Compare the manifest file changes against the RPM file changes to determine
	// any errors.
	errorList, warningList, err := analyzeMcaResults(b, results, fromInfo, toInfo, fromVer, toVer)
	if err != nil {
		return err
	}

	// Re-enable Stdout for the results
	os.Stdout = stdOut

	// Display errors and package/file statistics
	err = printMcaResults(results, fromInfo, toInfo, fromVer, toVer, tableWidth, errorList, warningList)
	if err != nil {
		return err
	}

	return nil
}

// mcaInfo collects manifest/RPM metadata and uses it to create a list
// of manifest files, subtracted packages, and subtracted package files for each
// bundle in the provided version.
func (b *Builder) mcaInfo(version, downloadRetries int, repoURLOverrides map[string]string) (map[string]*mcaBundleInfo, error) {
	allBundleInfo := make(map[string]*mcaBundleInfo)

	// Get manifest info for valid bundle entries in the MoM.
	mInfo, err := b.mcaManInfo(version)
	if err != nil {
		return nil, err
	}

	// Download and collect metadata for all packages
	pInfo, err := b.mcaPkgInfo(mInfo, version, downloadRetries, repoURLOverrides)
	if err != nil {
		return nil, err
	}

	// Get manifest file lists, subtracted package lists, and subtracted package file lists
	for _, m := range mInfo {
		info := &mcaBundleInfo{
			manFiles:    make(map[string]*swupd.File),
			subPkgFiles: make(map[string]*fileInfo),
			subPkgs:     make(map[string]bool),
			size:        m.Header.ContentSize,
		}

		err = info.getManFiles(m)
		if err != nil {
			return nil, err
		}

		err = info.getSubPkgs(m, pInfo)
		if err != nil {
			return nil, err
		}

		err = info.getSubPkgFiles(m, pInfo)
		if err != nil {
			return nil, err
		}

		allBundleInfo[m.Name] = info
	}
	return allBundleInfo, nil
}

// mcaManInfo collects manifest metadata for each bundle listed in
// the provided version's MoM.
func (b *Builder) mcaManInfo(version int) ([]*swupd.Manifest, error) {
	manifests := []*swupd.Manifest{}

	updateDir := filepath.Join(b.Config.Builder.ServerStateDir, "www")
	versionDir := filepath.Join(updateDir, fmt.Sprint(version))
	momPath := filepath.Join(versionDir, "Manifest.MoM")

	mom, err := swupd.ParseManifestFile(momPath)
	if err != nil {
		return nil, err
	}

	// Get bundle info for each MoM entry
	for _, f := range mom.Files {
		// os-core-update-index and iterative manifests are not checked by MCA
		if f.Name == "os-core-update-index" {
			continue
		}

		manifestPath := filepath.Join(updateDir, fmt.Sprint(f.Version), "Manifest."+f.Name)
		manifest, err := swupd.ParseManifestFile(manifestPath)
		if err != nil {
			return nil, err
		}

		bundleInfoPath := filepath.Join(b.Config.Builder.ServerStateDir, "image", fmt.Sprint(version), manifest.Name+"-info")
		err = manifest.GetBundleInfo(b.Config.Builder.ServerStateDir, bundleInfoPath)
		if err != nil {
			return nil, err
		}

		manifests = append(manifests, manifest)
	}

	// Set included manifests which will be used by file/pkg subtraction
	for _, m := range manifests {
		if m.Name == "full" || m.Name == "os-core" {
			continue
		}
		if err = m.ReadIncludesFromBundleInfo(manifests); err != nil {
			return nil, err
		}
	}

	return manifests, nil
}

// mcaPkgInfo downloads and queries all packages for each bundle to collect
// file metadata that will be used to create subtracted package/file lists.
func (b *Builder) mcaPkgInfo(manifests []*swupd.Manifest, version, downloadRetries int, repoURLOverrides map[string]string) (map[string]*mcaBundlePkgInfo, error) {
	var dnfConf string

	// map of repositories and their URIs with resolved $releasever
	repoURIs := make(map[string]string)

	// Download RPMs from correct upstream version
	upstreamVer, err := b.getLocalUpstreamVersion(strconv.Itoa(version))
	if err != nil {
		return nil, err
	}

	if repoURLOverrides != nil {
		// Create tmp DNF conf with overridden baseurl values for specified repos
		tmpConf, err := ioutil.TempFile(os.TempDir(), "mixerTmpConf")
		if err != nil {
			return nil, err
		}
		defer func() {
			path := tmpConf.Name()
			_ = tmpConf.Close()
			_ = os.Remove(path)
		}()

		repoURIs, err = b.WriteRepoURLOverrides(tmpConf, repoURLOverrides)
		if err != nil {
			return nil, err
		}

		dnfConf = tmpConf.Name()
	} else {
		dnfConf = b.Config.Builder.DNFConf
		for repo, r := range b.repos {
			repoURIs[repo] = r.url
		}
	}

	for repo, url := range repoURIs {
		repoURIs[repo] = strings.ReplaceAll(url, "$releasever", upstreamVer)
	}

	packagerCmd := []string{
		"dnf",
		"-y",
		"--config=" + dnfConf,
		"--releasever=" + upstreamVer,
	}

	set := make(bundleSet)
	for _, m := range manifests {
		// DirectPackages is used to populate AllPackages because AllPackages contains
		// unnecessary packages from included bundles that would be subtracted away.
		set[m.Name] = &bundle{
			Name:        m.BundleInfo.Name,
			AllPackages: m.BundleInfo.DirectPackages,
		}
	}

	emptyDir, err := ioutil.TempDir("", "MixerEmptyDirForNoopInstall")
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = os.RemoveAll(emptyDir)
	}()

	// TODO: resolvePackages is a bottleneck that could be removed by updating the bundleinfo files
	// to contain resolved packages and associate them with their repo, version, and arch.

	// Resolve the package dependencies and collect the repo, version, and arch for each package
	repoPkgs, err := resolvePackages(b.NumBundleWorkers, set, packagerCmd, emptyDir)
	if err != nil {
		return nil, err
	}

	// Obtain necessary package metadata by querying each rpm
	return b.resolveBundlePkgInfos(manifests, repoPkgs, packagerCmd, repoURIs, downloadRetries)
}

func (b *Builder) resolveBundlePkgInfos(manifests []*swupd.Manifest, repoPkgs *sync.Map, packagerCmd []string, repoURIs map[string]string, downloadRetries int) (map[string]*mcaBundlePkgInfo, error) {
	pkgCh := make(chan *pkgInfo, b.NumBundleWorkers)
	errCh := make(chan error, b.NumBundleWorkers)
	var wg sync.WaitGroup

	pInfo := make(map[string]*mcaBundlePkgInfo)

	// Duplicate packages re-use the same *pkgInfo
	pkgInfoCache := make(map[string]*pkgInfo)

	pkgWorker := func() {
		wg.Add(1)
		defer wg.Done()

		for p := range pkgCh {
			var err error
			p.files, err = b.resolvePkgFiles(p, downloadRetries)
			if err != nil {
				errCh <- err
			}
		}
	}
	for i := 0; i < b.NumBundleWorkers; i++ {
		go pkgWorker()
	}

	// TODO: file/package subtraction could be integrated here so that they don't need to be
	// subtracted later.

	// Query file metadata for each package in repoPkgs and update the allPkgs field for each bundle.
	for _, m := range manifests {
		bundlePkgInfo := &mcaBundlePkgInfo{
			name:     m.Name,
			allFiles: make(map[string]bool),
			allPkgs:  make(map[string]*pkgInfo),
		}
		pInfo[m.Name] = bundlePkgInfo

		// repoPkgs is a map of bundles -> map of repos -> list of packageMetadata
		repo, ok := repoPkgs.Load(m.Name)
		if !ok {
			return nil, errors.Errorf("Failed to load bundle %s from map", m.Name)
		}

		for repo, r := range repo.(repoPkgMap) {
			resolveList := []packageMetadata{}
			for _, p := range r {
				pkg, ok := pkgInfoCache[p.name]
				if !ok {
					pkg = &pkgInfo{
						name: p.name,
					}
					pkgInfoCache[p.name] = pkg
					resolveList = append(resolveList, p)
				}
				bundlePkgInfo.allPkgs[p.name] = pkg
			}

			// TODO: resolveRpmURIs is a bottleneck when all files are local. Running this function with a long
			// list of packages is significantly faster than running many times with small lists.

			// Resolve the URI of each new rpm
			if err := resolveRpmURIs(resolveList, repo, repoURIs[repo], pkgInfoCache, packagerCmd); err != nil {
				return nil, err
			}

			// Query file metadata for each new package
			for _, p := range resolveList {
				pkg, ok := pkgInfoCache[p.name]
				if !ok {
					return nil, errors.Errorf("Failed to load pkg %s from map", p.name)
				}
				select {
				case pkgCh <- pkg:
				case err := <-errCh:
					return nil, err
				}
			}
		}
	}
	close(pkgCh)
	wg.Wait()

	if len(errCh) > 0 {
		return nil, <-errCh
	}

	// Create map of files in each bundle
	for _, bundlePkgInfo := range pInfo {
		for _, p := range bundlePkgInfo.allPkgs {
			for _, f := range p.files {
				bundlePkgInfo.allFiles[f.name] = true
			}
		}
	}

	return pInfo, nil
}

// resolvePkgFiles queries a package for a list of file metadata.
func (b *Builder) resolvePkgFiles(pkg *pkgInfo, downloadRetries int) ([]*fileInfo, error) {
	var err error
	var out *bytes.Buffer

	queryCmd := "[%{filenames}\a%{filesizes}\a%{filedigests}\a%{filemodes:perms}\a%{filelinktos}\a%{fileusername}\a%{filegroupname}\n]"
	rpmCmd := []string{"rpm", "-qp", "--qf=" + queryCmd}

	// Query RPM for file metadata lists
	args := merge(rpmCmd, pkg.uri)
	for attempts := 0; attempts <= downloadRetries; attempts++ {
		out, err = helpers.RunCommandOutputEnv(args[0], args[1:], []string{"LC_ALL=en_US.UTF-8"})
		if err == nil {
			break
		}
		if attempts == downloadRetries {
			return nil, err
		}
	}

	pkgFiles := []*fileInfo{}

	// Each line contains metadata for a single file in the package
	queryLines := strings.Split(out.String(), "\n")
	for i, line := range queryLines {
		// The last line has no entry
		if i == len(queryLines)-1 {
			continue
		}

		fileMetadata := strings.Split(line, "\a")
		path := fileMetadata[0]

		// Paths that are banned from manifests are skipped by MCA
		if isBannedPath(path) {
			continue
		}

		// Files with blacklisted characters are skipped
		if swupd.FilenameBlacklisted(filepath.Base(path)) {
			continue
		}

		// Directories are omitted from MCA because they may be missed from rpm output.
		mode := fileMetadata[3]
		if mode[:1] == "d" {
			continue
		}

		// Some Clear Linux packages install files with path components that are
		// symlinks. MCA must resolve file paths to align with the manifests.
		path = resolveFileName(path)

		pkgFile := &fileInfo{
			name:    path,
			size:    fileMetadata[1],
			hash:    fileMetadata[2],
			modes:   fileMetadata[3],
			links:   fileMetadata[4],
			user:    fileMetadata[5],
			group:   fileMetadata[6],
			hashLen: len(fileMetadata[2]),
			pkg:     pkg.name,
		}
		pkgFiles = append(pkgFiles, pkgFile)
	}
	return pkgFiles, nil
}

// resolveRpmURIs resolves the rpm URIs for the pkgList and updates the pkgInfoCache
func resolveRpmURIs(pkgList []packageMetadata, repo, baseURI string, pkgInfoCache map[string]*pkgInfo, packagerCmd []string) error {
	if len(pkgList) == 0 {
		return nil
	}

	// Query the relative location of the rpm within the repo and the package name.
	// The relative rpm location is appended to the baseURI to determine the uri of
	// the rpm and the package name is used as the pkgInfoCache key to update the map.
	queryCmd := "%{location}\a%{name}\n"
	queryStringRpm := merge(
		packagerCmd,
		"repoquery",
		"--quiet",
		"--repo",
		repo,
		"--qf",
		queryCmd,
	)
	for _, p := range pkgList {
		// Use NVRA (name-version-release.arch) to avoid ambiguous results with
		// multiple packages. The p.version field is formatted as version-release.
		rpmName := p.name + "-" + p.version + "." + p.arch
		queryStringRpm = append(queryStringRpm, rpmName)
	}

	outBuf, err := helpers.RunCommandOutputEnv(queryStringRpm[0], queryStringRpm[1:], []string{"LC_ALL=en_US.UTF-8"})
	if err != nil {
		return err
	}

	repoURI, err := url.Parse(baseURI)
	if err != nil {
		return err
	}

	// Each line contains the relative location of the rpm within the repository and
	// a key for the pkgInfoCache. These values are separated by '\a'.
	queryLines := strings.Split(outBuf.String(), "\n")
	for _, line := range queryLines {
		queryResults := strings.Split(line, "\a")
		if len(queryResults) != 2 {
			continue
		}

		// Append the relative rpm path to repoURI
		u := *repoURI
		u.Path = path.Join(u.Path, queryResults[0])

		key := queryResults[1]

		pkg, ok := pkgInfoCache[key]
		if !ok {
			return errors.Errorf("%s not in pkgInfoCache", key)
		}
		pkg.uri = u.String()
	}
	return nil
}

// getManFiles collects the manifest's file list
func (info *mcaBundleInfo) getManFiles(manifest *swupd.Manifest) error {
	for _, file := range manifest.Files {
		// Skip directories and deleted/ghosted files
		if file.Type == swupd.TypeDirectory ||
			file.Status == swupd.StatusDeleted ||
			file.Status == swupd.StatusGhosted {

			continue
		}

		info.manFiles[file.Name] = file
	}
	return nil
}

// getSubPkgs gets the bundle's subtracted packages list
func (info *mcaBundleInfo) getSubPkgs(manifest *swupd.Manifest, pInfo map[string]*mcaBundlePkgInfo) error {
	includes := manifest.GetRecursiveIncludes()

	for p := range pInfo[manifest.Name].allPkgs {
		isIncluded := false

		for _, inc := range includes {
			if pInfo[inc.Name].allPkgs[p] != nil {
				isIncluded = true
				break
			}
		}
		if !isIncluded {
			info.subPkgs[p] = true
		}
	}
	return nil
}

// getSubPkgFiles gets the subtracted files that were resolved from RPMs
func (info *mcaBundleInfo) getSubPkgFiles(manifest *swupd.Manifest, pInfo map[string]*mcaBundlePkgInfo) error {
	includes := manifest.GetRecursiveIncludes()

	// Collect all files from bundle's subtracted packages
	for p := range info.subPkgs {
		for _, f := range pInfo[manifest.Name].allPkgs[p].files {
			isIncluded := false

			for _, inc := range includes {
				if pInfo[inc.Name].allFiles[f.name] {
					isIncluded = true
					break
				}
			}
			if !isIncluded {
				info.subPkgFiles[f.name] = f
			}
		}
	}
	return nil
}

// diffMcaInfo calculates the manifest file, package, and resolved package file
// differences between two versions and captures metadata required by printMcaResults.
func diffMcaInfo(fromInfo, toInfo map[string]*mcaBundleInfo) (*mcaDiffResults, error) {
	results := &mcaDiffResults{}

	// Bundles in toInfo are either added, modified, or unchanged
	for bName := range toInfo {
		bundleDiff := &mcaBundleDiff{
			name:          bName,
			pkgFileCounts: make(map[string]*bundlePkgStats),
		}

		// Initialize packages file counts to 0
		for p := range toInfo[bName].subPkgs {
			bundleDiff.pkgFileCounts[p] = &bundlePkgStats{}
		}
		if fromInfo[bName] != nil {
			for p := range fromInfo[bName].subPkgs {
				if bundleDiff.pkgFileCounts[p] == nil {
					bundleDiff.pkgFileCounts[p] = &bundlePkgStats{}
				}
			}
		}

		// Compare bundle in from/to versions
		if fromInfo[bName] != nil {
			// Bundles match
			err := bundleDiff.diffBundles(bName, fromInfo, toInfo)
			if err != nil {
				return nil, err
			}

			if bundleDiff.status == modified {
				results.modList = append(results.modList, bName)
			}
		} else {
			// Bundle added
			err := bundleDiff.addBundle(bName, toInfo)
			if err != nil {
				return nil, err
			}

			results.addList = append(results.addList, bName)
		}

		results.bundleDiff = append(results.bundleDiff, bundleDiff)
	}

	for bName := range fromInfo {
		if toInfo[bName] == nil {
			// Bundle deleted
			bundleDiff := &mcaBundleDiff{
				name:   bName,
				status: removed,
			}

			results.delList = append(results.delList, bName)
			results.bundleDiff = append(results.bundleDiff, bundleDiff)
		}
	}
	return results, nil
}

// diffBundles calculates resolved package file, package, and manifest file
// diffs between two versions and updates package file statistics.
func (bundleDiff *mcaBundleDiff) diffBundles(bundle string, fromInfo, toInfo map[string]*mcaBundleInfo) error {
	// File changes caused by the md5 to sha256 hash calc transition are skipped
	skipMap := getSkippedFiles(fromInfo[bundle].subPkgFiles, toInfo[bundle].subPkgFiles)

	// Diff package files
	diffList := getFileDiffLists(fromInfo[bundle].subPkgFiles, toInfo[bundle].subPkgFiles, skipMap)
	bundleDiff.pkgFileDiffs = diffList

	// For every added, modified, or deleted file, update package file statistics
	for _, f := range diffList.addList {
		pkg := toInfo[bundle].subPkgFiles[f].pkg
		bundleDiff.pkgFileCounts[pkg].add++
	}
	for _, f := range diffList.modList {
		pkg := toInfo[bundle].subPkgFiles[f].pkg
		bundleDiff.pkgFileCounts[pkg].mod++
	}
	for _, f := range diffList.delList {
		pkg := fromInfo[bundle].subPkgFiles[f].pkg
		bundleDiff.pkgFileCounts[pkg].del++
	}

	// diff pkgs
	diffList = getPkgDiffLists(fromInfo[bundle].subPkgs, toInfo[bundle].subPkgs, bundleDiff)
	bundleDiff.pkgDiffs = diffList

	// diff manifest files
	isMinversion, diffList := getManFileDiffLists(fromInfo[bundle].manFiles, toInfo[bundle].manFiles, skipMap)
	bundleDiff.manFileDiffs = diffList

	// When there is a file change, the bundle is modified
	bundleDiff.minversion = isMinversion

	if isBundleMod(diffList) {
		bundleDiff.status = modified
	}

	return nil
}

// getSkippedFiles returns a map of files to skip for the MCA check. rpm 4.12 used
// md5 file hashes by default and rpm 4.14 switched the default to sha256. As a result,
// in build 31680, rpms started to use sha256 hashes instead of md5. Since MCA assumes
// that hash changes indicate content changes, rpm comparisons with different hash types
// are not possible without expanding the scope of MCA to verify content hashes.
func getSkippedFiles(fromFiles, toFiles map[string]*fileInfo) map[string]bool {
	skipMap := make(map[string]bool)
	for fName, toFile := range toFiles {
		// The os-release file is expected to change every build, so a change caused
		// by a different hashing algorithm will not cause an MCA error.
		if fName == "/usr/lib/os-release" {
			continue
		}

		// Flag comparisons between md5 hashes (len == 32) and sha256 hashes (len == 64),
		// so they can be skipped later. Symlinks have a hashLen of 0 and are not skipped.
		fromFile := fromFiles[fName]
		if fromFile != nil {
			if (fromFile.hashLen == 32 || fromFile.hashLen == 64) &&
				(toFile.hashLen == 32 || toFile.hashLen == 64) &&
				fromFile.hashLen != toFile.hashLen {

				skipMap[fName] = true
			}
		}
	}
	return skipMap
}

// isBundleMod returns true when a manifest diffList is changed
func isBundleMod(lists diffLists) bool {
	return (len(lists.addList) + len(lists.modList) + len(lists.delList)) != 0
}

// addBundle marks a bundle's resolved package files, packages, and manifest
// files as added.
func (bundleDiff *mcaBundleDiff) addBundle(bundle string, toInfo map[string]*mcaBundleInfo) error {
	fDiffList := diffLists{}
	pDiffList := diffLists{}
	mDiffList := diffLists{}

	// Add package files
	for _, file := range toInfo[bundle].subPkgFiles {
		pkg := file.pkg
		bundleDiff.pkgFileCounts[pkg].add++

		fDiffList.addList = append(fDiffList.addList, file.name)
	}
	bundleDiff.pkgFileDiffs = fDiffList

	// Add packages
	for p := range toInfo[bundle].subPkgs {
		pDiffList.addList = append(pDiffList.addList, p)
	}
	bundleDiff.pkgDiffs = pDiffList

	// Add manifest
	for f := range toInfo[bundle].manFiles {
		mDiffList.addList = append(mDiffList.addList, f)
	}
	bundleDiff.manFileDiffs = mDiffList

	// Mark bundle added
	bundleDiff.status = added

	return nil
}

func getFileDiffLists(fromFiles, toFiles map[string]*fileInfo, skipMap map[string]bool) diffLists {
	addList := []string{}
	delList := []string{}
	modList := []string{}

	for f := range toFiles {
		if skipMap[f] {
			continue
		}
		if fromFiles[f] != nil {
			// Match
			if isFileMod(fromFiles[f], toFiles[f]) {
				modList = append(modList, f)
			}
		} else {
			// Added file
			addList = append(addList, f)
		}
	}

	for f := range fromFiles {
		if toFiles[f] == nil {
			// Deleted file
			delList = append(delList, f)
		}
	}
	return diffLists{modList: modList, addList: addList, delList: delList}
}

func getPkgDiffLists(fromPkgs, toPkgs map[string]bool, bundleDiff *mcaBundleDiff) diffLists {
	addList := []string{}
	delList := []string{}
	modList := []string{}

	for p := range toPkgs {
		if fromPkgs[p] {
			// Match
			if isPkgMod(p, bundleDiff) {
				modList = append(modList, p)
			}
		} else {
			// Added pkg
			addList = append(addList, p)
		}
	}

	for p := range fromPkgs {
		if !toPkgs[p] {
			// Deleted pkg
			delList = append(delList, p)
		}
	}
	return diffLists{modList: modList, addList: addList, delList: delList}
}

func getManFileDiffLists(fromFiles, toFiles map[string]*swupd.File, skipMap map[string]bool) (bool, diffLists) {
	addList := []string{}
	delList := []string{}
	modList := []string{}
	minversion := false

	for f := range toFiles {
		if skipMap[f] {
			continue
		}
		if fromFiles[f] != nil {
			// Match
			if isManFileMod(fromFiles[f], toFiles[f]) {
				modList = append(modList, f)
			} else if !minversion {
				minversion = isMinversion(fromFiles[f], toFiles[f])
			}
		} else {
			// Added file
			addList = append(addList, f)
		}
	}

	for f := range fromFiles {
		if toFiles[f] == nil {
			// Deleted file
			delList = append(delList, f)
		}
	}
	return minversion, diffLists{modList: modList, addList: addList, delList: delList}
}

func isFileMod(from, to *fileInfo) bool {
	return ((from.hash != to.hash) ||
		(from.modes != to.modes) ||
		(from.links != to.links) ||
		(from.user != to.user) ||
		(from.group != to.group))
}

func isPkgMod(key string, bundleDiff *mcaBundleDiff) bool {
	// When file(s) associated with a package are modified, the package is modified
	return (bundleDiff.pkgFileCounts[key].add + bundleDiff.pkgFileCounts[key].mod + bundleDiff.pkgFileCounts[key].del) != 0
}

func isManFileMod(from, to *swupd.File) bool {
	return (from.Hash != to.Hash)
}

func isMinversion(from, to *swupd.File) bool {
	return ((from.Name == to.Name) &&
		(from.Hash == to.Hash) &&
		(from.Version != to.Version))
}

// analyzeMcaResults compares manifest file changes against package file changes.
// When there are inconsistencies between the manifest and package file lists,
// a slice of error strings is returned.
func analyzeMcaResults(builder *Builder, results *mcaDiffResults, fromInfo, toInfo map[string]*mcaBundleInfo, fromVer, toVer int) ([]string, []string, error) {
	var errorList = []string{}

	// Compare manifest files against package files. Inconsistencies are added to the errorList.
	for _, b := range results.bundleDiff {
		// Removed bundles will have no corresponding packages
		if b.status == removed {
			continue
		}

		// Typically, Mixer updates the /usr/lib/os-release file, so an error is generated
		// when it is unchanged. When comparing the +10 to the +20, this error will be
		// removed by removeMcaErrorExceptions.
		if b.name == "os-core" {
			releaseFileMod := false
			for _, file := range b.manFileDiffs.modList {
				if file == "/usr/lib/os-release" {
					releaseFileMod = true
					break
				}
			}
			if !releaseFileMod {
				errorList = append(errorList, "ERROR: /usr/lib/os-release is not modified in manifest 'os-core'\n")
			}
		}

		errorList = append(errorList, diffResultLists(b, toInfo[b.name].subPkgFiles, "added")...)
		errorList = append(errorList, diffResultLists(b, toInfo[b.name].subPkgFiles, "modified")...)

		// Added bundles will not exist in the fromInfo object and added bundles cannot have deleted files.
		if fromInfo[b.name] != nil {
			errorList = append(errorList, diffResultLists(b, fromInfo[b.name].subPkgFiles, "deleted")...)
		}
	}

	// Remove errors generated by special case manifest files that have no package
	// equivalent. Any missing special case files will generate additional errors.
	return checkMcaFbErrors(builder, errorList, fromVer, toVer)
}

// diffResultLists compares manifest files against package files and generates
// an error string when they don't match.
func diffResultLists(bundleDiff *mcaBundleDiff, info map[string]*fileInfo, mode string) []string {
	var pkgFiles []string
	var manFiles []string

	switch mode {
	case "added":
		pkgFiles = bundleDiff.pkgFileDiffs.addList
		manFiles = bundleDiff.manFileDiffs.addList
	case "modified":
		pkgFiles = bundleDiff.pkgFileDiffs.modList
		manFiles = bundleDiff.manFileDiffs.modList
	case "deleted":
		pkgFiles = bundleDiff.pkgFileDiffs.delList
		manFiles = bundleDiff.manFileDiffs.delList
	}

	sort.Strings(pkgFiles)
	sort.Strings(manFiles)

	var errorList = []string{}

	trackingFileFound := false
	trackingFile := "/usr/share/clear/bundles/" + bundleDiff.name

	// Compare manifest and package file lists. When they don't match, add an
	// error string to the error slice.
	i := 0
	j := 0
	for i < len(pkgFiles) || j < len(manFiles) {
		var fileComp int
		if i >= len(pkgFiles) {
			fileComp = 1
		} else if j >= len(manFiles) {
			fileComp = -1
		} else {
			fileComp = strings.Compare(pkgFiles[i], manFiles[j])
		}

		switch fileComp {
		case 1:
			// File in manifest list, but not package
			if manFiles[j] == trackingFile && mode == "added" && bundleDiff.status == added {
				// Each manifest should have a tracking file that will not be
				// included in a package
				trackingFileFound = true
			} else {
				errorMsg := fmt.Sprintf("ERROR: %s is %s in manifest '%s', but not in a package\n", manFiles[j], mode, bundleDiff.name)
				errorList = append(errorList, errorMsg)
			}
			j++
		case -1:
			// File in package list, but not manifest
			errorMsg := fmt.Sprintf("ERROR: %s is %s in package '%s', but not %s in manifest '%s'\n", pkgFiles[i], mode, info[pkgFiles[i]].pkg, mode, bundleDiff.name)
			errorList = append(errorList, errorMsg)
			i++
		case 0:
			i++
			j++
		}
	}

	// Create error when new bundle doesn't add a bundle tracking file.
	if !trackingFileFound && mode == "added" && bundleDiff.status == added {
		errorMsg := fmt.Sprintf("ERROR: %s is missing from manifest '%s'\n", trackingFile, bundleDiff.name)
		errorList = append(errorList, errorMsg)
	}

	return errorList
}

// checkMcaFbErrors checks for error messages generated by special case manifest
// files that are not modified by packages.
func checkMcaFbErrors(b *Builder, diffErrors []string, fromVer, toVer int) ([]string, []string, error) {
	var errorList = []string{}
	var warningList = []string{}

	fromPlus10, err := b.isPlus10Version(fromVer)
	if err != nil {
		return nil, nil, err
	}
	toPlus10, err := b.isPlus10Version(toVer)
	if err != nil {
		return nil, nil, err
	}

	formatMatch, err := b.checkFormatsMatch(fromVer, toVer)
	if err != nil {
		return nil, nil, err
	}

	// The release file should be modified with the exception of the +10 to +20
	// comparison. This value will be overridden when the file is unchanged.
	releaseFileMod := true

	// Special case files that can be modified in a manifest, but have no package
	// equivalent will generate false positive errors. Remove the false positive
	// errors and generate error/warning messages when expected special case errors
	// are missing.
	var versionFile, versionstampFile, formatFile bool
	for _, err := range diffErrors {
		switch err {
		// Mixer modifies the version field in the /usr/lib/os-release file, but the
		// file also exists in the filesystem package. When the filesystem package
		// is modified, the false positive error will not be generated. To verify that
		// os-release is modified when not comparing the +10 to the +20, the below error
		// message is tracked in addition to the false positives.
		case "ERROR: /usr/lib/os-release is not modified in manifest 'os-core'\n":
			releaseFileMod = false
		case "ERROR: /usr/lib/os-release is modified in manifest 'os-core', but not in a package\n":
			continue
		case "ERROR: /usr/share/clear/version is modified in manifest 'os-core', but not in a package\n":
			versionFile = true
		case "ERROR: /usr/share/clear/versionstamp is modified in manifest 'os-core', but not in a package\n":
			versionstampFile = true
		case "ERROR: /usr/share/defaults/swupd/format is modified in manifest 'os-core-update', but not in a package\n":
			if toPlus10 || !formatMatch {
				formatFile = true
			}
		default:
			errorList = append(errorList, err)
		}
	}
	if fromPlus10 {
		// A comparison between +10 -> +20 versions is the only valid case when no changes
		// are expected in os-core/os-core-update, but the +20 version cannot be detected since Mixer does
		// not track the first file in a format. As a result, assume this case is a +10 -> +20
		// comparison and print a warning message.
		if !releaseFileMod && !versionFile && !versionstampFile && !formatFile {
			warningList = append(warningList, "WARNING: If this is not a +10 to +20 comparison, expected file changes are missing from os-core/os-core-update\n")
			return errorList, warningList, nil
		}
		warningList = append(warningList, "WARNING: If this is a +10 to +20 comparison, os-core/os-core-update have file exception errors\n")
	}

	// When the comparison is not between +10 -> +20 versions, re-add an error when /usr/lib/os-release
	// is not modified
	if !releaseFileMod {
		errorList = append(errorList, "ERROR: /usr/lib/os-release is not modified in manifest 'os-core'\n")
	}
	if !versionFile {
		errorList = append(errorList, "ERROR: /usr/share/clear/version is not modified in manifest 'os-core'\n")
	}
	if !versionstampFile {
		errorList = append(errorList, "ERROR: /usr/share/clear/versionstamp is not modified in manifest 'os-core'\n")
	}

	if (toPlus10 || !formatMatch) && !formatFile {
		if fromPlus10 {
			warningList = append(warningList, "WARNING: If comparing +10 to a version across multiple format boundaries, the format file in 'os-core-update' must be modified\n")
		} else {
			errorList = append(errorList, "ERROR: /usr/share/defaults/swupd/format is not modified in manifest 'os-core-update'\n")
		}
	}

	return errorList, warningList, nil
}

// isPlus10Version determines whether the version is the last version in a format.
func (b *Builder) isPlus10Version(ver int) (bool, error) {
	verStr := strconv.Itoa(ver)
	format, err := b.getFormatForVersion(verStr)
	if err != nil {
		return false, err
	}

	latest, err := b.getLatestForFormat(format)
	if err != nil {
		return false, err
	}

	// +10 is last version in a completed format
	if format < b.State.Mix.Format && verStr == latest {
		return true, nil
	}

	return false, nil
}

// checkFormatsMatch determines whether two versions are in the same format.
func (b *Builder) checkFormatsMatch(fromVer, toVer int) (bool, error) {
	fromFormat, err := b.getFormatForVersion(strconv.Itoa(fromVer))
	if err != nil {
		return false, err
	}
	toFormat, err := b.getFormatForVersion(strconv.Itoa(toVer))
	if err != nil {
		return false, err
	}

	if fromFormat == toFormat {
		return true, nil
	}
	return false, nil
}

// printMcaResults displays any MCA errors and prints bundle diff statistics when there are no errors.
func printMcaResults(results *mcaDiffResults, fromInfo, toInfo map[string]*mcaBundleInfo, fromVer, toVer, tableWidth int, errorList, warningList []string) error {
	// Print any warnings
	for _, msg := range warningList {
		fmt.Fprintf(os.Stderr, "%s", msg)
	}
	fmt.Print("\n")

	// An overwhelming number of errors can be generated when this test
	// identifies a manifest bug, so limit the error output to 50.
	if len(errorList) > 0 {
		for i, err := range errorList {
			if i == 50 {
				fmt.Print("WARNING: Error reporting is limited to 50, so additional errors were skipped.\n")
				break
			}
			fmt.Print(err)
		}
		return fmt.Errorf("Manifest errors were identified")
	}

	fmt.Printf("** Summary: No errors detected in manifests\n\n")
	fmt.Printf("Stats for manifests, from build %d to %d\n\n", fromVer, toVer)

	// Print bundle counts and lists of added/deleted bundles.
	fmt.Printf("Added bundles: %d\n", len(results.addList))
	sort.Strings(results.addList)
	for _, b := range results.addList {
		fmt.Printf("  - %s\n", b)
	}

	fmt.Printf("Changed bundles: %d\n", len(results.modList))

	fmt.Printf("Deleted bundles: %d\n", len(results.delList))
	sort.Strings(results.delList)
	for _, b := range results.delList {
		fmt.Printf("  - %s\n", b)
	}

	// No bundle information to print
	if len(results.bundleDiff) == 0 {
		return nil
	}

	// Skip statistics table when tableWidth too small
	if tableWidth < MinMcaTableWidth {
		return nil
	}

	// The results table requires 7 characters to build 2 column table. The bundle
	// column character limit is set to 25% of the available characters and the changes
	// column is set to the remaining 75% of available characters.
	bundleWidth := (tableWidth - 7) / 4
	changesWidth := tableWidth - 7 - bundleWidth

	// Create table with separator rows after each bundle entry. The table's text wrapping
	// does not work well, so text wrapping is handled by the appendMcaTableEntry function.
	table := tablewriter.NewWriter(os.Stdout)
	table.SetRowLine(true)
	table.SetAutoWrapText(false)
	table.SetHeader([]string{"BUNDLE", "CHANGES"})

	sort.Slice(results.bundleDiff, func(i, j int) bool {
		return results.bundleDiff[i].name < results.bundleDiff[j].name
	})

	// Print statistics for each bundle
	for _, b := range results.bundleDiff {
		var entryLine string

		// Skip unchanged and deleted bundles
		if (b.status == unchanged || b.status == removed) && !b.minversion {
			continue
		}
		changeStr := appendMcaTableEntry("", "Summary:", changesWidth)

		if b.minversion {
			changeStr = appendMcaTableEntry(changeStr, "** Minversion bump detected", changesWidth)
		}

		// Print bundle sizes in MB and calculate bundle content size
		// change as percentage.
		toSize := float64(toInfo[b.name].size) / 1048576

		if fromInfo[b.name] == nil || fromInfo[b.name].size == 0 {
			// Print added bundle size
			entryLine = fmt.Sprintf("Size: %.1fMB", toSize)
			changeStr = appendMcaTableEntry(changeStr, entryLine, changesWidth)
		} else {
			fromSize := float64(fromInfo[b.name].size) / 1048576
			sizeChange := ((toSize / fromSize) - 1) * 100

			if sizeChange <= float64(-0.01) {
				entryLine = fmt.Sprintf("Size change: %.1fMB -> %.1fMB (%.2f%%)", fromSize, toSize, sizeChange)
				changeStr = appendMcaTableEntry(changeStr, entryLine, changesWidth)
			} else if sizeChange >= float64(0.01) {
				entryLine = fmt.Sprintf("Size change: %.1fMB -> %.1fMB (+%.2f%%)", fromSize, toSize, sizeChange)
				changeStr = appendMcaTableEntry(changeStr, entryLine, changesWidth)
			} else {
				changeStr = appendMcaTableEntry(changeStr, "Size change: (none)", changesWidth)
			}
		}

		// Print bundle file statistics
		entryLine = fmt.Sprintf("Files added: %d", len(b.manFileDiffs.addList))
		changeStr = appendMcaTableEntry(changeStr, entryLine, changesWidth)

		entryLine = fmt.Sprintf("Files changed: %d", len(b.manFileDiffs.modList))
		changeStr = appendMcaTableEntry(changeStr, entryLine, changesWidth)

		entryLine = fmt.Sprintf("Files deleted: %d", len(b.manFileDiffs.delList))
		changeStr = appendMcaTableEntry(changeStr, entryLine, changesWidth)

		// Print added and deleted packages for bundle
		if len(b.pkgDiffs.addList) > 0 {
			changeStr = appendMcaTableEntry(changeStr, "Packages added:", changesWidth)
			sort.Strings(b.pkgDiffs.addList)
		}

		for _, p := range b.pkgDiffs.addList {
			entryLine := fmt.Sprintf("   %s", p)
			changeStr = appendMcaTableEntry(changeStr, entryLine, changesWidth)
		}
		if len(b.pkgDiffs.delList) > 0 {
			changeStr = appendMcaTableEntry(changeStr, "Packages deleted:", changesWidth)
			sort.Strings(b.pkgDiffs.delList)
		}

		for _, p := range b.pkgDiffs.delList {
			entryLine := fmt.Sprintf("   %s", p)
			changeStr = appendMcaTableEntry(changeStr, entryLine, changesWidth)
		}

		// Print file changes for each package in bundle
		changeStr = appendMcaTableEntry(changeStr, " Changes per package:", changesWidth)

		pkgList := append(b.pkgDiffs.addList, b.pkgDiffs.delList...)
		pkgList = append(pkgList, b.pkgDiffs.modList...)
		sort.Strings(pkgList)

		if len(pkgList) == 0 {
			changeStr = appendMcaTableEntry(changeStr, "  (none)", changesWidth)
		}

		for _, p := range pkgList {
			if b.pkgFileCounts[p] == nil {
				continue
			}
			if (b.pkgFileCounts[p].add + b.pkgFileCounts[p].mod + b.pkgFileCounts[p].del) == 0 {
				continue
			}
			entryLine = fmt.Sprintf("   %s (%d added, %d changed, %d deleted)", p, b.pkgFileCounts[p].add, b.pkgFileCounts[p].mod, b.pkgFileCounts[p].del)
			changeStr = appendMcaTableEntry(changeStr, entryLine, changesWidth)
		}
		bundleStr := appendMcaTableEntry("", b.name, bundleWidth)
		table.Append([]string{bundleStr, changeStr})
	}

	fmt.Printf("\nFiles changed per manifest:\n\n")

	table.Render()
	return nil
}

// appendMcaTableEntry adds a new row to an MCA results table entry. Rows longer
// than the maxLen will be wrapped to the next line.
func appendMcaTableEntry(baseStr, newStr string, maxLen int) string {
	if len(baseStr) > 0 {
		baseStr += "\n"
	}

	for len(newStr) > maxLen {
		baseStr += newStr[0:maxLen] + "\n"
		newStr = newStr[maxLen:]
	}
	if len(newStr) > 0 {
		baseStr += newStr
	}
	return baseStr
}
