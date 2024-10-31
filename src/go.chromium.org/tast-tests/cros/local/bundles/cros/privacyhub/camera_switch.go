// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package privacyhub contains tests for privacy hub.
package privacyhub

import (
	"context"
	"image"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/privacyhub/privacyhubutil"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

// Models, where the camera test should not be flaky.
var goldenCameraModels = []string{
	"eldrid",
	"chronicler",
	"volta",
	"jinlon",
	"dragonair",
	"dratini",
	"gimble",
	"redrix",
	"atlas",
	"eve",
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         CameraSwitch,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that PrivacyHub camera toggle switches off the camera. Should be used only on VMs where the vivid daemon ensures that there is some colorful pattern present in the camera feed. Also this test assumes default vm resolution when cropping the image, and hence different resolution might lead to spurious results",
		Contacts:     []string{"chromeos-privacyhub@google.com", "janlanik@google.com", "kisliaks@google.com"},
		// ChromeOS > Privacy > ChromeOS Privacy Feature Development.
		BugComponent: "b:1178745",
		SoftwareDeps: []string{"chrome"},
		Timeout:      5 * time.Minute,
		Attr: []string{
			"group:mainline",
			"informational",
			"group:release-health",
			"release-health_enterprise",
		},
		HardwareDeps: hwdep.D(hwdep.Model(goldenCameraModels...)),
	})
}

func CameraSwitch(ctx context.Context, s *testing.State) {
	// Shorten deadline to leave time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr, err := chrome.New(ctx, chrome.EnableFeatures("CrosPrivacyHub"))
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}

	settings, err := ossettings.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch OS settings: ", err)
	}
	defer settings.Close(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	s.Log("Enabling the camera in Privacy Hub")
	if err := privacyhubutil.SetToggleState(ctx, tconn, "Camera", true); err != nil {
		s.Fatal("Failed to switch the camera toggle in the Privacy Hub: ", err)
	}
	if enabled, err := privacyhubutil.IsToggleOn(ctx, tconn, "Camera"); err != nil {
		s.Fatal("Failed to read the camera toggle state: ", err)
	} else if !enabled {
		s.Fatal("Failed to enable the camera")
	}
	var sshot image.Image
	sshot, err = privacyhubutil.LaunchCameraAndTakeScreenshot(ctx, cr, tconn, s)
	if err != nil {
		s.Fatal("Couldn't get camera screenshot: ", err)
	}
	if isBlack, err := privacyhubutil.IsImageBlack(sshot); err != nil {
		s.Fatal("Failed to verify a camera screenshot: ", err)
	} else if isBlack {
		privacyhubutil.SaveImage(s.OutDir(), "should_be_colourfull.png", sshot)
		s.Fatal("Camera feed is black even though the camera should be enabled")
	}

	s.Log("Disabling the camera in Privacy Hub")
	if err := privacyhubutil.ClickToggle(ctx, tconn, "Camera"); err != nil {
		s.Fatal("Failed to switch the camera toggle in the Privacy Hub: ", err)
	}
	if enabled, err := privacyhubutil.IsToggleOn(ctx, tconn, "Camera"); err != nil {
		s.Fatal("Failed to read the camera toggle state: ", err)
	} else if enabled {
		s.Fatal("Failed to disable the camera")
	}
	sshot, err = privacyhubutil.LaunchCameraAndTakeScreenshot(ctx, cr, tconn, s)
	if err != nil {
		s.Fatal("Couldn't get camera screenshot: ", err)
	}
	if isBlack, err := privacyhubutil.IsImageBlack(sshot); err != nil {
		s.Fatal("Failed to verify a camera screenshot: ", err)
	} else if !isBlack {
		privacyhubutil.SaveImage(s.OutDir(), "should_be_black.png", sshot)
		s.Fatal("Camera feed is not black even though the camera should be disabled")
	}
}
