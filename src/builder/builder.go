package builder

import (
	"bytes"
	"crypto/rsa"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"helpers"
)

// A Builder contains all configurable fields required to perform a full mix
// operation, and is used to encapsulate life time data.
type Builder struct {
	Buildscript string
	Buildconf   string

	Bundledir   string
	Cert        string
	Clearver    string
	Format      string
	Mixver      string
	Repodir     string
	Rpmdir      string
	Statedir    string
	Versiondir  string
	Yumconf     string
	Yumtemplate string

	Signing int
	Bump    int
}

// New will return a new instance of Builder with some predetermined sane
// default values.
func New() *Builder {
	return &Builder{
		Buildscript: "bundle-chroot-builder.py",
		Yumtemplate: "/usr/share/defaults/mixer/yum.conf.in",

		Signing: 1,
		Bump:    0,
	}
}

// Get provides a useful wrapper function to pull a named field from the Builder
// instance through reflection, i.e. in assisting with parsing config files.
func (b *Builder) Get(name string) string {
	return reflect.ValueOf(b).Elem().FieldByName(name).String()
}

// CheckDeps will perform host validation to ensure that mixer has all programs
// that are required during our lifetime, available at startup.
// Missing dependencies are fatal, so we bail early to ensure we have access
// to them.
func (b *Builder) CheckDeps() bool {
	deps := []string{
		"createrepo_c",
		"git",
		"hardlink",
		"m4",
		"openssl",
		"parallel",
		"rpm",
		"yum",
	}
	for _, i := range deps {
		if _, err := exec.LookPath(i); err != nil {
			helpers.PrintError(err)
			fmt.Fprintf(os.Stderr, "ERROR: Failed to find package \"%s\"\n", i)
			return false
		}
	}
	return true
}

// LoadBuilderConf will read the builder configuration from the command line if
// it was provided, otherwise it will fall back to reading the configuration from
// the local builder.conf file.
func (b *Builder) LoadBuilderConf(builderconf string) {
	local, err := os.Getwd()
	if err != nil {
		helpers.PrintError(err)
		os.Exit(1)
	}

	// If builderconf is set via cmd line, use that one
	if len(builderconf) > 0 {
		b.Buildconf = builderconf
		return
	}

	// Check if there's a local builder.conf if one wasn't supplied
	localpath := local + "/builder.conf"
	if _, err := os.Stat(localpath); err == nil {
		b.Buildconf = localpath
	} else {
		helpers.PrintError(err)
		fmt.Println("ERROR: Cannot find any builder.conf to use!")
		os.Exit(1)
	}
}

// ReadBuilderConf will populate the configuration data from the builder
// configuration file, which is mandatory information for performing a mix.
func (b *Builder) ReadBuilderConf() {
	lines, err := helpers.ReadFileAndSplit(b.Buildconf)
	if err != nil {
		fmt.Println("ERROR: Failed to read buildconf")
		os.Exit(1)
	}

	// Map the builder values to the regex here to make it easier to assign
	fields := []struct {
		re   string
		dest *string
	}{
		{`^BUNDLE_DIR\s*=\s*`, &b.Bundledir},
		{`^CERT\s*=\s*`, &b.Cert},
		{`^CLEARVER\s*=\s*`, &b.Clearver},
		{`^FORMAT\s*=\s*`, &b.Format},
		{`^MIXVER\s*=\s*`, &b.Mixver},
		{`^REPODIR\s*=\s*`, &b.Repodir},
		{`^RPMDIR\s*=\s*`, &b.Rpmdir},
		{`^SERVER_STATE_DIR\s*=\s*`, &b.Statedir},
		{`^VERSIONS_PATH\s*=\s*`, &b.Versiondir},
		{`^YUM_CONF\s*=\s*`, &b.Yumconf},
	}

	for _, h := range fields {
		r := regexp.MustCompile(h.re)
		for _, i := range lines {
			if m := r.FindIndex([]byte(i)); m != nil {
				*h.dest = i[m[1]:]
			}
		}
	}
}

// ReadVersions will initialise the mix versions (mix and clearlinux) from
// the configuration files in the version directory.
func (b *Builder) ReadVersions() {
	ver, err := ioutil.ReadFile(b.Versiondir + "/.mixversion")
	if err != nil {
		helpers.PrintError(err)
		os.Exit(1)
	}
	b.Mixver = strings.TrimSpace(string(ver))

	ver, err = ioutil.ReadFile(b.Versiondir + "/.clearversion")
	if err != nil {
		helpers.PrintError(err)
		os.Exit(1)
	}
	b.Clearver = string(ver)
}

