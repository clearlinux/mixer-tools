// Copyright 2018 Intel Corporation
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
