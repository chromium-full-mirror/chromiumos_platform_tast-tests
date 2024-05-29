// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package webrtc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/webrtc/capturefromelement"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	"go.chromium.org/tast-tests/cros/local/media/pre"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CaptureFromElementPerf,
		LacrosStatus: testing.LacrosVariantUnknown,
		Desc:         "Collects performance values for WebRTC captureStream() (canvas, video)",
		Contacts: []string{
			"chromeos-gfx-video@google.com",
			"mcasas@chromium.org", // Test author.
			"hiroh@chromium.org",
		},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video
		SoftwareDeps: []string{"chrome"},
		Data:         capturefromelement.DataFiles(),
		Attr:         []string{"group:graphics", "graphics_video", "graphics_nightly"},
		Params: []testing.Param{{
			Name: "canvas",
			Val: capturefromelement.TestParam{
				CanvasSource: capturefromelement.UseGlClearColor,
				BrowserType:  browser.TypeAsh,
			},
			Fixture: "chromeVideo",
		}, {
			Name: "canvas_from_video",
			Val: capturefromelement.TestParam{
				CanvasSource: capturefromelement.UseVideo,
				BrowserType:  browser.TypeAsh,
			},
			Fixture: "chromeVideoWithFakeWebcam",
		}, {
			Name: "canvas_lacros",
			Val: capturefromelement.TestParam{
				CanvasSource: capturefromelement.UseGlClearColor,
				BrowserType:  browser.TypeLacros,
			},
			Fixture: pre.ChromeVideoLacrosFixture(pre.VideoFeatureFakeMediaStreamUI),
		}, {
			Name: "canvas_from_video_lacros",
			Val: capturefromelement.TestParam{
				CanvasSource: capturefromelement.UseVideo,
				BrowserType:  browser.TypeLacros,
			},
			Fixture: "chromeVideoLacrosWithFakeWebcam",
		}},
		//TODO(b/199174572): add a test case for "video" capture.
	})
}

// CaptureFromElementPerf collects perf metrics for the homonymous API.
func CaptureFromElementPerf(ctx context.Context, s *testing.State) {
	testParams := s.Param().(capturefromelement.TestParam)
	_, l, cs, err := lacros.Setup(ctx, s.FixtValue(), testParams.BrowserType)
	if err != nil {
		s.Fatal("Failed to initialize test: ", err)
	}
	defer lacros.CloseLacros(ctx, l)

	const measurementDuration = 25 * time.Second
	if err := capturefromelement.RunCaptureStream(ctx, s, cs, testParams.CanvasSource, measurementDuration); err != nil {
		s.Fatal("RunCaptureStream failed: ", err)
	}
}
