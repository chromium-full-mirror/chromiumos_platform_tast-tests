// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package quickanswers

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/event"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/quickanswers"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Translation,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test Quick Answers translation feature",
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
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-513191ba-7f4b-4ae1-84a2-697d1ad31a12",
		}},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{{
			Fixture: fixture.ChromeLoggedInWithGaia,
			Val:     browser.TypeAsh,
		}, {
			Name:              "lacros",
			Fixture:           quickanswers.LacrosFixture,
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               browser.TypeLacros,
		}},
	})
}

// Translation tests Quick Answers translation feature.
func Translation(ctx context.Context, s *testing.State) {
	// Reserve five seconds for various cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	if err := quickanswers.SetPrefValue(ctx, tconn, "settings.quick_answers.enabled", true); err != nil {
		s.Fatal("Failed to enable Quick Answers: ", err)
	}

	// Setup a browser.
	bt := s.Param().(browser.Type)
	br, closeBrowser, err := browserfixt.SetUp(ctx, cr, bt)
	if err != nil {
		s.Fatal("Failed to open the browser: ", err)
	}
	defer closeBrowser(cleanupCtx)

	// Open page with query word on it.
	const queryWord = "翻译"
	conn, err := br.NewConn(ctx, "https://google.com/search?q="+queryWord)
	if err != nil {
		s.Fatal("Failed to create new Chrome connection: ", err)
	}
	defer conn.Close()
	defer conn.CloseTarget(ctx)

	ui := uiauto.New(tconn)
	// Wait for the query word to appear.
	query := nodewith.Name(queryWord).Role(role.StaticText).First()
	if err := ui.WaitUntilExists(query)(ctx); err != nil {
		s.Fatal("Failed to wait for query to load: ", err)
	}

	// Select the word and setup watcher to wait for text selection event.
	if err := ui.WaitForEvent(nodewith.Root(),
		event.TextSelectionChanged,
		ui.Select(query, 0 /*startOffset*/, query, 2 /*endOffset*/))(ctx); err != nil {
		s.Fatal("Failed to select query: ", err)
	}

	// Right click the selected query word and ensure the Quick Answers UI shows up with the translation result.
	quickAnswers := nodewith.ClassName("QuickAnswersView")
	translationResult := nodewith.NameContaining("translate").ClassName("QuickAnswersTextLabel")
	if err := uiauto.Combine("Show context menu",
		ui.RightClick(query),
		ui.WaitUntilExists(quickAnswers),
		ui.WaitUntilExists(translationResult))(ctx); err != nil {
		s.Fatal("Quick Answers result not showing up: ", err)
	}

	// Dismiss the context menu and ensure the Quick Answers UI also dismiss.
	if err := uiauto.Combine("Dismiss context menu",
		ui.LeftClick(query),
		ui.WaitUntilGone(quickAnswers))(ctx); err != nil {
		s.Fatal("Quick Answers result not dismissed: ", err)
	}
}
