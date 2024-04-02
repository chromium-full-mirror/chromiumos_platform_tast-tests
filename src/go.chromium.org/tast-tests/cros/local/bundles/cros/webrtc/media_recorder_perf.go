// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package webrtc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/webrtc/mediarecorder"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	"go.chromium.org/tast-tests/cros/local/graphics"
	"go.chromium.org/tast-tests/cros/local/media/videotype"
	"go.chromium.org/tast/core/testing"
)

// mediaRecorderPerfTest is used to describe the config used to run each test case.
type mediaRecorderPerfTest struct {
	enableHWAccel bool                   // Instruct to use hardware or software encoding.
	profile       videotype.CodecProfile // Codec profile to used for recording.
	resolution    graphics.Size          // Capture resolution. 720p if it is not filled.
	browserType   browser.Type           // Type of Chrome browser to be used: Ash Chrome or Lacros Chrome.
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         MediaRecorderPerf,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Captures performance data about MediaRecorder for both SW and HW",
		Contacts: []string{
			"bchoobineh@google.com",
			"chromeos-gfx-video@google.com",
			"mcasas@chromium.org", // Test author.
		},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video
		SoftwareDeps: []string{"chrome"},
		Data:         []string{"loopback_media_recorder.html"},
		Attr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
		Timeout:      5 * time.Minute,
		Params: []testing.Param{{
			Name:              "h264_sw",
			Val:               mediaRecorderPerfTest{enableHWAccel: false, profile: videotype.H264BaselineProf, browserType: browser.TypeAsh},
			ExtraSoftwareDeps: []string{"proprietary_codecs"},
			Fixture:           "chromeVideoWithFakeWebcamAndSWEncoding",
		}, {
			Name:              "h264_high_1080p_sw",
			Val:               mediaRecorderPerfTest{enableHWAccel: false, profile: videotype.H264HighProf, resolution: graphics.Size{Width: 1920, Height: 1080}, browserType: browser.TypeAsh},
			ExtraSoftwareDeps: []string{"proprietary_codecs"},
			Fixture:           "chromeVideoWithFakeWebcamAndSWEncoding",
		}, {
			Name:    "vp8_sw",
			Val:     mediaRecorderPerfTest{enableHWAccel: false, profile: videotype.VP8Prof, browserType: browser.TypeAsh},
			Fixture: "chromeVideoWithFakeWebcamAndSWEncoding",
		}, {
			Name:    "vp8_1080p_sw",
			Val:     mediaRecorderPerfTest{enableHWAccel: false, profile: videotype.VP8Prof, resolution: graphics.Size{Width: 1920, Height: 1080}, browserType: browser.TypeAsh},
			Fixture: "chromeVideoWithFakeWebcamAndSWEncoding",
		}, {
			Name:    "vp9_sw",
			Val:     mediaRecorderPerfTest{enableHWAccel: false, profile: videotype.VP9Prof, browserType: browser.TypeAsh},
			Fixture: "chromeVideoWithFakeWebcamAndSWEncoding",
		}, {
			Name:    "vp9_1080p_sw",
			Val:     mediaRecorderPerfTest{enableHWAccel: false, profile: videotype.VP9Prof, resolution: graphics.Size{Width: 1920, Height: 1080}, browserType: browser.TypeAsh},
			Fixture: "chromeVideoWithFakeWebcamAndSWEncoding",
		}, {
			Name:    "av1_sw",
			Val:     mediaRecorderPerfTest{enableHWAccel: false, profile: videotype.AV1MainProf, browserType: browser.TypeAsh},
			Fixture: "chromeVideoWithFakeWebcamAndSWEncoding",
		}, {
			Name:    "av1_1080p_sw",
			Val:     mediaRecorderPerfTest{enableHWAccel: false, profile: videotype.AV1MainProf, resolution: graphics.Size{Width: 1920, Height: 1080}, browserType: browser.TypeAsh},
			Fixture: "chromeVideoWithFakeWebcamAndSWEncoding",
		}, {
			Name:              "h264_hw",
			Val:               mediaRecorderPerfTest{enableHWAccel: true, profile: videotype.H264BaselineProf, browserType: browser.TypeAsh},
			ExtraSoftwareDeps: []string{caps.HWEncodeH264, "proprietary_codecs"},
			Fixture:           "chromeVideoWithFakeWebcam",
		}, {
			Name:              "h264_1080p_hw",
			Val:               mediaRecorderPerfTest{enableHWAccel: true, profile: videotype.H264HighProf, resolution: graphics.Size{Width: 1920, Height: 1080}, browserType: browser.TypeAsh},
			ExtraSoftwareDeps: []string{caps.HWEncodeH264, "proprietary_codecs"},
			Fixture:           "chromeVideoWithFakeWebcam",
		}, {
			Name:              "h264_hw_lacros",
			Val:               mediaRecorderPerfTest{enableHWAccel: true, profile: videotype.H264BaselineProf, browserType: browser.TypeLacros},
			ExtraSoftwareDeps: []string{caps.HWEncodeH264, "proprietary_codecs", "lacros"},
			Fixture:           "chromeVideoLacrosWithFakeWebcam",
		}, {
			Name:              "vp8_hw",
			Val:               mediaRecorderPerfTest{enableHWAccel: true, profile: videotype.VP8Prof, browserType: browser.TypeAsh},
			ExtraSoftwareDeps: []string{caps.HWEncodeVP8},
			Fixture:           "chromeVideoWithFakeWebcam",
		}, {
			Name:              "vp8_1080p_hw",
			Val:               mediaRecorderPerfTest{enableHWAccel: true, profile: videotype.VP8Prof, resolution: graphics.Size{Width: 1920, Height: 1080}, browserType: browser.TypeAsh},
			ExtraSoftwareDeps: []string{caps.HWEncodeVP8},
			Fixture:           "chromeVideoWithFakeWebcam",
		}, {
			Name:              "vp8_hw_lacros",
			Val:               mediaRecorderPerfTest{enableHWAccel: true, profile: videotype.VP8Prof, browserType: browser.TypeLacros},
			ExtraSoftwareDeps: []string{caps.HWEncodeVP8, "lacros"},
			Fixture:           "chromeVideoLacrosWithFakeWebcam",
		}, {
			Name:              "vp9_hw",
			Val:               mediaRecorderPerfTest{enableHWAccel: true, profile: videotype.VP9Prof, browserType: browser.TypeAsh},
			ExtraSoftwareDeps: []string{caps.HWEncodeVP9},
			Fixture:           "chromeVideoWithFakeWebcam",
		}, {
			Name:              "vp9_1080p_hw",
			Val:               mediaRecorderPerfTest{enableHWAccel: true, profile: videotype.VP9Prof, resolution: graphics.Size{Width: 1920, Height: 1080}, browserType: browser.TypeAsh},
			ExtraSoftwareDeps: []string{caps.HWEncodeVP9},
			Fixture:           "chromeVideoWithFakeWebcam",
		}, {
			Name:              "vp9_hw_lacros",
			Val:               mediaRecorderPerfTest{enableHWAccel: true, profile: videotype.VP9Prof, browserType: browser.TypeLacros},
			ExtraSoftwareDeps: []string{caps.HWEncodeVP9, "lacros"},
			Fixture:           "chromeVideoLacrosWithFakeWebcam",
		}, {
			Name:              "av1_hw",
			Val:               mediaRecorderPerfTest{enableHWAccel: true, profile: videotype.AV1MainProf, browserType: browser.TypeAsh},
			ExtraSoftwareDeps: []string{caps.HWEncodeAV1},
			Fixture:           "chromeVideoWithFakeWebcam",
		}, {
			Name:              "av1_1080p_hw",
			Val:               mediaRecorderPerfTest{enableHWAccel: true, profile: videotype.AV1MainProf, resolution: graphics.Size{Width: 1920, Height: 1080}, browserType: browser.TypeAsh},
			ExtraSoftwareDeps: []string{caps.HWEncodeAV1},
			Fixture:           "chromeVideoWithFakeWebcam",
		}, {
			Name:              "av1_hw_lacros",
			Val:               mediaRecorderPerfTest{enableHWAccel: true, profile: videotype.AV1MainProf, browserType: browser.TypeLacros},
			ExtraSoftwareDeps: []string{caps.HWEncodeAV1, "lacros"},
			Fixture:           "chromeVideoLacrosWithFakeWebcam",
		}},
	})
}

// MediaRecorderPerf captures the perf data of MediaRecorder for HW and SW
// cases with a given codec and uploads to server.
func MediaRecorderPerf(ctx context.Context, s *testing.State) {
	testOpt := s.Param().(mediaRecorderPerfTest)

	cr, l, cs, err := lacros.Setup(ctx, s.FixtValue(), testOpt.browserType)
	if err != nil {
		s.Fatal("Failed to initialize test: ", err)
	}
	defer lacros.CloseLacros(ctx, l)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	var br *browser.Browser
	switch testOpt.browserType {
	case browser.TypeAsh:
		br = cr.Browser()
	case browser.TypeLacros:
		br = l.Browser()
	}
	bTconn, err := br.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to browser test API: ", err)
	}

	// If resolution is not filled, then 720p is set.
	if testOpt.resolution.Width == 0 && testOpt.resolution.Height == 0 {
		testOpt.resolution = graphics.Size{Width: 1280, Height: 720}
	}

	if err := mediarecorder.MeasurePerf(ctx, cs, tconn, bTconn, s.DataFileSystem(), s.OutDir(), testOpt.profile, testOpt.resolution, testOpt.enableHWAccel); err != nil {
		s.Error("Failed to measure performance: ", err)
	}
}
