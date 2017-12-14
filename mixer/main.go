package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/clearlinux/mixer-tools/builder"
	"github.com/clearlinux/mixer-tools/helpers"
)

const Version = "3.2.1"

type Command struct {
	Name        string
	Description string
	Run         func(args []string)
}

var commands []*Command

func init() {
	commands = []*Command{
		{"build-all", "Build all content for mix with default options", cmdBuildAll},
		{"build-chroots", "Build chroots for the mix", cmdBuildChroots},
		{"build-update", "Build all update content for the mix", cmdBuildUpdate},
		{"build-image", "Build an image from the mix content", cmdBuildImage},
		{"add-rpms", "Add rpms to local yum repository", cmdAddRPMs},
		{"get-bundles", "Get the clr-bundles from upstream", cmdGetBundles},
		{"add-bundles", "Add clr-bundles to your mix", cmdAddBundles},
		{"init-mix", "Initialize the mixer and workspace", cmdInitMix},
		{"help", "Show help options", cmdHelp},
	}
}

func PrintMainHelp() {
	fmt.Printf("usage: mixer <command> [args]\n")
	for _, cmd := range commands {
		fmt.Printf("\t%-20s\t%s\n", cmd.Name, cmd.Description)
	}
}

func CheckDeps() error {
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
	for _, dep := range deps {
		if _, err := exec.LookPath(dep); err != nil {
			return fmt.Errorf("failed to find program %q: %v\n", dep, err)
		}
	}
	return nil
}

func main() {
	fmt.Printf("Mixer %s\n", Version)
	os.Setenv("LD_PRELOAD", "/usr/lib64/nosync/nosync.so")

	if len(os.Args) == 1 {
		PrintMainHelp()
		return
	}

	var cmd *Command
	name := os.Args[1]
	if name == "-h" {
		name = "help"
	}
	if name != "version" && name != "help" {
		err := CheckDeps()
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
			os.Exit(1)
		}
	}

	for _, c := range commands {
		if c.Name == name {
			cmd = c
		}
	}

	if cmd == nil {
		fmt.Printf("%q is not a valid command.\n", name)
		os.Exit(-1)
	}

	args := os.Args[2:]
	cmd.Run(args)
}

type UpdateVars struct {
	Format     string
	Increment  bool
	MinVersion int
	NoSigning  bool
	Prefix     string
	NoPublish  bool
	KeepChroot bool
}

func setupUpdateFlags(v *UpdateVars, fs *flag.FlagSet) {
	fs.StringVar(&v.Format, "format", "", "Supply format to use")
	fs.BoolVar(&v.Increment, "increment", false, "Automatically increment the mixversion post build")
	fs.IntVar(&v.MinVersion, "minversion", 0, "Supply minversion to build update with")
	fs.BoolVar(&v.NoSigning, "no-signing", false, "Do not generate a certificate and do not sign the Manifest.MoM")
	fs.StringVar(&v.Prefix, "prefix", "", "Supply prefix for where the swupd binaries live")
	fs.BoolVar(&v.NoPublish, "no-publish", false, "Do not update the latest version after update")
	fs.BoolVar(&v.KeepChroot, "keep-chroots", false, "Keep individual chroots created and not just consolidated 'full'")
}

func cmdBuildAll(args []string) {
	fs := flag.NewFlagSet("build-all", flag.ExitOnError)
	config := fs.String("config", "", "Supply a specific builder.conf to use for mixing")

	v := &UpdateVars{}
	setupUpdateFlags(v, fs)

	fs.Parse(args)

	b := builder.NewFromConfig(*config)
	rpms, err := ioutil.ReadDir(b.Rpmdir)
	if err == nil {
		b.AddRPMList(rpms)
	}
	BuildChroots(b, v.NoSigning)
	err = b.BuildUpdate(v.Prefix, v.MinVersion, v.Format, v.NoSigning, !v.NoPublish, v.KeepChroot)
	if err != nil {
		os.Exit(-1)
	}

	b.UpdateMixVer()
}

func cmdBuildChroots(args []string) {
	fs := flag.NewFlagSet("build-chroots", flag.ExitOnError)
	config := fs.String("config", "", "Supply a specific builder.conf to use for mixing")
	noSigning := fs.Bool("no-signing", false, "Do not generate a certificate to sign the Manifest.MoM")

	fs.Parse(args)

	b := builder.NewFromConfig(*config)
	BuildChroots(b, *noSigning)
}

