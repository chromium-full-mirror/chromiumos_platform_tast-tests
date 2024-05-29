// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/camera/getusermedia"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	"go.chromium.org/tast-tests/cros/local/media/pre"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         WebCameraApp,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test for the functionality (such as taking photos, recording videos, and switching between cameras) of this camera app",
		Contacts:     []string{"chromeos-camera-eng@google.com", "chuhsuan@chromium.org"},
		Attr:         []string{"group:mainline", "group:camera-libcamera"},
		SoftwareDeps: []string{"chrome"},
		Data:         []string{"web_camera_app.html", "web_camera_app.js", "web_camera_app_test.js"},
		Params: []testing.Param{
			{
				Name:              "real",
				Fixture:           "chromeVideo",
				ExtraAttr:         []string{"informational"},
				ExtraSoftwareDeps: []string{caps.BuiltinCamera},
				Val:               browser.TypeAsh,
			},
			{
				Name:              "lacros",
				Fixture:           pre.ChromeVideoLacrosFixture(pre.VideoFeatureFakeMediaStreamUI),
				ExtraAttr:         []string{"informational"},
				ExtraSoftwareDeps: []string{caps.BuiltinCamera, "lacros"},
				Timeout:           7 * time.Minute, // A lenient limit for launching Lacros Chrome.
				Val:               browser.TypeLacros,
			},
		},
		BugComponent: "b:978428",
	})
}

// WebCameraApp calls getUserMedia call and renders the camera's media stream
// in a video tag. It performs video capturing with 120p and 480p.
// And then it calls  functions of ImageCapture APIs and MediaStream APIs and MediaDevices APIs.
// This test will fail when an error occurs.
func WebCameraApp(ctx context.Context, s *testing.State) {
	// Ensure camera service running to avoid bad state from previous tests.
	if err := upstart.EnsureJobRunning(ctx, "cros-camera"); err != nil {
		s.Fatal("Failed to start cros-camera: ", err)
	}

	duration := 3 * time.Second

	var ci getusermedia.ChromeInterface
	if s.Param().(browser.Type) == browser.TypeLacros {
		tconn, err := s.FixtValue().(chrome.HasChrome).Chrome().TestAPIConn(ctx)
		if err != nil {
			s.Fatal("Failed to connect to test API: ", err)
		}

		ci, err = lacros.Launch(ctx, tconn)
		if err != nil {
			s.Fatal("Failed to launch lacros-chrome: ", err)
		}
		defer ci.Close(ctx)
	} else {
		ci = s.FixtValue().(chrome.HasChrome).Chrome()
	}

	if err := getusermedia.RunWebCameraApp(ctx, s.DataFileSystem(), ci, duration, getusermedia.VerboseLogging); err != nil {
		s.Fatal("Failed to call WebCameraApp(): ", err)
	}
}
