package builder

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
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

const rpmDir = "/var/cache/yum/clear/"
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

func addFileAndPath(destination map[string]bool, absPathsToFiles ...string) {
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
			destination[path] = true
		}
		destination[file] = true
	}
}

func addOsCoreSpecialFiles(bundle *bundle) {
	filesToAdd := []string{
		"/usr/lib/os-release",
		"/usr/share/clear/version",
		"/usr/share/clear/versionstamp",
	}

	addFileAndPath(bundle.Files, filesToAdd...)
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

	addFileAndPath(bundle.Files, filesToAdd...)
}

// repoPkgMap is a map of repo names to the packages they provide
type repoPkgMap map[string][]string
type repoRpmMap map[string][]packageMetadata

func resolveFilesForBundle(bundle *bundle, repoPkgs repoPkgMap, packagerCmd []string) error {
	bundle.Files = make(map[string]bool)

	for repo, pkgs := range repoPkgs {
		queryString := merge(packagerCmd, "repoquery", "-l", "--quiet", "--repo", repo)
		queryString = append(queryString, pkgs...)
		outBuf, err := helpers.RunCommandOutputEnv(queryString[0], queryString[1:], []string{"LC_ALL=en_US.UTF-8"})
		if err != nil {
			return err
		}
		for _, f := range strings.Split(outBuf.String(), "\n") {
			if len(f) > 0 {
				addFileAndPath(bundle.Files, resolveFileName(f))
			}
		}
	}

	addFileAndPath(bundle.Files, fmt.Sprintf("/usr/share/clear/bundles/%s", bundle.Name))
	fmt.Printf("Bundle %s\t%d files\n", bundle.Name, len(bundle.Files))
	return nil
}

