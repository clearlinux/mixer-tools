package main

import (
	"bufio"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// TODO: Flag to set cacheDir.

// TODO: Use XDG_* environment variables instead of ~/.config and ~/.cache by default.

// TODO: Take into account deleted files, right now 'get' fails trying to download 0000...0.tar.

func main() {
	log.SetFlags(0)

	user, err := user.Current()
	if err != nil {
		log.Fatal(err)
	}

	applyUserAliases(filepath.Join(user.HomeDir, ".config/swupd-inspector/aliases"))

	// TODO: Use os.UserCacheDir instead of user.HomeDir+".cache".
	cacheDir := filepath.Join(user.HomeDir, ".cache", "swupd-inspector")
	err = os.MkdirAll(cacheDir, 0755)
	if err != nil {
		log.Fatalf("couldn't create cache directory: %s", err)
	}

	rootCmd := &cobra.Command{
		Use:   "swupd-inspector",
		Short: "Inspect and download swupd content",
		Long: `Inspect and download swupd content

The program will cache everything downloaded, and can keep content from
different sources. The cache directory is $HOME/.cache/swupd-inspector.

The URLs must refer to a specific version like
https://download.clearlinux.org/update/20520. Absolute local paths can
also be used instead of URLs.

It is possible to refer to content by aliases. The alias 'clear' works
by default, so clear/20520 refer to the same as the URL above. Other
aliases can be defined in $HOME/.config/swupd-inspector/aliases in the
format ALIAS=URL per line.
`,
	}

	diffFlags := &diffFlags{}
	diffCmd := &cobra.Command{
		Use:   "diff [flags] URL1 URL2",
		Short: "Compare two versions of swupd content",
		Long: `Compare two versions of swupd content.

The filenames and flags will be compared, recursing to the
bundles. Use --strict to also compare the version numbers of the
files. Use --no-color to not emit escape codes in the output.
`,
		Args: cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			runDiff(cacheDir, diffFlags, args[0], args[1])
		},
	}
	diffCmd.Flags().BoolVar(&diffFlags.noColor, "no-color", false, "disable colored output")
	diffCmd.Flags().BoolVar(&diffFlags.strict, "strict", false, "compare version numbers of files")
	rootCmd.AddCommand(diffCmd)

	getCmd := &cobra.Command{
		Use:   "get [flags] URL (FILENAME|HASH|Manifest.NAME)",
		Short: "Download content from a swupd repository",
		Long: `Download content from a swupd repository.

Different types of content can be downloaded:

  swupd-inspector get URL Manifest.NAME
      Download the contents of the given Manifest.

  swupd-inspector get URL FILENAME
      Search for the FILENAME and download the corresponding fullfile.
      The FILENAME must be an absolute path.

  swupd-inspector get URL HASH
      Download the fullfile corresponding to the hash.
`,
		Args: cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			runGet(cacheDir, args[0], args[1])
		},
	}
	rootCmd.AddCommand(getCmd)

	cleanCmd := &cobra.Command{
		Use:   "clean",
		Short: "Clean up any cached content",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			runClean(cacheDir)
		},
	}
	rootCmd.AddCommand(cleanCmd)

	catCmd := &cobra.Command{
		Use:   "cat [flags] URL Manifest.NAME",
		Short: "Print the contents of a Manifest",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			runCat(cacheDir, args[0], args[1])
		},
	}
	rootCmd.AddCommand(catCmd)

	logCmd := &cobra.Command{
		Use:   "log [flags] URL FILENAME",
		Short: "Print FILENAME version and all previous versions",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			runLog(cacheDir, args[0], args[1])
		},
	}
	rootCmd.AddCommand(logCmd)

	_ = rootCmd.Execute()
}

var aliases = map[string]string{
	"clear": "https://cdn.download.clearlinux.org/update",
}

func applyUserAliases(path string) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		log.Fatalf("ERROR: %s", err)
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		text := strings.TrimSpace(scanner.Text())
		if len(text) == 0 {
			continue
		}
		if text[0] == '#' {
			continue
		}
		sep := strings.Index(text, "=")
		if sep != -1 {
			alias := text[:sep]
			url := text[sep+1:]
			if strings.HasPrefix(alias, "/") {
				log.Printf("Rejecting alias %q that starts with /", alias)
				continue
			}
			if alias == "clear" {
				// TODO: Should we just let the user do this?
				log.Printf("Ignoring redefinition of 'clear' in %s", path)
				continue
			}
			if url == "" {
				log.Printf("Rejecting alias %q because URL is empty", alias)
				continue
			}
			aliases[alias] = url
		}
	}

	err = scanner.Err()
	if err != nil {
		log.Fatalf("ERROR: %s", scanner.Err())
	}
}

func parseURL(s string) (base string, version string) {
	if s == "" {
		log.Fatalf("ERROR: couldn't parse empty URL")
	}

	for alias, aliasBase := range aliases {
		if strings.HasPrefix(s, alias+"/") {
			s = aliasBase + s[len(alias):]
			break
		}
	}

	switch {
	case strings.HasPrefix(s, "/"):
		s = filepath.Clean(s)
	case strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://"):
		// Eat the extra slashes in the end.
		for len(s) > 0 && s[len(s)-1] == '/' {
			s = s[:len(s)-1]
		}
	default:
		log.Fatalf("ERROR: Invalid URL: %s", s)
	}

	sep := strings.LastIndex(s, "/")
	if sep == -1 {
		log.Fatalf("Invalid URL for content: %s", s)
	}
	base = s[:sep]
	version = s[sep+1:]

	parsed, err := strconv.ParseUint(version, 10, 32)
	if err != nil {
		log.Fatalf("Error parsing version in URL %s: %s", s, err)
	}
	if parsed == 0 {
		log.Fatalf("Invalid version 0 in URL %s", s)
	}

	return base, version
}

func convertContentBaseToDirname(content string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			return r
		default:
			return '_'
		}
	}, content)
}
