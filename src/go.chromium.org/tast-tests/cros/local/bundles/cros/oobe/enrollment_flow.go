// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package oobe

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         EnrollmentFlow,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test that clicks through OOBE to enrollment screen",
		Contacts: []string{
			"cros-oobe@google.com",
			"chromeos-sw-engprod@google.com",
			"cros-oac@google.com",
			"bchikhaoui@google.com",
			"bohdanty@google.com",
		},
		BugComponent: "b:1263090", // ChromeOS > Software > OOBE
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      fixture.UpdateEngine, // Ensure to update engine status is idle and to reset between tests (b/263421799).
	})
}

func EnrollmentFlow(ctx context.Context, s *testing.State) {
	cr, err := chrome.New(ctx,
		chrome.NoLogin(),
		chrome.ExtraArgs("--enable-features=OobeGaiaInfoScreen,OobeSoftwareUpdate"))
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(ctx)

	oobeConn, err := cr.WaitForOOBEConnection(ctx)
	if err != nil {
		s.Fatal("Failed to create OOBE connection: ", err)
	}
	defer oobeConn.Close()

	// TODO(b/294031030) nuke the repetitive code.
	if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.WelcomeScreen.isVisible()"); err != nil {
		s.Fatal("Failed to wait for the welcome screen to be visible: ", err)
	}
	if err := oobeConn.Eval(ctx, "OobeAPI.screens.WelcomeScreen.clickNext()", nil); err != nil {
		s.Fatal("Failed to click welcome page next button: ", err)
	}

	shouldSkipNetworkScreen := false
	if err := oobeConn.Eval(ctx, "OobeAPI.screens.NetworkScreen.shouldSkip()", &shouldSkipNetworkScreen); err != nil {
		s.Fatal("Failed to evaluate whether to skip Network screen: ", err)
	}

	if !shouldSkipNetworkScreen {
		if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.NetworkScreen.isVisible()"); err != nil {
			s.Fatal("Failed to wait for the network screen to be visible: ", err)
		}
		if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.NetworkScreen.nextButton.isEnabled()"); err != nil {
			s.Fatal("Failed to wait for the network screen next button to be enabled: ", err)
		}
		if err := oobeConn.Eval(ctx, "OobeAPI.screens.NetworkScreen.clickNext()", nil); err != nil {
			s.Fatal("Failed to click network page next button: ", err)
		}
	}

	if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.UserCreationScreen.isVisible()"); err != nil {
		s.Fatal("Failed to wait for the user creation screen to be visible: ", err)
	}

	if err := oobeConn.Eval(ctx, "OobeAPI.screens.UserCreationScreen.selectForWork()", nil); err != nil {
		s.Fatal("Failed to select for work cr-button: ", err)
	}

	if err := oobeConn.Eval(ctx, "OobeAPI.screens.UserCreationScreen.clickNext()", nil); err != nil {
		s.Fatal("Failed to click user creation screen next button: ", err)
	}

	if err := oobeConn.Eval(ctx, "OobeAPI.screens.UserCreationScreen.selectEnrollTriageButton()", nil); err != nil {
		s.Fatal("Failed to select enroll device cr-button: ", err)
	}

	if err := oobeConn.Eval(ctx, "OobeAPI.screens.UserCreationScreen.clickEnrollNextButton()", nil); err != nil {
		s.Fatal("Failed to click user creation screen enroll tiage step next button: ", err)
	}

	if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.EnrollmentScreen.isVisible()"); err != nil {
		s.Fatal("Failed to wait for the enrollment screen to be visible: ", err)
	}
}