func resolveFiles(numWorkers int, set bundleSet, bundleRepoPkgs *sync.Map, packagerCmd []string) error {
	var err error
	var wg sync.WaitGroup
	fmt.Printf("Resolving files using %d workers\n", numWorkers)
	wg.Add(numWorkers)
	bundleCh := make(chan *bundle)
	// buffer errorCh so it always has space
	// only one error can be returned to this channel per worker so
	// buffering to numWorkers will make sure we always have space for the
	// errors.
	errorCh := make(chan error, numWorkers)

	fileWorker := func() {
		for bundle := range bundleCh {
			fmt.Printf("processing %s\n", bundle.Name)
			// Resolve files for this bundle, passing it the map of repos to packages
			r, ok := bundleRepoPkgs.Load(bundle.Name)
			if !ok {
				errorCh <- fmt.Errorf("couldn't find %s bundle", bundle.Name)
				break
			}
			e := resolveFilesForBundle(bundle, r.(repoPkgMap), packagerCmd)
			if e != nil {
				// break on the first error we get
				// causes wg.Done to be called and the worker to exit
				// the break is important so we don't overflow the errorCh
				errorCh <- e
				break
			}
		}
		wg.Done()
	}

	// kick off the fileworkers
	for i := 0; i < numWorkers; i++ {
		go fileWorker()
	}

	for _, bundle := range set {
		select {
		case bundleCh <- bundle:
		case err = <-errorCh:
			// break as soon as there is a failure.
			break
		}
	}
	close(bundleCh)
	wg.Wait()

	// an error could happen after all the workers are spawned so check again for an
	// error after wg.Wait() completes.
	if err == nil && len(errorCh) > 0 {
		err = <-errorCh
	}

	return err
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
func parseNoopInstall(installOut string) []packageMetadata {
	// Split out the section between the install list and the transaction Summary.
	parts := strings.Split(installOut, "Installing:\n")
	if len(parts) < 2 {
		// If there is no such section, e.g. dnf fails because there's
		// no matching package (so no "Installing:"), return nil. Real
		// failure will happen later when doing the actual install.
		return nil
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

	return pkgs
}

func repoPkgFromNoopInstall(installOut string) (repoPkgMap, repoRpmMap) {
	repoPkgs := make(repoPkgMap)
	repoRpmMap := make(repoRpmMap)
	// TODO: parseNoopInstall may fail, so consider a way to stop the processing
	// once we find that failure. See how errorCh works in fullfiles.go.
	pkgs := parseNoopInstall(installOut)

	for _, p := range pkgs {
		repoPkgs[p.repo] = append(repoPkgs[p.repo], p.name)
		repoRpmMap[p.repo] = append(repoRpmMap[p.repo], p)
	}
	return repoPkgs, repoRpmMap
}

var fileSystemRpm string

func resolvePackages(numWorkers int, set bundleSet, packagerCmd []string, emptyDir string) *sync.Map {
	var wg sync.WaitGroup
	fmt.Printf("Resolving packages using %d workers\n", numWorkers)
	wg.Add(numWorkers)
	bundleCh := make(chan *bundle)
	// bundleRepoPkgs is a map of bundles -> map of repos -> list of packages
	var bundleRepoPkgs sync.Map

	packageWorker := func() {
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
			bundle.AllRpmPackages = make(map[string]bool)
			// ignore error from the --assumeno install. It is an error every time because
			// --assumeno forces the install command to "abort" and return a non-zero exit
			// status. This exit status is 1, which is the same as any other dnf install
			// error. Fortunately if this is a different error than we expect, it should
			// fail in the actual install to the full chroot.
			outBuf, _ := helpers.RunCommandOutputEnv(queryString[0], queryString[1:], []string{"LC_ALL=en_US.UTF-8"})

			rpm, fullRpm := repoPkgFromNoopInstall(outBuf.String())
			for _, pkgs := range fullRpm {
				// Add packages to bundle's AllPackages
				for _, pkg := range pkgs {
					name := pkg.name + "-" + pkg.version + "." + pkg.arch + ".rpm"
					bundle.AllPackages[pkg.name] = true
					bundle.AllRpmPackages[name] = true
					// to find the fullName of filesystem rpm, as filesystem needs to be extracted first
					if bundle.Name == "os-core" {
						if pkg.name == "filesystem" {
							fileSystemRpm = name
						}
					}
				}
			}

			bundleRepoPkgs.Store(bundle.Name, rpm)

			fmt.Printf("... done with %s\n", bundle.Name)
		}
		wg.Done()
	}
	for i := 0; i < numWorkers; i++ {
		go packageWorker()
	}

	// feed the channel
	for _, bundle := range set {
		bundleCh <- bundle
	}

	close(bundleCh)
	wg.Wait()

	return &bundleRepoPkgs
}

func installFilesystem(chrootDir string, localPath string, packagerCmd []string, downloadRetries int) error {
	var rpmFull string
	var err error

	if fileSystemRpm == "" {
		return nil
	}

	rpmFull = filepath.Join(localPath, fileSystemRpm)
	rpmMap[fileSystemRpm] = true
	if _, err = os.Stat(rpmFull); os.IsNotExist(err) {
		if !Offline {
			rpmPath, err := downloadRpm(packagerCmd, []string{"filesystem"}, chrootDir, downloadRetries)
			if err != nil {
				return err
			}
			rpmFull = filepath.Join(rpmPath, fileSystemRpm)
			if _, err = os.Stat(rpmFull); err != nil {
				fmt.Println("rpm not found: ", rpmFull)
				return err
			}
		} else {
			fmt.Println("rpm not found: ", rpmFull)
			return err
		}
	}

	for i := 0; i < extractRetries; i++ {
		err = extractRpm(chrootDir, rpmFull)
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

func initRPMDB(chrootDir string) error {
	err := os.MkdirAll(filepath.Join(chrootDir, "var/lib/rpm"), 0755)
	if err != nil {
		return err
	}

	return helpers.RunCommandSilent(
		"rpm",
		"--root", chrootDir,
		"--initdb",
	)
}

func buildOsCore(b *Builder, packagerCmd []string, chrootDir, version string) error {
	err := initRPMDB(chrootDir)
	if err != nil {
		return err
	}

	if err := createClearDir(chrootDir, version); err != nil {
		return err
	}

	if err := fixOSRelease(b, filepath.Join(chrootDir, "usr/lib/os-release"), version); err != nil {
		return errors.Wrap(err, "couldn't fix os-release file")
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

func downloadRpm(packagerCmd []string, rpmList []string, baseDir string, downloadRetries int) (string, error) {
	// Retry RPM downloads to avoid timeout failures due to slow network
	_, err := downloadRpms(packagerCmd, rpmList, baseDir, downloadRetries)
	if err != nil {
		return "", nil
	}
	path := baseDir + rpmDir

	fullRpmPath, err := filepath.Glob(filepath.Join(path, "clear-*"))
	if err != nil {
		fmt.Print("cant find path to rpm")
		return "", err
	}
	if len(fullRpmPath) < 0 {
		return "", fmt.Errorf("cant find rpms packages")
	}
	fullRpmPath[0] = fullRpmPath[0] + "/packages"

	return fullRpmPath[0], nil
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
		return fmt.Errorf("rpm2archive cmd failed with %v", err.Error())
	}

	err = tarCmd.Run()
	if err != nil {
		return fmt.Errorf("tarCmd failed with %v", err)
	}

	err = os.Remove(dir + rpmTar)
	if err != nil {
		fmt.Println("failed to remove file")
	}

	return nil
}

func installBundleToFull(packagerCmd []string, baseDir string, bundle *bundle, downloadRetries int, numWorkers int, localPath string) error {
	var err error
	var wg sync.WaitGroup
	rpmCh := make(chan string)

	var errCh error
	var rpmPath string
	var missingRpms []string

	if len(bundle.AllRpmPackages) < numWorkers {
		numWorkers = len(bundle.AllRpmPackages)
	}
	errorCh := make(chan error, numWorkers)
	wg.Add(numWorkers)

	rpmWorker := func() {
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
				errorCh <- e
				break
			}
		}
		wg.Done()
	}

	for rpm := range bundle.AllRpmPackages {
		if rpmMap[rpm] {
			continue
		}
		rpmFull := filepath.Join(localPath, rpm)
		if _, err = os.Stat(rpmFull); os.IsNotExist(err) {
			rpmName := strings.TrimSuffix(rpm, ".rpm")
			missingRpms = append(missingRpms, rpmName)
		}
	}

	if len(missingRpms) > 0 {
		// downloadRpm if not in offline mode, if in offline mode return error
		if !Offline {
			rpmPath, err = downloadRpm(packagerCmd, missingRpms, baseDir, downloadRetries)
			if err != nil {
				return err
			}
		} else {
			return fmt.Errorf("rpms not found: %s ", missingRpms)
		}
	}

	for i := 0; i < numWorkers; i++ {
		go rpmWorker()
	}

	// feed the channel
	for rpm := range bundle.AllRpmPackages {
		if rpmMap[rpm] {
			continue
		}
		rpmFull := filepath.Join(localPath, rpm)
		if _, err = os.Stat(rpmFull); os.IsNotExist(err) {
			rpmFull = filepath.Join(rpmPath, rpm)
		}
		rpmMap[rpm] = true
		select {
		case rpmCh <- rpmFull:
		case errCh = <-errorCh:
			fmt.Println("extraction failed for ", rpm)
			break
		}
		if errCh != nil {
			return errCh
		}
	}

	close(rpmCh)
	wg.Wait()

	bundleDir := filepath.Join(baseDir, "usr/share/clear/bundles")
	err = os.MkdirAll(filepath.Join(bundleDir), 0755)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(filepath.Join(bundleDir, bundle.Name), nil, 0644)
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

func buildFullChroot(b *Builder, set *bundleSet, packagerCmd []string, buildVersionDir, version string, downloadRetries int, numWorkers int) error {
	fmt.Println("Cleaning DNF cache before full install")
	if err := clearDNFCache(packagerCmd); err != nil {
		return err
	}
	fmt.Println("Installing all bundles to full chroot")
	totalBundles := len(*set)
	fullDir := filepath.Join(buildVersionDir, "full")
	err := os.MkdirAll(fullDir, 0755)
	if err != nil {
		return err
	}
	i := 0
	rpmMap = make(map[string]bool)

	if err := installFilesystem(fullDir, b.Config.Mixer.LocalRepoDir, packagerCmd, downloadRetries); err != nil {
		return err
	}

	for _, bundle := range *set {
		i++
		fmt.Printf("[%d/%d] %s\n", i, totalBundles, bundle.Name)

		if err := installBundleToFull(packagerCmd, fullDir, bundle, downloadRetries, numWorkers, b.Config.Mixer.LocalRepoDir); err != nil {
			return err
		}
		// special handling for os-core
		if bundle.Name == "os-core" {
			fmt.Println("... building special os-core content")
			if err := buildOsCore(b, packagerCmd, fullDir, version); err != nil {
				return err
			}
		}
		// special handling for update bundle
		if bundle.Name == b.Config.Swupd.Bundle {
			fmt.Printf("... Adding swupd default values to %s bundle\n", bundle.Name)
			if err := genUpdateBundleSpecialFiles(fullDir, b); err != nil {
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

	// Bootstrap the directories.
	err = os.MkdirAll(filepath.Join(bundleDir, "0"), 0755)
	if err != nil {
		return err
	}
	err = os.MkdirAll(filepath.Join(outputDir, "0"), 0755)
	if err != nil {
		return err
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

	numWorkers := b.NumBundleWorkers
	emptyDir, err := ioutil.TempDir("", "MixerEmptyDirForNoopInstall")
	if err != nil {
		return err
	}
	defer func() {
		_ = os.RemoveAll(emptyDir)
	}()

	// bundleRepoPkgs is a map of bundles -> map of repos -> list of packages
	bundleRepoPkgs := resolvePackages(numWorkers, set, packagerCmd, emptyDir)

	err = resolveFiles(numWorkers, set, bundleRepoPkgs, packagerCmd)
	if err != nil {
		return err
	}

	if osCore, ok := set["os-core"]; ok {
		addOsCoreSpecialFiles(osCore)
	}

	if updateBundle, ok := set[b.Config.Swupd.Bundle]; ok {
		addUpdateBundleSpecialFiles(b, updateBundle)
	}

	for _, bundle := range set {
		err = writeBundleInfo(bundle, filepath.Join(buildVersionDir, bundle.Name+"-info"))
		if err != nil {
			return err
		}
	}

	// install all bundles in the set (including os-core) to the full chroot
	err = buildFullChroot(b, &set, packagerCmd, buildVersionDir, version, downloadRetries, numWorkers)
	if err != nil {
		return err
	}

	// create os-packages file for validation tools
	err = createOsPackagesFile(buildVersionDir)
	if err != nil {
		return err
	}

	// now that all dnf/yum/rpm operations have completed
	// remove all packager state files from chroot
	// This is not a critical step, just to prevent these files from
	// making it into the Manifest.full
	rmDNFStatePaths(filepath.Join(buildVersionDir, "full"))
	return nil
}

// createOsPackagesFile creates a file that contains all the packages mapped to their
// srpm names for use by validation tooling to identify orphaned packages and verify
// there are no file collisions in the build.
func createOsPackagesFile(buildVersionDir string) error {
	fullChroot := filepath.Join(buildVersionDir, "full")
	packages, err := helpers.RunCommandOutput("rpm", "--root="+fullChroot, "-qa", "--queryformat", "%{NAME}\t%{SOURCERPM}\n")
	if err != nil {
		return err
	}

	return ioutil.WriteFile(filepath.Join(buildVersionDir, "os-packages"), packages.Bytes(), 0644)
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

func fixOSRelease(b *Builder, filename, version string) error {

	//Replace the default os-release file if customized os-release file found
	var err error
	var f *os.File
	if b.Config.Mixer.OSReleasePath != "" {
		f, err = os.Open(b.Config.Mixer.OSReleasePath)
	} else {
		f, err = os.Open(filename)
	}

	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
	}()

	var newBuf bytes.Buffer
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		text := scanner.Text()
		if strings.HasPrefix(text, "VERSION_ID=") {
			text = "VERSION_ID=" + version
		}
		_, _ = fmt.Fprintln(&newBuf, text)
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
