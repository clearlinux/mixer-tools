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
		certOut, err := os.Create("Swupd_Root.pem")
		if err != nil {
			fmt.Printf("failed to open cert.pem for writing: %v\n", err)
			PrintError(err)
		}
		pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: der})
		certOut.Close()

		// Write the private signing key out
		keyOut, err := os.OpenFile("private.pem", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
		if err != nil {
			fmt.Println("failed to open key.pem for writing")
			PrintError(err)
			return err
		}
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

// CopyFile is used during the build process to copy a given file to the target
// instead of dealing with the particulars of hardlinking.
func CopyFile(dest string, src string) error {
	source, err := os.Open(src)
	if err != nil {
		PrintError(err)
		return err
	}
	defer source.Close()

	destination, err := os.Create(dest)
	if err != nil {
		PrintError(err)
		return err
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	if err != nil {
		PrintError(err)
		return err
	}

	err = destination.Sync()
	if err != nil {
		PrintError(err)
		return err
	}

	return nil
}

// Download will attempt to download a from URL to the given filename
func Download(filename string, url string) (err error) {
	out, err := os.Create(filename)
	if err != nil {
		PrintError(err)
		return err
	}
	defer out.Close()

	infile, err := http.Get(url)
	if err != nil {
		PrintError(err)
		return err
	}
	defer infile.Body.Close()

	_, err = io.Copy(out, infile.Body)
	if err != nil {
		PrintError(err)
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

// GitInit attempts to initialize an empty git repository in the current
// directory, or exits on failure.
func GitInit() {
	gitcmd := exec.Command("git", "init")
	gitcmd.Stdout = os.Stdout
	err := gitcmd.Run()
	if err != nil {
		PrintError(err)
		fmt.Println("Failed to init git repo, exiting...")
		os.Exit(1)
	}
}

// GitAdd performs a 'git add .' in the current directory
func GitAdd() {
	gitcmd := exec.Command("git", "add", ".")
	gitcmd.Stdout = os.Stdout
	err := gitcmd.Run()
	if err != nil {
		PrintError(err)
		fmt.Println("Failed to add to git repo, exiting...")
		os.Exit(1)
	}
}

// GitCommit commits to a repo with a passed in string as the commit message
func GitCommit(commitmsg string) {
	gitcmd := exec.Command("git", "commit", "-m", commitmsg)
	gitcmd.Stdout = os.Stdout
	err := gitcmd.Run()
	if err != nil {
		PrintError(err)
		fmt.Println("Failed to commit to git repo, exiting...")
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
