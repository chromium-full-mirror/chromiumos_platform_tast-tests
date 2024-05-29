// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package video

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"go.chromium.org/tast-tests/cros/common/genparams"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/video/playback"
	"go.chromium.org/tast-tests/cros/local/media/pre"
)

// To regenerate the test parameters by running the following in a chroot:
// TAST_GENERATE_UPDATE=1 ~/chromiumos/src/platform/tast/tools/go.sh test -count=1 go.chromium.org/tast-tests/cros/local/bundles/cros/video

var (
	// The time to wait after CPU is stable so as to measure solid metric values for short video tests.
	measurementDurationShort = 25 * time.Second
	// The time to wait after CPU is stable so as to measure solid metric values for long video tests.
	measurementDurationLong = 100 * time.Second
)

// codec
var playbackPerfLongFile = map[string]string{
	"h264": "crosvideo/1080.mp4",
	"hevc": "crosvideo/1080-5-frag.mp4",
	"vp8":  "crosvideo/1080_vp8.webm",
	"vp9":  "crosvideo/1080.webm",
	"av1":  "crosvideo/av1_1080p_30fps.mp4",
}

func genPlaybackPerfParam(codec, file string, resolution, fps int, dec, nameSuffix, fixture string, extendDeps []string) playback.ParamData {
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
	deps := playback.GenSwDeps(codec, resolution, fps, dec)
	if len(extendDeps) > 0 {
		deps = append(deps, extendDeps...)
	}
	// FSI folks only want h264_1080p_60fps_hw and vp9_1080p_60fps_hw.
	var extraAttr []string
	if (codec == "h264" || codec == "vp9") && resolution == 1080 && fps == 60 && dec == "hw" && fixture == "chromeVideo" {
		extraAttr = []string{"group:graphics", "graphics_video", "graphics_nightly", "group:crosbolt", "crosbolt_fsi_check"}
	}
	var hwDeps []string
	if codec == "hevc" || codec == "hevc10" {
		hwDeps = []string{"hwdep.SupportsHEVCVideoDecodingInChrome()"}
	}
	return playback.ParamData{
		Name:            testName,
		File:            file,
		DecoderType:     decType,
		BrowserType:     brwType,
		SoftwareDeps:    deps,
		HardwareDeps:    hwDeps,
		Data:            []string{file},
		Fixture:         fixture,
		ExtraAttr:       extraAttr,
		PerfMeasurement: true,
		Duration:        measurementDurationShort,
		Timeout:         5 * time.Minute,
	}
}

