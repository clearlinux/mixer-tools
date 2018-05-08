// Copyright © 2018 Intel Corporation
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

var mixWS = "/usr/share/mix"
var config = "/usr/share/mix/builder.conf"

const builderConf = `[Mixer]
LOCAL_BUNDLE_DIR = /usr/share/mix/local-bundles

[Builder]
SERVER_STATE_DIR = /usr/share/mix/update
BUNDLE_DIR = /usr/share/mix/local-bundles
YUM_CONF = /usr/share/mix/.yum-mix.conf
CERT = /usr/share/mix/Swupd_Root.pem
VERSIONS_PATH =/usr/share/mix
LOCAL_RPM_DIR = /usr/share/mix/local-rpms
LOCAL_REPO_DIR = /usr/share/mix/local

[swupd]
BUNDLE=os-core
CONTENTURL=file:///usr/share/mix/update/www
VERSIONURL=file:///usr/share/mix/update/www
FORMAT=1
`

func main() {
	Execute()
}
