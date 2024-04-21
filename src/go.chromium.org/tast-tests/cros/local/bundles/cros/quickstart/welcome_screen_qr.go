// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package quickstart contains tests for the Quick Start feature in ChromeOS.
package quickstart

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/chrome/crossdevice"
	"go.chromium.org/tast-tests/cros/local/chrome/crossdevice/quickstart"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         WelcomeScreenQR,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test Quick Start starting on the Welcome Screen with QR code verification",
		Contacts: []string{
			"chromeos-cross-device-eng@google.com",
			"hansenmichael@google.com",
			"chromeos-sw-engprod@google.com",
		},
		BugComponent: "b:1155263",
		// Attr:         []string{"group:cross-device"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "crossdeviceNoSignIn",
		VarDeps: []string{
			"ui.signinProfileTestExtensionManifestKey",
		},
	})
}

func WelcomeScreenQR(ctx context.Context, s *testing.State) {
	androidDevice := s.FixtValue().(*crossdevice.FixtData).AndroidDevice
	if androidDevice == nil {
		s.Fatal("Fixture not associated with an android device")
	}
	cr := s.FixtValue().(*crossdevice.FixtData).Chrome
	if cr == nil {
		s.Fatal("Fixture not associated with Chrome")
	}
	oobeConn, err := cr.WaitForOOBEConnection(ctx)
	if err != nil {
		s.Fatal("Failed to create OOBE connection: ", err)
	}
	defer oobeConn.Close()

	// Set up a lockscreen PIN on the phone (required for Quick Start)
	if err := androidDevice.SetPIN(ctx); err != nil {
		s.Fatal("Failed to set a lockscreen PIN on the phone: ", err)
	}
	defer androidDevice.ClearPIN(ctx)

	// Wait for the Welcome Screen to appear
	s.Log("Waiting for the welcome screen")
	if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.WelcomeScreen.isVisible()"); err != nil {
		s.Fatal("Failed to wait for the welcome screen to be visible: ", err)
	}
	s.Log("Navigating to the quickstart screen")
	tconn, err := cr.SigninProfileTestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}

	// Begin the UI flow and accept the halfsheet prompt on the phone
	ui := uiauto.New(tconn)
	setupButton := nodewith.NameContaining("Android phone").Role(role.Button)
	if err := ui.LeftClick(setupButton)(ctx); err != nil {
		s.Fatal("Failed to click the Quick Start setup button: ", err)
	}
	s.Log("Calling accept fast pair half sheet")
	if err := androidDevice.AcceptFastPairHalfsheet(ctx); err != nil {
		s.Fatal("Failed to accept fast pair half sheet: ", err)
	}

	// Clear the lockscreen challenge on the phone
	if err := androidDevice.WaitForPINChallenge(ctx); err != nil {
		s.Fatal("Failed to wait for lockscreen PIN challenge on the phone: ", err)
	}
	if err := androidDevice.EnterPIN(ctx); err != nil {
		s.Fatal("Failed to enter lockscreen PIN on the phone: ", err)
	}

	// Select "For personal use" on the Chromebook
	s.Log("Waiting for user creation screen")
	if err := quickstart.SelectForPersonalUse(ctx, oobeConn, ui); err != nil {
		s.Fatal("Failed to select 'For personal use' on user creation screen: ", err)
	}

	// Confirm the Gaia account on the phone
	s.Log("Waiting for account confirmation screen")
	if err := androidDevice.TapNext(ctx); err != nil {
		s.Fatal("Failed to confirm Google Account: ", err)
	}

	// Wait for the completion screen.
	s.Log("Waiting for Quick Start completion screen")
	completionScreenTitle := nodewith.NameContaining("Android quick setup is done").Role(role.Heading)
	if err := ui.WaitUntilExists(completionScreenTitle)(ctx); err != nil {
		s.Fatal("Failed to wait for completion screen to appear: ", err)
	}

	// The subtitle on the setup complete screen is dynamic based on what we did
	// during Quick Start, so we can verify based on this message.
	connectedToWiFi := nodewith.NameContaining("connected to Wi-Fi").Role(role.StaticText)
	if err := ui.WaitUntilExists(connectedToWiFi)(ctx); err != nil {
		s.Fatal("Failed to confirm Wi-Fi credentials transfer: ", err)
	}
}
