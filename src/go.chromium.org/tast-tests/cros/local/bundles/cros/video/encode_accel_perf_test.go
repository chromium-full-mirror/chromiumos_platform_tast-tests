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
	"go.chromium.org/tast-tests/cros/local/media/videotype"
)

type videoTitle string

const (
	crowdRun      videoTitle = "crowd_run"
	desktop2      videoTitle = "desktop2"
	fallOut4      videoTitle = "fallout4"
	gipsRecMotion videoTitle = "gipsrecmotion"
	gipsReStat    videoTitle = "gipsrestat"
	lifeOfPixel   videoTitle = "life_of_pixel"
	lifeOfProcess videoTitle = "life_of_process"
	scWebBrowsing videoTitle = "sc_web_browsing"
)

func toCodecStr(profile videotype.CodecProfile) string {
	switch profile {
	case videotype.H264BaselineProf:
		return "h264"
	case videotype.H264HighProf:
		return "h264high"
	case videotype.VP8Prof:
		return "vp8"
	case videotype.VP9Prof:
		return "vp9"
	case videotype.AV1MainProf:
		return "av1"
	default:
		panic("Unknown profile")
	}
}

func toProfileStr(profile videotype.CodecProfile) string {
	switch profile {
	case videotype.H264BaselineProf:
		return "videotype.H264BaselineProf"
	case videotype.H264HighProf:
		return "videotype.H264HighProf"
	case videotype.VP8Prof:
		return "videotype.VP8Prof"
	case videotype.VP9Prof:
		return "videotype.VP9Prof"
	case videotype.AV1MainProf:
		return "videotype.AV1MainProf"
	default:
		panic("Unknown profile")
	}
}

func encodePerfSoftwareDeps(profile videotype.CodecProfile, height int, vbr bool) []string {
	var deps []string
	switch profile {
	case videotype.H264BaselineProf, videotype.H264HighProf:
		if height > 1080 {
			deps = append(deps, caps.HWEncodeH264_4K)
		} else {
			deps = append(deps, caps.HWEncodeH264)
		}
		if vbr {
			deps = append(deps, caps.HWEncodeH264VBR)
		}
	case videotype.VP8Prof:
		if height > 1080 {
			deps = append(deps, caps.HWEncodeVP8_4K)
		} else {
			deps = append(deps, caps.HWEncodeVP8)
		}
		if vbr {
			panic("No vbr test is intended for VP8")
		}
	case videotype.VP9Prof:
		if height > 1080 {
			deps = append(deps, caps.HWEncodeVP9_4K)
		} else {
			deps = append(deps, caps.HWEncodeVP9)
		}
		if vbr {
			panic("No vbr test is intended for VP9")
		}
	case videotype.AV1MainProf:
		if height > 1080 {
			deps = append(deps, caps.HWEncodeAV1_4K)
		} else {
			deps = append(deps, caps.HWEncodeAV1)
		}
		if vbr {
			panic("No vbr test is intended for AV1")
		}
	default:
		panic("Unsupported codec")
	}

	return deps
}

