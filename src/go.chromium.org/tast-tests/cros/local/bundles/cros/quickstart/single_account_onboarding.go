// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package quickstart contains tests for the Quick Start feature in ChromeOS.
package quickstart

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/chrome/crossdevice"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SingleAccountOnboarding,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test Quick Start onboarding flow with one user account on the phone",
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

func SingleAccountOnboarding(ctx context.Context, s *testing.State) {
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

	// Set up a PIN on the phone (required for Quick Start)
	if err := androidDevice.SetPIN(ctx); err != nil {
		s.Fatal("Failed to set a lockscreen PIN on the phone: ", err)
	}
	defer androidDevice.ClearPIN(ctx)

	// Begin the UI flow
	s.Log("Waiting for the welcome screen")
	if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.WelcomeScreen.isVisible()"); err != nil {
		s.Fatal("Failed to wait for the welcome screen to be visible: ", err)
	}
	s.Log("Navigating to the quickstart screen")
	tconn, err := cr.SigninProfileTestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}
	ui := uiauto.New(tconn)
	setupButton := nodewith.NameContaining("Android phone").Role(role.Button)
	if err := ui.LeftClick(setupButton)(ctx); err != nil {
		s.Fatal("Failed to click the Quick Start setup button: ", err)
	}
	s.Log("Calling accept fast pair half sheet")
	if err := androidDevice.AcceptFastPairHalfsheet(ctx); err != nil {
		s.Fatal("Failed to accept fast pair half sheet: ", err)
	}

	// Wait for the PIN verification screen on the phone and enter the PIN
	if err := androidDevice.WaitForPINVerificationPrompt(ctx); err != nil {
		s.Fatal("Failed to wait for PIN verification screen on the phone: ", err)
	}

	if err := androidDevice.EnterPIN(ctx); err != nil {
		s.Fatal("Failed to enter PIN on the phone: ", err)
	}

	// Wait for and click "For personal use" button
	s.Log("Waiting for user creation screen")
	if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.UserCreationScreen.isVisible()"); err != nil {
		s.Fatal("Failed to wait for the user creation screen to be visible: ", err)
	}

	s.Log("Selecting personal Google Account")
	personalUseButton := nodewith.NameContaining("personal use").Role(role.RadioButton)
	if err := ui.LeftClick(personalUseButton)(ctx); err != nil {
		s.Fatal("Failed to click the personal Google Account radio button: ", err)
	}

	nextButton := nodewith.Name("Next").Role(role.Button)
	if err := ui.LeftClick(nextButton)(ctx); err != nil {
		s.Fatal("Failed to click Next on the user creation screen: ", err)
	}
}
