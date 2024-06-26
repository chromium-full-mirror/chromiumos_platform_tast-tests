// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package video

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"go.chromium.org/tast-tests/cros/common/genparams"
)

// NB: If modifying any of the files or test specifications, be sure to
// regenerate the test parameters by running the following in a chroot:
// TAST_GENERATE_UPDATE=1 ~/chromiumos/src/platform/tast/tools/go.sh test -count=1 go.chromium.org/tast-tests/cros/local/bundles/cros/video

func genDataPath(codec, resolution, frameRate string) string {
	numFrames := "300"
	if frameRate == "60" {
		numFrames = "600"
	}

	extension := fmt.Sprintf("%s.ivf", codec)
	if codec == "h264" {
		extension = "h264"
	} else if codec == "hevc" {
		extension = "hevc"
	}

	return fmt.Sprintf("perf/%s/%sp_%sfps_%sframes.%s", codec, resolution, frameRate, numFrames, extension)
}

func fillSwDeps(codec, resolution, frameRate string) []string {
	swDeps := []string{fmt.Sprintf("autotest-capability:hw_dec_%s_%s_%s", codec, resolution, frameRate)}

	if codec == "h264" || codec == "hevc" {
		swDeps = append(swDeps, "proprietary_codecs")
	}
	return swDeps
}

func TestChromeStackDecoderPerfParams(t *testing.T) {
	type paramData struct {
		Name               string
		File               string
		ConcurrentDecoders bool
		HardwareDeps       string
		SoftwareDeps       []string
		Metadata           []string
		Attr               []string
		Timeout            time.Duration
		EnabledFeatures    []string
	}
	const defaultTimeout = 2 * time.Minute

	var params []paramData

	groups := map[string]paramData{
		"": paramData{},
		"v4l2_flat_": paramData{
			SoftwareDeps:    []string{"v4l2_codec"},
			EnabledFeatures: []string{"V4L2FlatStatefulVideoDecoder"},
			HardwareDeps:    "hwdep.D(hwdep.SupportsV4L2FlatVideoDecoding())",
		},
	}

	// Sort the keys because map traversal is not deterministic.
	groupsKeys := make([]string, 0, len(groups))
	for k := range groups {
		groupsKeys = append(groupsKeys, k)
	}
	sort.Strings(groupsKeys)

	var codecs = []string{"av1", "h264", "hevc", "vp8", "vp9"}
	var resolutions = []string{"1080", "2160"}
	var frameRates = []string{"30", "60"}
	for _, groupPrefix := range groupsKeys {
		groupParam := groups[groupPrefix]
		for _, codec := range codecs {
			for _, resolution := range resolutions {
				for _, frameRate := range frameRates {
					dataPath := genDataPath(codec, resolution, frameRate)
					param := paramData{
						Name:         fmt.Sprintf("%s%s_%sp_%sfps", groupPrefix, codec, resolution, frameRate),
						File:         dataPath,
						HardwareDeps: groupParam.HardwareDeps,
						SoftwareDeps: fillSwDeps(codec, resolution, frameRate),
						Metadata:     []string{dataPath, dataPath + ".json"},
						Attr:         []string{"graphics_video_decodeaccel"},
					}
					if resolution == "2160" {
						param.Timeout = 4 * time.Minute
					} else {
						param.Timeout = defaultTimeout
					}
					param.EnabledFeatures = append(param.EnabledFeatures, groupParam.EnabledFeatures...)
					param.SoftwareDeps = append(param.SoftwareDeps, groupParam.SoftwareDeps...)

					params = append(params, param)
				}
			}
		}
	}

	// Another round for the concurrent decoder tests.
	for _, codec := range codecs {
		resolution := "1080"
		frameRate := "60"
		dataPath := genDataPath(codec, resolution, frameRate)
		param := paramData{
			Name:               fmt.Sprintf("%s_%sp_%sfps_concurrent", codec, resolution, frameRate),
			File:               dataPath,
			ConcurrentDecoders: true,
			SoftwareDeps:       append(fillSwDeps(codec, resolution, frameRate), "thread_safe_libva_backend"),
			Metadata:           []string{dataPath, dataPath + ".json"},
			Attr:               []string{"graphics_video_decodeaccel"},
			Timeout:            defaultTimeout,
		}

		params = append(params, param)
	}

	code := genparams.Template(t, `{{ range . }}{
		Name: {{ .Name | fmt }},
		Val:  chromeStackDecoderPerfParams{
			dataPath: {{ .File | fmt }},
			runConcurrentDecodersOnly: {{ .ConcurrentDecoders | fmt }},
			enabledFeatures: {{ .EnabledFeatures | fmt }},
		},
		Timeout: {{ .Timeout | fmt }},
		{{ if .HardwareDeps }}
		ExtraHardwareDeps: {{ .HardwareDeps }},
		{{ end }}
		{{ if .SoftwareDeps }}
		ExtraSoftwareDeps: {{ .SoftwareDeps | fmt }},
		{{ end }}
		ExtraData: {{ .Metadata | fmt }},
		{{ if .Attr }}
		ExtraAttr: {{ .Attr | fmt }},
		{{ end }}
	},
	{{ end }}`, params)
	genparams.Ensure(t, "chrome_stack_decoder_perf.go", code)

	// Clear the params, and compose another set of params with a reduced set of
	// codecs, for the legacy variant of the test.
	params = nil
	var reducedCodecs = []string{"h264", "vp8", "vp9"}
	for _, codec := range reducedCodecs {
		for _, resolution := range resolutions {
			for _, frameRate := range frameRates {
				dataPath := genDataPath(codec, resolution, frameRate)
				param := paramData{
					Name:         fmt.Sprintf("%s_%sp_%sfps", codec, resolution, frameRate),
					File:         dataPath,
					SoftwareDeps: fillSwDeps(codec, resolution, frameRate),
					Metadata:     []string{dataPath, dataPath + ".json"},
					Attr:         []string{"graphics_video_decodeaccel"},
				}

				params = append(params, param)
			}
		}
	}

	legacyCode := genparams.Template(t, `{{ range . }}{
		Name: {{ .Name | fmt }},
		Val: {{ .File | fmt }},
		{{ if .SoftwareDeps }}
		ExtraSoftwareDeps: {{ .SoftwareDeps | fmt }},
		{{ end }}
		ExtraData: {{ .Metadata | fmt }},
		{{ if .Attr }}
		ExtraAttr: {{ .Attr | fmt }},
		{{ end }}
	},
	{{ end }}`, params)
	genparams.Ensure(t, "chrome_stack_decoder_legacy_perf.go", legacyCode)

}
