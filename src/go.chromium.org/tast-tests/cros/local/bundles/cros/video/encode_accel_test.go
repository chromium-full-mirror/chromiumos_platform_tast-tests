// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package video

import (
	"fmt"
	"strings"
	"testing"

	"go.chromium.org/tast-tests/cros/common/genparams"
	"go.chromium.org/tast-tests/cros/common/media/caps"
)

// NB: If modifying any of the test parameters, be sure to regenerate the test
// code by running the following in a CrOS SDK chroot:
// TAST_GENERATE_UPDATE=1 ~/chromiumos/src/platform/tast/tools/go.sh test -count=1 go.chromium.org/tast-tests/cros/local/bundles/cros/video

func toProfile(codec string) string {
	switch codec {
	case "h264":
		return "videotype.H264BaselineProf"
	case "vp8":
		return "videotype.VP8Prof"
	case "vp9":
		return "videotype.VP9Prof"
	case "av1":
		return "videotype.AV1MainProf"
	default:
		panic("Unknown profile")
	}
}

func encodeSoftwareDeps(codec string, height int, vbr bool) []string {
	var deps []string
	switch codec {
	case "h264":
		if height > 1080 {
			deps = append(deps, caps.HWEncodeH264_4K)
		} else {
			deps = append(deps, caps.HWEncodeH264)
		}
	case "vp8":
		if height > 1080 {
			deps = append(deps, caps.HWEncodeVP8_4K)
		} else {
			deps = append(deps, caps.HWEncodeVP8)
		}
	case "vp9":
		if height > 1080 {
			deps = append(deps, caps.HWEncodeVP9_4K)
		} else {
			deps = append(deps, caps.HWEncodeVP9)
		}
	case "av1":
		if height > 1080 {
			deps = append(deps, caps.HWEncodeAV1_4K)
		} else {
			deps = append(deps, caps.HWEncodeAV1)
		}
	default:
		panic("Unsupported codec")
	}

	if vbr {
		if codec == "h264" {
			deps = append(deps, caps.HWEncodeH264VBR)
		} else {
			panic(fmt.Sprintf("No vbr test is intended for %s", codec))
		}
	}
	if height%2 != 0 {
		switch codec {
		case "vp8":
			deps = append(deps, caps.HWEncodeVP8OddDimension)
		case "vp9":
			deps = append(deps, caps.HWEncodeVP9OddDimension)
		case "av1":
			deps = append(deps, caps.HWEncodeAV1OddDimension)
		default:
			panic(fmt.Sprintf("No odd dimension test is intended for %s", codec))
		}
	}

	return deps
}

func psnrThreshold(codec string, height int) float32 {
	psnrThresholdTable := map[string]map[int]float32{
		"h264": {
			180:  31.8,
			270:  30.0,
			360:  33.3,
			720:  36.0,
			1080: 35.0,
			2160: 34.5,
		},
		"vp8": {
			135:  25.6,
			180:  29.5,
			270:  28.5,
			360:  29.5,
			720:  35.7,
			1080: 35.3,
			2160: 35.3,
		},
		"vp9": {
			135:  25.6,
			180:  28.5,
			270:  29.5,
			360:  31.7,
			540:  33.3,
			720:  36.7,
			1080: 38.4,
			2160: 40.8,
		},
		"av1": {
			135:  25.7,
			180:  28.5,
			270:  29.5,
			360:  31.7,
			540:  33.3,
			720:  36.7,
			1080: 38.4,
			2160: 40.8,
		},
	}

	val, found := psnrThresholdTable[codec][height]
	if !found {
		panic(fmt.Sprintf("psnrThresholdTable[%s][%d] is not found", codec, height))
	}
	return val
}

func psnrThresholdSVC(codec string, height int, svcMode string) float32 {
	psnrThresholdSVCTable := map[string]map[int]map[string]float32{
		"h264": {
			720: {
				"l1t2": 35.0,
				"l1t3": 34.0,
			},
		},
		"vp8": {
			720: {
				// The same thresholds as vp8_720p is used because there is no
				// video.EncodeAccelPerf.vp8_720p_(l1t2|l1t3).
				"l1t2": 35.7,
				"l1t3": 35.7,
			},
			1080: {
				"l1t2": 37.8,
				"l1t3": 38.2,
			},
		},
		"vp9": {
			540: {
				"l1t2":     32.4,
				"l1t3":     32.7,
				"l2t3_key": 27.5,
				"l3t3_key": 25.0,
			},
			720: {
				"l1t2":     35.5,
				"l1t3":     36.0,
				"l2t3_key": 29.8,
				"l3t3_key": 25,
			},
		},
	}

	val, found := psnrThresholdSVCTable[codec][height][svcMode]
	if !found {
		panic(fmt.Sprintf("psnrThresholdTable[%s][%d][%s] is not found", codec, height, svcMode))
	}
	return val
}

