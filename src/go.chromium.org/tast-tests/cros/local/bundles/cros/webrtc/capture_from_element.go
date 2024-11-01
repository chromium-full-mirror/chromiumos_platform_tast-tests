// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package webrtc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/webrtc/capturefromelement"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: CaptureFromElement,
		Desc: "Verifies that WebRTC captureStream() (canvas, video) works",
		Contacts: []string{
			"chromeos-gfx-video@google.com",
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
			},
			Fixture: "chromeVideo",
		}, {
			Name: "canvas_from_video",
			Val: capturefromelement.TestParam{
				CanvasSource: capturefromelement.UseVideo,
			},
			Fixture: "chromeVideoWithFakeWebcam",
		}},
		//TODO(b/199174572): add a test case for "video" capture.
	})
}

// CaptureFromElement verifies that the homonymous API works as expected.
func CaptureFromElement(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	testParam := s.Param().(capturefromelement.TestParam)

	const noMeasurement = 0 * time.Second
	if err := capturefromelement.RunCaptureStream(ctx, s, cr, testParam.CanvasSource, noMeasurement); err != nil {
		s.Fatal("RunCaptureStream failed: ", err)
	}
}
