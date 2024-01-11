// Copyright 2024 The ChromiumOS Authors
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
)

// To regenerate the test parameters by running the following in a chroot:
// TAST_GENERATE_UPDATE=1 ~/chromiumos/src/platform/tast/tools/go.sh test -count=1 go.chromium.org/tast-tests/cros/local/bundles/cros/video

func genPlaybackStressParam(codec, file string, resolution, fps int, nameSuffix, fixture string, extendDeps []string, suspendResume bool, duration time.Duration) playback.ParamData {
	if file == "" {
		panic("file is empty")
	}
	testName := fmt.Sprintf("%s_%dp_%dfps_%v", codec, resolution, fps, duration)
	if nameSuffix != "" {
		testName += "_" + nameSuffix
	}
	decType := playback.Hardware
	if fixture == "" {
		fixture = "chromeVideo"
	}

	brwType := "browser.TypeAsh"
	if strings.Contains(nameSuffix, "lacros") {
		brwType = "browser.TypeLacros"
	}
	deps := append(playback.GenSwDeps(codec, resolution, fps, "hw"), extendDeps...)
	var extraAttr []string
	if duration > 30*time.Minute {
		extraAttr = append(extraAttr, []string{"graphics_av_analysis", "graphics_weekly"}...)
	} else {
		extraAttr = append(extraAttr, []string{"graphics_nightly"}...)
	}
	return playback.ParamData{
		Name:          testName,
		File:          file,
		DecoderType:   decType,
		BrowserType:   brwType,
		SoftwareDeps:  deps,
		Data:          []string{file},
		Fixture:       fixture,
		ExtraAttr:     extraAttr,
		SuspendResume: suspendResume,
		Duration:      duration,
		// Set the test timeout to 2 times the intended playback time.
		Timeout: 2 * duration,
	}
}

func TestPlaybackStressConfig(t *testing.T) {
	var params []playback.ParamData

	// One test case that have `SuspendResume: false` to test playback functionality.
	params = append(params, genPlaybackStressParam(
		"h264",
		playback.GenDataPath("h264", 720, 30),
		720,
		30,
		"smoke",
		"",
		nil,
		false,
		5*time.Minute,
	))
	// TODO: Set the duration to meet the total test duration instead of a fixed duration.
	for _, duration := range []time.Duration{5 * time.Minute} {
		// TODO: Add more codecs
		codecs := []string{"h264"}
		for _, codec := range codecs {
			for _, resolution := range []int{720, 1080} {
				fpss := []int{30}
				for _, fps := range fpss {
					param := genPlaybackStressParam(
						codec,
						playback.GenDataPath(codec, resolution, fps),
						resolution,
						fps,
						"",
						"",
						nil,
						true,
						duration,
					)
					if codec == "hevc" {
						param.HardwareDeps = "hwdep.SupportsHEVCVideoDecodingInChrome()"
					}
					params = append(params, param)
				}
			}
		}
		// lacros
		for _, resolution := range []int{720, 1080} {
			fpss := []int{30}
			for _, fps := range fpss {
				param := genPlaybackStressParam(
					"h264",
					playback.GenDataPath("h264", resolution, fps),
					resolution,
					fps,
					"lacros",
					"chromeVideoLacros",
					[]string{"lacros"},
					true,
					duration,
				)
				if resolution == 1080 && fps == 30 {
					param.MeasureSteadyStateMetrics = true
				}
				params = append(params, param)
			}
		}
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
				PerfTracing: {{ .PerfTracing | fmt }},
				MeasureSteadyStateMetrics: {{ .MeasureSteadyStateMetrics | fmt }},
				MeasureRoughness: {{ .MeasureRoughness | fmt }},
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
		{{ if .Timeout }}
		Timeout: {{ .Timeout | fmt }},
		{{ end }}
	},
	{{ end }}`, params)

	genparams.Ensure(t, "playback_stress.go", code)
}
