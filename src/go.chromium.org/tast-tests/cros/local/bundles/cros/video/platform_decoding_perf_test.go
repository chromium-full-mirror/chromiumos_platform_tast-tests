// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package video

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.chromium.org/tast-tests/cros/common/genparams"
	"go.chromium.org/tast-tests/cros/local/chrome"
)

// NB: If modifying any of the files or test specifications, be sure to
// regenerate the test parameters by running the following in a chroot:
// TAST_GENERATE_UPDATE=1 ~/trunk/src/platform/tast/tools/go.sh test -count=1 go.chromium.org/tast-tests/cros/local/bundles/cros/video

// getTestTimeout returns the test case timeout depending on resolution: larger resolutions need longer timeouts.
func getTestTimeout(resolution string) time.Duration {
	if resolution == "2160" {
		return 4 * time.Minute
	}
	return 2 * time.Minute
}

func TestPlatformDecodingPerfParams(t *testing.T) {
	type paramData struct {
		Name               string
		Decoder            string
		DecoderArgsBuilder string
		File               string
		SoftwareDeps       []string
		Metadata           []string
		Attr               []string
		Timeout            time.Duration
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
				testInfo := fmt.Sprintf("%s_%sp_%sfps", codec, resolution, frameRate)
				param := paramData{
					Name:               fmt.Sprintf("vaapi_%s", testInfo),
					Decoder:            filepath.Join(chrome.BinTestDir, "decode_test"),
					DecoderArgsBuilder: fmt.Sprintf("platform.%sDecodeVAAPIargsNoLogs", strings.ToUpper(codec)),
					File:               dataPath,
					SoftwareDeps:       append(fillSwDeps(codec, resolution, frameRate), "vaapi"),
					Metadata:           []string{dataPath},
					Attr:               []string{fmt.Sprintf("graphics_video_%s", codec)},
					Timeout:            getTestTimeout(resolution),
				}
				params = append(params, param)

				if strings.Contains(codec, "vp") {
					param = paramData{
						Name:               fmt.Sprintf("vpxdec_%s", testInfo),
						Decoder:            "/usr/local/bin/vpxdec",
						DecoderArgsBuilder: "platform.VPxDecodeArgs",
						File:               dataPath,
						Metadata:           []string{dataPath},
						Attr:               []string{fmt.Sprintf("graphics_video_%s", codec)},
						Timeout:            getTestTimeout(resolution),
					}
					params = append(params, param)
				}

				if codec == "av1" {
					param = paramData{
						Name:               fmt.Sprintf("dav1d_%s", testInfo),
						Decoder:            "/usr/local/bin/dav1d",
						DecoderArgsBuilder: "platform.Dav1dDecodeArgs",
						File:               dataPath,
						Metadata:           []string{dataPath},
						Attr:               []string{fmt.Sprintf("graphics_video_%s", codec)},
						Timeout:            getTestTimeout(resolution),
					}
					params = append(params, param)
				}
			}
		}
	}

	code := genparams.Template(t, `{{ range . }}{
		Name: {{ .Name | fmt }},
		Val:  platformDecodingPerfParams{
			filename: {{ .File | fmt }},
			decoder: {{ .Decoder | fmt }},
			decoderArgsBuilder: {{ .DecoderArgsBuilder }},
		},
		Timeout: {{ .Timeout | fmt }},
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
