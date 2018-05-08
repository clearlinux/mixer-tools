package main

import (
	"crypto/rsa"
	"crypto/x509"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/clearlinux/mixer-tools/builder"
	"github.com/clearlinux/mixer-tools/helpers"
	"github.com/clearlinux/mixer-tools/swupd"
	"github.com/pkg/errors"
)

var mixWS = "/usr/share/mix"

const builderConf = `[Mixer]
LOCAL_BUNDLE_DIR = /usr/share/mix/local-bundles

[Builder]
SERVER_STATE_DIR = /usr/share/mix/update
BUNDLE_DIR = /usr/share/mix/local-bundles
YUM_CONF = /usr/share/mix/.yum-mix.conf
CERT = /usr/share/mix/Swupd_Root.pem
VERSIONS_PATH =/usr/share/mix
LOCAL_RPM_DIR = /usr/share/mix/local-rpms
LOCAL_REPO_DIR = /usr/share/mix/local

[swupd]
BUNDLE=os-core
CONTENTURL=file:///usr/share/mix/update/www
VERSIONURL=file:///usr/share/mix/update/www
FORMAT=1
`

func getCurrentVersion() (int, error) {
	c, err := ioutil.ReadFile("/usr/lib/os-release")
	if err != nil {
		return -1, err
	}

	re := regexp.MustCompile(`(?m)^VERSION_ID=(\d+)\n`)
	m := re.FindStringSubmatch(string(c))
	if len(m) == 0 {
		return -1, errors.New("unable to determine OS version")
	}

	v, err := strconv.Atoi(m[1])
	if err != nil {
		return -1, err
	}

	return v, nil
}

func setUpMixDir(bundle string, upstreamVer, mixVer int) error {
	var err error
	err = os.MkdirAll(filepath.Join(mixWS, "local-rpms"), 755)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(filepath.Join(mixWS, "builder.conf"),
		[]byte(builderConf), 0644)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(filepath.Join(mixWS, "mixversion"),
		[]byte(fmt.Sprintf("%d", mixVer)), 0644)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(filepath.Join(mixWS, "mixbundles"),
		[]byte(fmt.Sprintf("%s\nos-core", bundle)), 0644)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(filepath.Join(mixWS, "upstreamversion"), []byte(fmt.Sprintf("%d", upstreamVer)), 0644)
}

func getLastVersion() int {
	c, err := ioutil.ReadFile(filepath.Join(mixWS, "update/image/LAST_VER"))
	if err != nil {
		return 0
	}

	v, err := strconv.Atoi(string(c))
	if err != nil {
		return 0
	}

	return v
}

func appendToFile(filename, text string) error {
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}

	defer func() {
		_ = f.Close()
	}()

	if _, err = f.WriteString(text); err != nil {
		return err
	}

	return nil
}

func failIfErr(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %s\n", err)
		os.Exit(1)
	}
}

func buildBundles(b *builder.Builder) error {
	var privkey *rsa.PrivateKey
	var template *x509.Certificate

	if _, err := os.Stat(b.Config.Builder.Cert); os.IsNotExist(err) {
		fmt.Println("Generating certificate for signature validation...")
		privkey, err = helpers.CreateKeyPair()
		if err != nil {
			return errors.Wrap(err, "Error generating OpenSSL keypair")
		}
		template = helpers.CreateCertTemplate()
	}

	return errors.Wrap(b.BuildBundles(template, privkey, false), "Error building bundles")
}

func excludeName(man *swupd.Manifest, exclude string) {
	for i := range man.Files {
		if man.Files[i].Name == exclude {
			man.Files = append(man.Files[:i], man.Files[i+1:]...)
			break
		}
	}
}

func mergeMoMs(mixWS string, mixVer, lastVer int) error {
	upstreamMoM, err := swupd.ParseManifestFile(filepath.Join(mixWS, "Manifest.MoM"))
	if err != nil {
		return err
	}

	mixerMoM, err := swupd.ParseManifestFile(
		filepath.Join(mixWS, fmt.Sprintf("update/www/%d/Manifest.MoM", mixVer)))
	if err != nil {
		return err
	}

	// add mixerMoM filecount minus os-core
	upstreamMoM.Header.FileCount += mixerMoM.Header.FileCount - 1
	// need to set previous version, without this mixer version is the currently built version
	if lastVer == 0 {
		upstreamMoM.Header.Previous = upstreamMoM.Header.Version
	} else {
		upstreamMoM.Header.Previous = uint32(mixVer - 10)
	}
	// format is 1 until auto-format-bump support
	upstreamMoM.Header.Format = 1
	// system now on mixVer instead of upstream version
	upstreamMoM.Header.Version = uint32(mixVer)
	// remove os-core entry from upstreamMoM, we will replace with ours
	excludeName(upstreamMoM, "os-core")
	for i := range mixerMoM.Files {
		if mixerMoM.Files[i].Name == "os-core-update-index" {
			continue
		}
		mixerMoM.Files[i].Rename = swupd.MixManifest
		upstreamMoM.Files = append(upstreamMoM.Files, mixerMoM.Files[i])
	}

	return upstreamMoM.WriteManifestFile("Manifest.MoM")
}

