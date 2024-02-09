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

const (
	// Targeted total playback duration for all subtests.

	// sumOfTestDuration is the total sum of the tests timeout.
	// The actual video playback time should be *(playback.SuspendSystemInterval/(playback.SuspendSystemTimeout+playback.SuspendSystemInterval))
	sumOfTestDuration = 120 * time.Minute
)

type playbackStressParam struct {
	codec         string
	file          string
	resolution    int
	fps           int
	nameSuffix    string
	extendDeps    []string
	suspendResume bool
	// duration is the video playback duration.
	duration time.Duration
	// timeout is the test timeout.
	timeout time.Duration
}

func genPlaybackStressParam(param playbackStressParam) playback.ParamData {
	testName := fmt.Sprintf("%s_%dp_%dfps", param.codec, param.resolution, param.fps)
	if param.nameSuffix != "" {
		testName += "_" + param.nameSuffix
	}

	fixture := "chromeVideoStress"
	if strings.Contains(param.nameSuffix, "lacros") {
		fixture = "chromeVideoStressLacros"
	}

	brwType := "browser.TypeAsh"
	if strings.Contains(param.nameSuffix, "lacros") {
		brwType = "browser.TypeLacros"
	}
	deps := append(playback.GenSwDeps(param.codec, param.resolution, param.fps, "hw"), param.extendDeps...)

	var extraAttr []string
	if param.duration > 30*time.Minute {
		extraAttr = append(extraAttr, []string{"graphics_av_analysis", "graphics_weekly"}...)
	} else {
		extraAttr = append(extraAttr, []string{"graphics_nightly"}...)
	}
	return playback.ParamData{
		Name:          testName,
		File:          param.file,
		DecoderType:   playback.Hardware,
		BrowserType:   brwType,
		SoftwareDeps:  deps,
		Data:          []string{param.file},
		Fixture:       fixture,
		ExtraAttr:     extraAttr,
		SuspendResume: param.suspendResume,
		Duration:      param.duration,
		Timeout:       param.timeout,
	}
}

func TestPlaybackStressConfig(t *testing.T) {
	var params []playback.ParamData

	// One test case that have `SuspendResume: false` to test playback functionality.
	params = append(params, genPlaybackStressParam(playbackStressParam{
		codec:      "h264",
		file:       playback.GenDataPath("h264", 720, 30),
		resolution: 720,
		fps:        30,
		nameSuffix: "smoke",
		duration:   2 * time.Minute,
		timeout:    6 * time.Minute,
	}))

	var testParams []playbackStressParam
	codecs := []string{"h264", "hevc", "vp8", "vp9", "av1"}
	resolutions := []int{720, 1080}
	for _, codec := range codecs {
		for _, resolution := range resolutions {
			fpss := []int{30}
			for _, fps := range fpss {
				param := playbackStressParam{
					codec:         codec,
					file:          playback.GenDataPath(codec, resolution, fps),
					resolution:    resolution,
					fps:           fps,
					suspendResume: true,
				}
				testParams = append(testParams, param)
			}
		}
	}
	// lacros
	for _, resolution := range resolutions {
		fpss := []int{30}
		for _, fps := range fpss {
			param := playbackStressParam{
				codec:         "h264",
				file:          playback.GenDataPath("h264", resolution, fps),
				resolution:    resolution,
				fps:           fps,
				nameSuffix:    "lacros",
				extendDeps:    []string{"lacros"},
				suspendResume: true,
			}
			testParams = append(testParams, param)
		}
	}

	convertInt := func(d time.Duration) int {
		return int(d / time.Second)
	}
	convertDuration := func(d int) time.Duration {
		return time.Duration(d) * time.Second
	}

	// Calculate the timeout/duration for each subtests.
	suspendInterval := convertInt(playback.SuspendSystemInterval)
	suspendTime := convertInt(playback.SuspendSystemTimeout)

	testTimeout := convertDuration(convertInt(sumOfTestDuration) / len(testParams))
	testDuration := convertDuration(convertInt(testTimeout) * 1.0 * suspendInterval / (suspendInterval + suspendTime))
	if testTimeout < 1*time.Minute {
		t.Fatalf("Unexpect test timeout, expect>: %v, got: %v. Adjust sumOfTestDuration to have longer timeout.", 1*time.Minute, testTimeout)
	}
	if testDuration < 10*time.Second {
		t.Fatalf("Unexpect test duration, expect>: %v, got: %v. Adjust sumOfTestDuration to have longer duration.", 10*time.Second, testDuration)
	}
	for _, param := range testParams {
		param.duration = testDuration
		param.timeout = testTimeout
		p := genPlaybackStressParam(param)
		params = append(params, p)
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
