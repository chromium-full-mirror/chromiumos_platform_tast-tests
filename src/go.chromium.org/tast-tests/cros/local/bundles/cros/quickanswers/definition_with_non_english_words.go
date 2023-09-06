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
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/quickanswers"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DefinitionWithNonEnglishWords,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test Quick Answers definition feature on non-English words",
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
			Value: "screenplay-71a93fd0-c626-4d4c-9434-3544261d46ce",
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

// DefinitionWithNonEnglishWords tests Quick Answers definition feature on non-English words.
func DefinitionWithNonEnglishWords(ctx context.Context, s *testing.State) {
	// Reserve five seconds for various cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// Setup a browser.
	bt := s.Param().(browser.Type)
	br, closeBrowser, err := browserfixt.SetUp(ctx, cr, bt)
	if err != nil {
		s.Fatal("Failed to open the browser: ", err)
	}
	defer closeBrowser(cleanupCtx)

	ui := uiauto.New(tconn)

	if err := quickanswers.SetPrefValue(ctx, tconn, "settings.quick_answers.enabled", true); err != nil {
		s.Fatal("Failed to enable Quick Answers: ", err)
	}

	const languagesList = "en,es,it,fr,pt,de"

	if err := quickanswers.SetPrefValue(ctx, tconn, "settings.language.preferred_languages", languagesList); err != nil {
		s.Fatal("Failed to set preferred languages: ", err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer kb.Close(ctx)

	for _, query := range []struct {
		queryWord       string
		languageName    string
		languageCode    string
		responseKeyword string
	}{
		{
			queryWord:       "pentágono",
			languageName:    "Spanish",
			languageCode:    "es",
			responseKeyword: "cinco lados",
		},
		{
			queryWord:       "settimana",
			languageName:    "Italian",
			languageCode:    "it",
			responseKeyword: "sette giorni",
		},
		{
			queryWord:       "semaine",
			languageName:    "French",
			languageCode:    "fr",
			responseKeyword: "sept jours",
		},
		{
			queryWord:       "futebol",
			languageName:    "Portuguese",
			languageCode:    "pt",
			responseKeyword: "11 jogadores",
		},
		{
			queryWord:       "verdreifachen",
			languageName:    "German",
			languageCode:    "de",
			responseKeyword: "dreimal",
		},
	} {
		s.Log("Testing definition query with " + query.languageName + " word: " + query.queryWord)

		// Open page with the query word on it.
		conn, err := br.NewConn(ctx, "https://google.com/search?q="+query.queryWord)
		if err != nil {
			s.Fatal("Failed to create new Chrome connection: ", err)
		}
		defer conn.Close()
		defer conn.CloseTarget(ctx)

		// Wait for the query word to appear.
		queryText := nodewith.Name(query.queryWord).Role(role.StaticText).First()
		if err := ui.WaitUntilExists(queryText)(ctx); err != nil {
			s.Fatal("Failed to wait for query to load: ", err)
		}

		// Right click the selected word and ensure the Quick Answers UI shows up with the definition result.
		quickAnswers := nodewith.ClassName("QuickAnswersView")
		definitionResult := nodewith.NameContaining(query.responseKeyword).ClassName("QuickAnswersTextLabel")
		if err := uiauto.Combine("Show context menu",
			ui.RightClick(queryText),
			ui.WaitUntilExists(quickAnswers),
			ui.WaitUntilExists(definitionResult))(ctx); err != nil {
			s.Fatal("Quick Answers result not showing up: ", err)
		}

		// Dismiss the context menu and ensure the Quick Answers UI also dismiss.
		if err := uiauto.Combine("Dismiss context menu",
			kb.AccelAction("Esc"),
			ui.WaitUntilGone(quickAnswers))(ctx); err != nil {
			s.Fatal("Quick Answers result not dismissed: ", err)
		}
	}
}
