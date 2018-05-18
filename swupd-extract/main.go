package main

// TODO: Flag to ignore included bundles (only extract the requested). Still need to parse
// the included to figure out the directories metadata. Skip the packs for includes, just
// download fullfile.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"github.com/clearlinux/mixer-tools/internal/client"
	"github.com/clearlinux/mixer-tools/swupd"
)

func usage() {
	fmt.Printf(`swupd-extract reads a swupd update repository and write it to disk

Usage:

    swupd-extract https://.../version        [bundles...]
    swupd-extract /local/path/to/.../version [bundles...]

The program downloads the content of a specific version in a swupd
repository and extract the content of the specified bundles (groups of
content). If no bundle is specified the program will list the bundles
available. Bundles specified will automatically trigger the extraction
of bundles that they include.

The program extracts the content to a directory called "output" or a
directory set with the -output flag. Intermediate data is saved in a
directory sibling to the output directory called "swupd-state" or a
directory set with the -state flag.

The state directory can hold intermediate data for multiple versions
from the same repository.

There is a shorthand for extracting content from Clear Linux. For this
case the version can be omitted so that the latest version is used:

    swupd-extract clear/20520 [bundles...]
    swupd-extract clear       [bundles...]

If not available locally, the certificate for Clear Linux is
automatically downloaded and verified.

Flags:
`)
	flag.PrintDefaults()
	os.Exit(1)
}

const (
	clearLinuxLatestURL         = "https://download.clearlinux.org/latest"
	clearLinuxBaseContent       = "https://cdn.download.clearlinux.org/update"
	clearLinuxCertificateURL    = "https://download.clearlinux.org/current/Swupd_Root.pem"
	clearLinuxCertificateSHA256 = "ff06fc76ec5148040acb4fcb2bc8105cc72f1963b55de0daf3a4ed664c6fe72c"
)

