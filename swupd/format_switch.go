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
	"archive/tar"
	"fmt"
	"os"
	"path/filepath"
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
	// formats 26 to 28 manifest template
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
	// format 29 manifest template
	// used for formats 29 and greater until a new format is required
	29: `
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
{{- end -}}
{{range .Optional}}
also-add:	{{.Name}}
{{- end}}
{{- end}}
{{ range .Files}}
{{.GetFlagString}}	{{.Hash}}	{{.Version}}	{{.Name}}
{{- end}}
`,
}

const statusExperimental = "Experimental"

func setManifestStatusForFormat(format uint, bundleStatus string, statusFlag *StatusFlag) {
	if format > 26 {
		// Experimental bundles are introduced in format 27 and should not be created in older formats
		if bundleStatus == statusExperimental {
			*statusFlag = StatusExperimental
		}
	}
}

// Delta manifests were introduced in format 26 and should not be created in older formats
func writeDeltaManifestForFormat(tw *tar.Writer, outputDir string, dManifest *Manifest, toVersion uint32) error {
	if dManifest == nil || dManifest.Header.Format <= 25 {
		return nil
	}

	return writeDeltaManifest(tw, outputDir, dManifest, toVersion)
}

// Iterative manifests were introduced in format 26 and will cause issues with older formats
func (m *Manifest) writeIterativeManifestsForFormat(newManifests []*Manifest, out string) ([]*Manifest, error) {
	if m.Header.Format <= 25 {
		return nil, nil
	}

	return m.writeIterativeManifests(newManifests, out)
}

// The index manifest is not generated after format 28
func writeIndexManifestForFormat(c *config, ui *UpdateInfo, bundles []*Manifest, format uint) (*Manifest, error) {
	if format <= 28 {
		bundleDir := filepath.Join(c.imageBase, fmt.Sprint(ui.version))
		baseDir := filepath.Join(bundleDir, "full")
		metaPath := filepath.Join(baseDir, "usr/share/clear/allbundles")

		err := os.MkdirAll(metaPath, 0755)
		if err != nil {
			return nil, err
		}

		// Create index manifest bundle entries when a bundle chroot doesn't exist
		if _, err = os.Stat(filepath.Join(bundleDir, "os-core-update-index")); os.IsNotExist(err) {
			for _, b := range bundles {
				// full and iterative manifests are not included in index manifest
				if b.Name == "full" || b.Type == ManifestIterative {
					continue
				}

				// Load bundle info files that haven't been loaded
				if b.BundleInfo.Name == "" {
					biPath := filepath.Join(bundleDir, b.Name+"-info")
					if err = b.GetBundleInfo(c.stateDir, biPath); err != nil {
						return nil, err
					}
				}

				// Write index manifest content to full chroot
				err = writeBundleInfoPretty(&b.BundleInfo, filepath.Join(metaPath, b.Name))
				if err != nil {
					return nil, err
				}
			}
		} else if err != nil {
			return nil, err
		}

		osIdx, err := writeIndexManifest(c, ui, bundles)
		if err != nil {
			return nil, err
		}
		return osIdx, nil
	}
	return nil, nil
}

// manifestTemplateForFormat returns the *template.Template for creating
// manifests for the provided format f
func manifestTemplateForFormat(f uint) (t *template.Template) {
	switch {
	case f <= 25:
		// initial format, everything 0-25 uses this format
		t = template.Must(template.New("manifest").Parse(manTemplates[25]))
	case f > 25 && f <= 28:
		// template for formats 26 to 28
		t = template.Must(template.New("manifest").Parse(manTemplates[26]))
	case f > 28:
		// template for formats 29 or higher
		t = template.Must(template.New("manifest").Parse(manTemplates[29]))
		// when a new format is required it must be added here and the 'case f
		// > 28' must be modified to 'case f > 28 && f < <new_format>'. The
		// <new_format> does not necessarily have to be 29 as format 29 may be
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
