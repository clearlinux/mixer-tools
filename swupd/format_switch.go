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
	"text/template"
)

// manTemplates is a map of format to relevant manifest template for that
// format
var manTemplates = map[uint]string{
	// format 25 manifest template
	// used for formats 1 - 25 as the initial default
	25: `
{{- with .Header -}}
MANIFEST	{{.Format}}
version:	{{.Version}}
previous:	{{.Previous}}
filecount:	{{.FileCount}}
timestamp:	{{(.TimeStamp.Unix)}}
contentsize:	{{.ContentSize -}}
{{range .Includes}}
includes:	{{.Name}}
{{- end}}
{{- end}}
{{ range .Files}}
{{.GetFlagString}}	{{.Hash}}	{{.Version}}	{{.Name}}
{{- end}}
`,
	// format 26 manifest template
	// used for formats 26 and greater until a new format is required
	26: `
{{- with .Header -}}
MANIFEST	{{.Format}}
version:	{{.Version}}
previous:	{{.Previous}}
{{ if ne .MinVersion 0 }}minversion:	{{.MinVersion}}
{{ end }}filecount:	{{.FileCount}}
timestamp:	{{(.TimeStamp.Unix)}}
contentsize:	{{.ContentSize -}}
{{range .Includes}}
includes:	{{.Name}}
{{- end}}
{{- end}}
{{ range .Files}}
{{.GetFlagString}}	{{.Hash}}	{{.Version}}	{{.Name}}
{{- end}}
`,
}

// manifestTemplateForFormat returns the *template.Template for creating
// manifests for the provided format f
func manifestTemplateForFormat(f uint) (t *template.Template) {
	switch {
	case f <= 25:
		// initial format, everything 0-25 uses this format
		t = template.Must(template.New("manifest").Parse(manTemplates[25]))
	case f > 25:
		// template for format 26
		t = template.Must(template.New("manifest").Parse(manTemplates[26]))
		// when a new format is required it must be added here and the 'case f
		// > 25' must be modified to 'case f > 25 && f < <new_format>'. The
		// <new_format> does not necessarily have to be 27 as format 27 may be
		// created due to a content breaking change instead of a manifest
		// format breaking change.
	}
	return
}

// this is a hack to allow users to update using swupd-client v3.15.3 which performs a
// check on contentsize with a maximum a couple of orders off the intended maximum.
const badMax uint64 = 2000000000

func (m *Manifest) setMaxContentSizeForFormat() {
	// this bug only existed in format 25
	if m.Header.Format == 25 && m.Header.ContentSize >= badMax {
		m.Header.ContentSize = badMax - 1
	}
}
