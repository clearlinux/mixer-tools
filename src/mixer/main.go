package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"

	"builder"
	"helpers"
)

// PrintMainHelp emits useful help text to the console
func PrintMainHelp() {
	fmt.Printf("usage: mixer <command> [args]\n")
	fmt.Printf("\tbuild-all\t\tBuild all content for mix with default options\n")
	fmt.Printf("\tbuild-chroots\t\tBuild chroots for the mix\n")
	fmt.Printf("\tbuild-update\t\tBuild all update content for the mix\n")
	fmt.Printf("\tbuild-image\t\tBuild an image from the mix content\n")
	fmt.Printf("\tadd-rpms\t\tAdd rpms to local yum repository\n")
	fmt.Printf("\tget-bundles\t\tGet the clr-bundles from upstream\n")
	fmt.Printf("\tinit-mix\t\tInitialize the mixer and workspace\n")
}

// SetupBuilder performs the initial bootstrap and configuration according to
// the local configuration.
func SetupBuilder(conf string, config interface{}) {
	builder := config.(*builder.Builder)
	builder.LoadBuilderConf(conf)
	builder.ReadBuilderConf()
	builder.ReadVersions()
}

func main() {
	fmt.Println("Mixer 3.06")
	os.Setenv("LD_PRELOAD", "/usr/lib64/nosync/nosync.so")

	addcmd := flag.NewFlagSet("add-rpms", flag.ExitOnError)
	addconf := addcmd.String("config", "", "Supply a specific builder.conf to use for mixing")

	buildall := flag.NewFlagSet("build-all", flag.ExitOnError)
	buildallconf := buildall.String("config", "", "Supply a specific builder.conf to use for mixing")

	chrootcmd := flag.NewFlagSet("build-chroots", flag.ExitOnError)
	certflag := chrootcmd.Bool("no-signing", false, "Do not generate a certificate to sign the Manifest.MoM")
	chrootconf := chrootcmd.String("config", "", "Supply a specific builder.conf to use for mixing")

	updatecmd := flag.NewFlagSet("build-update", flag.ExitOnError)
	updateconf := updatecmd.String("config", "", "Supply a specific builder.conf to use for mixing")
	formatflag := updatecmd.String("format", "", "Supply format to use")
	incrementflag := updatecmd.Bool("increment", false, "Automatically increment the mixversion post build")
	minvflag := updatecmd.Int("minversion", 0, "Supply minversion to build update with")
	signflag := updatecmd.Bool("no-signing", false, "Do not generate a certificate and do not sign the Manifest.MoM")
	prefixflag := updatecmd.String("prefix", "", "Supply prefix for where the swupd binaries live")
	publishflag := updatecmd.Bool("no-publish", false, "Do not update the latest version after update")
	keepchrootsflag := updatecmd.Bool("keep-chroots", false, "Keep individual chroots created and not just consolidated 'full'")

	bundlescmd := flag.NewFlagSet("get-bundles", flag.ExitOnError)
	bundleconf := bundlescmd.String("config", "", "Supply a specific builder.conf to use for mixing")

	initcmd := flag.NewFlagSet("init-mix", flag.ExitOnError)
	allflag := initcmd.Bool("all", false, "Create a mix with all Clear bundles included")
	clearflag := initcmd.Int("clearver", 0, "Supply the Clear version to compose the mix from")
	mixflag := initcmd.Int("mixver", 0, "Supply the Mix version to build")
	initconf := initcmd.String("config", "", "Supply a specific builder.conf to use for mixing")

	imagecmd := flag.NewFlagSet("build-image", flag.ExitOnError)
	imageformat := imagecmd.String("format", "", "Supply the format used for the Mix")

	if len(os.Args) == 1 {
		PrintMainHelp()
		return
	}

	switch os.Args[1] {
	case "build-all":
		buildall.Parse(os.Args[2:])
	case "build-chroots":
		chrootcmd.Parse(os.Args[2:])
	case "build-update":
		updatecmd.Parse(os.Args[2:])
	case "build-image":
		imagecmd.Parse(os.Args[2:])
	case "add-rpms":
		addcmd.Parse(os.Args[2:])
	case "get-bundles":
		bundlescmd.Parse(os.Args[2:])
	case "init-mix":
		initcmd.Parse(os.Args[2:])
	default:
		fmt.Printf("%q is not valid command.\n", os.Args[1])
		os.Exit(-1)
	}

	// Allocate a builder object to do all our mixing needs
	builder := builder.New()

	// If we got this far, the flags are correct, so read the conf from
	// the current directory or from the flag passed in
	if addcmd.Parsed() {
		SetupBuilder(*addconf, builder)
		rpms, err := ioutil.ReadDir(builder.Rpmdir)
		if err != nil {
			fmt.Printf("ERROR: cannot read %s\n", builder.Rpmdir)
		}
		builder.AddRPMList(rpms)
	}

	if buildall.Parsed() {
		SetupBuilder(*buildallconf, builder)
		rpms, err := ioutil.ReadDir(builder.Rpmdir)
		if err == nil {
			builder.AddRPMList(rpms)
		}
		BuildChroots(builder, *certflag)
		BuildUpdate(builder, *prefixflag, *minvflag, *formatflag, *signflag, !(*publishflag), *keepchrootsflag)
		builder.UpdateMixVer()
	}

	if bundlescmd.Parsed() {
		SetupBuilder(*bundleconf, builder)
		fmt.Println("Getting clr-bundles for version " + builder.Get("Clearver"))
		builder.UpdateRepo(builder.Get("Clearver"), false)
	}

	if initcmd.Parsed() {
		builder.LoadBuilderConf(*initconf)
		builder.ReadBuilderConf()
		builder.InitMix(strconv.Itoa(*clearflag), strconv.Itoa(*mixflag), *allflag)
	}

	if chrootcmd.Parsed() {
		SetupBuilder(*chrootconf, builder)
		BuildChroots(builder, *certflag)
	}

	if updatecmd.Parsed() {
		SetupBuilder(*updateconf, builder)
		BuildUpdate(builder, *prefixflag, *minvflag, *formatflag, *signflag, !(*publishflag), *keepchrootsflag)

		if *incrementflag == true {
			builder.UpdateMixVer()
		}
	}

	if imagecmd.Parsed() {
		SetupBuilder("", builder)
		builder.BuildImage(*imageformat)
	}
}

func BuildChroots(config interface{}, signflag bool) {
	builder := config.(*builder.Builder)
	// Create the signing and validation key/cert
	if _, err := os.Stat(builder.Get("Cert")); os.IsNotExist(err) {
		fmt.Println("Generating certificate for signature validation...")
		privkey, err := helpers.CreateKeyPair()
		if err != nil {
			os.Exit(1)
		}
		template := helpers.CreateCertTemplate()

		err = builder.BuildChroots(template, privkey, signflag)
		if err != nil {
			os.Exit(-1)
		}
	} else {
		err := builder.BuildChroots(nil, nil, true)
		if err != nil {
			os.Exit(-1)
		}
	}
}

func BuildUpdate(config interface{}, prefixflag string, minvflag int, formatflag string, signflag bool, publishflag bool, keepchrootsflag bool) {
	builder := config.(*builder.Builder)
	err := builder.BuildUpdate(prefixflag, minvflag, formatflag, signflag, publishflag, keepchrootsflag)
	if err != nil {
		os.Exit(-1)
	}
}
