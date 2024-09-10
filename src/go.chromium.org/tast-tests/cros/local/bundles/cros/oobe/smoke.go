// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package oobe

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/nebraska"
	"go.chromium.org/tast-tests/cros/local/oobe"
	"go.chromium.org/tast-tests/cros/local/updateengine"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Smoke,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Smoke test that clicks through OOBE",
		Contacts: []string{
			"cros-oobe@google.com",
			"chromeos-consumer-engprod@google.com",
			"cros-exp-wg+testresults@google.com", // for fieldtrial_testing_config
			"bohdanty@google.com",
			"rrsilva@google.com",
		},
		BugComponent: "b:1263090", // ChromeOS > Software > OOBE
		Attr:         []string{"group:mainline", "group:hw_agnostic", "group:cq-medium"},
		SoftwareDeps: []string{"chrome", "non_meet_device"},
		Params: []testing.Param{{
			Name: "fieldtrial_testing_config_on",
			Val:  chrome.FieldTrialConfigEnable,
		}, {
			Name: "fieldtrial_testing_config_off",
			Val:  chrome.FieldTrialConfigDisable,
		}},
		Fixture: fixture.UpdateEngine,
	})
}

func Smoke(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, time.Second*10)
	defer cancel()

	updateServer, err := nebraska.New(ctx, nebraska.ConfigureUpdateEngine())
	if err != nil {
		s.Fatal("Failed to start nebraska: ", err)
	}
	defer updateServer.Close(cleanupCtx)

	if err := updateServer.SetFakedMetadata(ctx); err != nil {
		s.Fatal("Failed to configure Nebraska with faked update metadata: ", err)
	}

	if err := updateServer.SetCriticalUpdate(ctx, false); err != nil {
		s.Fatal("Failed to configure Nebraska with non-critical update: ", err)
	}

	if err := updateServer.SetNoUpdateAvailable(ctx, true); err != nil {
		s.Fatal("Failed to configure Nebraska with no update: ", err)
	}

	if err := updateengine.RestartDaemon(ctx); err != nil {
		s.Fatal("Failed to restart updateengine daemon: ", err)
	}

	cr, err := chrome.New(ctx,
		chrome.FieldTrialConfig(s.Param().(chrome.FieldTrialConfigMode)),
		chrome.NoLogin())
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

	if err := oobe.ProceedThroughNetworkScreen(ctx, oobeConn); err != nil {
		s.Fatal("Failed to proceed through network screen: ", err)
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

	if err := oobe.ProceedThroughGaiaInfoScreen(ctx, oobeConn); err != nil {
		s.Fatal("Failed to proceed through Gaia info screen: ", err)
	}

	if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.GaiaScreen.isVisible()"); err != nil {
		s.Fatal("Failed to wait for the login screen to be visible: ", err)
	}
}