func main() {
	log.SetFlags(0)

	var (
		outputDir   string
		stateDir    string
		cert        string
		noCache     bool
		noOverwrite bool
	)

	flag.StringVar(&outputDir, "output", "output", "where to extract the files")
	flag.StringVar(&stateDir, "state", "", "directory to store intermediate files")
	flag.StringVar(&cert, "cert", "", "certificate used to verify content")
	flag.BoolVar(&noCache, "no-cache", false, "don't use cached files, force downloads")
	flag.BoolVar(&noOverwrite, "no-overwrite", false, "don't overwrite output files")
	flag.Parse()

	if os.Getuid() != 0 {
		log.Fatal("This program needs to run as root to write files with proper permissions.")
	}

	if len(flag.Args()) == 0 {
		usage()
		return
	}

	// Normalize content argument to not have ending slashes.
	content := flag.Arg(0)
	for content[len(content)-1] == '/' {
		content = content[:len(content)-1]
	}

	if strings.HasPrefix(content, "clear/") || strings.HasPrefix(content, "clearlinux/") {
		parts := strings.SplitN(content, "/", 2)
		content = clearLinuxBaseContent + "/" + parts[1]
	} else if content == "clear" || content == "clearlinux" {
		// Query latest version from Clear Linux.
		res, err := http.Get(clearLinuxLatestURL)
		if err != nil {
			log.Fatalf("ERROR: no version passed and couldn't query latest version of Clear Linux: %s", err)
		}
		if res.StatusCode != http.StatusOK {
			log.Fatalf("ERROR: no version passed and couldn't query latest version of Clear Linux: %s resulted in %d %s", clearLinuxLatestURL, res.StatusCode, http.StatusText(res.StatusCode))
		}
		latestBytes, err := ioutil.ReadAll(res.Body)
		if err != nil {
			log.Fatalf("ERROR: no version passed and couldn't query latest version of Clear Linux: %s", err)
		}
		content = clearLinuxBaseContent + "/" + string(bytes.TrimSpace(latestBytes))
	}

	// Extract baseContent and version information.
	sep := strings.LastIndex(content, "/")
	if sep == -1 {
		log.Fatalf("Invalid URL or directory for content: %s", content)
	}
	baseContent, version := content[:sep], content[sep+1:]

	if stateDir == "" {
		stateDir = filepath.Join(filepath.Dir(outputDir), "swupd-state")
	}

	parsed, err := strconv.ParseUint(version, 10, 32)
	if err != nil {
		log.Fatalf("ERROR: invalid content URL or directory %s: last part must be the version number, but found %q instead", content, version)
	}
	if parsed == 0 {
		log.Fatalf("ERROR: version must be greater than zero")
	}

	var mayDownloadClearLinuxCert bool
	if cert == "" {
		cert = findDefaultCert()
		if cert == "" {
			if baseContent != clearLinuxBaseContent {
				log.Fatalf("ERROR: couldn't find Swupd_Root.pem in current directory and no -cert flag was passed")
			}
			cert = filepath.Join(stateDir, "Swupd_Root.pem")
			mayDownloadClearLinuxCert = true
		}
	}

	fmt.Printf(`» Parameters

  Base content:     %s
  Version:          %s
  Use Cache:        %t
  Certificate:      %s
  State directory:  %s
  Output directory: %s

`, baseContent, version, !noCache, cert, stateDir, outputDir)

	fmt.Printf("» Verifying state directory\n")
	state, err := client.NewState(stateDir, baseContent)
	if err != nil {
		log.Fatalf("ERROR: %s", err)
	}
	state.NoCache = noCache
	state.Verbose = true

	if mayDownloadClearLinuxCert {
		if _, err = os.Stat(cert); err != nil {
			if !os.IsNotExist(err) {
				log.Fatalf("ERROR: couldn't open certificate file: %s", err)
			}
			tempCert := cert + ".temp"
			err = client.Download(clearLinuxCertificateURL, tempCert)
			if err != nil {
				log.Fatalf("ERROR: couldn't download Clear Linux certificate: %s", err)
			}
			var certBytes []byte
			certBytes, err = ioutil.ReadFile(tempCert)
			if err != nil {
				_ = os.Remove(tempCert)
				log.Fatalf("ERROR: couldn't downloaded Clear Linux certificate: %s", err)
			}
			tempSHA256Bytes := sha256.Sum256(certBytes)
			tempSHA256 := hex.EncodeToString(tempSHA256Bytes[:])
			if string(tempSHA256[:]) != clearLinuxCertificateSHA256 {
				_ = os.Remove(tempCert)
				log.Fatalf(`ERROR: verification of downloaded Clear Linux certificate failed:
  got SHA256 hash %q
     but expected %q`, tempSHA256, clearLinuxCertificateSHA256)
			}
			err = os.Rename(tempCert, cert)
			if err != nil {
				log.Fatalf("ERROR: couldn't rename downloaded certificate to its final name: %s", err)
			}
		}
	}

	fmt.Printf("» Reading metadata\n")
	momFile, err := state.GetFile(version, "Manifest.MoM")
	if err != nil {
		log.Fatalf("ERROR: %s", err)
	}
	momSig, err := state.GetFile(version, "Manifest.MoM.sig")
	if err != nil {
		log.Fatalf("ERROR: %s", err)
	}

	err = verifySignature(momFile, momSig, cert)
	if err != nil {
		log.Fatalf("ERROR: couldn't verify Manifest.MoM: %s", err)
	}

	mom, err := swupd.ParseManifestFile(momFile)
	if err != nil {
		log.Fatalf("ERROR: %s", err)
	}

	// If not bundles passed, just show the list of available bundles.
	if len(flag.Args()) == 1 {
		sort.Slice(mom.Files, func(i, j int) bool {
			return mom.Files[i].Name < mom.Files[j].Name
		})
		fmt.Printf("Available bundles in %s\n", content)
		for _, f := range mom.Files {
			fmt.Printf("  %s\n", f.Name)
		}
		return
	}

	requestedBundles := flag.Args()[1:]
	bundleMap, err := resolveBundles(state, mom, requestedBundles)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("» Extracting packs\n")
	for _, b := range bundleMap {
		err = state.GetZeroPack(fmt.Sprint(b.Header.Version), b.Name)
		if err != nil {
			log.Fatalf("ERROR: couldn't get pack for %s: %s", b.Name, err)
		}
	}

	fmt.Printf("» Copying files\n")
	allFileMap := make(map[string]*swupd.File)
	var allFiles []*swupd.File
	for _, b := range bundleMap {
		for _, f := range b.Files {
			if f.Status == swupd.StatusDeleted || f.Status == swupd.StatusGhosted {
				continue
			}
			// TODO: Should ignore files with certain f.Modifiers?
			if _, ok := allFileMap[f.Name]; ok {
				continue
			}
			allFileMap[f.Name] = f
			allFiles = append(allFiles, f)
		}
	}

	sort.Slice(allFiles, func(i, j int) bool {
		return allFiles[i].Name < allFiles[j].Name
	})

	err = copyAllFiles(state, outputDir, allFiles, noOverwrite)
	if err != nil {
		log.Fatalf("ERROR: couldn't copy files: %s", err)
	}
}

