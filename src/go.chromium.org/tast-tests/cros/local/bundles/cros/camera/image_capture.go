// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/camera/getusermedia"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ImageCapture,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Verifies availability of ImageCapture API outside CCA",
		Contacts:     []string{"chromeos-camera-app-eng@google.com", "dorahkim@chromium.org"},
		BugComponent: "b:978428", // ChromeOS > Platform > Technologies > Camera > App & Framework
		Attr:         []string{"group:mainline", "group:camera-libcamera", "informational"},
		SoftwareDeps: []string{"chrome", caps.BuiltinCamera},
		Data:         append(getusermedia.DataFiles(), "web_api.html"),
		Fixture:      "chromeVideo",
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

	ci := s.FixtValue().(chrome.HasChrome).Chrome()

	if err := getusermedia.RunImageCaptureAPI(ctx, s.DataFileSystem(), ci, getusermedia.VerboseLogging); err != nil {
		s.Fatal("Failed to run ImageCapture API: ", err)
	}
}