func webMAndJSONFile(title videoTitle, height int) (string, string) {
	webMFileNameTable := map[videoTitle]map[int]string{
		crowdRun: {
			180:  "encode/crowd_run-320x180_60frames.vp9.webm",
			270:  "encode/crowd_run-480x270_60frames.vp9.webm",
			360:  "encode/crowd_run-640x360_60frames.vp9.webm",
			540:  "encode/crowd_run-960x540_60frames.vp9.webm",
			720:  "encode/crowd_run-1280x720_60frames.vp9.webm",
			1080: "encode/crowd_run-1920x1080_60frames.vp9.webm",
			2160: "encode/crowd_run-3840x2160_60frames.vp9.webm",
		},
		desktop2: {
			180:  "encode/desktop2-320x180_850frames.vp9.webm",
			270:  "encode/desktop2-480x270_850frames.vp9.webm",
			360:  "encode/desktop2-640x360_850frames.vp9.webm",
			540:  "encode/desktop2-960x540_850frames.vp9.webm",
			720:  "encode/desktop2-1280x720_850frames.vp9.webm",
			1080: "encode/desktop2-1920x1080_490frames.vp9.webm",
			2160: "encode/desktop2-3840x2160_170frames.vp9.webm",
		},
		fallOut4: {
			720:  "encode/fallout4-1280x720_600frames.vp9.webm",
			1080: "encode/fallout4-1920x1080_290frames.vp9.webm",
			2160: "encode/fallout4-3840x2160_100frames.vp9.webm",
		},
		gipsRecMotion: {
			180:  "encode/gipsrecmotion-320x180_850frames.vp9.webm",
			270:  "encode/gipsrecmotion-480x270_850frames.vp9.webm",
			360:  "encode/gipsrecmotion-640x360_850frames.vp9.webm",
			540:  "encode/gipsrecmotion-960x540_850frames.vp9.webm",
			720:  "encode/gipsrecmotion-1280x720_850frames.vp9.webm",
			1080: "encode/gipsrecmotion-1920x1080_430frames.vp9.webm",
			2160: "encode/gipsrecmotion-3840x2160_140frames.vp9.webm",
		},
		gipsReStat: {
			180:  "encode/gipsrestat-320x180_846frames.vp9.webm",
			270:  "encode/gipsrestat-480x270_846frames.vp9.webm",
			360:  "encode/gipsrestat-640x360_846frames.vp9.webm",
			540:  "encode/gipsrestat-960x540_846frames.vp9.webm",
			720:  "encode/gipsrestat-1280x720_846frames.vp9.webm",
			1080: "encode/gipsrestat-1920x1080_450frames.vp9.webm",
			2160: "encode/gipsrestat-3840x2160_150frames.vp9.webm",
		},
		lifeOfPixel: {
			1080: "encode/life_of_pixel-1920x1080_600frames.vp9.webm",
			2160: "encode/life_of_pixel-3840x2160_600frames.vp9.webm",
		},
		lifeOfProcess: {
			1080: "encode/life_of_process-1920x1080_600frames.vp9.webm",
		},
		scWebBrowsing: {
			1080: "encode/sc_web_browsing-1920x1080_300frames.vp9.webm",
		},
	}
	webMFileName, found := webMFileNameTable[title][height]
	if !found {
		panic(fmt.Sprintf("no webm file is found for %s:%d", title, height))
	}
	webMJSONFileName := webMFileName + ".json"
	return webMFileName, webMJSONFileName
}

func hardwareDeps(height int) string {
	// Some grunt and octopus devices fails due to disk space shortage in 1080p and 2160p test cases.
	// Set 24GB requirement.
	if height >= 1080 {
		return "hwdep.D(hwdep.MinStorage(24))"
	}
	return ""
}