// SignManifestMOM will sign the Manifest.Mom file in in place based on the Mix
// version read from builder.conf.
// Shelling out to openssl because signing and pkcs7 stuff is not well supported
// in Go yet.. but the command works well and is how things worked previously
func (b *Builder) SignManifestMOM() {
	manifestMOM := b.Statedir + "/www/" + b.Mixver + "/Manifest.MoM"
	manifestMOMsig := manifestMOM + ".sig"
	cmd := exec.Command("openssl", "smime", "-sign", "-binary", "-in", manifestMOM,
		"-signer", b.Cert, "-inkey", "private.pem",
		"-outform", "DER", "-out", manifestMOMsig)

	// OpenSSL gives us useful info here so capture it if needed
	var out bytes.Buffer
	cmd.Stderr = &out
	err := cmd.Run()
	if err != nil {
		fmt.Println("ERROR: Failed to sign Manifest.MoM!")
		fmt.Printf("%s\n", out.String())
		helpers.PrintError(err)
	}
	fmt.Println("Signed Manifest.MoM")
}

// UpdateRepo will fetch the clr-bundles for our configured Clear Linux version
func (b *Builder) UpdateRepo(ver string, allbundles bool) {
	// Make the folder to store all clr-bundles version
	if _, err := os.Stat("clr-bundles"); err != nil {
		os.Mkdir("clr-bundles", 0777)
	}

	repo := "clr-bundles/clr-bundles-" + ver + ".tar.gz"
	if _, err := os.Stat(repo); err == nil {
		fmt.Println("Already downloaded " + repo)
		return
	}

	URL := "https://github.com/clearlinux/clr-bundles/archive/" + ver + ".tar.gz"
	err := helpers.Download(repo, URL)
	if err != nil {
		fmt.Println("ERROR: Failed to download new clr-bundles, make sure the version is valid")
		os.Exit(1)
	}

	// FIXME: Maybe use Go's tar or compress packages to do this
	_, err = exec.Command("tar", "-xzf", repo, "-C", "clr-bundles/").Output()
	bundles := b.Bundledir
	if _, err := os.Stat(bundles); os.IsNotExist(err) {
		clrbundles := "clr-bundles/clr-bundles-" + ver + "/bundles/"
		os.Mkdir(bundles, 0777)
		// Copy all bundles over into mix-bundles if -all passed
		if allbundles == true {
			files, err := ioutil.ReadDir("clr-bundles/clr-bundles-" + ver + "/bundles/")
			if err != nil {
				helpers.PrintError(err)
				os.Exit(1)
			}
			for _, file := range files {
				helpers.CopyFile(bundles+"/"+file.Name(), clrbundles+file.Name())
			}
		} else {
			// Install only a minimal set of bundles
			fmt.Println("Adding os-core, os-core-update, kernel-native, bootloader to mix-bundles...")
			helpers.CopyFile(bundles+"/os-core", clrbundles+"os-core")
			helpers.CopyFile(bundles+"/os-core-update", clrbundles+"os-core-update")
			helpers.CopyFile(bundles+"/kernel-native", clrbundles+"kernel-native")
			helpers.CopyFile(bundles+"/bootloader", clrbundles+"bootloader")
		}

		// Save current dir so we can get back to it
		curr, err := os.Getwd()
		if err != nil {
			helpers.PrintError(err)
			os.Exit(1)
		}
		os.Chdir(b.Bundledir)
		helpers.GitInit()
		helpers.GitAdd()
		helpers.GitCommit("Initial Mix version " + b.Mixver)
		os.Chdir(curr)
	}

	fmt.Println("Downloaded " + repo)
}

// InitMix will initialise a new swupd-client consumable "mix" with the given
// based Clear Linux version and specified mix version.
func (b *Builder) InitMix(clearver string, mixver string, all bool) error {
	if clearver == "0" || mixver == "0" {
		fmt.Println("ERROR: Please supply -clearver and -mixver")
		os.Exit(1)
	}
	err := ioutil.WriteFile(b.Versiondir+"/.clearversion", []byte(clearver), 0644)
	if err != nil {
		helpers.PrintError(err)
		os.Exit(1)
	}
	b.Mixver = mixver

	err = ioutil.WriteFile(b.Versiondir+"/.mixversion", []byte(mixver), 0644)
	if err != nil {
		helpers.PrintError(err)
		os.Exit(1)
	}
	b.Clearver = clearver

	b.UpdateRepo(clearver, all)

	return nil
}

