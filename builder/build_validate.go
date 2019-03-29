package builder

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/tabwriter"

	"github.com/clearlinux/mixer-tools/helpers"
	"github.com/clearlinux/mixer-tools/swupd"
)

type bundleStatus int

const (
	unchanged bundleStatus = iota
	added
	modified
)

// Contains all results from MCA diff and lists of added/deleted/modified bundles
type mcaDiffResults struct {
	bundleDiff map[string]*mcaBundleDiff

	addList []string
	delList []string
	modList []string
}

// MCA diff results for a bundle
type mcaBundleDiff struct {
	name          string
	status        bundleStatus
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
	name    string
	version string
	arch    string

	files map[string]*fileInfo
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

	pkg string
}

// CheckManifestCorrectness validates that the changes in manifest files between
// two versions aligns to the corresponding RPM changes. Any mismatched files
// between the manifests and RPMs will be printed as errors. When there are no
// errors, package and file statistics for each modified bundle will be displayed.
func (b *Builder) CheckManifestCorrectness(fromVer, toVer, downloadRetries int) error {
	if fromVer < 0 || toVer < 0 {
		return fmt.Errorf("Negative version not supported")
	}

	if fromVer >= toVer {
		return fmt.Errorf("From version must be less than to version")
	}

	// Get manifest file lists and subtracted RPM pkg/file lists
	fromInfo, err := b.mcaInfo(fromVer, downloadRetries)
	if err != nil {
		return err
	}
	toInfo, err := b.mcaInfo(toVer, downloadRetries)
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
	errorList, err := analyzeMcaResults(results, fromInfo, toInfo)
	if err != nil {
		return err
	}

	// Display errors and package/file statistics
	err = printMcaResults(results, fromInfo, toInfo, fromVer, toVer, errorList)
	if err != nil {
		return err
	}

	return nil
}

// mcaInfo collects manifest/RPM metadata and uses it to create a list
// of manifest files, subtracted packages, and subtracted package files for each
// bundle in the provided version.
func (b *Builder) mcaInfo(version, downloadRetries int) (map[string]*mcaBundleInfo, error) {
	allBundleInfo := make(map[string]*mcaBundleInfo)

	// Get manifest info for valid bundle entries in the MoM.
	mInfo, err := b.mcaManInfo(version)
	if err != nil {
		return nil, err
	}

	// Download and collect metadata for all packages
	pInfo, err := b.mcaPkgInfo(mInfo, version, downloadRetries)
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

		err = info.getSubPkgs(m.Name, mInfo, pInfo)
		if err != nil {
			return nil, err
		}

		err = info.getSubPkgFiles(m.Name, mInfo, pInfo)
		if err != nil {
			return nil, err
		}

		allBundleInfo[m.Name] = info
	}
	return allBundleInfo, nil
}

// mcaManInfo collects manifest metadata for each bundle listed in
// the provided version's MoM.
func (b *Builder) mcaManInfo(version int) (map[string]*swupd.Manifest, error) {
	mInfo := make(map[string]*swupd.Manifest)

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
		if f.Name == "os-core-update-index" || f.Type == swupd.TypeIManifest {
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

		mInfo[f.Name] = manifest
	}

	return mInfo, nil
}

