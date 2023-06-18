// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package intel

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/camera/cca"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CCAUIPreviewLongTakePhoto,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Preview Camera for 1 hour and take photo",
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		SoftwareDeps: []string{"camera_app", "chrome"},
		Fixture:      "ccaLaunched",
		Timeout:      62 * time.Minute, // Timeout for long duration.
		Attr:         []string{"group:camera_dependent", "group:intel-stress"},
		BugComponent: "b:157291", // ChromeOS > External > Intel
	})
}

// CCAUIPreviewLongTakePhoto previews camera for long duration and takes photo.
func CCAUIPreviewLongTakePhoto(ctx context.Context, s *testing.State) {
	app := s.FixtValue().(cca.FixtureData).App()

	if err := app.MaximizeWindow(ctx); err != nil {
		s.Fatal("Failed to maximize window: ", err)
	}

	// GoBigSleepLint: Sleeping for 1 hour.
	if err := testing.Sleep(ctx, 60*time.Minute); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}

	// Checking if the camera is still active after the sleep.
	if err := app.WaitForState(ctx, "view-camera", true); err != nil {
		s.Fatal("Failed to wait for view-camera becomes true: ", err)
	}

	// Take photo using camera.
	if _, err := app.TakeSinglePhoto(ctx, cca.TimerOff); err != nil {
		s.Fatal("Failed to take photo: ", err)
	}
}
