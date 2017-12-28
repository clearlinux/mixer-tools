package helpers

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"io/ioutil"
	"math/big"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	// ENOVERSION is returned when an a version is unknown
	ENOVERSION = 24
)

// PrintError is a utility function to emit an error to the console
func PrintError(e error) {
	fmt.Fprintf(os.Stderr, "***Error: %v\n", e)
}

// CreateCertTemplate will construct the template for needed openssl metadata
// instead of using an attributes.cnf file
func CreateCertTemplate() *x509.Certificate {
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialnumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		fmt.Println("ERROR: Failed to generate serial number")
		PrintError(err)
	}

	template := x509.Certificate{
		SerialNumber:          serialnumber,
		Subject:               pkix.Name{Organization: []string{"Mixer"}},
		SignatureAlgorithm:    x509.SHA256WithRSA,
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(1, 0, 0),
		BasicConstraintsValid: true,
		IsCA:        false, // This could be true since we are self signed, but set false for correctness
		KeyUsage:    x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature | x509.KeyUsageCRLSign,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageCodeSigning},
	}

	return &template
}

// CreateKeyPair constructs an RSA keypair in memory
func CreateKeyPair() (*rsa.PrivateKey, error) {
	rootKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		fmt.Println("ERROR: Failed to generate random key")
		PrintError(err)
	}
	return rootKey, nil
}

// GenerateCertificate will create the private signing key and public
// certificate for clients to use and writes them to disk
func GenerateCertificate(cert string, template, parent *x509.Certificate, pubkey interface{}, privkey interface{}) error {
	if _, err := os.Stat(cert); os.IsNotExist(err) {
		der, err := x509.CreateCertificate(rand.Reader, template, parent, pubkey, privkey)
		if err != nil {
			fmt.Println("ERROR: Failed to create certificate!")
			PrintError(err)
			return err
		}

		// Write the public certficiate out for clients to use
		certOut, err := os.Create(cert)
		if err != nil {
			fmt.Printf("failed to open cert.pem for writing: %v\n", err)
			PrintError(err)
		}
		pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: der})
		certOut.Close()

		// Write the private signing key out
		keyOut, err := os.OpenFile(filepath.Dir(cert)+"/private.pem", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
		if err != nil {
			fmt.Println("failed to open key.pem for writing")
			PrintError(err)
			return err
		}
		defer keyOut.Close()
		// Need type assertion for Marshal to work
		priv := privkey.(*rsa.PrivateKey)
		pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	}
	return nil
}

// ReadFileAndSplit tokenizes the given file and converts in into a slice split
// by the newline character.
func ReadFileAndSplit(filename string) ([]string, error) {
	builder, err := ioutil.ReadFile(filename)
	if err != nil {
		PrintError(err)
		return nil, err
	}
	data := string(builder)
	lines := strings.Split(data, "\n")

	return lines, nil
}

// GetIncludedBundles parses a bundle definition file and returns a list of all
// bundles it includes.
func GetIncludedBundles(filename string) ([]string, error) {
	lines, err := ReadFileAndSplit(filename)
	if err != nil {
		PrintError(err)
		return nil, err
	}

	// Note: Matches lines like "include(os-core-update)", pulling out
	// the string between the parens. The "\" needs to be escaped due to
	// Go's string literal parsing, so "\\(" matches "("
	r := regexp.MustCompile("^include\\(([A-Za-z0-9-]+)\\)$")
	var includes []string
	for _, line := range lines {
		if matches := r.FindStringSubmatch(line); len(matches) > 1 {
			includes = append(includes, matches[1])
		}
	}
	return includes, nil
}

// CopyFile is used during the build process to copy a given file to the target
// instead of dealing with the particulars of hardlinking.
func CopyFile(dest string, src string, overwrite bool) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	flags := os.O_RDWR | os.O_CREATE
	if !overwrite {
		flags |= os.O_EXCL
	}

	destination, err := os.OpenFile(dest, flags, 0666)
	if err != nil {
		return err
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	if err != nil {
		return err
	}

	err = destination.Sync()
	if err != nil {
		return err
	}

	return nil
}

// Download will attempt to download a from URL to the given filename
func Download(filename string, url string) (err error) {
	infile, err := http.Get(url)
	if err != nil {
		return err
	}
	defer infile.Body.Close()

	if infile.StatusCode != http.StatusOK {
		return fmt.Errorf("Get %s replied: %d (%s)", url, infile.StatusCode, http.StatusText(infile.StatusCode))
	}

	out, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, infile.Body)
	if err != nil {
		os.RemoveAll(filename)
		return err
	}

	return nil
}

// GetDirContents is an an assert-style helper to get the contents of a
// directory, or to exit on failure.
func GetDirContents(dirname string) []os.FileInfo {
	files, err := ioutil.ReadDir(dirname)
	if err != nil {
		PrintError(err)
		os.Exit(1)
	}

	return files
}

// Git runs git with arguments and exits in case of failure.
// IMPORTANT: the 'args' passed to this function _must_ be validated,
// as to avoid cases where input is received from a third party source.
// Such inputs could be something the likes of 'status; rm -rf .*'
// and need to be escaped or avoided properly.
func Git(args ...string) {
	cmd := exec.Command("git", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: failed to run git %s: %v\n", strings.Join(args, " "), err)
		os.Exit(1)
	}
}

// CheckRPM returns nil if file <name>.rpm shows a valid RPM v# output,
// in order to catch corrupt or invalid RPM files.
func CheckRPM(rpm string) error {
	output, err := exec.Command("file", rpm).Output()
	if err != nil {
		PrintError(err)
		return err
	}
	if strings.Contains(string(output), "RPM v") {
		return nil
	}
	return fmt.Errorf("ERROR: %s is not valid!", rpm)
}