func cmdBuildUpdate(args []string) {
	fs := flag.NewFlagSet("build-update", flag.ExitOnError)
	config := fs.String("config", "", "Supply a specific builder.conf to use for mixing")

	v := &UpdateVars{}
	setupUpdateFlags(v, fs)

	fs.Parse(args)

	b := builder.NewFromConfig(*config)
	err := b.BuildUpdate(v.Prefix, v.MinVersion, v.Format, v.NoSigning, !v.NoPublish, v.KeepChroot)
	if err != nil {
		os.Exit(-1)
	}

	if v.Increment {
		b.UpdateMixVer()
	}
}

func cmdBuildImage(args []string) {
	imagecmd := flag.NewFlagSet("build-image", flag.ExitOnError)
	imageformat := imagecmd.String("format", "", "Supply the format used for the Mix")
	conf := imagecmd.String("config", "", "Supply a specific builder.conf to use for mixing")
	imagetemplate := imagecmd.String("template", "", "Path to tempalte file to use")

	imagecmd.Parse(args)

	b := builder.NewFromConfig(*conf)
	b.BuildImage(*imageformat, *imagetemplate)
}

func cmdAddRPMs(args []string) {
	flags := flag.NewFlagSet("add-rpms", flag.ExitOnError)
	conf := flags.String("config", "", "Supply a specific builder.conf to use for mixing")
	flags.Parse(args)

	b := builder.NewFromConfig(*conf)
	rpms, err := ioutil.ReadDir(b.Rpmdir)
	if err != nil {
		fmt.Printf("ERROR: cannot read %s\n", b.Rpmdir)
	}
	b.AddRPMList(rpms)
}

func cmdGetBundles(args []string) {
	bundlescmd := flag.NewFlagSet("get-bundles", flag.ExitOnError)
	bundleconf := bundlescmd.String("config", "", "Supply a specific builder.conf to use for mixing")
	bundlescmd.Parse(args)
	b := builder.NewFromConfig(*bundleconf)
	fmt.Println("Getting clr-bundles for version " + b.Clearver)
	b.UpdateRepo(b.Clearver, false)
}

func cmdAddBundles(args []string) {
	flags := flag.NewFlagSet("add-bundles", flag.ExitOnError)
	bundlesarg := flags.String("bundles", "", "Comma-separated list of bundles to add")
	force := flags.Bool("force", false, "Override bundles that already exist")
	all := flags.Bool("all", false, "Add all bundles from CLR; takes precedence over -bundles")
	git := flags.Bool("git", false, "Automatically apply new git commit")
	conf := flags.String("config", "", "Supply a specific builder.conf to use for mixing")
	flags.Parse(args)

	var bundles []string
	if !*all {
		if len(*bundlesarg) == 0 {
			flags.Usage()
			os.Exit(1)
		} else {
			bundles = strings.Split(*bundlesarg, ",")
		}
	}

	b := builder.NewFromConfig(*conf)
	b.AddBundles(bundles, *force, *all, *git)
}

func cmdInitMix(args []string) {
	initcmd := flag.NewFlagSet("init-mix", flag.ExitOnError)
	allflag := initcmd.Bool("all", false, "Create a mix with all Clear bundles included")
	clearflag := initcmd.Int("clearver", 1, "Supply the Clear version to compose the mix from")
	mixflag := initcmd.Int("mixver", 0, "Supply the Mix version to build")
	initconf := initcmd.String("config", "", "Supply a specific builder.conf to use for mixing")
	upstreamurl := initcmd.String("upstreamurl", "https://download.clearlinux.org", "Supply an upstream URL to use for mixing")
	initcmd.Parse(args)
	b := builder.New()
	b.CreateDefaultConfig(*initconf)
	b.CreateRpmDirs()
	b.LoadBuilderConf(*initconf)
	b.ReadBuilderConf()
	b.InitMix(strconv.Itoa(*clearflag), strconv.Itoa(*mixflag), *allflag, *upstreamurl)
}

func cmdHelp(args []string) {
	PrintMainHelp()
}

func BuildChroots(builder *builder.Builder, signflag bool) {
	// Create the signing and validation key/cert
	if _, err := os.Stat(builder.Cert); os.IsNotExist(err) {
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