// mcaPkgInfo downloads and queries all packages for each bundle to collect
// file metadata that will be used to create subtracted package/file lists.
func (b *Builder) mcaPkgInfo(mInfo map[string]*swupd.Manifest, version, downloadRetries int) (map[string]*mcaBundlePkgInfo, error) {
	var wg sync.WaitGroup
	var rw sync.RWMutex

	wg.Add(b.NumBundleWorkers)
	mCh := make(chan *mcaBundlePkgInfo)
	errorCh := make(chan error, b.NumBundleWorkers)

	// Download RPMs from correct upstream version
	upstreamVer, err := b.getLocalUpstreamVersion(strconv.Itoa(version))
	if err != nil {
		return nil, err
	}

	bundleDir := filepath.Join(b.Config.Builder.ServerStateDir, "validation")
	buildVersionDir := filepath.Join(bundleDir, fmt.Sprint(version))
	packageDir := filepath.Join(buildVersionDir, "packages")

	packagerCmd := []string{
		"dnf",
		"-y",
		"--config=" + b.Config.Builder.DNFConf,
		"--releasever=" + upstreamVer,
		"--downloaddir=" + packageDir,
	}

	// Duplicate package entries can exist across bundles. Use a cache to
	// avoid re-calculating packages.
	pkgCache := make(map[string]*pkgInfo)

	pInfo := make(map[string]*mcaBundlePkgInfo)

	// Download and query file metadata from all packages in each bundle
	bundleWorker := func() {
		for m := range mCh {
			var pkgList = []string{}
			for pkg := range mInfo[m.name].BundleInfo.AllPackages {
				pkgList = append(pkgList, pkg)
			}

			out, err := downloadRpms(packagerCmd, pkgList, buildVersionDir, downloadRetries)
			if err != nil {
				errorCh <- err
				wg.Done()
				return
			}

			// Collect metadata to resolve installed RPM file names
			bundlePkgInfo := pkgInfoFromNoopInstall(out.String())
			for _, p := range bundlePkgInfo {
				// Resolve files for package when it doesn't exist in the cache
				rw.RLock()
				if pkgCache[p.name] == nil {
					rw.RUnlock()
					p.files, err = b.resolvePkgFiles(p, version)
					if err != nil {
						errorCh <- err
						wg.Done()
						return
					}

					rw.Lock()
					// Another Goroutine may have already populated the cache. The results should
					// be identical, so there shouldn't be a TOCTOU error.
					if pkgCache[p.name] == nil {
						pkgCache[p.name] = p
					}
					rw.Unlock()

					m.allPkgs[p.name] = p
				} else {
					m.allPkgs[p.name] = pkgCache[p.name]
					rw.RUnlock()
				}

				// Track all resolved package files in bundle to be used later
				// for file subtraction.
				for f := range m.allPkgs[p.name].files {
					m.allFiles[f] = true
				}
			}
		}
		wg.Done()
	}

	for i := 0; i < b.NumBundleWorkers; i++ {
		go bundleWorker()
	}
	for _, m := range mInfo {
		pInfo[m.Name] = &mcaBundlePkgInfo{
			name:     m.Name,
			allFiles: make(map[string]bool),
			allPkgs:  make(map[string]*pkgInfo),
		}

		select {
		case mCh <- pInfo[m.Name]:
		case err = <-errorCh:
			return nil, err
		}
	}
	close(mCh)
	wg.Wait()

	// An error could happen after all the workers are spawned so check again for an
	// error after wg.Wait() completes.
	if err == nil && len(errorCh) > 0 {
		err = <-errorCh
	}
	close(errorCh)

	return pInfo, err
}

