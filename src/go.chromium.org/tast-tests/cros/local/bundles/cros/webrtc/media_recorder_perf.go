// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package webrtc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/webrtc/mediarecorder"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/graphics"
	"go.chromium.org/tast-tests/cros/local/media/videotype"
	"go.chromium.org/tast/core/testing"
)

// mediaRecorderPerfTest is used to describe the config used to run each test case.
type mediaRecorderPerfTest struct {
	enableHWAccel bool                   // Instruct to use hardware or software encoding.
	profile       videotype.CodecProfile // Codec profile to used for recording.
	resolution    graphics.Size          // Capture resolution. 720p if it is not filled.
}

func init() {
	testing.AddTest(&testing.Test{
		Func: MediaRecorderPerf,
		Desc: "Captures performance data about MediaRecorder for both SW and HW",
		Contacts: []string{
			"chromeos-gfx-video@google.com",

			"bchoobineh@google.com",
		},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video
		SoftwareDeps: []string{"chrome"},
		Data:         []string{"loopback_media_recorder.html"},
		Attr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
		Timeout:      8 * time.Minute,
		Params: []testing.Param{{
			Name:              "h264_sw",
			Val:               mediaRecorderPerfTest{enableHWAccel: false, profile: videotype.H264BaselineProf},
			ExtraSoftwareDeps: []string{"proprietary_codecs"},
			Fixture:           "chromeVideoWithFakeWebcamAndSWEncoding",
		}, {
			Name:              "h264_high_1080p_sw",
			Val:               mediaRecorderPerfTest{enableHWAccel: false, profile: videotype.H264HighProf, resolution: graphics.Size{Width: 1920, Height: 1080}},
			ExtraSoftwareDeps: []string{"proprietary_codecs"},
			Fixture:           "chromeVideoWithFakeWebcamAndSWEncoding",
		}, {
			Name:    "vp8_sw",
			Val:     mediaRecorderPerfTest{enableHWAccel: false, profile: videotype.VP8Prof},
			Fixture: "chromeVideoWithFakeWebcamAndSWEncoding",
		}, {
			Name:    "vp8_1080p_sw",
			Val:     mediaRecorderPerfTest{enableHWAccel: false, profile: videotype.VP8Prof, resolution: graphics.Size{Width: 1920, Height: 1080}},
			Fixture: "chromeVideoWithFakeWebcamAndSWEncoding",
		}, {
			Name:    "vp9_sw",
			Val:     mediaRecorderPerfTest{enableHWAccel: false, profile: videotype.VP9Prof},
			Fixture: "chromeVideoWithFakeWebcamAndSWEncoding",
		}, {
			Name:    "vp9_1080p_sw",
			Val:     mediaRecorderPerfTest{enableHWAccel: false, profile: videotype.VP9Prof, resolution: graphics.Size{Width: 1920, Height: 1080}},
			Fixture: "chromeVideoWithFakeWebcamAndSWEncoding",
		}, {
			Name:    "av1_sw",
			Val:     mediaRecorderPerfTest{enableHWAccel: false, profile: videotype.AV1MainProf},
			Fixture: "chromeVideoWithFakeWebcamAndSWEncoding",
		}, {
			Name:    "av1_1080p_sw",
			Val:     mediaRecorderPerfTest{enableHWAccel: false, profile: videotype.AV1MainProf, resolution: graphics.Size{Width: 1920, Height: 1080}},
			Fixture: "chromeVideoWithFakeWebcamAndSWEncoding",
		}, {
			Name:              "h264_hw",
			Val:               mediaRecorderPerfTest{enableHWAccel: true, profile: videotype.H264BaselineProf},
			ExtraSoftwareDeps: []string{caps.HWEncodeH264, "proprietary_codecs"},
			Fixture:           "chromeVideoWithFakeWebcam",
		}, {
			Name:              "h264_1080p_hw",
			Val:               mediaRecorderPerfTest{enableHWAccel: true, profile: videotype.H264HighProf, resolution: graphics.Size{Width: 1920, Height: 1080}},
			ExtraSoftwareDeps: []string{caps.HWEncodeH264, "proprietary_codecs"},
			Fixture:           "chromeVideoWithFakeWebcam",
		}, {
			Name:              "vp8_hw",
			Val:               mediaRecorderPerfTest{enableHWAccel: true, profile: videotype.VP8Prof},
			ExtraSoftwareDeps: []string{caps.HWEncodeVP8},
			Fixture:           "chromeVideoWithFakeWebcam",
		}, {
			Name:              "vp8_1080p_hw",
			Val:               mediaRecorderPerfTest{enableHWAccel: true, profile: videotype.VP8Prof, resolution: graphics.Size{Width: 1920, Height: 1080}},
			ExtraSoftwareDeps: []string{caps.HWEncodeVP8},
			Fixture:           "chromeVideoWithFakeWebcam",
		}, {
			Name:              "vp9_hw",
			Val:               mediaRecorderPerfTest{enableHWAccel: true, profile: videotype.VP9Prof},
			ExtraSoftwareDeps: []string{caps.HWEncodeVP9},
			Fixture:           "chromeVideoWithFakeWebcam",
		}, {
			Name:              "vp9_1080p_hw",
			Val:               mediaRecorderPerfTest{enableHWAccel: true, profile: videotype.VP9Prof, resolution: graphics.Size{Width: 1920, Height: 1080}},
			ExtraSoftwareDeps: []string{caps.HWEncodeVP9},
			Fixture:           "chromeVideoWithFakeWebcam",
		}, {
			Name:              "av1_hw",
			Val:               mediaRecorderPerfTest{enableHWAccel: true, profile: videotype.AV1MainProf},
			ExtraSoftwareDeps: []string{caps.HWEncodeAV1},
			Fixture:           "chromeVideoWithFakeWebcam",
		}, {
			Name:              "av1_1080p_hw",
			Val:               mediaRecorderPerfTest{enableHWAccel: true, profile: videotype.AV1MainProf, resolution: graphics.Size{Width: 1920, Height: 1080}},
			ExtraSoftwareDeps: []string{caps.HWEncodeAV1},
			Fixture:           "chromeVideoWithFakeWebcam",
		}},
	})
}

// MediaRecorderPerf captures the perf data of MediaRecorder for HW and SW
// cases with a given codec and uploads to server.
func MediaRecorderPerf(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	testOpt := s.Param().(mediaRecorderPerfTest)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	// If resolution is not filled, then 720p is set.
	if testOpt.resolution.Width == 0 && testOpt.resolution.Height == 0 {
		testOpt.resolution = graphics.Size{Width: 1280, Height: 720}
	}

	if err := mediarecorder.MeasurePerf(ctx, cr, tconn, s.DataFileSystem(), s.OutDir(), testOpt.profile, testOpt.resolution, testOpt.enableHWAccel); err != nil {
		s.Error("Failed to measure performance: ", err)
	}
}
