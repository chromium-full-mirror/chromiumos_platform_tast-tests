// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package quickanswers

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/quickanswers"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ConsentAllowed,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test Quick Answers consent flow",
		Contacts: []string{
			"assistive-eng@google.com",
			"chromeos-sw-engprod@google.com",
		},
		BugComponent: "b:905229", // ChromeOS > Software > Assistive
		Attr: []string{
			"group:hw_agnostic",
			"group:mainline",
			"informational",
		},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{{
			Fixture: quickanswers.BasicFixture,
			Val:     browser.TypeAsh,
		}, {
			Name:              "lacros",
			Fixture:           quickanswers.LacrosFixture,
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               browser.TypeLacros,
		}},
	})
}

// ConsentAllowed tests Quick Answers consent flow.
func ConsentAllowed(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	const queryWord = "icosahedron"
	conn, _, closeBrowser, err := browserfixt.SetUpWithURL(
		ctx, cr, s.Param().(browser.Type),
		quickanswers.BuildDataURL(queryWord))
	if err != nil {
		s.Fatal("Failed to open a browser: ", err)
	}
	defer closeBrowser(cleanupCtx)
	defer conn.Close()

	// Stacked defers are executed in last-in first-out order.
	// DumpUITreeWithScreenshotOnError should be after a defer of closing a
	// browser as we want to capture browser UI.
	defer faillog.DumpUITreeWithScreenshotOnError(
		ctx, s.OutDir(), s.HasError, cr, "ui")

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	query, err := quickanswers.SelectQueryWord(ctx, tconn, queryWord)
	if err != nil {
		s.Fatal("Failed to select a query word: ", err)
	}

	ui := uiauto.New(tconn)
	// Right click the selected word and ensure the consent UI shows up.
	userConsent := nodewith.ClassName("UserConsentView")
	allowButton := nodewith.Name("Allow").ClassName("CustomizedLabelButton")
	if err := uiauto.Combine("Show consent UI",
		ui.RightClick(query),
		ui.WaitUntilExists(userConsent))(ctx); err != nil {
		s.Fatal("Quick Answers consent UI not showing up: ", err)
	}

	// Left click the allow button and ensure the Quick Answers UI shows up
	// with the query result.
	quickAnswers := nodewith.ClassName("QuickAnswersView")
	definitionResult := nodewith.NameContaining("twenty plane faces").ClassName("QuickAnswersTextLabel")
	if err := uiauto.Combine("Show Quick Answers query result",
		ui.LeftClick(allowButton),
		ui.WaitUntilExists(quickAnswers),
		ui.WaitUntilExists(definitionResult))(ctx); err != nil {
		s.Fatal("Quick Answers query result not showing up: ", err)
	}

	// Dismiss the context menu and ensure the Quick Answers UI also dismiss.
	if err := uiauto.Combine("Dismiss context menu",
		ui.LeftClick(query),
		ui.WaitUntilGone(quickAnswers))(ctx); err != nil {
		s.Fatal("Quick Answers result not dismissed: ", err)
	}
}
