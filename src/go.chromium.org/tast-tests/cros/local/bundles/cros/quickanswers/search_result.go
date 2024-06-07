// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package quickanswers

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/quickanswers"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SearchResult,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test Quick Answers card click should bring up search result",
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
			Value: "screenplay-2f2657f3-db9a-4fd1-a277-708fb17af8f6",
		}},
		SoftwareDeps: []string{"chrome", "gaia"},
		Params: []testing.Param{{
			Fixture: quickanswers.Parameterize(
				quickanswers.EnabledWithBrowserFixture,
				quickanswers.VariantSingleWord,
			),
		}, {
			Name: "lacros",
			Fixture: quickanswers.Parameterize(
				quickanswers.EnabledWithBrowserLacrosFixture,
				quickanswers.VariantSingleWord,
			),
			ExtraSoftwareDeps: []string{"lacros"},
		}}})
}

// SearchResult tests Quick Answers card click should bring up search result.
func SearchResult(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	queryWord := s.FixtValue().(quickanswers.HasQueryWord).QueryWord()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	queryFinder, err := quickanswers.SelectQueryWord(ctx, tconn, queryWord)
	if err != nil {
		s.Fatal("Failed to select a query: ", err)
	}

	// Right click the selected word and ensure the Quick Answers UI shows up with the definition result.
	ui := uiauto.New(tconn)
	quickAnswers := nodewith.ClassName("QuickAnswersView")
	definitionResult := nodewith.NameContaining("twenty plane faces").ClassName("QuickAnswersTextLabel")
	if err := uiauto.Combine("Show context menu",
		ui.RightClick(queryFinder),
		ui.WaitUntilExists(quickAnswers),
		ui.WaitUntilExists(definitionResult))(ctx); err != nil {
		s.Fatal("Quick Answers result not showing up: ", err)
	}

	// Left click on the Quick Answers card and ensure the Google search result show up.
	const definitionQueryPrefix = "Define "
	searchResult := nodewith.Name(definitionQueryPrefix + queryWord).Role(role.StaticText).First()
	if err := uiauto.Combine("Show context menu",
		ui.LeftClick(quickAnswers),
		ui.WaitUntilExists(searchResult))(ctx); err != nil {
		s.Fatal("Quick Answers web result not showing up: ", err)
	}
}