// UpdatMixVer automatically bumps the mixversion file +10 to prepare for the next build
// without requiring user intervention. This makes the flow slightly more automatable.
func (b *Builder) UpdateMixVer() {
	mixver, _ := strconv.Atoi(b.Mixver)
	err := ioutil.WriteFile(b.Versiondir+"/.mixversion", []byte(strconv.Itoa(mixver+10)), 0644)
	if err != nil {
		helpers.PrintError(err)
		os.Exit(1)
	}
}

// BuildChroots will attempt to construct the chroots required by populating roots
// using the m4 bundle configurations in conjunction with the YUM configuration file,
// installing all required named packages into the roots.
func (b *Builder) BuildChroots(template *x509.Certificate, privkey *rsa.PrivateKey, signflag bool) error {
	// Generate the yum config file if it does not exist.
	// This takes the template and adds the relevant local rpm repo path if needed
	fmt.Println("Building chroots..")
	if _, err := os.Stat(b.Yumconf); os.IsNotExist(err) {
		outfile, err := os.Create(b.Yumconf)
		if err != nil {
			helpers.PrintError(err)
			panic(err)
		}
		defer outfile.Close()
		if b.Repodir == "" {
			cmd := exec.Command("m4", b.Yumtemplate)
			cmd.Stdout = outfile
			cmd.Run()

		} else {
			cmd := exec.Command("m4", "-D", "MIXER_REPO", "-D", "MIXER_REPOPATH="+b.Repodir, b.Yumtemplate)
			cmd.Stdout = outfile
			cmd.Run()
		}
		outfile.Close()
		if err != nil {
			helpers.PrintError(err)
			return err
		}
	}

	// If MIXVER already exists, wipe it so it's a fresh build
	if _, err := os.Stat(b.Statedir + "/image/" + b.Mixver); err == nil {
		fmt.Printf("Wiping away previous version %s...\n", b.Mixver)
		err = os.RemoveAll(b.Statedir + "/www/" + b.Mixver)
		if err != nil {
			return err
		}
		err = os.RemoveAll(b.Statedir + "/image/" + b.Mixver)
		if err != nil {
			return err
		}
	}

	// If this is a mix, we need to build with the Clear version, but publish the mix version
	chrootcmd := exec.Command(b.Buildscript, "-c", b.Buildconf, "-m", b.Mixver, b.Clearver)
	chrootcmd.Stdout = os.Stdout
	chrootcmd.Stderr = os.Stderr
	err := chrootcmd.Run()
	if err != nil {
		helpers.PrintError(err)
		return err
	}

	// Generate the certificate needed for signing verification if it does not exist and insert it into the chroot
	if signflag == false && template != nil {
		err = helpers.GenerateCertificate(b.Cert, template, template, &privkey.PublicKey, privkey)
		if err != nil {
			return err
		}
	}

	// Only copy the certificate into the mix if it exists
	if _, err := os.Stat(b.Cert); err == nil {
		certdir := b.Statedir + "/image/" + b.Mixver + "/os-core-update/usr/share/clear/update-ca"
		err = os.MkdirAll(certdir, 0755)
		if err != nil {
			helpers.PrintError(err)
			return err
		}
		chrootcert := certdir + "/Swupd_Root.pem"
		fmt.Println("Copying Certificate into chroot...")
		err = helpers.CopyFile(chrootcert, b.Cert)
		if err != nil {
			helpers.PrintError(err)
			return err
		}
	}

	// TODO: Remove all the files-* entries since they're now copied into the noship dir
	// do code stuff here

	return nil
}

// Set the published versions to what was just built
func (b *Builder) setVersion(publish bool) {
	if publish == false {
		return
	}

	// Create the www/version/format# dir if it doesn't exist
	formatdir := b.Statedir + "/www/version/format" + b.Format
	if _, err := os.Stat(formatdir); os.IsNotExist(err) {
		os.MkdirAll(formatdir, 0777)
	}

	fmt.Println("Setting latest version to " + b.Mixver)
	err := ioutil.WriteFile(formatdir+"/latest", []byte(b.Mixver), 0644)
	if err != nil {
		helpers.PrintError(err)
		os.Exit(1)
	}

	err = ioutil.WriteFile(b.Statedir+"/image/LAST_VER", []byte(b.Mixver), 0644)
	if err != nil {
		helpers.PrintError(err)
		os.Exit(1)
	}

}

// CleanChroots will remove chroots based on what bundles are defined
func (b *Builder) CleanChroots() {
	files := helpers.GetDirContents(b.Bundledir)
	basedir := b.Statedir + "/image/" + b.Mixver + "/"

	for _, f := range files {
		if f.Name() == "full" {
			continue
		}
		err := os.RemoveAll(basedir + f.Name())
		if err != nil {
			helpers.PrintError(err)
		}
	}
}

