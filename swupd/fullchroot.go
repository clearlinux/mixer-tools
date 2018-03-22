package swupd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func syncToFull(version uint32, bundle string, imageBase string) error {
	fullPath := filepath.Join(imageBase, fmt.Sprint(version), "full")
	// MkdirAll returns nil when the path exists, so we continue to do the
	// full chroot creation over the existing one
	if err := os.MkdirAll(fullPath, 0777); err != nil {
		return err
	}

	// append trailing slash to get contents only
	bundlePath := filepath.Join(imageBase, fmt.Sprint(version), bundle) + "/"
	if _, err := os.Stat(bundlePath); err == nil {
		cmd := exec.Command("rsync", "-aAX", "--ignore-existing", bundlePath, fullPath)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("rsync error: %v", err)
		}
	}

	return nil
}