func resolveBundles(state *client.State, mom *swupd.Manifest, requested []string) (map[string]*swupd.Manifest, error) {
	var includes []string

	// Ignore requested duplicates.
	sort.Strings(requested)
	for i, name := range requested {
		if i > 0 && requested[i-1] == name {
			continue
		}
		includes = append(includes, name)
	}

	// Visit bundles and their included files. For the purposes of extracting, we
	// don't care if there are cycles in the bundles, so don't validate that.
	max := 0
	bundles := make(map[string]*swupd.Manifest)
	for len(includes) > 0 {
		var name string
		name, includes = includes[0], includes[1:]

		// Already included.
		if _, ok := bundles[name]; ok {
			continue
		}

		if max < len(name) {
			max = len(name)
		}

		var bundleFile *swupd.File
		for _, f := range mom.Files {
			if f.Name == name {
				bundleFile = f
				break
			}
		}
		if bundleFile == nil {
			return nil, fmt.Errorf("bundle %s not found in Manifest.MoM", name)
		}

		m, err := state.GetBundleManifest(fmt.Sprint(bundleFile.Version), name, bundleFile.Hash.String())
		if err != nil {
			return nil, err
		}

		for _, inc := range m.Header.Includes {
			includes = append(includes, inc.Name)
		}
		bundles[name] = m
	}

	fmt.Println()
	for name, b := range bundles {
		fmt.Printf("  %-*s %d\n", max, name, b.Header.Version)
	}
	fmt.Println()

	return bundles, nil
}

