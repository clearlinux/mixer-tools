package builder

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/clearlinux/mixer-tools/helpers"
	"github.com/clearlinux/mixer-tools/swupd"
	"github.com/pkg/errors"
)

type packageMetadata struct {
	name    string
	arch    string
	version string
	repo    string
}

var bannedPaths = [...]string{
	"/var/lib/",
	"/var/cache/",
	"/var/log/",
	"/dev/",
	"/run/",
	"/tmp/",
}

// Default exportable paths
var exportablePaths = [...]string{
	"/bin/",
	"/usr/bin/",
	"/usr/local/bin/",
}

const extractRetries = 5

func isBannedPath(path string) bool {
	if path == "/" {
		return true
	}

	for _, p := range bannedPaths {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

func isExportable(path string, unExport map[string]bool) bool {
	// if the user has specified the file to be un-exportable, then return false
	if unExport[path] {
		return false
	}

	for _, p := range exportablePaths {
		if strings.HasPrefix(path, p) { // if the file is in one of the default exportable paths, then return true
			return true
		}
	}
	return false
}

var convertPathPrefixes = regexp.MustCompile(`^/(bin|sbin|usr/sbin|lib64|lib)/`)

// Clear Linux OS ships several symlinks in the filesystem package, and RPM
// packages may install files to paths with one of these symlinks as a path
// prefix. Short term, resolve file paths using the symlinks from the
// filesystem. Long term, we need to check every path component and resolve.
func resolveFileName(path string) string {
	if !convertPathPrefixes.MatchString(path) {
		return path
	}

	pathLen := len(path)
	if pathLen < 5 {
		return path
	}
	if path[:5] == "/bin/" {
		return filepath.Join("/usr", path)
	}
	if path[:5] == "/lib/" {
		return filepath.Join("/usr/lib", path[4:])
	}
	if pathLen < 6 {
		return path
	}
	if path[:6] == "/sbin/" {
		return filepath.Join("/usr/bin", path[5:])
	}
	if pathLen < 7 {
		return path
	}
	if path[:7] == "/lib64/" {
		return filepath.Join("/usr/lib64", path[6:])
	}
	if pathLen < 10 {
		return path
	}
	if path[:10] == "/usr/sbin/" {
		return filepath.Join("/usr/bin", path[9:])
	}

	// should not reach this due to short circuit
	return path
}

func addFileAndPath(destination, unExport map[string]bool, absPathsToFiles ...string) {
	for _, file := range absPathsToFiles {
		if isBannedPath(file) {
			continue
		}

		path := "/"
		dir := filepath.Dir(file)
		// skip first empty string so we don't add '/'
		for _, part := range strings.Split(dir, "/")[1:] {
			if len(part) == 0 {
				continue
			}
			path = filepath.Join(path, part)
			destination[path] = isExportable(path, unExport)
		}

		destination[file] = isExportable(file, unExport)
	}
}

func addOsCoreSpecialFiles(bundle *bundle) {
	filesToAdd := []string{
		"/usr/lib/os-release",
		"/usr/share/clear/version",
		"/usr/share/clear/versionstamp",
	}

	addFileAndPath(bundle.Files, bundle.UnExport, filesToAdd...)
}

func addUpdateBundleSpecialFiles(b *Builder, bundle *bundle) {
	filesToAdd := []string{
		"/usr/share/defaults/swupd/contenturl",
		"/usr/share/defaults/swupd/versionurl",
		"/usr/share/defaults/swupd/format",
	}

	if _, err := os.Stat(b.Config.Builder.Cert); err == nil {
		filesToAdd = append(filesToAdd, "/usr/share/clear/update-ca/Swupd_Root.pem")
	}

	addFileAndPath(bundle.Files, bundle.UnExport, filesToAdd...)
}

// repoPkgMap is a map of repo names to the package metadata they provide
type repoPkgMap map[string][]packageMetadata

func resolveFilesForBundle(bundle *bundle, repoPkgs repoPkgMap, packagerCmd []string) error {
	bundle.Files = make(map[string]bool)

	for repo, pkgs := range repoPkgs {
		queryString := merge(packagerCmd, "repoquery", "-l", "--quiet", "--repo", repo)
		for _, pkg := range pkgs {
			queryString = append(queryString, pkg.name)
		}
		outBuf, err := helpers.RunCommandOutputEnv(queryString[0], queryString[1:], []string{"LC_ALL=en_US.UTF-8"})
		if err != nil {
			return err
		}
		for _, f := range strings.Split(outBuf.String(), "\n") {
			if len(f) > 0 {
				addFileAndPath(bundle.Files, bundle.UnExport, resolveFileName(f))
			}
		}
	}

	addFileAndPath(bundle.Files, bundle.UnExport, fmt.Sprintf("/usr/share/clear/bundles/%s", bundle.Name))
	fmt.Printf("Bundle %s\t%d files\n", bundle.Name, len(bundle.Files))
	return nil
}

// NO-OP INSTALL OUTPUT EXAMPLE
// Truncated at 80 characters to make it more readable, but the full output is a
// five-column list of Package, Arch, Version, Repository, and Size.
// The section we care about is everything from "Installing:" to "Transaction
// Summary"
//
// Last metadata expiration check: 0:00:00 ago on Wed 07 Mar 2018 04:05:44 PM PS
// Dependencies resolved.
// =============================================================================
//  Package                                   Arch                Version
// =============================================================================
// Installing:
//  systemd-boot                              x86_64              234-166
// Installing dependencies:
//  Linux-PAM                                 x86_64              1.2.1-33
//  <more packages>
//  zlib-lib                                  x86_64              1.2.8.jtkv4-43
//
// Transaction Summary
// =============================================================================
// <more metadata>
//
// The other case we need to be careful about is when there are no dependencies.
//
// Last metadata expiration check: 0:00:00 ago on Wed 07 Mar 2018 04:33:18 PM PS
// Dependencies resolved.
// =============================================================================
//  Package                   Arch                        Version
// =============================================================================
// Installing:
//  shim                      x86_64                      12-10
//
// Transaction Summary
// =============================================================================
// <more metadata>
func parseNoopInstall(installOut string) ([]packageMetadata, error) {
	// Split out the section between the install list and the transaction Summary.
	parts := strings.Split(installOut, "Installing:\n")
	if len(parts) < 2 {
		// If there is no such section, e.g. dnf fails because there's
		// no matching package (so no "Installing:"), return error with
		// list of packages not found.
		var pkgs string
		if len(parts) == 1 {
			parts = strings.Split(parts[0], "No match for argument:")
			parts = parts[1:]
			for i, v := range parts {
				pkgs = pkgs + strings.TrimSpace(v)
				if i != len(parts)-1 {
					pkgs = pkgs + ", "
				}
			}
		}
		if len(pkgs) > 0 {
			return nil, fmt.Errorf("unable to resolve package(s): %s", pkgs)
		} else {
			return nil, fmt.Errorf("dnf error occurred")
		}
	}
	pkgList := strings.Split(parts[1], "\nTransaction Summary")[0]

	// When not running in a TTY, dnf sets terminal line-width to a default 80
	// characters, then wraps their fields in an unpredictable way to be
	// pleasing to the eye but not to a parser. The result always represents a
	// five-column, whitespace-separated list, but the whitespace between the
	// columns can be variable length and include a newline. Note that the fifth
	// column itself contains a space.
	//
	// example lines:
	// ^ really-long-package-name-overflows-field$
	// ^                         x86_64             12-10         clear   4.2 k$
	// ^ reasonablepkgname       x86_64             deadcafedeadbeefdeadcafedeadbeef$
	// ^                                                          clear   5.5 M$
	var r = regexp.MustCompile(` (\S+)\s+(\S+)\s+(\S+)\s+(\S+)\s+(\S+) \S+\n`)
	var pkgs = []packageMetadata{}

	for _, match := range r.FindAllStringSubmatch(pkgList, -1) {
		pkg := packageMetadata{
			name:    match[1],
			arch:    match[2],
			version: match[3],
			repo:    match[4],
		}
		pkgs = append(pkgs, pkg)
	}

	return pkgs, nil
}

func repoPkgFromNoopInstall(installOut string) (repoPkgMap, error) {
	repoPkgs := make(repoPkgMap)
	pkgs, err := parseNoopInstall(installOut)
	if err != nil {
		return nil, err
	}
	for _, p := range pkgs {
		repoPkgs[p.repo] = append(repoPkgs[p.repo], p)
	}
	return repoPkgs, nil
}

// queryRpmFullPath returns the expected rpm full path for a package using dnf repoquery.
// It queries the dnf database for the location.
// If the query result is empty, it returns an err.
// If the query result has a "file" scheme, it returns the full path without the scheme.
// If the query result has a non "file" scheme, it assumes the full path is within the corresponding repo cache dir.
func queryRpmFullPath(packageCmd []string, pkgName string, repo string, repos map[string]repoInfo) (string, error) {
	queryStringRpm := merge(
		packageCmd,
		"repoquery",
		"--location",
		"--repo",
		repo,
	)
	queryStringRpm = append(queryStringRpm, pkgName)
	outBuf, _ := helpers.RunCommandOutputEnv(queryStringRpm[0], queryStringRpm[1:], []string{"LC_ALL=en_US.UTF-8"})

	if outBuf.String() == "" {
		return "", fmt.Errorf("rpm not found for pkg: %s", pkgName)
	}

	out := strings.Split(outBuf.String(), "\n")
	pURL, err := url.Parse(out[0])
	if err != nil {
		return "", err
	}

	var rpmFullPath string
	if pURL.Scheme == "file" { // obtain the full path without the url scheme
		pURL.Scheme = ""
		rpmFullPath = pURL.String()
		// This is an attempt to improve performance in case the rpms are not stored directly within the repo baseurl.
		repoInfo := repos[repo]
		repoInfo.cacheDirs[filepath.Dir(rpmFullPath)] = true
		repos[repo] = repoInfo
	} else { // obtain only the rpm name and append to repo cache dir
		rpm := filepath.Base(out[0])
		rpmFullPath = filepath.Join(dnfDownloadDir, rpm)
	}
	return rpmFullPath, nil
}

var fileSystemInfo packageMetadata

// resolvePackagesWithOptions updates set with resolved packages for each bundle. When validationResolve is set,
// files are not resolved and the bundleRepoPkgs map is populated. Otherwise, files are resolved and the bundleRepoPkgs
// map is not populated.
func resolvePackagesWithOptions(numWorkers int, set bundleSet, packagerCmd []string, validationResolve bool) (*sync.Map, error) {
	var err error
	var wg sync.WaitGroup
	fmt.Printf("Resolving packages using %d workers\n", numWorkers)
	wg.Add(numWorkers)
	bundleCh := make(chan *bundle)
	// bundleRepoPkgs is a map of bundles -> map of repos -> list of packageMetadata
	var bundleRepoPkgs sync.Map
	errorCh := make(chan error, numWorkers)
	defer close(errorCh)

	packageWorker := func() {
		defer wg.Done()
		emptyDir, err := ioutil.TempDir("", "MixerEmptyDirForNoopInstall")
		if err != nil {
			errorCh <- err
			return
		}
		defer func() {
			_ = os.RemoveAll(emptyDir)
		}()

		for bundle := range bundleCh {
			fmt.Printf("processing %s\n", bundle.Name)
			queryString := merge(
				packagerCmd,
				"--installroot="+emptyDir,
				"--assumeno",
				"install",
			)
			for p := range bundle.AllPackages {
				queryString = append(queryString, p)
			}
			bundle.AllRpms = make(map[string]packageMetadata)
			// Ignore error from the --assumeno install, but save the output for later
			// printing, in case the output indicates an error. An error is returned every
			// time because --assumeno forces the command to "abort" and return a non-zero
			// exit status. This exit status is 1, which is the same as any other dnf install
			// error. Fortunately if this is a different error than we expect, it should fail
			// in the actual install to the full chroot.
			outBuf, errStr := helpers.RunCommandOutputEnv(queryString[0], queryString[1:], []string{"LC_ALL=en_US.UTF-8"})
			rpm, e := repoPkgFromNoopInstall(outBuf.String())
			if len(bundle.AllPackages) != 0 && e != nil {
				e = errors.Wrapf(e, bundle.Name)
				fmt.Println(e)
				fmt.Println("error details:")
				fmt.Println(errStr)
				errorCh <- e
				return
			}
			for _, pkgs := range rpm {
				// Add packages to bundle's AllPackages
				for _, pkg := range pkgs {
					rpmName := pkg.name + "-" + pkg.version + "." + pkg.arch + ".rpm"
					bundle.AllPackages[pkg.name] = true
					bundle.AllRpms[rpmName] = pkg
					// to find the pkg Metadata of filesystem rpm, as filesystem needs to be extracted first
					if pkg.name == "filesystem" && fileSystemInfo == (packageMetadata{}) {
						fileSystemInfo = pkg
					}
				}
			}

			// When resolving packages to validate the build output, populate the bundleRepoPkgs map for later processing
			// and skip file resolution
			if validationResolve {
				bundleRepoPkgs.Store(bundle.Name, rpm)
				fmt.Printf("... done with %s\n", bundle.Name)
			} else {
				e = resolveFilesForBundle(bundle, rpm, packagerCmd)
				if e != nil {
					errorCh <- e
					return
				}
			}
		}
	}
	for i := 0; i < numWorkers; i++ {
		go packageWorker()
	}

	for _, bundle := range set {
		select {
		case bundleCh <- bundle:
		case err = <-errorCh:
			// break as soon as there is a failure
			break
		}
		if err != nil {
			break
		}
	}
	close(bundleCh)
	wg.Wait()

	if err != nil {
		return nil, err
	}
	// an error could happen after all the workers are spawned so check again for an
	// error after wg.Wait() completes.
	if len(errorCh) > 0 {
		return nil, <-errorCh
	}

	return &bundleRepoPkgs, err
}

// resolvePackages resolves packages and files for each bundle without populating the map
// of bundles to a map of repos to a list of packageMetadata.
func resolvePackages(numWorkers int, set bundleSet, packagerCmd []string) error {
	_, err := resolvePackagesWithOptions(numWorkers, set, packagerCmd, false)
	return err
}

// resolvePackagesValidation resolves packages and returns a map of bundles to a map of repos to
// a list of packageMetadata which is used during build validation.
func resolvePackagesValidation(numWorkers int, set bundleSet, packagerCmd []string) (*sync.Map, error) {
	return resolvePackagesWithOptions(numWorkers, set, packagerCmd, true)
}

func installFilesystem(chrootDir string, packagerCmd []string, downloadRetries int, repos map[string]repoInfo) error {
	var err error

	if repos[fileSystemInfo.repo].urlScheme != "file" {
		packagerCmdNew := merge(packagerCmd, "--destdir", dnfDownloadDir)
		_, err := downloadRpms(packagerCmdNew, []string{fileSystemInfo.name}, chrootDir, downloadRetries)
		if err != nil {
			return err
		}
	}

	pkgFull := fileSystemInfo.name + "-" + fileSystemInfo.version + "." + fileSystemInfo.arch
	rpm := pkgFull + ".rpm"
	var rpmFullPath string
	for cacheDir := range repos[fileSystemInfo.repo].cacheDirs {
		rpmFullPath = filepath.Join(cacheDir, rpm) // assuming the full path based on Clear Linux repo and naming conventions
		_, err = os.Stat(rpmFullPath)
		if err == nil {
			break
		}
	}

	if os.IsNotExist(err) {
		// If rpm is not found, the rpm filename may not be in autospec generated format and/or in another location
		// within the repo. In this case, determine the actual rpm filename with its full path.
		rpmFullPath, err = queryRpmFullPath(packagerCmd, pkgFull, fileSystemInfo.repo, repos)
		if err != nil {
			return err
		}
		if _, err = os.Stat(rpmFullPath); os.IsNotExist(err) {
			return fmt.Errorf("rpm not found for pkg: %s", pkgFull)
		}
		if err != nil {
			return err
		}
	}
	rpmMap[rpm] = true

	for i := 0; i < extractRetries; i++ {
		err = extractRpm(chrootDir, rpmFullPath)
		if err != nil {
			continue
		}
		break
	}
	return err
}

func createClearDir(chrootDir, version string) error {
	clearDir := filepath.Join(chrootDir, "usr/share/clear")
	err := os.MkdirAll(filepath.Join(clearDir, "bundles"), 0755)
	if err != nil {
		return err
	}

	// Writing special files identifying the version in os-core.
	err = ioutil.WriteFile(filepath.Join(clearDir, "version"), []byte(version), 0644)
	if err != nil {
		return err
	}
	versionstamp := fmt.Sprint(time.Now().Unix())
	return ioutil.WriteFile(filepath.Join(clearDir, "versionstamp"), []byte(versionstamp), 0644)
}

func buildOsCore(b *Builder, packagerCmd []string, chrootDir, version string) error {

	if err := createClearDir(chrootDir, version); err != nil {
		return err
	}

	if err := updateOSReleaseFile(b, filepath.Join(chrootDir, "usr/lib/os-release"), version, b.Config.Swupd.ContentURL, b.UpstreamVer); err != nil {
		return errors.Wrap(err, "couldn't update os-release file")
	}

	if err := createVersionsFile(filepath.Dir(chrootDir), packagerCmd); err != nil {
		return errors.Wrapf(err, "couldn't create the versions file")
	}

	return nil
}

func genUpdateBundleSpecialFiles(chrootDir string, b *Builder) error {
	swupdDir := filepath.Join(chrootDir, "usr/share/defaults/swupd")
	if err := os.MkdirAll(swupdDir, 0755); err != nil {
		return err
	}
	cURLBytes := []byte(b.Config.Swupd.ContentURL)
	if err := ioutil.WriteFile(filepath.Join(swupdDir, "contenturl"), cURLBytes, 0644); err != nil {
		return err
	}
	vURLBytes := []byte(b.Config.Swupd.VersionURL)
	if err := ioutil.WriteFile(filepath.Join(swupdDir, "versionurl"), vURLBytes, 0644); err != nil {
		return err
	}

	// Only copy the certificate into the mix if it exists
	if _, err := os.Stat(b.Config.Builder.Cert); err == nil {
		certdir := filepath.Join(chrootDir, "/usr/share/clear/update-ca")
		err = os.MkdirAll(certdir, 0755)
		if err != nil {
			return err
		}
		chrootcert := filepath.Join(certdir, "Swupd_Root.pem")
		err = helpers.CopyFile(chrootcert, b.Config.Builder.Cert)
		if err != nil {
			return err
		}
	}

	return ioutil.WriteFile(filepath.Join(swupdDir, "format"), []byte(b.State.Mix.Format), 0644)
}

func downloadRpms(packagerCmd, rpmList []string, baseDir string, maxRetries int) (*bytes.Buffer, error) {
	var downloadErr error
	var out *bytes.Buffer

	if maxRetries < 0 {
		return nil, errors.Errorf("maxRetries value < 0 for RPM downloads")
	}

	args := merge(packagerCmd, "--installroot="+baseDir, "install", "--downloadonly")
	args = append(args, rpmList...)

	for attempts := 0; attempts <= maxRetries; attempts++ {
		out, downloadErr = helpers.RunCommandOutputEnv(args[0], args[1:], []string{"LC_ALL=en_US.UTF-8"})
		if downloadErr == nil {
			return out, downloadErr
		}

		fmt.Printf("RPM download attempt %d failed. Maximum of %d attempts.\n", attempts+1, maxRetries+1)
	}
	return nil, downloadErr
}

func extractRpm(baseDir string, rpm string) error {
	dir, file := filepath.Split(rpm)
	rpm2Cmd := exec.Command("rpm2archive", file)
	rpm2Cmd.Dir = dir
	rpm2Cmd.Env = os.Environ()

	rpmTar := file + ".tgz"
	tarCmd := exec.Command("tar", "-xf", rpmTar, "-C", baseDir)
	tarCmd.Env = os.Environ()
	tarCmd.Dir = dir

	var err error

	err = rpm2Cmd.Run()
	if err != nil {
		return fmt.Errorf("rpm2archive cmd failed for %s with %s", file, err.Error())
	}

	err = tarCmd.Run()
	if err != nil {
		return fmt.Errorf("tarCmd failed for %s with %s", rpmTar, err.Error())
	}

	err = os.Remove(dir + rpmTar)
	if err != nil {
		fmt.Println("failed to remove file", rpmTar)
	}

	return nil
}

func installBundleToFull(packagerCmd []string, baseDir string, bundle *bundle, downloadRetries int, numWorkers int, repos map[string]repoInfo) error {
	var err error
	var wg sync.WaitGroup
	rpmCh := make(chan string)

	var missingRpms []string

	if len(bundle.AllRpms) < numWorkers {
		numWorkers = len(bundle.AllRpms)
	}
	errorCh := make(chan error, numWorkers)
	defer close(errorCh)
	wg.Add(numWorkers)

	rpmWorker := func() {
		defer wg.Done()
		var e error
		for rpm := range rpmCh {
			// running for extractRetries times as rpm2archive and tar can fail if two files are extracted at exact same path
			for i := 0; i < extractRetries; i++ {
				e = extractRpm(baseDir, rpm)
				if e == nil {
					break
				}
			}
			if e != nil {
				fmt.Println(e)
				errorCh <- e
				return
			}
		}
	}

	for rpm, pkgInfo := range bundle.AllRpms {
		if rpmMap[rpm] {
			continue
		}
		// urlScheme not equal to file needs to be downloaded
		if repos[pkgInfo.repo].urlScheme != "file" {
			missingRpms = append(missingRpms, pkgInfo.name)
		}
	}

	if len(missingRpms) > 0 {
		packagerCmdNew := merge(packagerCmd, "--destdir", dnfDownloadDir)
		_, err = downloadRpms(packagerCmdNew, missingRpms, baseDir, downloadRetries)
		if err != nil {
			return err
		}
	}

	for i := 0; i < numWorkers; i++ {
		go rpmWorker()
	}

	// feed the channel
	for rpm, pkgInfo := range bundle.AllRpms {
		if rpmMap[rpm] {
			continue
		}
		var rpmFullPath string
		pkgFull := pkgInfo.name + "-" + pkgInfo.version + "." + pkgInfo.arch
		for cacheDir := range repos[pkgInfo.repo].cacheDirs {
			rpmFullPath = filepath.Join(cacheDir, rpm) // assuming the full path based on Clear Linux repo and naming conventions
			_, err = os.Stat(rpmFullPath)
			if err == nil {
				break
			}
		}

		if os.IsNotExist(err) {
			// If rpm is not found, the rpm filename may not be in autospec generated format and/or in another location
			// within the repo. In this case, determine the actual rpm filename with its full path.
			rpmFullPath, err = queryRpmFullPath(packagerCmd, pkgFull, pkgInfo.repo, repos)
			if err != nil {
				return err
			}
			if _, err = os.Stat(rpmFullPath); os.IsNotExist(err) {
				return fmt.Errorf("rpm not found for pkg: %s", pkgFull)
			}
			if err != nil {
				return err
			}
		}
		rpmMap[rpm] = true

		select {
		case rpmCh <- rpmFullPath:
		case err = <-errorCh:
			break
		}
		if err != nil {
			break
		}
	}

	close(rpmCh)
	wg.Wait()

	if err != nil {
		return err
	}

	// Sending loop might finish before any goroutine could send an error back, so check for
	// error again after they are all done.
	if len(errorCh) > 0 {
		return <-errorCh
	}

	return nil
}

func clearDNFCache(packagerCmd []string) error {
	args := merge(packagerCmd, "clean", "all")
	_, err := helpers.RunCommandOutputEnv(args[0], args[1:], []string{"LC_ALL=en_US.UTF-8"})
	return err
}

func rmDNFStatePaths(fullDir string) {
	dnfStatePaths := []string{
		"/etc/dnf",
		"/var/lib/cache/yum",
		"/var/cache/yum",
		"/var/cache/dnf",
		"/var/lib/dnf",
		"/var/lib/rpm",
	}
	for _, p := range dnfStatePaths {
		_ = os.RemoveAll(filepath.Join(fullDir, p))
	}

	// Now remove the logs from /var/log, but leave the directory
	logFiles, err := filepath.Glob(filepath.Join(fullDir, "/var/log/*.log"))
	if err != nil {
		return
	}
	for _, f := range logFiles {
		_ = os.RemoveAll(f)
	}
}

var rpmMap map[string]bool
var dnfDownloadDir string

func buildFullChroot(b *Builder, set *bundleSet, packagerCmd []string, buildVersionDir, version string, downloadRetries int, numWorkers int) error {
	dnfDownloadDir = filepath.Join(buildVersionDir, "downloadedRpms")
	err := os.MkdirAll(dnfDownloadDir, 0755)
	if err != nil {
		return err
	}
	defer func() {
		_ = os.RemoveAll(dnfDownloadDir)
	}()

	fmt.Println("Available repos: ")
	err = b.ListRepos()
	if err != nil {
		return err
	}

	fmt.Println("Installing all bundles to full chroot")
	totalBundles := len(*set)

	fullDir := filepath.Join(buildVersionDir, "full")
	err = os.MkdirAll(fullDir, 0755)
	if err != nil {
		return err
	}

	i := 0
	rpmMap = make(map[string]bool)

	if fileSystemInfo != (packageMetadata{}) {
		if err := installFilesystem(fullDir, packagerCmd, downloadRetries, b.repos); err != nil {
			return err
		}
	}

	for _, bundle := range *set {
		i++
		fmt.Printf("[%d/%d] %s\n", i, totalBundles, bundle.Name)

		if err := installBundleToFull(packagerCmd, fullDir, bundle, downloadRetries, numWorkers, b.repos); err != nil {
			return err
		}
	}

	// Resolve bundle content chroots against the full chroot. New files are copied
	// to the full chroot and the bundle file lists are updated to contain the files
	// within the chroot.
	if err = addBundleContentChroots(set, fullDir); err != nil {
		return err
	}

	return installSpecialFilesToFull(b, packagerCmd, set, fullDir, version)
}

// installSpecialFilesToFull installs the bundle tracking files and installs special case files for the
// os-core and update bundles.
func installSpecialFilesToFull(b *Builder, packagerCmd []string, set *bundleSet, fullDir, version string) error {
	// bundle tracking files
	bundleDir := filepath.Join(fullDir, "usr/share/clear/bundles")
	if err := os.MkdirAll(bundleDir, 0755); err != nil {
		return err
	}
	for _, bundle := range *set {
		if err := ioutil.WriteFile(filepath.Join(bundleDir, bundle.Name), nil, 0644); err != nil {
			return err
		}
	}

	// special handling for os-core
	fmt.Println("... building special os-core content")
	if err := buildOsCore(b, packagerCmd, fullDir, version); err != nil {
		return err
	}

	// special handling for update bundle
	fmt.Printf("... Adding swupd default values to %s bundle\n", b.Config.Swupd.Bundle)
	return genUpdateBundleSpecialFiles(fullDir, b)
}

// addBundleContentChroots resolves each bundle's content choots against the full chroot
// and updates the bundle file lists.
func addBundleContentChroots(set *bundleSet, fullDir string) error {
	// Resolve bundle content chroots against the full chroot. Content chroot files
	// that do not conflict with the full chroot are copied into the full chroot and
	// added to the corresponding bundle's bundle-info file. Files that conflict with
	// the full chroot will generate an error.
	for _, bundle := range *set {
		for chrootPath := range bundle.ContentChroots {
			if _, err := os.Stat(chrootPath); err != nil {
				return err
			}

			err := filepath.Walk(chrootPath, func(path string, fi os.FileInfo, err error) error {
				if err != nil {
					return err
				}

				filePath := strings.TrimPrefix(path, chrootPath)
				fullChrootFile := filepath.Join(fullDir, filePath)

				// Skip content chroot directory
				if filePath == "" {
					return nil
				}

				// Add file to bundle-info file list
				bundle.Files[filePath] = isExportable(filePath, bundle.UnExport)

				// When the content chroot file exists in the full chroot verify that they
				// are the same.
				if fullInfo, err := os.Lstat(fullChrootFile); err == nil {
					if fullInfo.IsDir() && fi.IsDir() {
						if fullInfo.Mode() != fi.Mode() {
							return errors.Errorf("Directory permission mismatch: %s, %s", fullChrootFile, path)
						}

						srcDir, ok := fi.Sys().(*syscall.Stat_t)
						if !ok {
							return errors.Errorf("Cannot get directory ownership: %s", path)
						}
						targDir, ok := fullInfo.Sys().(*syscall.Stat_t)
						if !ok {
							return errors.Errorf("Cannot get directory ownership: %s", fullChrootFile)
						}
						if srcDir.Uid != targDir.Uid || srcDir.Gid != targDir.Gid {
							return errors.Errorf("Directory ownership mismatch: %s, %s", fullChrootFile, path)
						}

						return nil
					}

					h1, err := swupd.Hashcalc(fullChrootFile)
					if err != nil {
						return err
					}
					h2, err := swupd.Hashcalc(path)
					if err != nil {
						return err
					}
					if !swupd.HashEquals(h1, h2) {
						return errors.Errorf("Chroot File conflict: %s, %s", fullChrootFile, path)
					}
					return nil
				}

				if fi.IsDir() {
					if err = os.Mkdir(fullChrootFile, fi.Mode()); err != nil {
						return err
					}

					dirStat, ok := fi.Sys().(*syscall.Stat_t)
					if !ok {
						return errors.Errorf("Cannot get directory ownership: %s", path)
					}
					err = os.Chown(fullChrootFile, int(dirStat.Uid), int(dirStat.Gid))
					if err != nil {
						return err
					}

					// umask prevents setting the permissions correctly when creating the target directory,
					// so the permissions are set after the directory is created.
					return os.Chmod(fullChrootFile, fi.Mode())
				}

				// Do not resolve symlinks so that the links can be copied, do not
				// sync to disk which significantly improves I/O performance, and
				// use the source file's permissions for the target.
				return helpers.CopyFileWithOptions(fullChrootFile, path, false, false, true)
			})
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func writeBundleInfo(bundle *bundle, path string) error {
	b, err := json.Marshal(*bundle)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(path, b, 0644)
}

func (b *Builder) buildBundles(set bundleSet, downloadRetries int) error {
	var err error

	if b.Config.Builder.ServerStateDir == "" {
		return errors.Errorf("invalid empty state dir")
	}

	bundleDir := filepath.Join(b.Config.Builder.ServerStateDir, "image")

	// TODO: Remove remaining references to outputDir. Let "build update" take care of
	// bootstraping or cleaning up.
	outputDir := filepath.Join(b.Config.Builder.ServerStateDir, "www")

	if _, ok := set["os-core"]; !ok {
		return fmt.Errorf("os-core bundle not found")
	}

	// Bootstrap the directories.
	err = os.MkdirAll(filepath.Join(bundleDir, "0"), 0755)
	if err != nil {
		return err
	}
	err = os.MkdirAll(filepath.Join(outputDir, "0"), 0755)
	if err != nil {
		return err
	}

	if _, ok := set[b.Config.Swupd.Bundle]; !ok {
		return fmt.Errorf("couldn't find bundle %q specified in configuration as the update bundle",
			b.Config.Swupd.Bundle)
	}

	// Write INI files. These are used to communicate to the next step of mixing (build update).
	var serverINI bytes.Buffer
	_, _ = fmt.Fprintf(&serverINI, `[Server]
emptydir=%s/empty
imagebase=%s/image/
outputdir=%s/www/

[Debuginfo]
banned=%s
lib=%s
src=%s
`, b.Config.Builder.ServerStateDir, b.Config.Builder.ServerStateDir,
		b.Config.Builder.ServerStateDir, b.Config.Server.DebugInfoBanned,
		b.Config.Server.DebugInfoLib, b.Config.Server.DebugInfoSrc)

	err = ioutil.WriteFile(filepath.Join(b.Config.Builder.ServerStateDir, "server.ini"), serverINI.Bytes(), 0644)
	if err != nil {
		return err
	}
	// TODO: If we are using INI files that are case insensitive, we need to be more restrictive
	// in bundleset to check for that. See also readGroupsINI in swupd package.
	var groupsINI bytes.Buffer
	for _, bundle := range set {
		_, _ = fmt.Fprintf(&groupsINI, "[%s]\ngroup=%s\n\n", bundle.Name, bundle.Name)
	}
	err = ioutil.WriteFile(filepath.Join(b.Config.Builder.ServerStateDir, "groups.ini"), groupsINI.Bytes(), 0644)
	if err != nil {
		return err
	}

	// Mixer is used to create both Clear Linux or a mix of it.
	var version string
	if b.MixVer != "" {
		fmt.Printf("Creating bundles for version %s based on Clear Linux %s\n", b.MixVer, b.UpstreamVer)
		version = b.MixVer
	} else {
		fmt.Printf("Creating bundles for version %s\n", b.UpstreamVer)
		version = b.UpstreamVer
		// TODO: This validation should happen when reading the configuration.
		if version == "" {
			return errors.Errorf("no Mixver or Clearver set, unable to proceed")
		}
	}

	buildVersionDir := filepath.Join(bundleDir, version)
	fmt.Printf("Preparing new %s\n", buildVersionDir)
	fmt.Printf("  and dnf config: %s\n", b.Config.Builder.DNFConf)

	err = os.MkdirAll(buildVersionDir, 0755)
	if err != nil {
		return err
	}
	for name, bundle := range set {
		// TODO: Should we embed this information in groups.ini? (Maybe rename it to bundles.ini)
		var includes bytes.Buffer
		for _, inc := range bundle.DirectIncludes {
			_, _ = fmt.Fprintf(&includes, "%s\n", inc)
		}
		err = ioutil.WriteFile(filepath.Join(buildVersionDir, name+"-includes"), includes.Bytes(), 0644)
		if err != nil {
			return err
		}
	}

	packagerCmd := []string{
		"dnf",
		"--config=" + b.Config.Builder.DNFConf,
		"-y",
		"--releasever=" + b.UpstreamVer,
	}

	fmt.Printf("Packager command-line: %s\n", strings.Join(packagerCmd, " "))

	// Existing DNF cache content can cause incorrect queries with stale results
	fmt.Println("Cleaning DNF cache")
	if err := clearDNFCache(packagerCmd); err != nil {
		return err
	}

	numWorkers := b.NumBundleWorkers

	err = resolvePackages(numWorkers, set, packagerCmd)
	if err != nil {
		return err
	}

	updateBundle := set[b.Config.Swupd.Bundle]
	var osCore *bundle
	for _, bundle := range set {
		if bundle.Name == "os-core" {
			osCore = bundle
			break
		}
	}
	addOsCoreSpecialFiles(osCore)
	addUpdateBundleSpecialFiles(b, updateBundle)

	// install all bundles in the set (including os-core) to the full chroot
	err = buildFullChroot(b, &set, packagerCmd, buildVersionDir, version, downloadRetries, numWorkers)
	if err != nil {
		return err
	}

	for _, bundle := range set {
		err = writeBundleInfo(bundle, filepath.Join(buildVersionDir, bundle.Name+"-info"))
		if err != nil {
			return err
		}
	}

	// now that all dnf/yum/rpm operations have completed
	// remove all packager state files from chroot
	// This is not a critical step, just to prevent these files from
	// making it into the Manifest.full
	rmDNFStatePaths(filepath.Join(buildVersionDir, "full"))
	return nil
}

// createVersionsFile creates a file that contains all the packages available for a specific
// version. It uses one chroot to query information from the repositories using dnf.
func createVersionsFile(baseDir string, packagerCmd []string) error {
	args := merge(packagerCmd,
		"--installroot="+filepath.Join(baseDir, "full"),
		"--quiet",
		"list",
	)

	var outBuf bytes.Buffer
	var errBuf bytes.Buffer
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	cmd.Env = append(os.Environ(), "LC_ALL=en_US.UTF-8")
	err := cmd.Run()
	if err != nil {
		msg := fmt.Sprintf("couldn't list packages: %s\nCOMMAND LINE: %s", err, args)
		if errBuf.Len() > 0 {
			msg += "\nOUTPUT:\n%s" + errBuf.String()
		}
		return errors.New(msg)
	}

	type pkgEntry struct {
		name, version string
	}
	var versions []*pkgEntry

	scanner := bufio.NewScanner(&outBuf)
	skippedPrefixes := []string{
		// Default output from list command.
		"Available",
		"Installed",

		// dnf message about expiration.
		"Last metadata",

		// TODO: Review if those errors appear in stdout or stderr, if the former we can
		// remove them. The rpm/yum cause the packages to be removed from the list.
		"BDB2053", // Some Berkley DB error?
		"rpm",
		"yum",
	}
	for scanner.Scan() {
		text := scanner.Text()

		var skip bool
		for _, p := range skippedPrefixes {
			if strings.HasPrefix(text, p) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}

		fields := strings.Fields(text)
		if len(fields) != 3 {
			// The output for dnf list wraps at 80 when lacking information about the
			// terminal, so we workaround by joining the next line and evaluating. See
			// https://bugzilla.redhat.com/show_bug.cgi?id=584525 for the wrapping.
			if scanner.Scan() {
				text = text + scanner.Text()
			} else {
				return fmt.Errorf("couldn't parse line %q from dnf list output", text)
			}
			fields = strings.Fields(text)
			if len(fields) != 3 {
				return fmt.Errorf("couldn't parse merged line %q from dnf list output", text)
			}
		}

		e := &pkgEntry{
			name:    fields[0],
			version: fields[1],
		}
		versions = append(versions, e)
	}
	err = scanner.Err()
	if err != nil {
		return err
	}

	sort.Slice(versions, func(i, j int) bool {
		ii := versions[i]
		jj := versions[j]
		if ii.name == jj.name {
			return ii.version < jj.version
		}
		return ii.name < jj.name
	})

	f, err := os.Create(filepath.Join(baseDir, "versions"))
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
	}()

	w := bufio.NewWriter(f)
	for _, e := range versions {
		// TODO: change users of "versions" file to not rely on this exact formatting (version
		// starting at column 51). E.g. this doesn't handle very well packages with large names.
		_, err = fmt.Fprintf(w, "%-50s%s\n", e.name, e.version)
		if err != nil {
			return err
		}
	}
	return w.Flush()
}

func updateOSReleaseFile(b *Builder, filename, version string, homeURL string, upstreamVer string) error {
	//Replace the default os-release file if customized os-release file found
	var err error
	var f *os.File

	if b.Config.Mixer.OSReleasePath != "" {
		err = os.MkdirAll(filepath.Dir(filename), 0655)
		if err != nil {
			return err
		}
		f, err = os.Open(b.Config.Mixer.OSReleasePath)
	} else {
		if _, err = os.Stat(filename); os.IsNotExist(err) {
			return createOSReleaseFile(filename, homeURL, version, upstreamVer)
		}
		if err != nil {
			return err
		}
		f, err = os.Open(filename)
	}

	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
	}()
	fmt.Println("... updating os-release file")

	var newBuf bytes.Buffer
	buildIDFlag := true
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		text := scanner.Text()
		if strings.HasPrefix(text, "VERSION_ID=") {
			text = "VERSION_ID=" + version
		} else if strings.HasPrefix(text, "BUILD_ID=") {
			text = "BUILD_ID=" + upstreamVer
			buildIDFlag = false
		}
		_, err = fmt.Fprintln(&newBuf, text)
		if err != nil {
			return err
		}
	}
	if buildIDFlag {
		_, err = fmt.Fprintln(&newBuf, "BUILD_ID="+upstreamVer)
		if err != nil {
			return err
		}
	}

	err = scanner.Err()
	if err != nil {
		return err
	}

	return ioutil.WriteFile(filename, newBuf.Bytes(), 0644)
}

func merge(a []string, b ...string) []string {
	var result []string
	result = append(result, a...)
	result = append(result, b...)
	return result
}

// getClosestAncestorOwner returns the owner uid/gid of the closest existing
// ancestor of the file or dir pointed to by path. There is no fully cross-
// platform concept of "ownership", so this method only works on *nix systems.
func getClosestAncestorOwner(path string) (int, int, error) {
	path = filepath.Dir(path)
	fi, err := os.Stat(path)
	if os.IsNotExist(err) {
		return getClosestAncestorOwner(path)
	} else if err != nil {
		return 0, 0, err
	}
	uid := int(fi.Sys().(*syscall.Stat_t).Uid)
	gid := int(fi.Sys().(*syscall.Stat_t).Gid)
	return uid, gid, nil
}

func createOSReleaseFile(filename string, homeURL string, version string, upstreamVer string) error {
	fmt.Println("... creating os-release file")

	var f *os.File
	err := os.MkdirAll(filepath.Dir(filename), 0655)
	if err != nil {
		return err
	}
	f, err = os.Create(filename)
	if err != nil {
		return err
	}

	err = f.Chmod(0644)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintln(f, "NAME="+"\""+"Clear Linux OS"+"\"")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(f, "ID="+"clear-linux-os")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(f, "ID_LIKE="+"clear-linux-os")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(f, "VERSION_ID="+version)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(f, "PRETTY_NAME="+"\""+"Clear Linux OS"+"\"")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(f, "ANSI_COLOR="+"\""+"1;35"+"\"")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(f, "HOME_URL="+"\""+homeURL+"\"")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(f, "SUPPORT_URL="+"")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(f, "BUG_REPORT_URL="+"")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(f, "PRIVACY_POLICY_URL="+"")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(f, "BUILD_ID="+upstreamVer)
	if err != nil {
		return err
	}

	err = f.Close()

	return err
}
