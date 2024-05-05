// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package quickstart contains tests for the Quick Start feature in ChromeOS.
package quickstart

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/crossdevice"
	"go.chromium.org/tast-tests/cros/local/chrome/crossdevice/quickstart"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ForcedUpdate,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test Quick Start starting on the Welcome Screen with PIN verification",
		Contacts: []string{
			"chromeos-cross-device-eng@google.com",
			"hansenmichael@google.com",
			"bhartmire@google.com",
		},
		BugComponent: "b:1155263",
		Attr:         []string{"group:cross-device"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "crossdeviceNoSignIn",
		VarDeps: []string{
			"ui.signinProfileTestExtensionManifestKey",
		},
	})
}

func ForcedUpdate(ctx context.Context, s *testing.State) {
	androidDevice := s.FixtValue().(*crossdevice.FixtData).AndroidDevice
	if androidDevice == nil {
		s.Fatal("Fixture not associated with an android device")
	}

	// Set up a lockscreen PIN on the phone (required for Quick Start)
	if err := androidDevice.SetPIN(ctx); err != nil {
		s.Fatal("Failed to set a lockscreen PIN on the phone: ", err)
	}
	defer androidDevice.ClearPIN(ctx)

	// Wrap this portion in an anonymous function to limit scope of our
	// connections, allowing for cleanup before we relaunch Chrome and
	// re-establish connections below.
	func() {
		// Start Chrome with the forced update test flag
		testing.ContextLog(ctx, "Starting Chrome")
		opts := []chrome.Option{
			chrome.EnableFeatures("OobeQuickStart"),
			chrome.DontSkipOOBEAfterLogin(),
			chrome.NoLogin(),
			chrome.DeferLogin(),
			chrome.ExtraArgs("--quick-start-test-forced-update"),
			chrome.LoadSigninProfileExtension(s.RequiredVar("ui.signinProfileTestExtensionManifestKey")),
		}
		cr, err := chrome.New(ctx, opts...)
		if err != nil {
			s.Fatal("Unable to launch Chrome: ", err)
		}
		tconn, err := cr.SigninProfileTestAPIConn(ctx)
		if err != nil {
			s.Fatal("Failed to create test API connection: ", err)
		}
		ui := uiauto.New(tconn)
		oobeConn, err := cr.WaitForOOBEConnection(ctx)
		if err != nil {
			s.Fatal("Failed to create OOBE connection: ", err)
		}
		defer oobeConn.Close()

		// Wait for the Welcome Screen to appear
		s.Log("Waiting for the welcome screen")
		if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.WelcomeScreen.isVisible()"); err != nil {
			s.Fatal("Failed to wait for the welcome screen to be visible: ", err)
		}

		// Enable ChromeVox via keyboard shortcut
		kw, err := input.Keyboard(ctx)
		if err != nil {
			s.Fatal("Failed to get keyboard handle: ", err)
		}
		defer kw.Close(ctx)
		shortcut := "Ctrl+Alt+Z"
		if err := kw.Accel(ctx, shortcut); err != nil {
			s.Fatal("Failed to enable ChromeVox: ", err)
		}

		// Begin the UI flow and accept the halfsheet prompt on the phone
		s.Log("Navigating to the Quick Start screen")
		setupButton := nodewith.NameContaining("Android phone").Role(role.Button)
		if err := ui.LeftClick(setupButton)(ctx); err != nil {
			s.Fatal("Failed to click the Quick Start setup button: ", err)
		}
		s.Log("Calling accept fast pair half sheet")
		if err := androidDevice.AcceptFastPairHalfsheet(ctx); err != nil {
			s.Fatal("Failed to accept fast pair half sheet: ", err)
		}

		// Get the verification PIN from the phone
		s.Log("Waiting for PIN verification screen")
		pinVerificationScreenTitle := nodewith.NameContaining("Verify the code on your Android phone").Role(role.Heading)
		if err := ui.WithTimeout(60 * time.Second).WaitUntilExists(pinVerificationScreenTitle)(ctx); err != nil {
			s.Fatal("Failed to wait for PIN verification screen to appear: ", err)
		}
		if err := quickstart.VerifyPIN(ctx, ui, androidDevice); err != nil {
			s.Fatal("Failed to verify matching PIN: ", err)
		}
		if err := androidDevice.TapNext(ctx); err != nil {
			s.Fatal("Failed to confirm verification PIN: ", err)
		}

		// Clear the lockscreen challenge on the phone
		if err := androidDevice.WaitForPINChallenge(ctx); err != nil {
			s.Fatal("Failed to wait for lockscreen PIN challenge on the phone: ", err)
		}
		if err := androidDevice.EnterPIN(ctx); err != nil {
			s.Fatal("Failed to enter lockscreen PIN on the phone: ", err)
		}

		// Wait for phone to acknowledge the pending update
		s.Log("Waiting for the update screen to appear on the phone")
		if err := androidDevice.WaitForUpdateScreen(ctx); err != nil {
			s.Fatal("Failed to wait for the update screen to appear on the phone: ", err)
		}
	}()

	// Re-launch Chrome, preserving state
	testing.ContextLog(ctx, "Starting Chrome")
	opts := []chrome.Option{
		chrome.EnableFeatures("OobeQuickStart"),
		chrome.KeepState(),
		chrome.DontSkipOOBEAfterLogin(),
		chrome.NoLogin(),
		chrome.DeferLogin(),
		chrome.LoadSigninProfileExtension(s.RequiredVar("ui.signinProfileTestExtensionManifestKey")),
	}
	cr, err := chrome.New(ctx, opts...)
	if err != nil {
		s.Fatal("Unable to launch Chrome: ", err)
	}
	tconn, err := cr.SigninProfileTestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}
	ui := uiauto.New(tconn)
	oobeConn, err := cr.WaitForOOBEConnection(ctx)
	if err != nil {
		s.Fatal("Failed to create OOBE connection: ", err)
	}
	defer oobeConn.Close()

	// Select "For personal use" on the Chromebook
	s.Log("Waiting for user creation screen")
	if err := quickstart.SelectForPersonalUse(ctx, oobeConn, ui); err != nil {
		s.Fatal("Failed to select 'For personal use' on user creation screen: ", err)
	}

	// Wait for connection
	testing.ContextLog(ctx, "Waiting for devices to reconnect")
	accountConfirmationTitle := nodewith.NameContaining("Confirm your Google Account").Role(role.Heading)
	if err := ui.WithTimeout(60 * time.Second).WaitUntilExists(accountConfirmationTitle)(ctx); err != nil {
		s.Fatal("Failed to wait for devices to reconnect: ", err)
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
}
