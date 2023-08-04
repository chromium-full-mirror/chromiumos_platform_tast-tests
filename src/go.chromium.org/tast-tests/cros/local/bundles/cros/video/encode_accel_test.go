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

	return deps
}

func TestEncodeAccelParams(t *testing.T) {
	type encodeAccelParam struct {
		Name                   string
		WebMName               string
		Profile                string
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
		1080: "encode/desktop2-1920x1080_850frames.vp9.webm",
		2160: "encode/desktop2-3840x2160_430frames.vp9.webm",
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