// resolvePkgFiles queries a package for a list of file metadata.
func (b *Builder) resolvePkgFiles(pkg *pkgInfo, version int) (map[string]*fileInfo, error) {
	queryCmd := "[%{filenames}, %{filesizes}, %{filedigests}, %{filemodes:perms}, %{filelinktos}, %{fileusername}, %{filegroupname}\n]"
	rpmCmd := []string{"rpm", "-qp", "--qf=" + queryCmd}

	validationDir := filepath.Join(b.Config.Builder.ServerStateDir, "validation")
	buildVersionDir := filepath.Join(validationDir, fmt.Sprint(version))
	pkgDir := filepath.Join(buildVersionDir, "packages")

	pkgFileName := pkg.name + "-" + pkg.version + "." + pkg.arch + ".rpm"
	pkgPath := filepath.Join(pkgDir, pkgFileName)

	// Query RPM for file metadata lists
	args := merge(rpmCmd, pkgPath)
	out, err := helpers.RunCommandOutputEnv(args[0], args[1:], []string{"LC_ALL=en_US.UTF-8"})
	if err != nil {
		return nil, err
	}

	pkgFiles := make(map[string]*fileInfo)

	// Each line contains metadata for a single file in the package
	queryLines := strings.Split(out.String(), "\n")
	for i, line := range queryLines {
		// The last line has no entry
		if i == len(queryLines)-1 {
			continue
		}

		fileMetadata := strings.Split(line, ", ")

		// Paths that are banned from manifests are skipped by MCA
		if isBannedPath(fileMetadata[0]) {
			continue
		}

		// Directories are omitted from MCA because they may be missed from rpm output.
		mode := fileMetadata[3]
		if mode[:1] == "d" {
			continue
		}

		// Some Clear Linux packages install files with path components that are
		// symlinks. MCA must resolve file paths to align with the manifests.
		if strings.HasPrefix(fileMetadata[0], "/bin/") {
			fileMetadata[0] = strings.Replace(fileMetadata[0], "/bin/", "/usr/bin/", 1)
		} else if strings.HasPrefix(fileMetadata[0], "/sbin/") {
			fileMetadata[0] = strings.Replace(fileMetadata[0], "/sbin/", "/usr/bin/", 1)
		} else if strings.HasPrefix(fileMetadata[0], "/lib64/") {
			fileMetadata[0] = strings.Replace(fileMetadata[0], "/lib64/", "/usr/lib64/", 1)
		} else if strings.HasPrefix(fileMetadata[0], "/lib/") {
			fileMetadata[0] = strings.Replace(fileMetadata[0], "/lib/", "/usr/lib/", 1)
		} else if strings.HasPrefix(fileMetadata[0], "/usr/sbin/") {
			fileMetadata[0] = strings.Replace(fileMetadata[0], "/usr/sbin/", "/usr/bin/", 1)
		}

		pkgFiles[fileMetadata[0]] = &fileInfo{
			name:  fileMetadata[0],
			size:  fileMetadata[1],
			hash:  fileMetadata[2],
			modes: fileMetadata[3],
			links: fileMetadata[4],
			user:  fileMetadata[5],
			group: fileMetadata[6],
			pkg:   pkg.name,
		}
	}
	return pkgFiles, nil
}

// pkgInfoFromNoopInstall parses DNF install output to collect and store package metadata
func pkgInfoFromNoopInstall(installOut string) map[string]*pkgInfo {
	pInfo := make(map[string]*pkgInfo)

	// Parse DNF install output
	pkgs := parseNoopInstall(installOut)

	for _, p := range pkgs {
		pInfo[p.name] = &pkgInfo{
			name:    p.name,
			arch:    p.arch,
			version: p.version,
		}
	}
	return pInfo
}

// getManFiles collects the manifest's file list
func (info *mcaBundleInfo) getManFiles(manifest *swupd.Manifest) error {
	for _, file := range manifest.Files {
		// Skip directories, the bundle file, and deleted/ghosted files
		if file.Type == swupd.TypeDirectory ||
			file.Name == "/usr/share/clear/bundles/"+manifest.Name ||
			file.Status == swupd.StatusDeleted ||
			file.Status == swupd.StatusGhosted {

			continue
		}

		info.manFiles[file.Name] = file
	}
	return nil
}

// getSubPkgs gets the bundle's subtracted packages list
func (info *mcaBundleInfo) getSubPkgs(bundle string, mInfo map[string]*swupd.Manifest, pInfo map[string]*mcaBundlePkgInfo) error {
	for p := range pInfo[bundle].allPkgs {
		subtract := false

		// Subtract os-core packages from other bundles
		if bundle != "os-core" && pInfo["os-core"].allPkgs[p] != nil {
			continue
		}

		// Subtract packages included by other bundles
		for _, inc := range mInfo[bundle].BundleInfo.DirectIncludes {
			if pInfo[inc].allPkgs[p] != nil {
				subtract = true
				break
			}
		}

		if subtract == false {
			info.subPkgs[p] = true
		}
	}
	return nil
}

