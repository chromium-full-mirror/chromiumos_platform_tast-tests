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

// mediaRecorderTest is used to describe the config used to run each test case.
type mediaRecorderTest struct {
	profile videotype.CodecProfile
	// Capture resolution. 720p if it is not filled.
	resolution graphics.Size
}

func init() {
	testing.AddTest(&testing.Test{
		Func: MediaRecorder,
		Desc: "Verifies that MediaRecorder uses video encode acceleration",
		Contacts: []string{
			"bchoobineh@google.com",
			"chromeos-gfx-video@google.com",
		},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video
		SoftwareDeps: []string{"chrome"},
		Data:         []string{"loopback_media_recorder.html"},
		Attr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
		Params: []testing.Param{{
			Name:              "h264",
			Val:               mediaRecorderTest{profile: videotype.H264BaselineProf},
			ExtraSoftwareDeps: []string{caps.HWEncodeH264, "proprietary_codecs"},
			Fixture:           "chromeVideoWithFakeWebcam",
		}, {
			Name:              "h264_high",
			Val:               mediaRecorderTest{profile: videotype.H264HighProf, resolution: graphics.Size{Width: 1920, Height: 1080}},
			ExtraSoftwareDeps: []string{caps.HWEncodeH264, "proprietary_codecs"},
			Fixture:           "chromeVideoWithFakeWebcam",
		}, {
			Name:              "h264_high_1080p",
			Val:               mediaRecorderTest{profile: videotype.H264HighProf, resolution: graphics.Size{Width: 1920, Height: 1080}},
			ExtraSoftwareDeps: []string{caps.HWEncodeH264, "proprietary_codecs"},
			Fixture:           "chromeVideoWithFakeWebcam",
		}, {
			Name:              "vp8",
			Val:               mediaRecorderTest{profile: videotype.VP8Prof},
			ExtraSoftwareDeps: []string{caps.HWEncodeVP8},
			Fixture:           "chromeVideoWithFakeWebcam",
		}, {
			Name:              "vp8_1080p",
			Val:               mediaRecorderTest{profile: videotype.VP8Prof, resolution: graphics.Size{Width: 1920, Height: 1080}},
			ExtraSoftwareDeps: []string{caps.HWEncodeVP8},
			Fixture:           "chromeVideoWithFakeWebcam",
		}, {
			Name:              "vp8_cam",
			Val:               mediaRecorderTest{profile: videotype.VP8Prof},
			ExtraSoftwareDeps: []string{caps.BuiltinCamera, caps.HWEncodeVP8},
			Fixture:           "chromeCameraPerf",
		}, {
			Name:              "vp9",
			Val:               mediaRecorderTest{profile: videotype.VP9Prof},
			ExtraSoftwareDeps: []string{caps.HWEncodeVP9},
			Fixture:           "chromeVideoWithFakeWebcam",
		}, {
			Name:              "vp9_1080p",
			Val:               mediaRecorderTest{profile: videotype.VP9Prof, resolution: graphics.Size{Width: 1920, Height: 1080}},
			ExtraSoftwareDeps: []string{caps.HWEncodeVP9},
			Fixture:           "chromeVideoWithFakeWebcam",
		}, {
			Name:              "av1",
			Val:               mediaRecorderTest{profile: videotype.AV1MainProf},
			ExtraSoftwareDeps: []string{caps.HWEncodeAV1},
			Fixture:           "chromeVideoWithFakeWebcam",
		}, {
			Name:              "av1_1080p",
			Val:               mediaRecorderTest{profile: videotype.AV1MainProf, resolution: graphics.Size{Width: 1920, Height: 1080}},
			ExtraSoftwareDeps: []string{caps.HWEncodeAV1},
			Fixture:           "chromeVideoWithFakeWebcam",
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

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	params := s.Param().(mediaRecorderTest)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	// If resolution is not filled, then 720p is set.
	if params.resolution.Width == 0 && params.resolution.Height == 0 {
		params.resolution = graphics.Size{Width: 1280, Height: 720}
	}

	if err := mediarecorder.VerifyMediaRecorderUsesEncodeAccelerator(ctx, cr, tconn, s.DataFileSystem(), params.profile, params.resolution, recordDuration); err != nil {
		s.Error("Failed to run VerifyMediaRecorderUsesEncodeAccelerator: ", err)
	}
}
