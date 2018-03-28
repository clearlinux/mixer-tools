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
	"time"

	"github.com/clearlinux/mixer-tools/helpers"
	"github.com/go-ini/ini"
	"github.com/pkg/errors"
)

// TODO: Move this to the more general configuration handling.
type buildBundlesConfig struct {
	// [Server] section.
	HasServerSection bool
	DebugInfoBanned  string
	DebugInfoLib     string
	DebugInfoSrc     string

	// [swupd] section.
	UpdateBundle string
	ContentURL   string
	VersionURL   string
	// Format is already in b.Config.Swupd.Format.
}

// TODO: Move this to the more general configuration handling.
func readBuildBundlesConfig(path string) (*buildBundlesConfig, error) {
	iniFile, err := ini.InsensitiveLoad(path)
	if err != nil {
		return nil, err
	}

	cfg := &buildBundlesConfig{}

	// TODO: Validate early the fields we read.
	server, err := iniFile.GetSection("Server")
	if err == nil {
		cfg.HasServerSection = true
		cfg.DebugInfoBanned = server.Key("debuginfo_banned").Value()
		cfg.DebugInfoLib = server.Key("debuginfo_lib").Value()
		cfg.DebugInfoSrc = server.Key("debuginfo_src").Value()
	}

	swupd, err := iniFile.GetSection("swupd")
	if err != nil {
		return nil, fmt.Errorf("error in configuration file %s: %s", path, err)
	}

	getKey := func(section *ini.Section, name string) (string, error) {
		key, kerr := section.GetKey(name)
		if kerr != nil {
			return "", fmt.Errorf("error in configuration file %s: %s", path, kerr)
		}
		return key.Value(), nil
	}

	cfg.UpdateBundle, err = getKey(swupd, "BUNDLE")
	if err != nil {
		return nil, err
	}
	cfg.ContentURL, err = getKey(swupd, "CONTENTURL")
	if err != nil {
		return nil, err
	}
	cfg.VersionURL, err = getKey(swupd, "VERSIONURL")
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

var bannedPaths = [...]string{
	"/var/lib/",
	"/var/cache/",
	"/var/log/",
	"/dev/",
	"/run/",
	"/tmp/",
}

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

func resolveFilesForBundle(bundle *bundle, packagerCmd []string) error {
	bundle.Files = make(map[string]bool)

	queryString := merge(packagerCmd, "repoquery", "-l", "--quiet")
	for p := range bundle.AllPackages {
		queryString = append(queryString, p)
	}
	outBuf, err := helpers.RunCommandOutput(queryString[0], queryString[1:]...)
	if err != nil {
		return err
	}
	for _, f := range strings.Split(outBuf.String(), "\n") {
		if len(f) > 0 {
			addFileAndPath(bundle.Files, resolveFileName(f))
		}
	}

	addFileAndPath(bundle.Files, fmt.Sprintf("/usr/share/clear/bundles/%s", bundle.Name))
	fmt.Printf("Bundle %s\t%d files\n", bundle.Name, len(bundle.Files))
	return nil
}

func resolveFiles(numWorkers int, set bundleSet, packagerCmd []string) error {
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
			err = resolveFilesForBundle(bundle, packagerCmd)
			if err != nil {
				// break on the first error we get
				// causes wg.Done to be called and the worker to exit
				// the break is important so we don't overflow the errorCh
				errorCh <- err
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
// Truncated at 80 columns to make it more readable
// The section we care about is everything from "Installing dependencies:" to
// the next blank line.
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
func parseNoopInstall(installOut string) []string {
	pkgDeps := []string{}

	if !strings.Contains(installOut, "Installing dependencies:") {
		return pkgDeps
	}
	// we know it will have at least a length of 2 since the split string exists
	pkgList := strings.Split(installOut, "Installing dependencies:\n")[1]
	pkgList = strings.Split(pkgList, "\n\nTransaction Summary")[0]
	for _, line := range strings.Split(pkgList, "\n") {
		if len(line) == 0 {
			break
		}

		// When not running in a TTY dnf sets terminal line-width to a
		// default 80 characters, then wraps their fields in an
		// unpredictable way to be pleasing for they eye but not for
		// the parser. If the line starts with more than one space (' ')
		// character the previous line was wrapped and continues on
		// this line.  Since we only care about the first field, the
		// package name, just continue to the next line.
		//
		// example lines:
		// ^ really-long-package-name-overflows-field$
		// ^                         x86_64             12-10         clear$
		// ^ reasonablepkgname       x86_64             deadcafedeadbeefdeadcafedeadbeef$
		// ^                                                          clear$
		if strings.HasPrefix(line, "  ") {
			continue
		}

		pkgDeps = append(pkgDeps, strings.Fields(line)[0])
	}
	return pkgDeps
}

func resolvePackages(numWorkers int, set bundleSet, packagerCmd []string, emptyDir string) {
	var wg sync.WaitGroup
	fmt.Printf("Resolving packages using %d workers\n", numWorkers)
	wg.Add(numWorkers)
	bundleCh := make(chan *bundle)

	packageWorker := func() {
		for bundle := range bundleCh {
			fmt.Printf("processing %s\n", bundle.Name)
			packageNames := make(map[string]bool)
			for k := range bundle.AllPackages {
				packageNames[k] = true
			}
			queryString := merge(
				packagerCmd,
				"--installroot="+emptyDir,
				"--assumeno",
				"install",
			)
			for p := range packageNames {
				queryString = append(queryString, p)
			}
			// ignore error from the --assumeno install. It is an error every time because
			// --assumeno forces the install command to "abort" and return a non-zero exit
			// status. This exit status is 1, which is the same as any other dnf install
			// error. Fortunately if this is a different error than we expect, it should
			// fail in the actual install to the full chroot.
			outBuf, _ := helpers.RunCommandOutput(queryString[0], queryString[1:]...)
			depPkgs := parseNoopInstall(outBuf.String())
			for _, d := range depPkgs {
				bundle.AllPackages[d] = true
			}

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
}

func installFilesystem(packagerCmd []string, chrootDir string) error {
	installArgs := merge(packagerCmd,
		"--installroot="+chrootDir,
		"install",
		"filesystem",
	)
	return helpers.RunCommandSilent(installArgs[0], installArgs[1:]...)
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

func buildOsCore(packagerCmd []string, chrootDir, version string) error {
	err := initRPMDB(chrootDir)
	if err != nil {
		return err
	}

	if err := installFilesystem(packagerCmd, chrootDir); err != nil {
		return err
	}

	if err := createClearDir(chrootDir, version); err != nil {
		return err
	}

	if err := fixOSRelease(filepath.Join(chrootDir, "usr/lib/os-release"), version); err != nil {
		return errors.Wrap(err, "couldn't fix os-release file")
	}

	if err := createVersionsFile(filepath.Dir(chrootDir), packagerCmd); err != nil {
		return errors.Wrapf(err, "couldn't create the versions file")
	}

	return nil
}

func genUpdateBundleSpecialFiles(chrootDir string, cfg *buildBundlesConfig, b *Builder) error {
	swupdDir := filepath.Join(chrootDir, "usr/share/defaults/swupd")
	if err := os.MkdirAll(swupdDir, 0755); err != nil {
		return err
	}
	cURLBytes := []byte(cfg.ContentURL)
	if err := ioutil.WriteFile(filepath.Join(swupdDir, "contenturl"), cURLBytes, 0644); err != nil {
		return err
	}
	vURLBytes := []byte(cfg.VersionURL)
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

	return ioutil.WriteFile(filepath.Join(swupdDir, "format"), []byte(b.Config.Swupd.Format), 0644)
}

func installBundleToFull(packagerCmd []string, buildVersionDir string, bundle *bundle) error {
	var err error
	baseDir := filepath.Join(buildVersionDir, "full")
	args := merge(packagerCmd, "--installroot="+baseDir, "install")
	if len(bundle.AllPackages) > 0 {
		// There were packages directly included for this bundle so
		// install to full chroot. This check is necessary so we don't
		// call dnf install with no package listed.
		for p := range bundle.AllPackages {
			args = append(args, p)
		}
		err = helpers.RunCommandSilent(args[0], args[1:]...)
		if err != nil {
			return err
		}
	}

	bundleDir := filepath.Join(baseDir, "usr/share/clear/bundles")
	err = os.MkdirAll(filepath.Join(bundleDir), 0755)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(filepath.Join(bundleDir, bundle.Name), nil, 0644)
}

func clearDNFCache(packagerCmd []string) error {
	args := merge(packagerCmd, "clean", "all")
	return helpers.RunCommandSilent(args[0], args[1:]...)
}

func buildFullChroot(cfg *buildBundlesConfig, b *Builder, set *bundleSet, packagerCmd []string, buildVersionDir, version string) error {
	fmt.Println("Cleaning DNF cache before full install")
	if err := clearDNFCache(packagerCmd); err != nil {
		return err
	}
	fmt.Println("Installing all bundles to full chroot")
	totalBundles := len(*set)
	i := 0
	for _, bundle := range *set {
		i++
		fmt.Printf("[%d/%d] %s\n", i, totalBundles, bundle.Name)
		fullDir := filepath.Join(buildVersionDir, "full")
		// special handling for os-core
		if bundle.Name == "os-core" {
			fmt.Println("... building special os-core content")
			if err := buildOsCore(packagerCmd, fullDir, version); err != nil {
				return err
			}
		}

		if err := installBundleToFull(packagerCmd, buildVersionDir, bundle); err != nil {
			return err
		}

		// special handling for update bundle
		if bundle.Name == cfg.UpdateBundle {
			fmt.Printf("... Adding swupd default values to %s bundle\n", bundle.Name)
			if err := genUpdateBundleSpecialFiles(fullDir, cfg, b); err != nil {
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

func (b *Builder) buildBundles(set bundleSet) error {
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

	// TODO: Do not touch config code that is in flux at the moment, reparsing it here to grab
	// information that previously Mixer didn't care about. Move that to the configuration part
	// of Mixer.
	cfg, err := readBuildBundlesConfig(b.BuildConf)
	if err != nil {
		return err
	}

	if _, ok := set[cfg.UpdateBundle]; !ok {
		return fmt.Errorf("couldn't find bundle %q specified in configuration as the update bundle", cfg.UpdateBundle)
	}

	// Write INI files. These are used to communicate to the next step of mixing (build update).
	var serverINI bytes.Buffer
	fmt.Fprintf(&serverINI, `[Server]
emptydir=%s/empty
imagebase=%s/image/
outputdir=%s/www/
`, b.Config.Builder.ServerStateDir, b.Config.Builder.ServerStateDir, b.Config.Builder.ServerStateDir)
	if cfg.HasServerSection {
		fmt.Fprintf(&serverINI, `
[Debuginfo]
banned=%s
lib=%s
src=%s
`, cfg.DebugInfoBanned, cfg.DebugInfoLib, cfg.DebugInfoSrc)
	}
	err = ioutil.WriteFile(filepath.Join(b.Config.Builder.ServerStateDir, "server.ini"), serverINI.Bytes(), 0644)
	if err != nil {
		return err
	}
	// TODO: If we are using INI files that are case insensitive, we need to be more restrictive
	// in bundleset to check for that. See also readGroupsINI in swupd package.
	var groupsINI bytes.Buffer
	for _, bundle := range set {
		fmt.Fprintf(&groupsINI, "[%s]\ngroup=%s\n\n", bundle.Name, bundle.Name)
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
			fmt.Fprintf(&includes, "%s\n", inc)
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

	resolvePackages(numWorkers, set, packagerCmd, emptyDir)

	err = resolveFiles(numWorkers, set, packagerCmd)
	if err != nil {
		return err
	}

	updateBundle := set[cfg.UpdateBundle]
	var osCore *bundle
	for _, bundle := range set {
		if bundle.Name == "os-core" {
			osCore = bundle
			break
		}
	}
	addOsCoreSpecialFiles(osCore)
	addUpdateBundleSpecialFiles(b, updateBundle)

	for _, bundle := range set {
		err = writeBundleInfo(bundle, filepath.Join(buildVersionDir, bundle.Name+"-info"))
		if err != nil {
			return err
		}
	}

	// install all bundles in the set (including os-core) to the full chroot
	err = buildFullChroot(cfg, b, &set, packagerCmd, buildVersionDir, version)
	if err != nil {
		return err
	}

	// create os-packages file for validation tools
	return createOsPackagesFile(buildVersionDir)
}

// createOsPackagesFile creates a file that contains all the packages mapped to their
// srpm names for use by validation tooling to identify orphaned packages and verify
// there are no file collisions in the build.
func createOsPackagesFile(buildVersionDir string) error {
	fullChroot := filepath.Join(buildVersionDir, "full")
	packages, err := helpers.RunCommandOutput(
		"rpm", "--root="+fullChroot, "-qa", "--queryformat", "%{NAME}\t%{SOURCERPM}\n",
	)
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
		fmt.Fprintf(w, "%-50s%s\n", e.name, e.version)
	}
	return w.Flush()
}

func fixOSRelease(filename, version string) error {
	f, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
	}()

	// TODO: If this is a mix, NAME and ID should probably change too. Create a section in
	// configuration that will be used as reference to fill this.
	// TODO: If this is a mix, add extra field for keeping track of the Clear Linux version
	// used. Maybe also put the UPSTREAM URL, so we are ready to support mixes of mixes.
	//
	// See also: https://github.com/clearlinux/mixer-tools/issues/113

	var newBuf bytes.Buffer
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		text := scanner.Text()
		if strings.HasPrefix(text, "VERSION_ID=") {
			text = "VERSION_ID=" + version
		}
		fmt.Fprintln(&newBuf, text)
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
