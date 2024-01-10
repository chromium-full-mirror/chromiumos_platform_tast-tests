// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package video

import (
	"fmt"
	"strings"
	"testing"

	"go.chromium.org/tast-tests/cros/common/genparams"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/video/playback"
	"go.chromium.org/tast-tests/cros/local/coords"
)

// To regenerate the test parameters by running the following in a chroot:
// TAST_GENERATE_UPDATE=1 ~/chromiumos/src/platform/tast/tools/go.sh test -count=1 go.chromium.org/tast-tests/cros/local/bundles/cros/video

// codec
var playbackPerfLongFile = map[string]string{
	"h264": "crosvideo/1080.mp4",
	"hevc": "crosvideo/1080-5-frag.mp4",
	"vp8":  "crosvideo/1080_vp8.webm",
	"vp9":  "crosvideo/1080.webm",
	"av1":  "crosvideo/av1_1080p_30fps.mp4",
}

// TODO(hiroh): genPlaybackPerfDataPath() and genPlaybackPerfSwDeps() can be
// reused by other parameter generator code. Put the functions in common places.

func genPlaybackPerfDataPath(codec string, resolution, fps int) string {
	if fps != 30 && fps != 60 && fps != 120 {
		panic("Unexpected fps")
	}
	numFrames := fps * 10

	extension := ""
	switch codec {
	case "h264", "hevc", "hevc10", "av1":
		extension = codec + ".mp4"
	case "vp8", "vp9":
		extension = codec + ".webm"
	default:
		panic("Unexpected codec")
	}

	return fmt.Sprintf("perf/%s/%dp_%dfps_%dframes.%s",
		codec, resolution, fps, numFrames, extension)
}

func genPlaybackPerfSwDeps(codec string, resolution, fps int, dec string) []string {
	var swDeps []string

	if codec == "h264" || codec == "hevc" || codec == "hevc10" {
		swDeps = append(swDeps, "proprietary_codecs")
	}

	if dec != "hw" {
		return swDeps
	}

	if fps == 120 {
		fps = 60 // For SwDeps purposes (a.k.a. capabilities), 60 is good enough for 120fps.
	}

	if resolution < 1080 {
		resolution = 1080
	}
	if codec == "hevc10" {
		swDeps = append(swDeps, fmt.Sprintf("autotest-capability:hw_dec_hevc_%d_%d_10bpp", resolution, fps))
	} else {
		swDeps = append(swDeps, fmt.Sprintf("autotest-capability:hw_dec_%s_%d_%d", codec, resolution, fps))
	}
	return swDeps
}

type playbackParamData struct {
	Name string

	// playback.Config
	File                      string
	DecoderType               playback.DecoderType
	BrowserType               string
	Grid                      coords.Size
	PerfTracing               bool
	MeasureSteadyStateMetrics bool
	MeasureRoughness          bool

	SoftwareDeps []string
	HardwareDeps string
	Data         []string
	Attr         []string
	Fixture      string
	ExtraAttr    []string
}

func genPlaybackParam(codec, file string, resolution, fps int, dec, nameSuffix, fixture string, extendDeps []string) playbackParamData {
	if file == "" {
		fmt.Println(codec, resolution, fps, dec, nameSuffix)
		panic("file is empty")
	}
	testName := fmt.Sprintf("%s_%dp_%dfps_%s", codec, resolution, fps, dec)
	if nameSuffix != "" {
		testName += "_" + nameSuffix
	}
	decType := playback.Hardware
	if dec == "sw" {
		decType = playback.Software
	}
	if fixture == "" {
		if dec == "hw" {
			fixture = "chromeVideo"
		} else {
			fixture = "chromeVideoWithSWDecoding"
		}
	}

	brwType := "browser.TypeAsh"
	if strings.Contains(nameSuffix, "lacros") {
		brwType = "browser.TypeLacros"
	}
	deps := genPlaybackPerfSwDeps(codec, resolution, fps, dec)
	if len(extendDeps) > 0 {
		deps = append(deps, extendDeps...)
	}
	// FSI folks only want h264_1080p_60fps_hw and vp9_1080p_60fps_hw.
	var extraAttr []string
	if (codec == "h264" || codec == "vp9") && resolution == 1080 && fps == 60 && dec == "hw" && fixture == "chromeVideo" {
		extraAttr = []string{"group:graphics", "graphics_video", "graphics_nightly", "group:crosbolt", "crosbolt_fsi_check"}
	}
	return playbackParamData{
		Name:         testName,
		File:         file,
		DecoderType:  decType,
		BrowserType:  brwType,
		SoftwareDeps: deps,
		Data:         []string{file},
		Fixture:      fixture,
		ExtraAttr:    extraAttr,
	}
}

