// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package quickanswers

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/quickanswers"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DefinitionWithNonEnglishWord,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test Quick Answers definition feature on non-English words",
		Contacts: []string{
			"cros-assistive@google.com",
			"chromeos-consumer-engprod@google.com",
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
		SoftwareDeps: []string{"chrome", "gaia"},
		Params: []testing.Param{
			{
				Name: "es",
				Fixture: quickanswers.Parameterize(
					quickanswers.EnabledWithBrowserFixture,
					quickanswers.VariantSingleWordEs,
				),
				Val: "cinco lados",
			},
			{
				Name: "it",
				Fixture: quickanswers.Parameterize(
					quickanswers.EnabledWithBrowserFixture,
					quickanswers.VariantSingleWordIt,
				),
				Val: "sette giorni",
			},
			{
				Name: "fr",
				Fixture: quickanswers.Parameterize(
					quickanswers.EnabledWithBrowserFixture,
					quickanswers.VariantSingleWordFr,
				),
				Val: "sept jours",
			},
			{
				Name: "pt",
				Fixture: quickanswers.Parameterize(
					quickanswers.EnabledWithBrowserFixture,
					quickanswers.VariantSingleWordPt,
				),
				Val: "11 jogadores",
			},
			{
				Name: "de",
				Fixture: quickanswers.Parameterize(
					quickanswers.EnabledWithBrowserFixture,
					quickanswers.VariantSingleWordDe,
				),
				Val: "dreimal",
			},
			// We won't need to test all variants for Lacros.
			// For lacros, focus is to test that preferred languages pref is
			// working correctly with Lacros.
			{
				Name: "lacros",
				Fixture: quickanswers.Parameterize(
					quickanswers.EnabledWithBrowserLacrosFixture,
					quickanswers.VariantSingleWordEs,
				),
				Val:               "cinco lados",
				ExtraSoftwareDeps: []string{"lacros"},
			},
		},
	})
}

// DefinitionWithNonEnglishWord tests Quick Answers definition feature on non-English words.
func DefinitionWithNonEnglishWord(ctx context.Context, s *testing.State) {
	responseKeyword := s.Param().(string)
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	queryWord := s.FixtValue().(quickanswers.HasQueryWord).QueryWord()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	queryFinder, err := quickanswers.SelectQueryWord(ctx, tconn, queryWord)
	if err != nil {
		s.Fatal("Failed to select a query word: ", err)
	}

	ui := uiauto.New(tconn)

	// Right click the selected word and ensure the Quick Answers UI shows
	// up with the definition result.
	quickAnswers := nodewith.ClassName("QuickAnswersView")
	definitionResult := nodewith.NameContaining(responseKeyword).ClassName("QuickAnswersTextLabel")
	if err := uiauto.Combine("Show context menu",
		ui.RightClick(queryFinder),
		ui.WaitUntilExists(quickAnswers),
		ui.WaitUntilExists(definitionResult))(ctx); err != nil {
		s.Fatal("Quick Answers result not showing up: ", err)
	}

	// Dismiss the context menu and ensure the Quick Answers UI also dismiss.
	if err := uiauto.Combine("Dismiss context menu",
		ui.LeftClick(queryFinder),
		ui.WaitUntilGone(quickAnswers))(ctx); err != nil {
		s.Fatal("Quick Answers result not dismissed: ", err)
	}

}
