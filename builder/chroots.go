package builder

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/clearlinux/mixer-tools/helpers"
	"github.com/go-ini/ini"
	"github.com/pkg/errors"
)

// TODO: Move this to the more general configuration handling.
type buildChrootsConfig struct {
	// [Server] section.
	HasServerSection bool
	DebugInfoBanned  string
	DebugInfoLib     string
	DebugInfoSrc     string

	// [swupd] section.
	UpdateBundle string
	ContentURL   string
	VersionURL   string
	// Format is already in b.Format.
}

// TODO: Move this to the more general configuration handling.
func readBuildChrootsConfig(path string) (*buildChrootsConfig, error) {
	iniFile, err := ini.InsensitiveLoad(path)
	if err != nil {
		return nil, err
	}

	cfg := &buildChrootsConfig{}

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

func (b *Builder) buildBundleChroots(set *bundleSet, packager string) error {
	var err error

	if b.Statedir == "" {
		return errors.Errorf("invalid empty state dir")
	}

	chrootDir := filepath.Join(b.Statedir, "image")

	// TODO: Remove remaining references to outputDir. Let "build update" take care of
	// bootstraping or cleaning up.
	outputDir := filepath.Join(b.Statedir, "www")

	if _, ok := set.Bundles["os-core"]; !ok {
		return fmt.Errorf("os-core bundle not found")
	}

	// Bootstrap the directories.
	err = os.MkdirAll(filepath.Join(chrootDir, "0"), 0755)
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
	cfg, err := readBuildChrootsConfig(b.Buildconf)
	if err != nil {
		return err
	}

	if _, ok := set.Bundles[cfg.UpdateBundle]; !ok {
		return fmt.Errorf("couldn't find bundle %q specified in configuration as the update bundle", cfg.UpdateBundle)
	}

	// Write INI files. These are used to communicate to the next step of mixing (build update).
	var serverINI bytes.Buffer
	fmt.Fprintf(&serverINI, `[Server]
emptydir=%s/empty
imagebase=%s/image/
outputdir=%s/www/
`, b.Statedir, b.Statedir, b.Statedir)
	if cfg.HasServerSection {
		fmt.Fprintf(&serverINI, `
[Debuginfo]
banned=%s
lib=%s
src=%s
`, cfg.DebugInfoBanned, cfg.DebugInfoLib, cfg.DebugInfoSrc)
	}
	err = ioutil.WriteFile(filepath.Join(b.Statedir, "server.ini"), serverINI.Bytes(), 0644)
	if err != nil {
		return err
	}
	// TODO: If we are using INI files that are case insensitive, we need to be more restrictive
	// in bundleset to check for that. See also readGroupsINI in swupd package.
	var groupsINI bytes.Buffer
	for _, bundle := range set.Bundles {
		fmt.Fprintf(&groupsINI, "[%s]\ngroup=%s\n\n", bundle.Name, bundle.Name)
	}
	err = ioutil.WriteFile(filepath.Join(b.Statedir, "groups.ini"), groupsINI.Bytes(), 0644)
	if err != nil {
		return err
	}

	// Mixer is used to create both Clear Linux or a mix of it.
	var version string
	if b.Mixver != "" {
		fmt.Printf("Creating chroots for version %s based on Clear Linux %s\n", b.Mixver, b.Clearver)
		version = b.Mixver
	} else {
		fmt.Printf("Creating chroots for version %s\n", b.Clearver)
		version = b.Clearver
		// TODO: This validation should happen when reading the configuration.
		if version == "" {
			return errors.Errorf("no Mixver or Clearver set, unable to proceed")
		}
	}

	chrootVersionDir := filepath.Join(chrootDir, version)
	fmt.Printf("Preparing new %s\n", chrootVersionDir)
	fmt.Printf("  and yum config: %s\n", b.Yumconf)

	err = os.MkdirAll(chrootVersionDir, 0755)
	if err != nil {
		return err
	}
	for name, bundle := range set.Bundles {
		// TODO: Should we embed this information in groups.ini? (Maybe rename it to bundles.ini)
		var includes bytes.Buffer
		for _, inc := range bundle.DirectIncludes {
			fmt.Fprintf(&includes, "%s\n", inc)
		}
		err = ioutil.WriteFile(filepath.Join(chrootVersionDir, name+"-includes"), includes.Bytes(), 0644)
		if err != nil {
			return err
		}
	}

	fmt.Println("Creating os-core chroot")
	osCoreDir := filepath.Join(chrootVersionDir, "os-core")
	err = os.MkdirAll(filepath.Join(osCoreDir, "var/lib/rpm"), 0755)
	if err != nil {
		return err
	}

	fmt.Println("Initializing RPM database")
	err = helpers.RunCommandSilent(
		"rpm",
		"--root", osCoreDir,
		"--initdb",
	)
	if err != nil {
		return err
	}

	// TODO: Check then change to always use dnf. See https://github.com/clearlinux/mixer-tools/issues/115.
	yumCmd := []string{
		"yum",
		"--config=" + b.Yumconf,
		"-y",
		"--releasever=" + b.Clearver,
	}
	if packager == "" {
		// When in Fedora, call dnf instead of yum.
		if osInfo, oerr := readOSInfo(); oerr == nil {
			if osInfo.ID == "fedora" {
				// TODO: Simplify this when we can assume all Fedora users will be >= 22?
				versionID, _ := strconv.Atoi(osInfo.VersionID)
				if versionID >= 22 {
					yumCmd[0] = "dnf"
				}
			}
		}
	} else {
		yumCmd[0] = packager
	}

	fmt.Printf("Packager command-line: %s\n", strings.Join(yumCmd, " "))

	fmt.Println("Installing filesystem package in os-core")
	installArgs := merge(yumCmd,
		"--installroot="+osCoreDir,
		"install",
		"filesystem",
	)
	err = helpers.RunCommandSilent(installArgs[0], installArgs[1:]...)
	if err != nil {
		return err
	}

	clearDir := filepath.Join(osCoreDir, "usr/share/clear")
	err = os.MkdirAll(filepath.Join(clearDir, "bundles"), 0755)
	if err != nil {
		return err
	}

	fmt.Println("Installing packages in os-core")
	err = installPackagesToBundleChroot(yumCmd, chrootVersionDir, set.Bundles["os-core"])
	if err != nil {
		return err
	}

	// Writing special files identifying the version in os-core.
	err = ioutil.WriteFile(filepath.Join(clearDir, "version"), []byte(version), 0644)
	if err != nil {
		return err
	}
	// TODO: This seems to be the only thing that makes two consecutive chroots of the same
	// version to be different. Use SOURCE_DATE_EPOCH if available?
	versionstamp := fmt.Sprint(time.Now().Unix())
	err = ioutil.WriteFile(filepath.Join(clearDir, "versionstamp"), []byte(versionstamp), 0644)
	if err != nil {
		return err
	}

	fmt.Println("Creating 'versions' file")
	err = createVersionsFile(chrootVersionDir, yumCmd)
	if err != nil {
		return errors.Wrapf(err, "couldn't create the versions file")
	}

	err = fixOSRelease(filepath.Join(osCoreDir, "usr/lib/os-release"), version)
	if err != nil {
		return errors.Wrap(err, "couldn't fix os-release file")
	}

	fmt.Println("Creating chroots for bundles")
	var wg sync.WaitGroup

	for _, b := range set.Bundles {
		if b.Name == "os-core" {
			continue
		}
		// Only add here since os-core may be skipped
		wg.Add(1)
		go func(bundle *bundle) {
			defer wg.Done()
			fmt.Printf("Creating %s chroot\n", bundle.Name)
			err = helpers.RunCommandSilent("cp", "-a", "--preserve=all", osCoreDir, filepath.Join(chrootVersionDir, bundle.Name))
			if err != nil {
				//return err
			}

			fmt.Printf("Installing packages to %s\n", bundle.Name)
			err = installPackagesToBundleChroot(yumCmd, chrootVersionDir, bundle)
			if err != nil {
				//return err
			}

			err = cleanBundleChroot(chrootVersionDir, bundle)
			if err != nil {
				//return err
			}
		}(b)
	}
	wg.Wait()
	fmt.Printf("Adding swupd default values to '%s' bundle\n", cfg.UpdateBundle)
	swupdDir := filepath.Join(chrootVersionDir, cfg.UpdateBundle, "usr/share/defaults/swupd")
	err = os.MkdirAll(swupdDir, 0755)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(filepath.Join(swupdDir, "contenturl"), []byte(cfg.ContentURL), 0644)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(filepath.Join(swupdDir, "versionurl"), []byte(cfg.VersionURL), 0644)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(filepath.Join(swupdDir, "format"), []byte(b.Format), 0644)
	if err != nil {
		return err
	}

	// Cleaning os-core bundles after all the other bundles already used it for bootstrapping.
	err = cleanBundleChroot(chrootVersionDir, set.Bundles["os-core"])
	if err != nil {
		return err
	}

	err = os.RemoveAll(filepath.Join(outputDir, version))
	if err != nil {
		return err
	}

	// TODO: Remove usage of noship directory. The image/NN is not shipped as part of the
	// update, so the files can remain where they are.
	fmt.Println("Copying files to noship/ directory")
	noshipDir := filepath.Join(chrootVersionDir, "noship")
	err = os.MkdirAll(noshipDir, 0755)
	if err != nil {
		return err
	}
	fis, err := ioutil.ReadDir(chrootVersionDir)
	if err != nil {
		return err
	}
	for _, fi := range fis {
		name := fi.Name()
		if !strings.HasPrefix(name, "packages-") && !strings.HasSuffix(name, "-includes") {
			continue
		}
		err = helpers.CopyFile(filepath.Join(noshipDir, name), filepath.Join(chrootVersionDir, name))
		if err != nil {
			return err
		}
	}
	return nil
}

// createVersionsFile creates a file that contains all the packages available for a specific
// version. It uses one chroot to query information from the repositories using yum.
func createVersionsFile(baseDir string, yumCmd []string) error {
	// TODO: See if we query the list of packages some other way? Yum output is a bit
	// unfriendly, see the workarounds below. When we move to dnf we may have better options.
	args := merge(yumCmd,
		"--installroot="+filepath.Join(baseDir, "os-core"),
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
			// The output for yum list wraps at 80 when lacking information about the
			// terminal, so we workaround by joining the next line and evaluating. See
			// https://bugzilla.redhat.com/show_bug.cgi?id=584525 for the wrapping.
			if scanner.Scan() {
				text = text + scanner.Text()
			} else {
				return fmt.Errorf("couldn't parse line %q from yum list output", text)
			}
			fields = strings.Fields(text)
			if len(fields) != 3 {
				return fmt.Errorf("couldn't parse merged line %q from yum list output", text)
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

type osInfo struct {
	ID        string
	VersionID string
}

// readOSInfo reads the os-release(5) file to collect information about the system.
func readOSInfo() (*osInfo, error) {
	f, err := os.Open("/etc/os-release")
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		f, err = os.Open("/usr/lib/os-release")
		if err != nil {
			return nil, err
		}
	}
	defer func() {
		_ = f.Close()
	}()

	release, err := ini.Load(f)
	if err != nil {
		return nil, err
	}
	section := release.Section("")

	info := &osInfo{
		ID:        section.Key("ID").Value(),
		VersionID: section.Key("VERSION_ID").Value(),
	}
	return info, nil
}

func installPackagesToBundleChroot(yumCmd []string, chrootVersionDir string, bundle *bundle) error {
	baseDir := filepath.Join(chrootVersionDir, bundle.Name)
	args := merge(yumCmd,
		"--installroot="+baseDir,
		"install",
	)
	args = append(args, bundle.AllPackages...)
	err := helpers.RunCommandSilent(args[0], args[1:]...)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(filepath.Join(baseDir, "usr/share/clear/bundles", bundle.Name), nil, 0644)
	if err != nil {
		return err
	}

	// Generate packages-{BUNDLE} file, that contains the list of packages and package versions
	// present in each bundle.
	packages, err := helpers.RunCommandOutput(
		"rpm",
		"--root="+baseDir,
		"-qa",
		"--queryformat", "%{NAME}\t%{SOURCERPM}\n",
	)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(filepath.Join(chrootVersionDir, "packages-"+bundle.Name), packages.Bytes(), 0644)
}

// cleanBundleChroot removes from the chroot files that were used during the chroot creation but
// shouldn't be part of the bundle contents, e.g. temporary files, RPM database, yum cache.
func cleanBundleChroot(chrootVersionDir string, bundle *bundle) error {
	resetDir := func(path string, perm os.FileMode) error {
		err := os.RemoveAll(path)
		if err != nil {
			return err
		}
		err = os.MkdirAll(path, perm)
		if err != nil {
			return err
		}
		// When creating, perm might get filtered with umask, so Chmod it.
		return os.Chmod(path, perm)
	}
	var err error
	baseDir := filepath.Join(chrootVersionDir, bundle.Name)
	err = resetDir(filepath.Join(baseDir, "var/lib"), 0755)
	if err != nil {
		return err
	}
	err = resetDir(filepath.Join(baseDir, "var/cache"), 0755)
	if err != nil {
		return err
	}
	err = resetDir(filepath.Join(baseDir, "var/log"), 0755)
	if err != nil {
		return err
	}
	err = resetDir(filepath.Join(baseDir, "dev"), 0755)
	if err != nil {
		return err
	}
	err = resetDir(filepath.Join(baseDir, "run"), 0755)
	if err != nil {
		return err
	}
	err = resetDir(filepath.Join(baseDir, "tmp"), 01777)
	if err != nil {
		return err
	}
	return nil
}

func merge(a []string, b ...string) []string {
	var result []string
	result = append(result, a...)
	result = append(result, b...)
	return result
}