func TestPlaybackPerfConfig(t *testing.T) {
	var params []playbackParamData

	codecs := []string{"h264", "vp8", "vp9", "av1", "hevc"}
	for _, codec := range codecs {
		for _, resolution := range []int{720, 1080, 2160, 4320} {
			if resolution >= 4320 && (codec == "vp8" || codec == "h264" || codec == "av1") {
				// VP8 doesn't support 8K.
				// There are no SoCs with 8K H.264 decode capabilities.
				// TODO(b/250099630): Craft the AV1 test vectors and add them.
				continue
			}
			fpss := []int{30}
			if resolution >= 1080 {
				fpss = append(fpss, 60)
			}
			if resolution == 1080 {
				fpss = append(fpss, 120)
			}
			decs := []string{"hw"}
			if codec != "hevc" && resolution <= 2160 {
				decs = append(decs, "sw")
			}
			for _, fps := range fpss {
				if fps == 120 && (codec == "vp8") {
					// VP8 120fps streams aren't common in the wild.
					continue
				}
				for _, dec := range decs {
					param :=
						genPlaybackParam(codec, genPlaybackPerfDataPath(codec, resolution, fps),
							resolution, fps, dec, "", "", nil)
					if codec == "hevc" {
						param.HardwareDeps = "hwdep.SupportsHEVCVideoDecodingInChrome()"
					}
					params = append(params, param)
				}
			}
		}
	}
	// HEVC10
	for _, fps := range []int{30, 60} {
		for _, resolution := range []int{2160, 4320} {
			codec, dec := "hevc10", "hw"
			param := genPlaybackParam(codec, genPlaybackPerfDataPath(codec, resolution, fps),
				resolution, fps, dec, "", "", []string{})
			param.HardwareDeps = "hwdep.SupportsHEVCVideoDecodingInChrome()"
			params = append(params, param)
		}
	}
	// V4L2 Flat Stateful.
	for _, codec := range []string{"h264", "vp8", "vp9"} {
		resolutions := []int{1080}
		if codec == "vp9" {
			resolutions = append(resolutions, 2160)
		}
		for _, resolution := range resolutions {
			fps, dec := 60, "hw"
			param := genPlaybackParam(codec, genPlaybackPerfDataPath(codec, resolution, fps),
				resolution, fps, dec, "v4l2_flat_stateful", "chromeVideoWithV4L2FlatStatefulDecoder",
				[]string{"v4l2_codec"})
			// E.g. MT8173 Hana and QC SC7180 Trogdor.
			param.HardwareDeps = "hwdep.SupportsV4L2StatefulVideoDecoding()"
			params = append(params, param)
		}
	}
	// long
	for _, codec := range []string{"h264", "hevc", "vp8", "vp9", "av1"} {
		for _, dec := range []string{"hw", "sw"} {
			if codec == "hevc" && dec == "sw" {
				// There is no support for HEVC SW decoding in Chrome-on-ChromeOS.
				continue
			}
			resolution, fps := 1080, 30
			file := playbackPerfLongFile[codec]
			param := genPlaybackParam(codec, file, resolution, fps, dec,
				"long", "", []string{"drm_atomic"})
			// "rogue" is for MT8173 hana.
			param.HardwareDeps = "hwdep.SkipGPUFamily(\"rogue\"), hwdep.InternalDisplay()"
			if codec == "hevc" {
				param.HardwareDeps += ", hwdep.SupportsHEVCVideoDecodingInChrome()"
			}
			param.MeasureRoughness = true
			params = append(params, param)
		}
	}

	// Out-of-process video decoding (ash-chrome).
	for _, resolution := range []int{720, 1080, 2160} {
		fpss := []int{30}
		if resolution >= 1080 {
			fpss = append(fpss, 60)
		}
		for _, fps := range fpss {
			param := genPlaybackParam("h264", genPlaybackPerfDataPath("h264", resolution, fps),
				resolution, fps, "hw", "oopvd", "chromeVideoOOPVD", nil)
			if resolution == 1080 && fps == 30 {
				param.MeasureSteadyStateMetrics = true
			}
			params = append(params, param)
		}
	}

	// Long Out-of-process video decoding (ash-chrome)
	for _, codec := range []string{"h264", "hevc", "vp9", "av1"} {
		resolution, fps, dec := 1080, 30, "hw"
		file := playbackPerfLongFile[codec]
		param := genPlaybackParam(codec, file, resolution, fps, dec,
			"long_oopvd", "chromeVideoOOPVD",
			[]string{"drm_atomic"})
		// "rogue" is for MT8173 hana.
		param.HardwareDeps = "hwdep.SkipGPUFamily(\"rogue\"), hwdep.InternalDisplay()"
		if codec == "h264" {
			param.MeasureSteadyStateMetrics = true
		}
		param.MeasureRoughness = true
		params = append(params, param)
	}

	// grid
	// TODO(b/234643665): Reduce these to 2x2 1080p (as many pixels as 4K).
	for _, codec := range []string{"h264", "hevc", "vp8", "vp9", "av1"} {
		resolution, fps, dec := 720, 30, "hw"
		param := genPlaybackParam(codec, genPlaybackPerfDataPath(codec, resolution, fps),
			resolution, fps, dec, "3x3", "", nil)
		param.Grid.Width = 3
		param.Grid.Height = 3
		params = append(params, param)
	}

	// grid and oop-vd with a different thread type
	{
		codec, resolution, fps, dec := "h264", 720, 30, "hw"
		param := genPlaybackParam(codec, genPlaybackPerfDataPath(codec, resolution, fps),
			resolution, fps, dec, "3x3_oopvd", "chromeVideoOOPVD", nil)
		param.Grid.Width = 3
		param.Grid.Height = 3
		params = append(params, param)
		param = genPlaybackParam(codec, genPlaybackPerfDataPath(codec, resolution, fps),
			resolution, fps, dec, "3x3_oopvd_decoder_thread", "chromeVideoOOPVDAndDedicatedDecoderThread", nil)
		param.Grid.Width = 3
		param.Grid.Height = 3
		params = append(params, param)
	}

	// lacros
	for _, resolution := range []int{720, 1080, 2160} {
		fpss := []int{30}
		if resolution >= 1080 {
			fpss = append(fpss, 60)
		}
		for _, fps := range fpss {
			param := genPlaybackParam("h264", genPlaybackPerfDataPath("h264", resolution, fps),
				resolution, fps, "hw", "lacros", "chromeVideoLacros", []string{"lacros"})
			if resolution == 1080 && fps == 30 {
				param.MeasureSteadyStateMetrics = true
			}
			params = append(params, param)
		}
	}

	// Long lacros
	for _, codec := range []string{"h264", "hevc", "vp9", "av1"} {
		resolution, fps, dec := 1080, 30, "hw"
		file := playbackPerfLongFile[codec]
		param := genPlaybackParam(codec, file, resolution, fps, dec,
			"long_lacros", "chromeVideoLacros",
			[]string{"drm_atomic", "lacros"})
		// "rogue" is for MT8173 hana.
		param.HardwareDeps = "hwdep.SkipGPUFamily(\"rogue\"), hwdep.InternalDisplay()"
		if codec == "h264" {
			param.MeasureSteadyStateMetrics = true
		}
		param.MeasureRoughness = true
		params = append(params, param)
	}

	for _, codec := range []string{"h264", "vp9"} {
		// 1080p x 2 ~= 2K, 480p x 9  ~= 2K, 360p x 16 ~= 2K, 180p x 49 ~= 1260p
		// TODO(b/237600904): Add {180, 7, 7} once the issue is resolved.
		for _, resGrid := range [][3]int{{1080, 2, 1}, {480, 3, 3}, {360, 4, 4}} {
			resolution, gridW, gridH := resGrid[0], resGrid[1], resGrid[2]
			numVideos := gridW * gridH
			fps, dec := 30, "hw"
			testNameSuffix := fmt.Sprintf("x%d", numVideos)
			param := genPlaybackParam(codec,
				genPlaybackPerfDataPath(codec, resolution, fps),
				resolution, fps, dec, testNameSuffix, "", nil)
			param.Grid.Width = gridW
			param.Grid.Height = gridH
			param.PerfTracing = true
			if numVideos > 10 {
				// More than 10 videos in parallel is too much for Grunt, see b/290637628.
				param.HardwareDeps = "hwdep.SkipGPUFamily(\"stoney\")"
			}
			param.ExtraAttr = []string{"group:graphics", "graphics_video", "graphics_nightly"}
			params = append(params, param)

		}
	}

	for _, codec := range []string{"h264", "vp9", "av1"} {
		dec := "hw"
		for _, resolution := range []int{1080, 2160} {
			for _, fps := range []int{30, 60} {
				param := genPlaybackParam(codec, genPlaybackPerfDataPath(codec, resolution, fps), resolution, fps, dec,
					"intel_mc", "chromeVideoWithIntelMediaCompression",
					[]string{})
				param.HardwareDeps = "hwdep.GPUFamily(\"meteorlake\", \"alderlake\", \"raptorlake\")"
				params = append(params, param)
			}

		}
	}

	// V4L2 Flat stateful decoder
	for _, codec := range []string{"h264", "vp8", "vp9"} {
		resolution, fps, dec := 1080, 30, "hw"
		file := playbackPerfLongFile[codec]
		param := genPlaybackParam(codec, file, resolution, fps, dec,
			"v4l2_flat_stateful_long", "chromeVideoWithV4L2FlatStatefulDecoder",
			[]string{"v4l2_codec"})
		param.HardwareDeps = "hwdep.SupportsV4L2FlatStatefulVideoDecoding()"
		param.MeasureRoughness = true
		params = append(params, param)
	}

	code := genparams.Template(t, `{{ range . }}{
		Name: {{ .Name | fmt }},
		Val:  playback.Config{
			FileName: {{ .File | fmt }},
			DecoderType: {{ .DecoderType }},
			BrowserType: {{ .BrowserType }},
			{{ if or (ne .Grid.Width 0) (ne .Grid.Height 0) }}
			Grid: coords.Size{
				Width: {{ .Grid.Width | fmt }},
				Height: {{ .Grid.Height | fmt }},
			},
			{{ end }}
			PerfMeasurement: true,
			PerfSetting: playback.PerfSetting {
				{{ if .PerfTracing }} PerfTracing: {{ .PerfTracing | fmt }}, {{ end }}
				{{ if .MeasureSteadyStateMetrics }} MeasureSteadyStateMetrics: {{ .MeasureSteadyStateMetrics | fmt }}, {{ end }}
				{{ if .MeasureRoughness }} MeasureRoughness: {{ .MeasureRoughness | fmt }}, {{ end }}
			},
		},
		{{ if .HardwareDeps }}
		ExtraHardwareDeps: hwdep.D({{ .HardwareDeps }}),
		{{ end }}
		{{ if .SoftwareDeps }}
		ExtraSoftwareDeps: {{ .SoftwareDeps | fmt }},
		{{ end }}
		ExtraData: {{ .Data | fmt }},
		{{ if .ExtraAttr }}
		ExtraAttr: {{ .ExtraAttr | fmt }},
		{{ end }}
		Fixture: {{ .Fixture | fmt }},
	},
	{{ end }}`, params)

	genparams.Ensure(t, "playback_perf.go", code)
}
