package main

import (
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
)

func runClean(cacheDir string) {
	fis, err := ioutil.ReadDir(cacheDir)
	if err != nil {
		log.Fatalf("ERROR: %s", err)
	}

	for _, fi := range fis {
		if !fi.IsDir() {
			log.Printf("Skipping unknown file %s/%s", cacheDir, fi.Name())
			continue
		}
		stateDir := filepath.Join(cacheDir, fi.Name())
		_, err = os.Stat(filepath.Join(stateDir, "content"))
		if err != nil {
			log.Printf("Skipping unrecognized directory %s", stateDir)
			continue
		}
		log.Printf("Deleting directory %s", stateDir)
		err = os.RemoveAll(stateDir)
		if err != nil {
			log.Fatalf("ERROR: %s", err)
		}
	}
}