// BuildUpdate will produce an update consumable by the swupd client
func (b *Builder) BuildUpdate(prefixflag string, minvflag int, formatflag string, signflag bool, publishflag bool, keepchrootsflag bool) error {
	if formatflag != "" {
		b.Format = formatflag
	}

	if _, err := os.Stat(b.Statedir + "www/version/format" + b.Format); os.IsNotExist(err) {
		os.Mkdir(b.Statedir+"www/version/format"+b.Format, 0777)
	}

	// Step 1: create update content for the current mix
	updatecmd := exec.Command(prefixflag+"swupd_create_update", "-S", b.Statedir, "--minversion", strconv.Itoa(minvflag), "-F", b.Format, "--osversion", b.Mixver)
	updatecmd.Stdout = os.Stdout
	updatecmd.Stderr = os.Stderr
	err := updatecmd.Run()
	if err != nil {
		helpers.PrintError(err)
		return err
	}

	// We only need the full chroot from this point on, so cleanup the others to save space
	if keepchrootsflag == false {
		b.CleanChroots()
	}

	// Step 1.5: sign the Manifest.MoM that was just created
	if signflag == false {
		b.SignManifestMOM()
	}

	// Step 2: create fullfiles
	output, err := exec.Command(prefixflag+"swupd_make_fullfiles", "-S", b.Statedir, b.Mixver).Output()
	if err != nil {
		helpers.PrintError(err)
		return err
	}
	fmt.Println(string(output))

	// Step 3: create zero packs
	if prefixflag == "" {
		output, err = exec.Command("mixer-pack-maker.sh", "--to", b.Mixver, "-S", b.Statedir).Output()
	} else {
		output, err = exec.Command("mixer-pack-maker.sh", "--to", b.Mixver, "-S", b.Statedir, "--repodir", prefixflag).Output()
	}
	if err != nil {
		helpers.PrintError(err)
		return err
	}
	fmt.Println(string(output))

	// Step 4: hardlink relevant dirs
	_, err = exec.Command("hardlink", "-f", b.Statedir+"/image/"+b.Mixver+"/").Output()

	// Step 5: update the latest version
	b.setVersion(publishflag)

	return nil
}

// BuildImage will now proceed to build the full image with the previously
// validated configuration.
func (b *Builder) BuildImage(format string) {
	// If the user did not pass in a format, default to builder.conf
	if format == "" {
		format = b.Format
	}

	content := "file://" + b.Statedir + "/www"
	imagecmd := exec.Command("ister.py", "-t", "release-image-config.json", "-V", content, "-C", content, "-f", format, "-s", b.Cert)
	imagecmd.Stdout = os.Stdout
	imagecmd.Stderr = os.Stderr

	err := imagecmd.Run()
	if err != nil {
		helpers.PrintError(err)
		fmt.Println("Failed to create image, check /var/log/ister")
	}
}

// AddRPMList copies rpms into the repodir and calls createrepo_c on it to
// generate a yum-consumable repository for the chroot builder to use.
func (b *Builder) AddRPMList(rpms []os.FileInfo) {
	for _, rpm := range rpms {
		if err := helpers.CheckRPM(b.Rpmdir + "/" + rpm.Name()); err != nil {
			fmt.Println("ERROR: RPM is not valid! Please make sure it was built correctly.")
			os.Exit(1)
		} else {
			if _, err = os.Stat(b.Repodir + "/" + rpm.Name()); err == nil {
				continue
			}
		}
		fmt.Printf("Hardlinking %s to repodir\n", rpm.Name())
		err := os.Link(b.Rpmdir+"/"+rpm.Name(), b.Repodir+"/"+rpm.Name())
		if err != nil {
			err = helpers.CopyFile(b.Repodir+"/"+rpm.Name(), b.Rpmdir+"/"+rpm.Name())
			if err != nil {
				helpers.PrintError(err)
				os.Exit(1)
			}
		}
	}
	// Save current dir so we can get back to it
	curr, err := os.Getwd()
	if err != nil {
		helpers.PrintError(err)
		os.Exit(1)
	}
	os.Chdir(b.Repodir)
	createcmd := exec.Command("createrepo_c", ".")
	createcmd.Stdout = os.Stdout
	createcmd.Stderr = os.Stderr
	err = createcmd.Run()
	if err != nil {
		helpers.PrintError(err)
		os.Exit(1)
	}
	os.Chdir(curr)
}
