// Copyright 2017 Intel Corporation
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

import "strings"

func (f *File) setModifierFromPathname() {
	temp := strings.TrimPrefix(f.Name, "/V5")
	if temp != f.Name {
		f.Modifier = Apx4
		f.Name = temp
		return
	}
	temp = strings.TrimPrefix(f.Name, "/V4")
	if temp != f.Name {
		f.Modifier = Avx512_2
		f.Name = temp
		return
	}
	temp = strings.TrimPrefix(f.Name, "/V3")
	if temp != f.Name {
		f.Modifier = Avx2_1
		f.Name = temp
		return
	}
}

func (f *File) setPrefixFromModifier() {
	f.Name = modifierPrefixes[f.Modifier] + f.Name
}

func (f *File) setFullModifier(bits uint64) {
	switch f.Modifier {
	case Sse0:
		switch bits {
		case 0:
			f.Modifier = Sse0
		case 1:
			f.Modifier = Sse1
		case 2:
			f.Modifier = Sse2
		case 3:
			f.Modifier = Sse3
		case 4:
			f.Modifier = Sse4
		case 5:
			f.Modifier = Sse5
		case 6:
			f.Modifier = Sse6
		case 7:
			f.Modifier = Sse7
		}
	case Avx2_1:
		switch bits {
		case 1:
			f.Modifier = Avx2_1
		case 3:
			f.Modifier = Avx2_3
		case 5:
			f.Modifier = Avx2_5
		case 7:
			f.Modifier = Avx2_7
		}
	case Avx512_2:
		switch bits {
		case 2:
			f.Modifier = Avx512_2
		case 3:
			f.Modifier = Avx512_3
		case 6:
			f.Modifier = Avx512_6
		case 7:
			f.Modifier = Avx512_7
		}
	case Apx4:
		switch bits {
		case 4:
			f.Modifier = Apx4
		case 5:
			f.Modifier = Apx5
		case 6:
			f.Modifier = Apx6
		case 7:
			f.Modifier = Apx7
		}
	}
}

func (f *File) setGhostedFromPathname() {
	bootPaths := []string{
		"/boot/",
		"/usr/lib/modules/",
		"/usr/lib/kernel/",
	}
	for _, path := range bootPaths {
		if strings.HasPrefix(f.Name, path) {
			if f.Status == StatusDeleted {
				f.Status = StatusGhosted
			}
			return
		}
	}
}

func (m *Manifest) applyHeuristics() {
	for _, f := range m.Files {
		f.setGhostedFromPathname()
	}
}
