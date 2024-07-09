// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package oobe

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type testParam struct {
	isRecommendScreenEligible bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         PersonalizedOnboarding,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test Use-Case selection and Personalized Recommend Apps screens",
		Contacts: []string{
			"cros-oobe@google.com",
			"chromeos-consumer-engprod@google.com",
			"bohdanty@google.com",
			"bchikhaoui@google.com",
		},
		BugComponent: "b:1263090", // ChromeOS > Software > OOBE
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      chrome.LoginTimeout + 3*time.Minute,
		Params: []testing.Param{{
			Name: "recommend_apps_not_eligible",
			Val:  testParam{false},
		}, {
			Name: "recommend_apps_eligible",
			Val:  testParam{true},
		}},
	})
}

func PersonalizedOnboarding(ctx context.Context, s *testing.State) {
	skipButton := nodewith.Name("Skip").Role(role.Button)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	isRecommendAppsEligible := s.Param().(testParam).isRecommendScreenEligible

	options := []chrome.Option{
		chrome.DontSkipOOBEAfterLogin(),
		chrome.EnableFeatures("OobePersonalizedOnboarding"),
	}

	if isRecommendAppsEligible {
		options = append(options,
			chrome.ExtraArgs("--oobe-skip-new-user-check-for-testing"),
			chrome.ARCSupported())
	}

	cr, err := chrome.New(ctx, options...)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	oobeConn, err := cr.WaitForOOBEConnection(ctx)
	if err != nil {
		s.Fatal("Failed to create OOBE connection: ", err)
	}
	defer oobeConn.Close()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}

	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	ui := uiauto.New(tconn).WithTimeout(30 * time.Second)

	// To prevent potential race conditions wait for the Consolidated Consent
	// screen to show up first. After that it should be safe to call
	// `advanceToScreen` and move to the Personalized Onboarding flow.
	var shouldSkipConsolidatedConsentScreen bool
	if err := oobeConn.Eval(ctx, "OobeAPI.screens.ConsolidatedConsentScreen.shouldSkip()", &shouldSkipConsolidatedConsentScreen); err != nil {
		s.Fatal("Failed to evaluate whether to skip consolidated consent screen: ", err)
	}

	if !shouldSkipConsolidatedConsentScreen {
		if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.ConsolidatedConsentScreen.isReadyForTesting()"); err != nil {
			s.Fatal("Failed to wait for consolidated consent screen to be visible: ", err)
		}
	}

	// Device Use-Case screen skip flow.
	if err := oobeConn.Eval(ctx, "OobeAPI.advanceToScreen('categories-selection')", nil); err != nil {
		s.Fatal("Failed to advance to the device use-case screen screen: ", err)
	}

	if err := oobeConn.WaitForExprFailOnErr(ctx, "!document.querySelector('#categories-selection').hidden"); err != nil {
		s.Fatal("Failed to wait for the device use-case screen: ", err)
	}

	if err := uiauto.Combine("click skip on the device use-case screen",
		ui.WaitUntilEnabled(skipButton),
		ui.DoDefault(skipButton),
	)(ctx); err != nil {
		s.Fatal("Failed to click skip button on the device use-case screen: ", err)
	}

	// Personalized Recommend Apps screen skip flow.
	if isRecommendAppsEligible {
		if err := oobeConn.WaitForExprFailOnErr(ctx, "!document.querySelector('#personalized-apps').hidden"); err != nil {
			s.Fatal("Failed to wait for the personalized apps screen: ", err)
		}

		if err := uiauto.Combine("click skip on the personalized apps screen",
			ui.WaitUntilEnabled(skipButton),
			ui.DoDefault(skipButton),
		)(ctx); err != nil {
			s.Fatal("Failed to click skip button on the personalized apps screen: ", err)
		}
	}
}