func copyAllFiles(state *client.State, outputDir string, allFiles []*swupd.File, noOverwrite bool) error {
	err := os.MkdirAll(outputDir, 0755)
	if err != nil {
		return err
	}

	for _, f := range allFiles {
		src := state.Path("staged/", f.Hash.String())
		dst := filepath.Join(outputDir, f.Name)

		// Check if we have the source file, if not download it.
		srcFI, err := os.Lstat(src)
		if err != nil {
			if !os.IsNotExist(err) {
				return fmt.Errorf("couldn't access existing staged file %s for %s: %s", src, dst, err)
			}
			err = state.GetFullfile(fmt.Sprint(f.Version), f.Hash.String())
			if err != nil {
				return fmt.Errorf("couldn't download fullfile for %s with hash %s: %s", dst, f.Hash.String(), err)
			}
			srcFI, err = os.Lstat(src)
			if err != nil {
				return fmt.Errorf("couldn't access staged file for %s: %s", dst, err)
			}
		}

		// Check if destination file is already what we want.
		dstFI, err := os.Lstat(dst)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("couldn't access existing file %s: %s", dst, err)
		}
		if err == nil {
			var hash string
			hash, err = swupd.GetHashForFile(dst)
			if err != nil {
				return err
			}
			if hash == f.Hash.String() {
				continue
			}

			// Try to fix the permissions of directory, to avoid destroying
			// its contents if not necessary.
			if srcFI.IsDir() && dstFI.IsDir() {
				if srcFI.Mode() != dstFI.Mode() {
					fmt.Printf("! fixing mode for %s from %s to %s\n", dst, dstFI.Mode(), srcFI.Mode())
					err = os.Chmod(dst, srcFI.Mode())
					if err != nil {
						return fmt.Errorf("couldn't fix mode for existing file %s: %s", dst, err)
					}
				}
				srcStat := srcFI.Sys().(*syscall.Stat_t)
				dstStat := dstFI.Sys().(*syscall.Stat_t)
				if srcStat.Uid != dstStat.Uid || srcStat.Gid != dstStat.Gid {
					fmt.Printf("! fixing ownership for %s from %d:%d to %d:%d\n", dst, dstStat.Uid, dstStat.Gid, srcStat.Uid, srcStat.Gid)
					err = os.Chown(dst, int(srcStat.Uid), int(srcStat.Gid))
					if err != nil {
						return fmt.Errorf("couldn't fix ownership of existing file %s: %s", dst, err)
					}
				}
				continue
			}

			if noOverwrite {
				fmt.Printf("! skipping %s\n", dst)
				continue
			}

			fmt.Printf("! overwriting %s\n", dst)
			err = os.RemoveAll(dst)
			if err != nil {
				return fmt.Errorf("couldn't remove %s for overwriting: %s", dst, err)
			}
		}

		switch f.Type {
		case swupd.TypeFile:
			err = copyFile(dst, src, srcFI)
		case swupd.TypeDirectory:
			err = os.Mkdir(dst, srcFI.Mode())
			if err != nil {
				return err
			}
			err = os.Chmod(dst, srcFI.Mode())
		case swupd.TypeLink:
			var linkname string
			linkname, err = os.Readlink(src)
			if err != nil {
				return err
			}
			err = os.Symlink(linkname, dst)
		default:
			err = fmt.Errorf("unknown type for %s", f.Name)
		}
		if err != nil {
			return err
		}
	}

	return nil
}

func copyFile(dst string, src string, srcFI os.FileInfo) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}

	dstFile, err := os.OpenFile(dst, os.O_RDWR|os.O_CREATE|os.O_TRUNC, srcFI.Mode())
	if err != nil {
		_ = srcFile.Close()
		return err
	}

	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		_ = srcFile.Close()
		_ = dstFile.Close()
		return fmt.Errorf("failure to copy data from %s to %s: %s", src, dst, err)
	}
	err = srcFile.Close()
	if err != nil {
		_ = dstFile.Close()
		return err
	}

	srcStat := srcFI.Sys().(*syscall.Stat_t)
	err = dstFile.Chown(int(srcStat.Uid), int(srcStat.Gid))
	if err != nil {
		_ = dstFile.Close()
		return err
	}
	if srcFI.Mode()&(os.ModeSticky|os.ModeSetgid|os.ModeSetuid) != 0 {
		err = dstFile.Chmod(srcFI.Mode())
		if err != nil {
			_ = dstFile.Close()
			return err
		}
	}
	return dstFile.Close()
}

func verifySignature(content, sig, cert string) error {
	cmd := exec.Command(
		"openssl", "smime", "-verify",
		"-in", sig,
		"-inform", "der",
		"-content", content,
		"-CAfile", cert,
		"-purpose", "crlsign",
	)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("%s\nsignature verification failed: %s\nCOMMAND LINE: %s", buf.Bytes(), err, strings.Join(cmd.Args, " "))
	}
	return nil
}

func findDefaultCert() string {
	certInCurrentDir, err := filepath.Abs("Swupd_Root.pem")
	if err != nil {
		log.Fatal(err)
	}

	defaultCerts := []string{
		certInCurrentDir,
		"/usr/share/clear/update-ca/Swupd_Root.pem",
	}

	for _, cert := range defaultCerts {
		if _, err := os.Stat(cert); err == nil {
			return cert
		}
	}

	return ""
}