func TestEncodeAccelPerfParams(t *testing.T) {
	type encodeAccelPerfParam struct {
		Name         string
		TestType     string
		WebMName     string
		Profile      string
		BitrateMode  string
		Bitrate      int
		Content      string
		SVCMode      string
		Attr         []string
		Data         []string
		SoftwareDeps []string
		HardwareDeps string
	}
	type testPatterns struct {
		title    videoTitle
		testType string
		heights  []int
	}

	basicHeights := []int{180, 270, 360, 540, 720, 1080, 2160}
	testVideos := []testPatterns{
		{crowdRun, "encode.Speed", basicHeights},
		{desktop2, "encode.SpeedAndQuality", basicHeights},
		{fallOut4, "encode.SpeedAndQuality", []int{720, 1080, 2160}},
		{gipsRecMotion, "encode.SpeedAndQuality", basicHeights},
		{gipsReStat, "encode.SpeedAndQuality", basicHeights},
		{lifeOfPixel, "encode.SpeedAndQuality", []int{1080, 2160}},
		{lifeOfProcess, "encode.SpeedAndQuality", []int{1080}},
		{scWebBrowsing, "encode.SpeedAndQuality", []int{1080}},
	}

	var params []encodeAccelPerfParam
	for _, testVideo := range testVideos {
		for _, profile := range []videotype.CodecProfile{videotype.H264BaselineProf,
			videotype.VP8Prof, videotype.VP9Prof, videotype.AV1MainProf} {
			for _, height := range testVideo.heights {
				if height == 540 && profile != videotype.VP9Prof {
					continue
				}
				webMFile, webMJSONFile := webMAndJSONFile(testVideo.title, height)
				param := encodeAccelPerfParam{
					Name:         fmt.Sprintf("%s_%dp_%s", toCodecStr(profile), height, testVideo.title),
					WebMName:     webMFile,
					Profile:      toProfileStr(profile),
					BitrateMode:  "cbr",
					TestType:     testVideo.testType,
					Content:      "encode.Camera",
					Attr:         []string{"graphics_perbuild"},
					Data:         []string{webMFile, webMJSONFile},
					SoftwareDeps: encodePerfSoftwareDeps(profile, height, false),
					HardwareDeps: hardwareDeps(height),
				}
				params = append(params, param)
			}
		}
	}

	// Screen sharing: 1080p
	for _, title := range []videoTitle{
		lifeOfPixel,
		lifeOfProcess,
		scWebBrowsing,
	} {
		for _, profile := range []videotype.CodecProfile{videotype.VP8Prof, videotype.VP9Prof, videotype.AV1MainProf} {
			height := 1080
			webMFile, webMJSONFile := webMAndJSONFile(title, height)
			param := encodeAccelPerfParam{
				Name:         fmt.Sprintf("%s_%dp_%s_display", toCodecStr(profile), height, title),
				WebMName:     webMFile,
				Profile:      toProfileStr(profile),
				BitrateMode:  "cbr",
				Content:      "encode.Display",
				TestType:     "encode.SpeedAndQuality",
				Attr:         []string{"graphics_perbuild"},
				Data:         []string{webMFile, webMJSONFile},
				SoftwareDeps: encodePerfSoftwareDeps(profile, height, false),
				HardwareDeps: hardwareDeps(height),
			}
			params = append(params, param)
		}
	}

	//Temporal and spatial layer encoding.
	for _, c := range [][]interface{}{
		{videotype.H264BaselineProf, 720, "L1T2", desktop2},
		{videotype.H264BaselineProf, 720, "L1T3", desktop2},
		{videotype.VP9Prof, 540, "L1T2", desktop2},
		{videotype.VP9Prof, 540, "L1T3", desktop2},
		{videotype.VP9Prof, 540, "L2T3_KEY", desktop2},
		{videotype.VP9Prof, 540, "L3T3_KEY", desktop2},
		{videotype.VP9Prof, 540, "S2T3", desktop2},
		{videotype.VP9Prof, 540, "S3T3", desktop2},
		{videotype.VP9Prof, 720, "L1T2", desktop2},
		{videotype.VP9Prof, 720, "L1T3", desktop2},
		{videotype.VP9Prof, 720, "L2T3_KEY", desktop2},
		{videotype.VP9Prof, 720, "L3T3_KEY", desktop2},
		{videotype.VP9Prof, 720, "S2T3", desktop2},
		{videotype.VP9Prof, 720, "S3T3", desktop2},
		{videotype.VP8Prof, 1080, "L1T2", lifeOfPixel},
		{videotype.VP8Prof, 1080, "L1T3", lifeOfPixel},
		{videotype.VP8Prof, 1080, "L1T2", lifeOfProcess},
		{videotype.VP8Prof, 1080, "L1T3", lifeOfProcess},
		{videotype.VP8Prof, 1080, "L1T2", scWebBrowsing},
		{videotype.VP8Prof, 1080, "L1T3", scWebBrowsing},
	} {
		profile := c[0].(videotype.CodecProfile)
		height := c[1].(int)
		svc := c[2].(string)
		deps := encodePerfSoftwareDeps(profile, height, false)
		deps = append(deps, "vaapi")
		if profile == videotype.VP9Prof && height == 540 && svc == "L3T3_KEY" {
			deps = append(deps, caps.HWEncodeVP9OddDimension)
		}

		title := c[3].(videoTitle)
		webMFile, webMJSONFile := webMAndJSONFile(title, height)
		param := encodeAccelPerfParam{
			Name:         fmt.Sprintf("%s_%dp_%s_%s", toCodecStr(profile), height, strings.ToLower(svc), title),
			WebMName:     webMFile,
			Profile:      toProfileStr(profile),
			SVCMode:      svc,
			BitrateMode:  "cbr",
			Content:      "encode.Camera",
			TestType:     "encode.SpeedAndQuality",
			Attr:         []string{"graphics_perbuild"},
			Data:         []string{webMFile, webMJSONFile},
			SoftwareDeps: deps,
			HardwareDeps: hardwareDeps(height),
		}
		params = append(params, param)
	}

	// VBR encoding.
	for _, height := range []int{720, 1080, 2160} {
		title := fallOut4
		profile := videotype.H264BaselineProf
		webMFile, webMJSONFile := webMAndJSONFile(title, height)
		param := encodeAccelPerfParam{
			Name:         fmt.Sprintf("%s_%dp_vbr_%s", toCodecStr(profile), height, title),
			WebMName:     webMFile,
			Profile:      toProfileStr(profile),
			BitrateMode:  "vbr",
			Content:      "encode.Camera",
			TestType:     "encode.Quality",
			Attr:         []string{"graphics_perbuild"},
			Data:         []string{webMFile, webMJSONFile},
			SoftwareDeps: encodePerfSoftwareDeps(profile, height, true),
			HardwareDeps: hardwareDeps(height),
		}
		params = append(params, param)
	}
	// VBR encoding: H264 high profile and high bitrates.
	for _, multiplier := range []int{1, 2, 4, 6, 8} {
		title := fallOut4
		height := 1080
		webMFile, webMJSONFile := webMAndJSONFile(title, height)
		defaultBitrate := 2480 * 1000 * 2
		profile := videotype.H264HighProf
		param := encodeAccelPerfParam{
			Name:         fmt.Sprintf("%s_%dp_x%d_vbr_%s", toCodecStr(profile), height, multiplier, title),
			WebMName:     webMFile,
			Profile:      toProfileStr(profile),
			BitrateMode:  "vbr",
			Bitrate:      defaultBitrate * multiplier,
			Content:      "encode.Camera",
			TestType:     "encode.Quality",
			Attr:         []string{"graphics_weekly"},
			Data:         []string{webMFile, webMJSONFile},
			SoftwareDeps: encodePerfSoftwareDeps(profile, height, true),
			HardwareDeps: hardwareDeps(height),
		}
		params = append(params, param)
	}

	code := genparams.Template(t, `{{ range . }}{
			Name: {{ .Name | fmt }},
            Val: encode.TestOptions{
            WebMName: {{ .WebMName | fmt}},
            Profile: {{ .Profile }},
            TestType: {{ .TestType }},
            {{ if .SVCMode }}
            SVCMode : {{ .SVCMode | fmt}},
            {{ end }}
            BitrateMode : {{ .BitrateMode | fmt}},
            {{ if .Bitrate }}
            Bitrate : {{ .Bitrate | fmt}},
            {{ end }}
            Content: {{ .Content }},
            },
            ExtraAttr: {{ .Attr | fmt}},
            ExtraData: {{ .Data | fmt}},
            ExtraSoftwareDeps: {{ .SoftwareDeps | fmt}},
            {{ if .HardwareDeps }}
            ExtraHardwareDeps : {{ .HardwareDeps }},
            {{ end }}
		},
		{{ end }}`, params)
	genparams.Ensure(t, "encode_accel_perf.go", code)
}
