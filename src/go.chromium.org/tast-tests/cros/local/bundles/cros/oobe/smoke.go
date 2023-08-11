// Copyright 2021 The ChromiumOS Authors
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
		Func:         Smoke,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Smoke test that clicks through OOBE",
		Contacts: []string{
			"cros-oobe@google.com",
			"bohdanty@google.com",
			"rrsilva@google.com",
			"chromeos-sw-engprod@google.com",
			"cros-oac@google.com",
			"cros-exp-wg+testresults@google.com", // for finch
		},
		BugComponent: "b:1263090", // ChromeOS > Software > OOBE
		Attr:         []string{"group:mainline", "group:hw_agnostic", "group:cq-medium"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      fixture.UpdateEngine, // Ensure to update engine status is idle and to reset between tests (b/263421799).
		Params: []testing.Param{{
			Name: "finch_on",
			Val:  chrome.FieldTrialConfigEnable,
		}, {
			Name: "finch_off",
			Val:  chrome.FieldTrialConfigDisable,
		}},
	})
}

func Smoke(ctx context.Context, s *testing.State) {
	cr, err := chrome.New(ctx,
		chrome.FieldTrialConfig(s.Param().(chrome.FieldTrialConfigMode)),
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

	if err := oobeConn.Eval(ctx, "OobeAPI.screens.UserCreationScreen.selectPersonalUser()", nil); err != nil {
		s.Fatal("Failed to select for personal user cr-button: ", err)
	}

	if err := oobeConn.Eval(ctx, "OobeAPI.screens.UserCreationScreen.clickNext()", nil); err != nil {
		s.Fatal("Failed to click user creation screen next button: ", err)
	}

	if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.GaiaInfoScreen.isVisible()"); err != nil {
		s.Fatal("Failed to wait for the gaia info screen to be visible: ", err)
	}
	if err := oobeConn.Eval(ctx, "OobeAPI.screens.GaiaInfoScreen.clickNext()", nil); err != nil {
		s.Fatal("Failed to click gaia info screen next button: ", err)
	}

	if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.GaiaScreen.isVisible()"); err != nil {
		s.Fatal("Failed to wait for the login screen to be visible: ", err)
	}
}