// getSubPkgFiles gets the subtracted files that were resolved from RPMs
func (info *mcaBundleInfo) getSubPkgFiles(bundle string, mInfo map[string]*swupd.Manifest, pInfo map[string]*mcaBundlePkgInfo) error {
	// Collect all files from bundle's subtracted packages
	for p := range info.subPkgs {
		for _, f := range pInfo[bundle].allPkgs[p].files {
			info.subPkgFiles[f.name] = f
		}
	}

	// Subtract files included by other bundles
	for _, inc := range mInfo[bundle].BundleInfo.DirectIncludes {
		for f := range pInfo[inc].allFiles {
			if info.subPkgFiles[f] != nil {
				delete(info.subPkgFiles, f)
			}
		}
	}
	return nil
}

// diffMcaInfo calculates the manifest file, package, and resolved package file
// differences between two versions and captures metadata required by printMcaResults.
func diffMcaInfo(fromInfo, toInfo map[string]*mcaBundleInfo) (*mcaDiffResults, error) {
	results := &mcaDiffResults{
		bundleDiff: make(map[string]*mcaBundleDiff),
	}

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

		results.bundleDiff[bName] = bundleDiff
	}

	for bName := range fromInfo {
		if toInfo[bName] == nil {
			return nil, fmt.Errorf("MCA does not support bundle deletes")
		}
	}
	return results, nil
}

