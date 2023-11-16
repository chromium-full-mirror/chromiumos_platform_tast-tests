// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package oobe

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/oobe/fixture"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/input"
	oobeHelper "go.chromium.org/tast-tests/cros/local/oobe"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           HidScreenUsbKeyboardOnly,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Checks that a single usb keyboard device can be connected in OOBE HID Detection screen",
		Contacts: []string{
			"cros-connectivity@google.com",
			"tjohnsonkanu@google.com",
		},
		VarDeps: []string{
			"ui.signinProfileTestExtensionManifestKey",
		},
		BugComponent: "b:1318544", // ChromeOS > Software > System Services > Connectivity > General
		Attr:         []string{"group:cr_oobe", "cr_oobe_chromebox_chromebase"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.FormFactor(hwdep.Chromebox)),
		Fixture:      "chromeEnterOobeHidDetectionServoOff",
		Timeout:      time.Second * 60,
	})
}

func HidScreenUsbKeyboardOnly(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, time.Second*10)
	defer cancel()

	cr := s.FixtValue().(*fixture.ChromeOobeHidDetection).Chrome
	oobeConn, err := cr.WaitForOOBEConnection(ctx)
	if err != nil {
		s.Fatal("Failed to create OOBE connection: ", err)
	}
	defer oobeConn.Close()

	tconn, err := cr.SigninProfileTestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create the signin profile test API connection: ", err)
	}
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	// Check that the HID detection screen is visible.
	if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.HIDDetectionScreen.isVisible()"); err != nil {
		s.Fatal("Failed to wait for the HID detection screen to be visible: ", err)
	}

	func() {
		// Create a virtual keyboard.
		keyboard, err := input.Keyboard(ctx)
		if err != nil {
			s.Fatal("Failed to create a virtual keyboard: ", err)
		}

		defer keyboard.Close(ctx)

		// Check that a keyboard is detected.
		if err := oobeHelper.IsHidDetectionSearchingForKeyboard(ctx, oobeConn, tconn); err == nil {
			s.Fatal("Expected keyboard device to be found: ", err)
		}

		if err := oobeHelper.IsHidDetectionContinueButtonEnabled(ctx, oobeConn); err != nil {
			s.Fatal("Expected continue button to be enabled: ", err)
		}
	}()

	// Check that no keyboard is detected.
	if err := oobeHelper.IsHidDetectionSearchingForKeyboard(ctx, oobeConn, tconn); err != nil {
		s.Fatal("Expected keyboard device to be disconnected: ", err)
	}

	if err := oobeHelper.IsHidDetectionContinueButtonDisabled(ctx, oobeConn); err != nil {
		s.Fatal("Expected continue button to be disabled: ", err)
	}

	// Reconnect keyboard device.
	keyboard, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to create a virtual keyboard: ", err)
	}

	defer keyboard.Close(ctx)

	// Check that a keyboard is detected.
	if err := oobeHelper.IsHidDetectionSearchingForKeyboard(ctx, oobeConn, tconn); err == nil {
		s.Fatal("Expected keyboard device to be found: ", err)
	}

	if err := oobeHelper.IsHidDetectionContinueButtonEnabled(ctx, oobeConn); err != nil {
		s.Fatal("Expected continue button to be enabled: ", err)
	}

	// Click the next button.
	if err := oobeHelper.ClickHidScreenNextButton(ctx, oobeConn, tconn); err != nil {
		s.Fatal("Failed click on next button: ", err)
	}

	// Check that the Welcome screen is visible.
	if err := oobeHelper.IsWelcomeScreenVisible(ctx, oobeConn); err != nil {
		s.Fatal("Failed to wait for the welcome screen to be visible: ", err)
	}
}
