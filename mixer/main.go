// Copyright © 2017 Intel Corporation
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

package main

import (
	"fmt"
	"os"

	"github.com/clearlinux/mixer-tools/mixer/cmd"
)

const Version = "3.2.1"

func main() {
	fmt.Printf("Mixer %s\n", Version)
	os.Setenv("LD_PRELOAD", "/usr/lib64/nosync/nosync.so")

	cmd.Execute()
}
