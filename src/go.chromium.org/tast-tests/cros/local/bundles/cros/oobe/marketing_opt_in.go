// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package oobe

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         MarketingOptIn,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test marketing opt-in screen in tablet and laptop modes",
		Contacts: []string{
			"cros-oobe@google.com",
			"bohdanty@google.com",
			"chromeos-sw-engprod@google.com",
		},
		BugComponent: "b:1263090", // ChromeOS > Software > OOBE
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{
			{
				Name:    "laptop",
				Val:     false,
				Fixture: "chromeLoggedInWithOobe",
			},
			{
				Name:    "tablet",
				Val:     true,
				Fixture: "chromeLoggedInWithOobeAndAccessibilityButtonEnabled",
			},
		},
		Timeout: 3 * time.Minute,
	})
}

func MarketingOptIn(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 20*time.Second)
	defer cancel()

	tabletMode := s.Param().(bool)

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, tabletMode)
	if err != nil {
		s.Fatal("Failed to ensure in tablet mode: ", err)
	}
	defer cleanup(cleanupCtx)

	oobeConn, err := cr.WaitForOOBEConnection(ctx)
	if err != nil {
		s.Fatal("Failed to create OOBE connection: ", err)
	}
	defer oobeConn.Close()

	if err := oobeConn.Eval(ctx, "OobeAPI.advanceToScreen('marketing-opt-in')", nil); err != nil {
		s.Fatal("Failed to advance to the marketing opt-in screen: ", err)
	}

	if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.MarketingOptInScreen.isVisible()"); err != nil {
		s.Fatal("Failed to wait for the marketing opt-in screen: ", err)
	}

	ui := uiauto.New(tconn).WithTimeout(60 * time.Second)

	var a11ButtonVisible bool
	if err := oobeConn.Eval(ctx, "OobeAPI.screens.MarketingOptInScreen.isAccessibilityButtonVisible()", &a11ButtonVisible); err != nil {
		s.Fatal("Failed to fetch accessibility button visibility: ", err)
	}

	if a11ButtonVisible != tabletMode {
		s.Fatalf("Accessibility button should have visibility set to %v, but received %v", tabletMode, a11ButtonVisible)
	}

	// Laptop-mode flow.
	if !tabletMode {
		var getStartedButtonName string
		if err := oobeConn.Eval(ctx, "OobeAPI.screens.MarketingOptInScreen.getGetStartedButtonName()", &getStartedButtonName); err != nil {
			s.Fatal("Failed to retrieve the get started button name: ", err)
		}

		getStartedButton := nodewith.Name(getStartedButtonName).Role(role.Button)
		if err := ui.WaitUntilExists(getStartedButton)(ctx); err != nil {
			s.Fatal("Failed to wait until get started button is shown: ", err)
		}
		if err := ui.LeftClick(getStartedButton)(ctx); err != nil {
			s.Fatal("Failed to click on get started button: ", err)
		}

		return
	}

	// Tablet-mode flow.
	var a11yButtonName string
	if err := oobeConn.Eval(ctx, "OobeAPI.screens.MarketingOptInScreen.getAccessibilityButtonName()", &a11yButtonName); err != nil {
		s.Fatal("Failed to retrieve the accessibility button name: ", err)
	}

	a11yButton := nodewith.Name(a11yButtonName).Role(role.Button)
	if err := ui.WaitUntilExists(a11yButton)(ctx); err != nil {
		s.Fatal("Failed to wait until accessibility button is shown: ", err)
	}
	if err := ui.LeftClick(a11yButton)(ctx); err != nil {
		s.Fatal("Failed to click on accessibility button: ", err)
	}

	if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.MarketingOptInScreen.isAccessibilityStepReadyForTesting()"); err != nil {
		s.Fatal("Failed to wait until accessibility step is shown: ", err)
	}

	var a11yToggleStatus bool
	if err := oobeConn.Eval(ctx, "OobeAPI.screens.MarketingOptInScreen.isAccessibilityToggleOn()", &a11yToggleStatus); err != nil {
		s.Fatal("Failed to wait fetch accessibility toggle status: ", err)
	}

	if a11yToggleStatus {
		s.Fatal("Accessibility toggle should be turned off by default")
	}

	var a11yDoneButtonName string
	if err := oobeConn.Eval(ctx, "OobeAPI.screens.MarketingOptInScreen.getAccessibilityDoneButtonName()", &a11yDoneButtonName); err != nil {
		s.Fatal("Failed to retrieve the accessibility done button name: ", err)
	}

	a11yDoneButton := nodewith.Name(a11yDoneButtonName).Role(role.Button)
	if err := ui.WaitUntilExists(a11yDoneButton)(ctx); err != nil {
		s.Fatal("Failed to wait until accessibility done button is shown: ", err)
	}
	if err := ui.LeftClick(a11yDoneButton)(ctx); err != nil {
		s.Fatal("Failed to click on accessibility done button: ", err)
	}
}