// diffBundles calculates resolved package file, package, and manifest file
// diffs between two versions and updates package file statistics.
func (bundleDiff *mcaBundleDiff) diffBundles(bundle string, fromInfo, toInfo map[string]*mcaBundleInfo) error {
	// Diff package files
	diffList := getFileDiffLists(fromInfo[bundle].subPkgFiles, toInfo[bundle].subPkgFiles)
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
	diffList = getManFileDiffLists(fromInfo[bundle].manFiles, toInfo[bundle].manFiles)
	bundleDiff.manFileDiffs = diffList

	// When there is a file change, the bundle is modified
	if isBundleMod(diffList) {
		bundleDiff.status = modified
	}

	return nil
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

func getFileDiffLists(fromFiles, toFiles map[string]*fileInfo) diffLists {
	addList := []string{}
	delList := []string{}
	modList := []string{}

	for f := range toFiles {
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
		if fromPkgs[p] != false {
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
		if toPkgs[p] == false {
			// Deleted pkg
			delList = append(delList, p)
		}
	}
	return diffLists{modList: modList, addList: addList, delList: delList}
}

func getManFileDiffLists(fromFiles, toFiles map[string]*swupd.File) diffLists {
	addList := []string{}
	delList := []string{}
	modList := []string{}

	for f := range toFiles {
		if fromFiles[f] != nil {
			// Match
			if isManFileMod(fromFiles[f], toFiles[f]) {
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

func isFileMod(from, to *fileInfo) bool {
	return ((from.hash != to.hash) ||
		(from.modes != to.modes) ||
		(from.links != to.links) ||
		(from.user != to.user) ||
		(from.group != to.group))
}

func isPkgMod(key string, bundleDiff *mcaBundleDiff) bool {
	// When file(s) associated with a package are modified, the package is modified
	if (bundleDiff.pkgFileCounts[key].add + bundleDiff.pkgFileCounts[key].mod + bundleDiff.pkgFileCounts[key].del) != 0 {
		return true
	}
	return false
}

func isManFileMod(from, to *swupd.File) bool {
	return (from.Hash != to.Hash)
}

// analyzeMcaResults compares manifest file changes against package file changes.
// When there are inconsistencies between the manifest and package file lists,
// a slice of error strings is returned.
func analyzeMcaResults(results *mcaDiffResults, fromInfo, toInfo map[string]*mcaBundleInfo) ([]string, error) {
	var errorList = []string{}
	var tmp = []string{}

	if results.bundleDiff["os-core"] == nil {
		return nil, fmt.Errorf("os-core does not have mandatory file changes")
	}

	// The os-release, version, and versiontimestamp files are required to be
	// modified by the os-core bundle and are not included by an rpm. Confirm
	// that these files were modified in the manifest, but omit them from the
	// package comparison.
	mandatoryFiles := 0
	for _, f := range results.bundleDiff["os-core"].manFileDiffs.modList {
		switch f {
		case "/usr/lib/os-release":
			mandatoryFiles++
		case "/usr/share/clear/version":
			mandatoryFiles++
		case "/usr/share/clear/versionstamp":
			mandatoryFiles++
		default:
			tmp = append(tmp, f)
		}
	}
	results.bundleDiff["os-core"].manFileDiffs.modList = tmp

	if mandatoryFiles != 3 {
		errorList = append(errorList, "ERROR: os-core is missing mandatory modified file(s)\n")
	}

	// Compare manifest files against package files. Inconsistencies are added to the errorList.
	for _, b := range results.bundleDiff {
		errorList = append(errorList, diffResultLists(b.pkgFileDiffs.addList, b.manFileDiffs.addList, toInfo[b.name].subPkgFiles, b.name, "added")...)
		errorList = append(errorList, diffResultLists(b.pkgFileDiffs.modList, b.manFileDiffs.modList, toInfo[b.name].subPkgFiles, b.name, "modified")...)

		// Added bundles will not exist in the fromInfo object and added bundles cannot have deleted files.
		if fromInfo[b.name] != nil {
			errorList = append(errorList, diffResultLists(b.pkgFileDiffs.delList, b.manFileDiffs.delList, fromInfo[b.name].subPkgFiles, b.name, "deleted")...)
		}
	}

	return errorList, nil
}

// diffResultLists compares manifest files against package files and generates
// an error string when they don't match.
func diffResultLists(pkgFiles, manFiles []string, info map[string]*fileInfo, bundle, mode string) []string {
	sort.Strings(pkgFiles)
	sort.Strings(manFiles)

	var i, j int
	var errorList = []string{}

	// Compare manifest and package file lists. When they don't match, add an
	// error string to the error slice.
	for i, j = 0, 0; i < len(pkgFiles) && j < len(manFiles); {
		switch strings.Compare(pkgFiles[i], manFiles[j]) {
		case 1:
			// File in manifest list, but not package
			errorMsg := fmt.Sprintf("ERROR: %s is %s in manifest '%s', but not in a package\n", manFiles[j], mode, bundle)
			errorList = append(errorList, errorMsg)
			j++
		case -1:
			// File in package list, but not manifest
			errorMsg := fmt.Sprintf("ERROR: %s is %s in package '%s', but not %s in manifest '%s'\n", pkgFiles[i], mode, info[pkgFiles[i]].pkg, mode, bundle)
			errorList = append(errorList, errorMsg)
			i++
		case 0:
			i++
			j++
		}
	}

	for ; i < len(pkgFiles); i++ {
		errorMsg := fmt.Sprintf("ERROR: %s is %s in package '%s', but not %s in manifest '%s'\n", pkgFiles[i], mode, info[pkgFiles[i]].pkg, mode, bundle)
		errorList = append(errorList, errorMsg)
	}
	for ; j < len(manFiles); j++ {
		errorMsg := fmt.Sprintf("ERROR: %s is %s in manifest '%s', but not in a package\n", manFiles[j], mode, bundle)
		errorList = append(errorList, errorMsg)
	}

	return errorList
}

// printMcaResults displays any MCA errors and prints bundle diff statistics when there are no errors.
func printMcaResults(results *mcaDiffResults, fromInfo, toInfo map[string]*mcaBundleInfo, fromVer, toVer int, errorList []string) error {
	var err error

	// An overwhelming number of errors can be generated when this test
	// identifies a manifest bug, so limit the error output to 50.
	if len(errorList) > 0 {
		for i, err := range errorList {
			if i == 50 {
				fmt.Printf("WARNING: Error reporting is limited to 50, so additional errors were skipped.\n")
				break
			}
			fmt.Printf(err)
		}
		return nil
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

	// The output is formatted into a BUNDLE column and a CHANGES column with
	// rows for each changed bundle. The BUNDLE column contains the bundle name
	// and the CHANGES column contains content size, file, and package statistics.
	w := tabwriter.NewWriter(os.Stdout, 30, 0, 1, ' ', tabwriter.Debug)
	defer func() {
		_ = w.Flush()
	}()

	// No bundle information to print
	if len(results.bundleDiff) == 0 {
		return nil
	}

	if _, err = fmt.Fprintf(w, "\n+---------------------------------------------------------------+\n"); err != nil {
		return err
	}
	if _, err = fmt.Fprintf(w, "|BUNDLE\t CHANGES\n"); err != nil {
		return err
	}
	if _, err = fmt.Fprintf(w, "+---------------------------------------------------------------+\n"); err != nil {
		return err
	}

	// Print statistics for each bundle
	for _, b := range results.bundleDiff {
		// Skip unchanged bundles
		if b.status == unchanged {
			continue
		}
		if _, err = fmt.Fprintf(w, "|%s\t Summary:\n", b.name); err != nil {
			return err
		}

		// Print bundle sizes in MB and calculate bundle content size
		// change as percentage.
		toSize := float64(toInfo[b.name].size) / 1048576

		if fromInfo[b.name] == nil || fromInfo[b.name].size == 0 {
			// Print added bundle size
			if _, err = fmt.Fprintf(w, "|\t Size: %.1fMB\n", toSize); err != nil {
				return err
			}
		} else {
			fromSize := float64(fromInfo[b.name].size) / 1048576
			sizeChange := ((toSize / fromSize) - 1) * 100

			if sizeChange <= 0 {
				if _, err = fmt.Fprintf(w, "|\t Size change: %.1fMB -> %.1fMB (%.2f%%)\n", fromSize, toSize, sizeChange); err != nil {
					return err
				}
			} else {
				if _, err = fmt.Fprintf(w, "|\t Size change: %.1fMB -> %.1fMB (+%.2f%%)\n", fromSize, toSize, sizeChange); err != nil {
					return err
				}
			}
		}

		// Print bundle file statistics
		if _, err = fmt.Fprintf(w, "|\t Files added: %d\n", len(b.manFileDiffs.addList)); err != nil {
			return err
		}
		if _, err = fmt.Fprintf(w, "|\t Files changed: %d\n", len(b.manFileDiffs.modList)); err != nil {
			return err
		}
		if _, err = fmt.Fprintf(w, "|\t Files deleted: %d\n", len(b.manFileDiffs.delList)); err != nil {
			return err
		}

		// Print added and deleted packages for bundle
		if len(b.pkgDiffs.addList) > 0 {
			if _, err = fmt.Fprintf(w, "|\t Packages added\n"); err != nil {
				return err
			}
			sort.Strings(b.pkgDiffs.addList)
		}

		for _, p := range b.pkgDiffs.addList {
			if _, err = fmt.Fprintf(w, "|\t    %s\n", p); err != nil {
				return err
			}
		}
		if len(b.pkgDiffs.delList) > 0 {
			if _, err = fmt.Fprintf(w, "|\t Packages deleted\n"); err != nil {
				return err
			}
			sort.Strings(b.pkgDiffs.delList)
		}

		for _, p := range b.pkgDiffs.delList {
			if _, err = fmt.Fprintf(w, "|\t    %s\n", p); err != nil {
				return err
			}
		}

		// Print file changes for each package in bundle
		if _, err = fmt.Fprintf(w, "|\t Changes per package:\n"); err != nil {
			return err
		}

		pkgList := append(b.pkgDiffs.addList, b.pkgDiffs.delList...)
		pkgList = append(pkgList, b.pkgDiffs.modList...)
		sort.Strings(pkgList)

		if len(pkgList) == 0 {
			if _, err = fmt.Fprintf(w, "|\t   (none)\n"); err != nil {
				return err
			}
		}

		for _, p := range pkgList {
			if b.pkgFileCounts[p] == nil {
				continue
			}
			if (b.pkgFileCounts[p].add + b.pkgFileCounts[p].mod + b.pkgFileCounts[p].del) == 0 {
				continue
			}
			_, err = fmt.Fprintf(w, "|\t    %s (%d added, %d changed, %d deleted)\n", p, b.pkgFileCounts[p].add, b.pkgFileCounts[p].mod, b.pkgFileCounts[p].del)
			if err != nil {
				return err
			}
		}
		if _, err = fmt.Fprintf(w, "+---------------------------------------------------------------+\n"); err != nil {
			return err
		}
	}
	return nil
}
