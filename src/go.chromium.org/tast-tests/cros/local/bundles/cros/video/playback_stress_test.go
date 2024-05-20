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
	"go.chromium.org/tast-tests/cros/local/graphics"
)

// To regenerate the test parameters by running the following in a chroot:
// TAST_GENERATE_UPDATE=1 ~/chromiumos/src/platform/tast/tools/go.sh test -count=1 go.chromium.org/tast-tests/cros/local/bundles/cros/video

const (
	// Targeted total playback duration for all subtests.

	// sumOfTestTimeout is the total sum of the tests timeout.
	// The actual timeout and duration is calculated in |calculateTimeoutDuration|.
	// Note, since DUT only supports 1 suspend mode by default, half the tests would skip on the DUT and total test duration will be halfed.
	sumOfTestTimeout = 4 * time.Hour
)

type playbackStressParam struct {
	codec         string
	file          string
	resolution    int
	fps           int
	nameSuffix    string
	extendDeps    []string
	suspendResume bool
	pmTestMode    graphics.PmTestMode
	suspendMode   graphics.SuspendMode
	// duration is the video playback duration.
	duration time.Duration
	// timeout is the test timeout.
	timeout time.Duration
}

func calculateTimeoutDuration(count int) (time.Duration, time.Duration) {
	convertInt := func(d time.Duration) int {
		return int(d / time.Second)
	}
	convertDuration := func(d int) time.Duration {
		return time.Duration(d) * time.Second
	}

	// Calculate the timeout/duration for each subtests.
	suspendInterval := convertInt(playback.DefaultSuspendSystemInterval)
	suspendTime := convertInt(playback.DefaultSuspendSystemTimeout)

	testTimeout := convertDuration(convertInt(sumOfTestTimeout) / count)
	// There is 20 seconds timeout reserved for resource cleanup.
	testDuration := convertDuration(int(float64(convertInt(testTimeout)-20) * float64(suspendInterval) / float64(suspendInterval+suspendTime)))
	return testTimeout, testDuration
}

// trimEmptyString removes any item in the `list` with empty String representation.
func trimEmptyString(list []interface{}) []string {
	var r []string
	for _, l := range list {
		rep := fmt.Sprintf("%v", l)
		if rep == "" {
			continue
		}
		r = append(r, rep)
	}
	return r
}

func genPlaybackStressParam(param playbackStressParam) playback.ParamData {
	nameSuffices := trimEmptyString([]interface{}{
		param.codec,
		fmt.Sprintf("%vp", param.resolution),
		fmt.Sprintf("%vfps", param.fps),
		strings.ToLower(fmt.Sprintf("%v", param.suspendMode)),
		param.pmTestMode,
		param.nameSuffix})
	testName := strings.Join(nameSuffices, "_")

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

	var hwdeps []string
	if param.suspendMode == graphics.SuspendS0ix {
		hwdeps = append(hwdeps, "hwdep.SuspendToIdle()")
	} else if param.suspendMode == graphics.SuspendS3 {
		hwdeps = append(hwdeps, "hwdep.SuspendToMem()")
	}
	if param.codec == "hevc" {
		hwdeps = append(hwdeps, "hwdep.SupportsHEVCVideoDecodingInChrome()")
	}

	susMode := ""
	if param.suspendMode == graphics.SuspendS0ix {
		susMode = "graphics.SuspendS0ix"
	} else {
		susMode = "graphics.SuspendS3"
	}
	return playback.ParamData{
		Name:          testName,
		File:          param.file,
		DecoderType:   playback.Hardware,
		BrowserType:   brwType,
		SoftwareDeps:  deps,
		HardwareDeps:  hwdeps,
		Data:          []string{param.file},
		Fixture:       fixture,
		ExtraAttr:     extraAttr,
		SuspendResume: param.suspendResume,
		PmTestMode:    param.pmTestMode,
		SuspendMode:   susMode,
		Duration:      param.duration,
		Timeout:       param.timeout,
	}
}

func TestPlaybackStressConfig(t *testing.T) {
	var params []playback.ParamData

	// One test case that has `SuspendResume: false` to test playback functionality.
	params = append(params, playback.ParamData{
		Name:          "h264_720p_30fps_smoke",
		File:          playback.GenDataPath("h264", 720, 30),
		DecoderType:   playback.Hardware,
		BrowserType:   "browser.TypeAsh",
		SoftwareDeps:  playback.GenSwDeps("h264", 720, 30, "hw"),
		Data:          []string{playback.GenDataPath("h264", 720, 30)},
		Fixture:       "chromeVideoStress",
		ExtraAttr:     []string{"graphics_nightly"},
		SuspendResume: false,
		Duration:      2 * time.Minute,
		Timeout:       6 * time.Minute,
	})

	inList := func(str string, list []string) bool {
		for _, l := range list {
			if str == l {
				return true
			}
		}
		return false
	}

	var testParams []playbackStressParam
	pmTestModes := []string{"none", "freezer", "devices", "platform", "processors", "core"}
	suspendModes := []graphics.SuspendMode{graphics.SuspendS0ix, graphics.SuspendS3}
	codecs := []string{"h264", "hevc", "vp8", "vp9", "av1"}
	resolutions := []int{720, 1080}
	for _, pmMode := range pmTestModes {
		for _, sMode := range suspendModes {
			if sMode == graphics.SuspendS0ix && inList(pmMode, []string{"processors", "core"}) {
				// Only none/freezer/devices/platform are supported in S2idle, see kernel/power/suspend.c
				continue
			}
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
							suspendMode:   sMode,
							pmTestMode:    graphics.PmTestMode(pmMode),
						}
						testParams = append(testParams, param)
					}
				}
			}
		}
	}

	testTimeout, testDuration := calculateTimeoutDuration(len(testParams))
	if testTimeout < 1*time.Minute {
		t.Fatalf("Unexpect test timeout, expect>: %v, got: %v. Adjust sumOfTestTimeout to have longer timeout.", 1*time.Minute, testTimeout)
	}
	if testDuration < 10*time.Second {
		t.Fatalf("Unexpect test duration, expect>: %v, got: %v. Adjust sumOfTestTimeout to have longer duration.", 10*time.Second, testDuration)
	}
	for _, param := range testParams {
		param.duration = testDuration
		param.timeout = testTimeout
		p := genPlaybackStressParam(param)
		params = append(params, p)
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
				PerfTracing: {{ .PerfTracing | fmt }},
				MeasureSteadyStateMetrics: {{ .MeasureSteadyStateMetrics | fmt }},
				MeasureRoughness: {{ .MeasureRoughness | fmt }},
			},
			{{ end }}
			{{ if .SuspendResume }}
			SuspendResume: {{ .SuspendResume | fmt }},
			SuspendSetting: playback.SuspendSetting {
				PmTestMode: "{{ .PmTestMode }}",
				SuspendMode: {{ .SuspendMode }},
			},
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
		{{ if .Timeout }}
		Timeout: {{ .Timeout | fmt }},
		{{ end }}
	},
	{{ end }}`, params)

	genparams.Ensure(t, "playback_stress.go", code)
}
