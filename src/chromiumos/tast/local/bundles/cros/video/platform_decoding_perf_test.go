// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package video

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"chromiumos/tast/common/genparams"
	"chromiumos/tast/local/chrome"
)

// NB: If modifying any of the files or test specifications, be sure to
// regenerate the test parameters by running the following in a chroot:
// TAST_GENERATE_UPDATE=1 ~/trunk/src/platform/tast/tools/go.sh test -count=1 chromiumos/tast/local/bundles/cros/video

func TestPlatformDecodingPerfParams(t *testing.T) {
	type paramData struct {
		Name         string
		Decoder      string
		CmdBuilder   string
		File         string
		SoftwareDeps []string
		Metadata     []string
		Attr         []string
	}

	var params []paramData

	// Add VAAPI decode_test perf variants.
	var codecs = []string{"av1", "h264", "hevc", "vp8", "vp9"}
	var resolutions = []string{"1080", "2160"}
	var frameRates = []string{"30", "60"}
	for _, codec := range codecs {
		for _, resolution := range resolutions {
			for _, frameRate := range frameRates {
				dataPath := genDataPath(codec, resolution, frameRate)
				param := paramData{
					Name:         fmt.Sprintf("vaapi_%s_%sp_%sfps", codec, resolution, frameRate),
					Decoder:      filepath.Join(chrome.BinTestDir, "decode_test"),
					CmdBuilder:   fmt.Sprintf("platform.%sDecodeVAAPIargs", strings.ToUpper(codec)),
					File:         dataPath,
					SoftwareDeps: append(fillSwDeps(codec, resolution, frameRate), "vaapi"),
					Metadata:     []string{dataPath},
					Attr:         []string{fmt.Sprintf("graphics_video_%s", codec)},
				}

				params = append(params, param)
			}
		}
	}

	code := genparams.Template(t, `{{ range . }}{
		Name: {{ .Name | fmt }},
		Val:  platformDecodingPerfParams{
			filename: {{ .File | fmt }},
			decoder: {{ .Decoder | fmt }},
			commandBuilder: {{ .CmdBuilder }},
		},
		{{ if .SoftwareDeps }}
		ExtraSoftwareDeps: {{ .SoftwareDeps | fmt }},
		{{ end }}
		ExtraData: {{ .Metadata | fmt }},
		{{ if .Attr }}
		ExtraAttr: {{ .Attr | fmt }},
		{{ end }}
	},
	{{ end }}`, params)
	genparams.Ensure(t, "platform_decoding_perf.go", code)
}
