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

// mediaRecorderTest is used to describe the config used to run each test case.
type mediaRecorderTest struct {
	profile     videotype.CodecProfile
	browserType browser.Type
	// Capture resolution. 720p if it is not filled.
	resolution graphics.Size
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         MediaRecorder,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Verifies that MediaRecorder uses video encode acceleration",
		Contacts: []string{
			"bchoobineh@google.com",
			"chromeos-gfx-video@google.com",
			"mcasas@chromium.org", // Test author.
		},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video
		SoftwareDeps: []string{"chrome"},
		Data:         []string{"loopback_media_recorder.html"},
		Attr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
		Params: []testing.Param{{
			Name:              "h264",
			Val:               mediaRecorderTest{profile: videotype.H264BaselineProf, browserType: browser.TypeAsh},
			ExtraSoftwareDeps: []string{caps.HWEncodeH264, "proprietary_codecs"},
			Fixture:           "chromeVideoWithFakeWebcam",
		}, {
			Name:              "h264_high",
			Val:               mediaRecorderTest{profile: videotype.H264HighProf, browserType: browser.TypeAsh, resolution: graphics.Size{Width: 1920, Height: 1080}},
			ExtraSoftwareDeps: []string{caps.HWEncodeH264, "proprietary_codecs"},
			Fixture:           "chromeVideoWithFakeWebcam",
		}, {
			Name:              "h264_high_1080p",
			Val:               mediaRecorderTest{profile: videotype.H264HighProf, browserType: browser.TypeAsh, resolution: graphics.Size{Width: 1920, Height: 1080}},
			ExtraSoftwareDeps: []string{caps.HWEncodeH264, "proprietary_codecs"},
			Fixture:           "chromeVideoWithFakeWebcam",
		}, {
			Name:              "h264_lacros",
			Val:               mediaRecorderTest{profile: videotype.H264BaselineProf, browserType: browser.TypeLacros},
			ExtraSoftwareDeps: []string{caps.HWEncodeH264, "proprietary_codecs", "lacros"},
			Fixture:           "chromeVideoLacrosWithFakeWebcam",
		}, {
			// TODO(b/236546408): Remove once hardware variable bitrate encoding is enabled by default.
			Name:              "h264_vbr",
			Val:               mediaRecorderTest{profile: videotype.H264BaselineProf, browserType: browser.TypeAsh},
			ExtraSoftwareDeps: []string{caps.HWEncodeH264VBR, "proprietary_codecs"},
			Fixture:           "chromeVideoWithFakeWebcamAndHWVBREncoding",
		}, {
			Name:              "vp8",
			Val:               mediaRecorderTest{profile: videotype.VP8Prof, browserType: browser.TypeAsh},
			ExtraSoftwareDeps: []string{caps.HWEncodeVP8},
			Fixture:           "chromeVideoWithFakeWebcam",
		}, {
			Name:              "vp8_1080p",
			Val:               mediaRecorderTest{profile: videotype.VP8Prof, browserType: browser.TypeAsh, resolution: graphics.Size{Width: 1920, Height: 1080}},
			ExtraSoftwareDeps: []string{caps.HWEncodeVP8},
			Fixture:           "chromeVideoWithFakeWebcam",
		}, {
			Name:              "vp8_lacros",
			Val:               mediaRecorderTest{profile: videotype.VP8Prof, browserType: browser.TypeLacros},
			ExtraSoftwareDeps: []string{caps.HWEncodeVP8, "lacros"},
			Fixture:           "chromeVideoLacrosWithFakeWebcam",
		}, {
			Name:              "vp8_cam",
			Val:               mediaRecorderTest{profile: videotype.VP8Prof, browserType: browser.TypeAsh},
			ExtraSoftwareDeps: []string{caps.BuiltinCamera, caps.HWEncodeVP8},
			Fixture:           "chromeCameraPerf",
		}, {
			Name:              "vp9",
			Val:               mediaRecorderTest{profile: videotype.VP9Prof, browserType: browser.TypeAsh},
			ExtraSoftwareDeps: []string{caps.HWEncodeVP9},
			Fixture:           "chromeVideoWithFakeWebcam",
		}, {
			Name:              "vp9_1080p",
			Val:               mediaRecorderTest{profile: videotype.VP9Prof, browserType: browser.TypeAsh, resolution: graphics.Size{Width: 1920, Height: 1080}},
			ExtraSoftwareDeps: []string{caps.HWEncodeVP9},
			Fixture:           "chromeVideoWithFakeWebcam",
		}, {
			Name:              "vp9_lacros",
			Val:               mediaRecorderTest{profile: videotype.VP9Prof, browserType: browser.TypeLacros},
			ExtraSoftwareDeps: []string{caps.HWEncodeVP9, "lacros"},
			Fixture:           "chromeVideoLacrosWithFakeWebcam",
		}, {
			Name:              "av1",
			Val:               mediaRecorderTest{profile: videotype.AV1MainProf, browserType: browser.TypeAsh},
			ExtraSoftwareDeps: []string{caps.HWEncodeAV1},
			Fixture:           "chromeVideoWithFakeWebcam",
		}, {
			Name:              "av1_1080p",
			Val:               mediaRecorderTest{profile: videotype.AV1MainProf, browserType: browser.TypeAsh, resolution: graphics.Size{Width: 1920, Height: 1080}},
			ExtraSoftwareDeps: []string{caps.HWEncodeAV1},
			Fixture:           "chromeVideoWithFakeWebcam",
		}, {
			Name:              "av1_lacros",
			Val:               mediaRecorderTest{profile: videotype.AV1MainProf, browserType: browser.TypeLacros},
			ExtraSoftwareDeps: []string{caps.HWEncodeAV1, "lacros"},
			Fixture:           "chromeVideoLacrosWithFakeWebcam",
		}},
	})
}

// MediaRecorder verifies that a video encode accelerator was used.
func MediaRecorder(ctx context.Context, s *testing.State) {
	const (
		// Let the MediaRecorder accumulate a few milliseconds, otherwise we might
		// receive just bits and pieces of the container header.
		recordDuration = 100 * time.Millisecond
	)
	params := s.Param().(mediaRecorderTest)

	cr, l, cs, err := lacros.Setup(ctx, s.FixtValue(), params.browserType)
	if err != nil {
		s.Fatal("Failed to initialize test: ", err)
	}
	defer lacros.CloseLacros(ctx, l)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	var br *browser.Browser
	switch params.browserType {
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
	if params.resolution.Width == 0 && params.resolution.Height == 0 {
		params.resolution = graphics.Size{Width: 1280, Height: 720}
	}

	if err := mediarecorder.VerifyMediaRecorderUsesEncodeAccelerator(ctx, cs, tconn, bTconn, s.DataFileSystem(), params.profile, params.resolution, recordDuration); err != nil {
		s.Error("Failed to run VerifyMediaRecorderUsesEncodeAccelerator: ", err)
	}
}
