// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"time"

	"chromiumos/tast/common/media/caps"
	"chromiumos/tast/local/bundles/cros/camera/getusermedia"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/lacros"
	"chromiumos/tast/local/media/pre"
	"chromiumos/tast/local/upstart"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ImageCapture,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Verifies availability of ImageCapture API outside CCA",
		Contacts:     []string{"chromeos-camera-eng@google.com", "dorahkim@chromium.org"},
		Attr:         []string{"group:mainline", "group:camera-libcamera"},
		SoftwareDeps: []string{"chrome"},
		Data:         append(getusermedia.DataFiles(), "web_api.html"),
		Params: []testing.Param{
			{
				Name:              "real",
				Pre:               pre.ChromeVideo(),
				ExtraAttr:         []string{"informational"},
				ExtraSoftwareDeps: []string{caps.BuiltinCamera},
				Val:               browser.TypeAsh,
			},
			{
				Name:              "lacros",
				Fixture:           "chromeVideoLacros",
				ExtraAttr:         []string{"informational"},
				ExtraSoftwareDeps: []string{caps.BuiltinCamera, "lacros"},
				Timeout:           7 * time.Minute, // A lenient limit for launching Lacros Chrome.
				Val:               browser.TypeLacros,
			},
		},
		BugComponent: "b:978428",
	})
}

// ImageCapture calls getUserMedia call and renders the camera's media stream
// in a video tag. It performs video capturing with 480p and 720p.
// And then it calls four functions of ImageCapture APIs.
// This test will fail when an error occurs among ImageCapture API functions.
func ImageCapture(ctx context.Context, s *testing.State) {
	// Ensure camera service running to avoid bad state from previous tests.
	if err := upstart.EnsureJobRunning(ctx, "cros-camera"); err != nil {
		s.Fatal("Failed to start cros-camera: ", err)
	}

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
		ci = s.PreValue().(*chrome.Chrome)
	}

	if err := getusermedia.RunImageCaptureAPI(ctx, s.DataFileSystem(), ci, getusermedia.VerboseLogging); err != nil {
		s.Fatal("Failed to run ImageCapture API: ", err)
	}
}
