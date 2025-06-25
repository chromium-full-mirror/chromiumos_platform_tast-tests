// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package quickanswers

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/dma"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/common/ui"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/quickanswers"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: MagicBoostConsentAllowed,
		Desc: "Test Quick Answers Magic Boost consent flow",
		Contacts: []string{
			"assistive-eng@google.com",
			"chromeos-consumer-engprod@google.com",
			"hdchuong@google.com",
		},
		BugComponent: "b:905229", // ChromeOS > Software > Assistive
		Attr: []string{
			"group:hw_agnostic",
			"group:mainline",
			"informational",
			"group:release-health",
			"group:cbx",
			"cbx_feature_enabled",
			"cbx_unstable",
		},
		SoftwareDeps: []string{"chrome", "gaia"},
		HardwareDeps: hwdep.D(hwdep.FeatureLevel(1)), // Only enable on CBX devices.
		TestBedDeps:  []string{tbdep.Cbx(true)},
	})
}

func MagicBoostConsentAllowed(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, time.Second*10)
	defer cancel()

	opts := []chrome.Option{
		chrome.GAIALoginPool(dma.CredsFromPool(ui.GaiaPoolDefaultVarName)),
		chrome.ExtraArgs("--enable-features=MagicBoostRevampForQuickAnswers"),
		// Force magic boost to be enabled.
		chrome.ExtraArgs("--mahi-restrictions-override"),
	}

	cr, err := chrome.New(ctx, opts...)

	if err != nil {
		s.Fatal("Chrome login failed: ", err)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	recorder := uiauto.CreateAndStartScreenRecorder(ctx, tconn, cr)
	defer uiauto.StopAndSaveOnError(cleanupCtx, recorder, filepath.Join(s.OutDir(), "screen_recording.webm"), s.HasError)

	queryWord := "hello"

	if _, err := cr.NewConn(ctx, quickanswers.BuildDataURL(queryWord)); err != nil {
		s.Fatal("Failed to open the Chrome Browser: ", err)
	}

	query, err := quickanswers.SelectQueryWord(ctx, tconn, queryWord)
	if err != nil {
		s.Fatal("Failed to select a query word: ", err)
	}

	ui := uiauto.New(tconn)
	// Right click the selected word and ensure the consent UI shows up.
	userConsent := nodewith.ClassName("MagicBoostUserConsentView")
	defineButton := nodewith.Name("Define \"hello\"").ClassName("LabelButton")
	consentGotItButton := nodewith.Name("Got it").ClassName("MdTextButton")
	if err := uiauto.Combine("Show Quick Answer Magic Boost pre-consent UI and accept it",
		ui.RightClick(query),
		ui.WaitUntilExists(userConsent),
		ui.LeftClick(defineButton),
		ui.WaitUntilExists(consentGotItButton),
		ui.LeftClick(consentGotItButton),
		ui.WaitUntilGone(consentGotItButton))(ctx); err != nil {
		s.Fatal("Quick Answer Magic Boost pre-consent UI not showing up: ", err)
	}

	quickAnswers := nodewith.ClassName("QuickAnswersView")
	if err := uiauto.Combine("Show Quick Answers query result",
		// The consent UI closes context menu. Right click again to see
		// if Quick Answers card is shown this time.
		ui.RightClick(query),
		ui.WaitUntilExists(quickAnswers))(ctx); err != nil {
		s.Fatal("Quick Answers query result not showing up: ", err)
	}

	// Dismiss the context menu and ensure the Quick Answers UI also dismiss.
	if err := uiauto.Combine("Dismiss context menu",
		ui.LeftClick(query),
		ui.WaitUntilGone(quickAnswers))(ctx); err != nil {
		s.Fatal("Quick Answers result not dismissed: ", err)
	}
}