func main() {
	var err error

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage of %s:
	%s <bundle-name> <path/to/rpm>`, os.Args[0], os.Args[0])
	}

	flag.Parse()
	if len(flag.Args()) != 2 {
		flag.Usage()
		os.Exit(1)
	}

	args := flag.Args()
	bundle := args[0]
	pkg := args[1]

	ver, err := getCurrentVersion()
	failIfErr(err)
	mixVer := ver * 1000
	if _, err = os.Stat(filepath.Join(mixWS, "builder.conf")); os.IsNotExist(err) {
		err = setUpMixDir(bundle, ver, mixVer)
		failIfErr(err)
	}

	// do this before changing into the mixWS so relative paths can be used
	err = os.Link(pkg, filepath.Join(mixWS, "local-rpms", filepath.Base(pkg)))
	if !os.IsExist(err) {
		failIfErr(err)
	}

	err = os.Chdir(mixWS)
	failIfErr(err)

	b, err := builder.NewFromConfig(filepath.Join(mixWS, "builder.conf"))
	failIfErr(err)
	err = b.InitMix(fmt.Sprintf("%d", ver), fmt.Sprintf("%d", mixVer),
		false, false, "https://download.clearlinux.org", false)
	failIfErr(err)
	b.NumBundleWorkers = runtime.NumCPU()
	b.NumFullfileWorkers = runtime.NumCPU()

	err = b.EditBundles([]string{bundle}, true, true, false)
	failIfErr(err)

	pkgName := filepath.Base(strings.TrimRight(pkg, ".rpm"))
	err = appendToFile(filepath.Join(mixWS, "local-bundles", bundle), fmt.Sprintf("%s\n", pkgName))
	failIfErr(err)

	err = b.AddRPMList([]string{filepath.Base(pkg)})
	failIfErr(err)

	lastVer := getLastVersion()
	if lastVer == 0 {
		// no last version, build the mix
		err = buildBundles(b)
		failIfErr(err)
		err = b.BuildUpdate("", 0, "", false, true, false)
		failIfErr(err)
	} else {
		// older version mix exists, make the mix clean (pre-merge) before building
		oldMix := filepath.Join(mixWS, fmt.Sprintf("update/www/%d", mixVer-10))
		_ = os.Rename(filepath.Join(oldMix, "Manifest.MoM"),
			filepath.Join(oldMix, "FullManifest.MoM"))
		_ = os.Rename(filepath.Join(oldMix, fmt.Sprintf("Manifest.MoM.%d", mixVer-10)),
			filepath.Join(oldMix, "Manifest.MoM"))
		err = buildBundles(b)
		failIfErr(err)
		err = b.BuildUpdate("", 0, "", false, true, false)
		failIfErr(err)
		_ = os.Rename(filepath.Join(oldMix, "Manifest.MoM"),
			filepath.Join(oldMix, fmt.Sprintf("Manifest.MoM.%d", mixVer-10)))
		_ = os.Rename(filepath.Join(oldMix, "FullManifest.MoM"),
			filepath.Join(oldMix, "Manifest.MoM"))
	}

	upstreamMoM := fmt.Sprintf("https://download.clearlinux.org/update/%d/Manifest.MoM", ver)
	err = helpers.Download("Manifest.MoM", upstreamMoM)
	failIfErr(err)
	err = helpers.Download("Manifest.MoM.sig", upstreamMoM+".sig")
	failIfErr(err)

	cert := "/usr/share/ca-certs/Swupd_Root.pem"

	_, err = helpers.RunCommandOutput("openssl", "smime", "-verify", "-in", "Manifest.MoM.sig",
		"-inform", "der", "-content", "Manifest.MoM", "-CAfile", cert)
	failIfErr(err)
	fmt.Println("* Verified upstream Manifest.MoM")

	// merge upstream MoM with mixer MoM
	err = mergeMoMs(mixWS, mixVer, ver)
	failIfErr(err)

	mixDir := filepath.Join(mixWS, fmt.Sprintf("update/www/%d", mixVer))
	_, err = helpers.RunCommandOutput("openssl", "smime", "-sign", "-binary", "-in",
		filepath.Join(mixDir, "Manifest.MoM"),
		"-signer", filepath.Join(mixWS, "Swupd_Root.pem"),
		"-inkey", filepath.Join(mixWS, "private.pem"),
		"-outform", "DER",
		"-out", filepath.Join(mixDir, "Manifest.MoM.sig"))
	failIfErr(err)

	err = os.Rename(filepath.Join(mixDir, "Manifest.MoM"),
		filepath.Join(mixDir, fmt.Sprintf("Manifest.MoM.%d", mixVer)))
	failIfErr(err)
	err = os.Rename("Manifest.MoM", filepath.Join(mixDir, "Manifest.MoM"))
	failIfErr(err)
	_, err = helpers.RunCommandOutput("tar", "-C", mixDir, "-cvf", "Manifest.MoM.tar", "Manifest.MoM")
	failIfErr(err)
	failIfErr(os.Remove("Manifest.MoM.sig"))
}