func TestPlaybackPerfConfig(t *testing.T) {
	var params []playback.ParamData

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
						genPlaybackPerfParam(codec, playback.GenDataPath(codec, resolution, fps),
							resolution, fps, dec, "", "", nil)
					params = append(params, param)
				}
			}
		}
	}
	// HEVC10
	for _, fps := range []int{30, 60} {
		for _, resolution := range []int{2160, 4320} {
			codec, dec := "hevc10", "hw"
			param := genPlaybackPerfParam(codec, playback.GenDataPath(codec, resolution, fps),
				resolution, fps, dec, "", "", []string{})
			params = append(params, param)
		}
	}
	// V4L2 Flat decoder
	for _, codec := range []string{"h264", "hevc", "vp8", "vp9", "av1"} {
		resolutions := []int{1080}
		if codec == "vp9" {
			resolutions = append(resolutions, 2160)
		}
		for _, resolution := range resolutions {
			fps, dec := 60, "hw"
			param := genPlaybackPerfParam(codec, playback.GenDataPath(codec, resolution, fps),
				resolution, fps, dec, "v4l2_flat", "chromeVideoWithV4L2FlatDecoder",
				[]string{"v4l2_codec"})
			param.HardwareDeps = append(param.HardwareDeps, "hwdep.SupportsV4L2FlatVideoDecoding()")
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
			param := genPlaybackPerfParam(codec, file, resolution, fps, dec,
				"long", "", []string{"drm_atomic"})
			// "rogue" is for MT8173 hana.
			param.HardwareDeps = append(param.HardwareDeps, "hwdep.SkipGPUFamily(\"rogue\")", "hwdep.InternalDisplay()")
			param.MeasureRoughness = true
			param.Duration = measurementDurationLong
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
			param := genPlaybackPerfParam("h264", playback.GenDataPath("h264", resolution, fps),
				resolution, fps, "hw", "inpvd", "chromeVideoINPVD", nil)
			if resolution == 1080 && fps == 30 {
				param.MeasureSteadyStateMetrics = true
			}
			params = append(params, param)
		}
	}

	// Long in-process video decoding (ash-chrome)
	for _, codec := range []string{"h264", "hevc", "vp9", "av1"} {
		resolution, fps, dec := 1080, 30, "hw"
		file := playbackPerfLongFile[codec]
		param := genPlaybackPerfParam(codec, file, resolution, fps, dec,
			"long_inpvd", "chromeVideoINPVD",
			[]string{"drm_atomic"})
		// "rogue" is for MT8173 hana.
		param.HardwareDeps = append(param.HardwareDeps, "hwdep.SkipGPUFamily(\"rogue\")", "hwdep.InternalDisplay()")
		if codec == "h264" {
			param.MeasureSteadyStateMetrics = true
		}
		param.MeasureRoughness = true
		param.Duration = measurementDurationLong
		params = append(params, param)
	}

	// Long GTFO OOP-VD (ash-chrome)
	{
		codec, resolution, fps, dec := "h264", 1080, 30, "hw"
		file := playbackPerfLongFile[codec]
		param := genPlaybackPerfParam(codec, file, resolution, fps, dec,
			"long_gtfo", "chromeVideoGTFO", []string{"drm_atomic"})
		// "rogue" is for MT8173 hana.
		param.HardwareDeps = append(param.HardwareDeps, "hwdep.SkipGPUFamily(\"rogue\")", "hwdep.InternalDisplay()")
		param.MeasureRoughness = true
		param.Duration = measurementDurationLong
		params = append(params, param)
	}

	// lacros
	for _, resolution := range []int{720, 1080, 2160} {
		fpss := []int{30}
		if resolution >= 1080 {
			fpss = append(fpss, 60)
		}
		for _, fps := range fpss {
			param := genPlaybackPerfParam("h264", playback.GenDataPath("h264", resolution, fps),
				resolution, fps, "hw", "lacros", pre.ChromeVideoLacrosFixture(pre.FakeMediaStreamUI), []string{"lacros"})
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
		param := genPlaybackPerfParam(codec, file, resolution, fps, dec,
			"long_lacros", pre.ChromeVideoLacrosFixture(pre.FakeMediaStreamUI),
			[]string{"drm_atomic", "lacros"})
		// "rogue" is for MT8173 hana.
		param.HardwareDeps = append(param.HardwareDeps, "hwdep.SkipGPUFamily(\"rogue\")", "hwdep.InternalDisplay()")
		if codec == "h264" {
			param.MeasureSteadyStateMetrics = true
		}
		param.MeasureRoughness = true
		param.Duration = measurementDurationLong
		params = append(params, param)
	}

	// Long lacros with GTFO OOP-VD
	{
		codec, resolution, fps, dec := "h264", 1080, 30, "hw"
		file := playbackPerfLongFile[codec]
		param := genPlaybackPerfParam(codec, file, resolution, fps, dec,
			"long_lacros_gtfo", pre.ChromeVideoLacrosFixture(pre.FakeMediaStreamUI, pre.GTFO),
			[]string{"drm_atomic", "lacros"})
		// "rogue" is for MT8173 hana.
		param.HardwareDeps = append(param.HardwareDeps, "hwdep.SkipGPUFamily(\"rogue\")", "hwdep.InternalDisplay()")
		param.MeasureSteadyStateMetrics = true
		param.MeasureRoughness = true
		param.Duration = measurementDurationLong
		params = append(params, param)
	}

	// All 1080p x 2, 720p x 4, 480p x 9 and 360p x 16 are equivalent to each
	// other in number of pixels decoded per second. The next logical steps would
	// be 240p x36 (6x6 grid) and maybe even 144p x100 (10x10 grid).
	// TODO(b/237600904): Add more cases once the issue is resolved.
	var resGrids = [][3]int{
		{1080, 2, 1},
		{720, 2, 2},
		{480, 3, 3},
		{360, 4, 4},
	}
	// Above this number of videos in the grid, we only run for codecs used in
	// video conference applications.
	const manyVideos = 9
	for _, codec := range []string{"h264", "hevc", "vp8", "vp9", "av1"} {
		for _, resGrid := range resGrids {
			resolution, gridW, gridH := resGrid[0], resGrid[1], resGrid[2]
			numVideos := gridW * gridH
			if numVideos >= manyVideos && (codec == "hevc" || codec == "av1") {
				continue
			}
			fps, dec := 30, "hw"
			testNameSuffix := fmt.Sprintf("x%d", numVideos)
			param := genPlaybackPerfParam(codec,
				playback.GenDataPath(codec, resolution, fps),
				resolution, fps, dec, testNameSuffix, "", nil)
			param.Grid.Width = gridW
			param.Grid.Height = gridH
			param.PerfTracing = true
			if numVideos > 10 {
				// More than 10 videos in parallel is too much for Grunt, see b/290637628.
				param.HardwareDeps = append(param.HardwareDeps, "hwdep.SkipGPUFamily(\"stoney\")")
			}
			if resolution != 720 {
				// "graphics_weekly" amounts to 2 or 3 times per week.
				param.ExtraAttr = []string{"group:graphics", "graphics_video", "graphics_weekly"}
			}
			params = append(params, param)

		}
	}

	for _, codec := range []string{"h264", "vp9", "av1"} {
		dec := "hw"
		for _, resolution := range []int{1080, 2160} {
			for _, fps := range []int{30, 60} {
				param := genPlaybackPerfParam(codec, playback.GenDataPath(codec, resolution, fps), resolution, fps, dec,
					"intel_mc", "chromeVideoWithIntelMediaCompression",
					[]string{})
				param.HardwareDeps = append(param.HardwareDeps, "hwdep.GPUFamily(\"meteorlake\", \"alderlake\", \"raptorlake\")")
				params = append(params, param)
			}

		}
	}

	// V4L2 Flat decoder
	for _, codec := range []string{"h264", "hevc", "vp8", "vp9", "av1"} {
		resolution, fps, dec := 1080, 30, "hw"
		file := playbackPerfLongFile[codec]
		param := genPlaybackPerfParam(codec, file, resolution, fps, dec,
			"v4l2_flat_long", "chromeVideoWithV4L2FlatDecoder",
			[]string{"v4l2_codec"})
		param.HardwareDeps = append(param.HardwareDeps, "hwdep.SupportsV4L2FlatVideoDecoding()")
		param.MeasureRoughness = true
		param.Duration = measurementDurationLong
		params = append(params, param)
	}

	for _, codec := range []string{"h264", "vp8", "vp9", "av1"} {
		resolution, dec := 1080, "hw"
		for _, fps := range []int{30, 60} {
			param := genPlaybackPerfParam(codec, playback.GenDataPath(codec, resolution, fps), resolution, fps, dec,
				"batch_decoding_in_renderer", "chromeVideoWithBatchDecodingInRenderer",
				[]string{})
			param.HardwareDeps = append(param.HardwareDeps, "hwdep.CPUSocFamily(\"intel\")")
			params = append(params, param)
		}

	}

	for _, codec := range []string{"h264", "vp9", "av1"} {
		resolution, dec := 1080, "hw"
		for _, fps := range []int{30, 60} {
			param := genPlaybackPerfParam(codec, playback.GenDataPath(codec, resolution, fps), resolution, fps, dec,
				"reduced_hardware_video_decoder_buffers", "chromeVideoWithReducedHardwareVideoDecoderBuffers",
				[]string{})
			params = append(params, param)
		}

	}

	for i, param := range params {
		params[i].FlattenedHardwareDeps = strings.Join(param.HardwareDeps, ",")
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
			{{ if .PerfMeasurement }}
			PerfMeasurement: {{ .PerfMeasurement | fmt }},
			PerfSetting: playback.PerfSetting {
				{{ if .PerfTracing }} PerfTracing: {{ .PerfTracing | fmt }}, {{ end }}
				{{ if .MeasureSteadyStateMetrics }} MeasureSteadyStateMetrics: {{ .MeasureSteadyStateMetrics | fmt }}, {{ end }}
				{{ if .MeasureRoughness }} MeasureRoughness: {{ .MeasureRoughness | fmt }}, {{ end }}
			},
			{{ end }}
			{{ if .SuspendResume }}
			SuspendResume: {{ .SuspendResume | fmt }},
			{{ end }}
			{{ if .Duration }}
			Duration: {{ .Duration | fmt }},
			{{ end }}
		},
		{{ if .HardwareDeps }}
		ExtraHardwareDeps: hwdep.D({{ .FlattenedHardwareDeps }}),
		{{ end }}
		{{ if .SoftwareDeps }}
		ExtraSoftwareDeps: {{ .SoftwareDeps | fmt }},
		{{ end }}
		ExtraData: {{ .Data | fmt }},
		{{ if .ExtraAttr }}
		ExtraAttr: {{ .ExtraAttr | fmt }},
		{{ end }}
		Fixture: {{ .Fixture | fmt }},
		Timeout: {{ .Timeout | fmt }},
	},
	{{ end }}`, params)

	genparams.Ensure(t, "playback_perf.go", code)
}