func TestEncodeAccelParams(t *testing.T) {
	type encodeAccelParam struct {
		Name                   string
		WebMName               string
		Profile                string
		PSNRThreshold          float32
		SVCMode                string
		DisableGlobalVaapiLock bool
		BitrateMode            string
		ExtraSoftwareDeps      []string
		ExtraData              []string
	}
	type testPatterns struct {
		title    string
		testType string
		heights  []int
	}

	basicHeights := []int{135, 180, 270, 360, 720, 1080, 2160}
	testVideos := map[int]string{
		135: "encode/desktop2-240x135_850frames.vp9.webm",
		180: "encode/desktop2-320x180_850frames.vp9.webm",
		270: "encode/desktop2-480x270_850frames.vp9.webm",
		360: "encode/desktop2-640x360_850frames.vp9.webm",
		// 540p is only used in VP9 SVC cases.
		540:  "encode/desktop2-960x540_850frames.vp9.webm",
		720:  "encode/desktop2-1280x720_850frames.vp9.webm",
		1080: "encode/desktop2-1920x1080_490frames.vp9.webm",
		2160: "encode/desktop2-3840x2160_170frames.vp9.webm",
	}

	var params []encodeAccelParam
	// Standard cases.
	for _, codec := range []string{"h264", "vp8", "vp9", "av1"} {
		for _, height := range basicHeights {
			if codec == "h264" && height == 135 {
				continue
			}
			webMFile := testVideos[height]
			webMJSONFile := webMFile + ".json"
			param := encodeAccelParam{
				Name:              fmt.Sprintf("%s_%dp", codec, height),
				WebMName:          webMFile,
				Profile:           toProfile(codec),
				PSNRThreshold:     psnrThreshold(codec, height),
				BitrateMode:       "cbr",
				ExtraSoftwareDeps: encodeSoftwareDeps(codec, height, false),
				ExtraData:         []string{webMFile, webMJSONFile},
			}
			params = append(params, param)
		}
	}

	// SVC encoding.
	type svcParam struct {
		codec    string
		height   int
		svcModes []string
	}
	for _, p := range []svcParam{
		{"h264", 720, []string{"l1t2", "l1t3"}},
		{"vp8", 720, []string{"l1t2", "l1t3"}},
		{"vp8", 1080, []string{"l1t2", "l1t3"}},
		{"vp9", 540, []string{"l2t3_key", "l3t3_key"}},
		{"vp9", 720, []string{"l1t2", "l1t3", "l2t3_key", "l3t3_key"}},
	} {
		codec := p.codec
		height := p.height
		for _, svcMode := range p.svcModes {
			webMFile := testVideos[height]
			webMJSONFile := webMFile + ".json"
			param := encodeAccelParam{
				Name:              fmt.Sprintf("%s_%dp_%s", codec, height, svcMode),
				WebMName:          webMFile,
				Profile:           toProfile(codec),
				PSNRThreshold:     psnrThresholdSVC(codec, height, svcMode),
				SVCMode:           strings.ToUpper(svcMode),
				BitrateMode:       "cbr",
				ExtraSoftwareDeps: append(encodeSoftwareDeps(codec, height, false), "vaapi"),
				ExtraData:         []string{webMFile, webMJSONFile},
			}
			params = append(params, param)
		}
	}

	// VBR cases
	for _, svcMode := range []string{"", "l1t2", "l1t3"} {
		codec := "h264"
		height := 720
		webMFile := testVideos[height]
		webMJSONFile := webMFile + ".json"
		deps := encodeSoftwareDeps(codec, height, true)
		var svcModeStr string
		if svcMode != "" {
			svcModeStr = "_" + svcMode
			deps = append(deps, "vaapi")
		}
		param := encodeAccelParam{
			Name:              fmt.Sprintf("%s_720p%s_vbr", codec, svcModeStr),
			WebMName:          webMFile,
			Profile:           toProfile(codec),
			PSNRThreshold:     psnrThreshold(codec, height),
			SVCMode:           strings.ToUpper(svcMode),
			BitrateMode:       "vbr",
			ExtraSoftwareDeps: deps,
			ExtraData:         []string{webMFile, webMJSONFile},
		}
		params = append(params, param)
	}
	// Vaapi lock is disabled
	for _, codec := range []string{"h264", "vp8", "vp9", "av1"} {
		height := 1080
		webMFile := testVideos[height]
		webMJSONFile := webMFile + ".json"
		param := encodeAccelParam{
			Name:                   fmt.Sprintf("%s_1080p_global_vaapi_lock_disabled", codec),
			WebMName:               webMFile,
			Profile:                toProfile(codec),
			PSNRThreshold:          psnrThreshold(codec, height),
			BitrateMode:            "cbr",
			DisableGlobalVaapiLock: true,
			ExtraSoftwareDeps:      append(encodeSoftwareDeps(codec, height, false), "thread_safe_libva_backend"),
			ExtraData:              []string{webMFile, webMJSONFile},
		}
		params = append(params, param)
	}
	code := genparams.Template(t, `{{ range . }}{
			Name: {{ .Name | fmt }},
	        Val: encode.TestOptions{
	        WebMName: {{ .WebMName | fmt}},
	        Profile: {{ .Profile }},
	        PSNRThreshold: {{ .PSNRThreshold }},
	        {{ if .SVCMode }}
	        SVCMode : {{ .SVCMode | fmt}},
	        {{ end }}
	        BitrateMode : {{ .BitrateMode | fmt}},
	        {{ if .DisableGlobalVaapiLock }}
	        DisableGlobalVaapiLock : {{ .DisableGlobalVaapiLock | fmt}},
	        {{ end }}
	        },
	        ExtraSoftwareDeps: {{ .ExtraSoftwareDeps | fmt}},
	        ExtraData: {{ .ExtraData | fmt}},
		},
		{{ end }}`, params)
	genparams.Ensure(t, "encode_accel.go", code)
}
