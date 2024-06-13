// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package apps

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/apps/fixture"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         AppParentalControls,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test on-device parental controls for apps",
		Contacts: []string{"cros-families-eng+test@google.com",
			"chromeos-consumer-engprod@google.com"},
		// ChromeOS > Software > Family > Parental guidance
		BugComponent: "b:1090157",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      fixture.LoggedInAppParentalControls,
	})
}

func AppParentalControls(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(fixture.FixtData).Chrome
	tconn := s.FixtValue().(fixture.FixtData).TestAPIConn

	ui := uiauto.New(tconn)
	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get Keyboard: ", err)
	}
	defer kb.Close(ctx)

	const (
		confirmPinDialogText        = "Confirm your PIN"
		confirmButtonText           = "Confirm"
		continueButtonText          = "Continue"
		googleTVAppBlockedText      = "Google TV is blocked on your Chrome device"
		okButtonLabel               = "OK"
		parentalControlsButtonLabel = "Parental controls for apps"
		parentalControlsSubpage     = "Parental controls for apps subpage back button"
		pin                         = "123456"
		setupPinDialogText          = "Set up your PIN"
		verifyPinDialogText         = "Enter your PIN for parental controls"
	)

	if _, err := apps.LaunchOSSettings(ctx, cr, "chrome://os-settings/apps"); err != nil {
		s.Fatal("Failed to open apps section in OS settings: ", err)
	}

	parentalControlsSetupButton := nodewith.Name(parentalControlsButtonLabel).Role(role.Button)
	parentalControlsSetupDialogHeader := nodewith.Name(setupPinDialogText).Role(role.StaticText)

	if err := uiauto.Combine("Failed to launch setup pin dialog for parental controls for apps",
		ui.DoDefault(parentalControlsSetupButton),
		ui.WaitUntilExists(parentalControlsSetupDialogHeader),
	)(ctx); err != nil {
		s.Fatal("Failed to launch setup pin dialog for parental controls for apps: ", err)
	}

	continueButton := nodewith.Name(continueButtonText).Role(role.Button)
	confirmButton := nodewith.Name(confirmButtonText).Role(role.Button)
	confirmDialogHeader := nodewith.Name(confirmPinDialogText).Role(role.StaticText)

	if err := uiauto.Combine("Failed to setup the pin for parental controls for apps",
		kb.TypeAction(pin),
		ui.WaitUntilExists(continueButton),
		ui.LeftClick(continueButton),
		ui.WaitUntilExists(confirmDialogHeader),
		kb.TypeAction(pin),
		ui.LeftClick(confirmButton),
		ui.WaitUntilGone(confirmDialogHeader),
	)(ctx); err != nil {
		s.Fatal("Failed to setup the pin for parental controls for apps: ", err)
	}

	googleTVAppToggle := nodewith.Name("Google TV").Role(role.ToggleButton)

	if err := ui.LeftClick(googleTVAppToggle)(ctx); err != nil {
		s.Fatal("Failed to left click googleTVAppToggle: ", err)
	}

	if err := launcher.Open(tconn)(ctx); err != nil {
		s.Fatal("Failed to open the launcher: ", err)
	}

	googleTVApp := launcher.AppItemViewFinder(apps.GoogleTV.ShortName()).First()
	if err := ui.LeftClick(googleTVApp)(ctx); err != nil {
		s.Fatal("Failed to click on the google tv app: ", err)
	}

	okButton := nodewith.Name(okButtonLabel).Role(role.Button)
	googleTVAppBlockedHeading := nodewith.Name(googleTVAppBlockedText).Role(role.Heading)

	if err := uiauto.Combine("Failed to show Google TV blocked dialog",
		ui.WaitUntilExists(googleTVAppBlockedHeading),
		ui.LeftClick(okButton),
		ui.WaitUntilGone(googleTVAppBlockedHeading),
	)(ctx); err != nil {
		s.Fatal("Failed to see and close blocked app dialog: ", err)
	}

	if err := ui.LeftClick(googleTVAppToggle)(ctx); err != nil {
		s.Fatal("Failed to left click googleTVAppToggle: ", err)
	}

	parentalControlsSubpageButton := nodewith.Name(parentalControlsSubpage).Role(role.Button)
	parentalControlsToggle := nodewith.Name(parentalControlsButtonLabel).Role(role.ToggleButton)
	verifyPinDialogHeading := nodewith.Name(verifyPinDialogText).Role(role.StaticText)

	if err := uiauto.Combine("Failed to reset parental controls for apps PIN",
		ui.LeftClick(parentalControlsSubpageButton),
		ui.DoDefault(parentalControlsToggle),
		ui.WaitUntilExists(confirmButton),
		kb.TypeAction(pin),
		ui.LeftClick(confirmButton),
		ui.WaitUntilGone(verifyPinDialogHeading),
	)(ctx); err != nil {
		s.Fatal("Failed to reset pin: ", err)
	}
}
