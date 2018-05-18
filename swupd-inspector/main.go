package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
)

// TODO: Flag to set cacheDir.

// TODO: Use XDG_* environment variables instead of ~/.config and ~/.cache by default.

// TODO: Take into account deleted files, right now 'get' fails trying to download 0000...0.tar.

func usage() {
	fmt.Printf(`swupd-inspector analyzes swupd content

Commands:

    cat URL Manifest.NAME
        Print the contents of the given Manifest.

    get URL Manifest.NAME
        Download the contents of the given Manifest.

    get URL FILENAME
        Search for the FILENAME and download the corresponding fullfile.
        The FILENAME must be an absolute path.

    get URL HASH
        Download the fullfile corresponding to the hash.

    diff [--strict] [--no-color] URL1 URL2
        Compare two versions of swupd content. The filenames and flags
        will be compared, recursing to the bundles. Use --strict to
        also compare the version numbers of the files. Use --no-color
        to not emit escape codes in the output.

    log URL FILENAME
        Show FILENAME version and its previous versions.

    clean
        Clean up any cached content.

The program will cache everything downloaded, and can keep content from
different sources. The cache directory is $HOME/.cache/swupd-inspector.

The URLs must refer to a specific version like
https://download.clearlinux.org/update/20520. Absolute local paths can
also be used instead of URLs.

It is possible to refer to content by aliases. The alias 'clear' works
by default, so clear/20520 refer to the same as the URL above. Other
aliases can be defined in $HOME/.config/swupd-inspector/aliases in the
format ALIAS=URL per line.
`)
}

func main() {
	log.SetFlags(0)

	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

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

	cmd := os.Args[1]
	args := os.Args[2:]
	switch cmd {
	case "help", "-h", "--help":
		usage()
		os.Exit(0)
	case "diff":
		runDiff(cacheDir, args)
	case "get":
		runGet(cacheDir, args)
	case "cat":
		runCat(cacheDir, args)
	case "log":
		runLog(cacheDir, args)
	case "clean":
		runClean(cacheDir, args)
	default:
		usage()
		os.Exit(2)
	}
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
