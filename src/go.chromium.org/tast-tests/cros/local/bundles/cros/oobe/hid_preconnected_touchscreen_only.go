// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package oobe

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/oobe/fixture"
	"go.chromium.org/tast-tests/cros/local/oobe"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         HidPreconnectedTouchscreenOnly,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that OOBE HID Detection screen is shown with the correct devices enabled on touchscreen devices",
		Contacts: []string{
			"cros-connectivity@google.com",
		},
		BugComponent: "b:1131776",
		VarDeps: []string{
			"ui.signinProfileTestExtensionManifestKey",
		},
		Attr:         []string{"group:cr_oobe", "cr_oobe_chromebox_chromebase"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.FormFactor(hwdep.Chromebase), hwdep.SkipOnModel("kalista", "karma")),
		Fixture:      "chromeEnterOobeHidDetectionServoOff",
		Timeout:      time.Second * 15,
	})
}

// HidPreconnectedTouchscreenOnly checks that touchscreen devices have the correct HIDs detected.
func HidPreconnectedTouchscreenOnly(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*fixture.ChromeOobeHidDetection).Chrome
	oobeConn, err := cr.WaitForOOBEConnection(ctx)
	if err != nil {
		s.Fatal("Failed to create OOBE connection: ", err)
	}
	defer oobeConn.Close()

	if err := oobe.IsHidDetectionScreenVisible(ctx, oobeConn); err != nil {
		s.Fatal("Failed to wait for the HID Detection screen to be visible: ", err)
	}

	if err := oobe.IsHidDetectionTouchscreenDetected(ctx, oobeConn); err != nil {
		s.Fatal("Failed to find the text indicating that a pointer is connected: ", err)
	}

	tconn, err := cr.SigninProfileTestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create the signin profile test API connection: ", err)
	}

	if err := oobe.IsHidDetectionSearchingForMouse(ctx, oobeConn, tconn); err != nil {
		s.Fatal("Mouse was detected when it should not have been: ", err)
	}

	if err := oobe.IsHidDetectionSearchingForKeyboard(ctx, oobeConn, tconn); err != nil {
		s.Fatal("Keyboard was detected when it should not have been: ", err)
	}

	if err := oobe.IsHidDetectionContinueButtonEnabled(ctx, oobeConn); err != nil {
		s.Fatal("Failed to detect an enabled continue button: ", err)
	}

	if err := oobe.ClickHidScreenNextButton(ctx, oobeConn, tconn); err != nil {
		s.Fatal("Failed to click next button: ", err)
	}

	if err := oobe.IsWelcomeScreenVisible(ctx, oobeConn); err != nil {
		s.Fatal("Failed to wait for the welcome screen to be visible: ", err)
	}
}
