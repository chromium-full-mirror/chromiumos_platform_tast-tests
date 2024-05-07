// Copyright 2022 The ChromiumOS Authors
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
		Func:         DefinitionWithSimpleWord,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test Quick Answers always trigger for single word feature",
		Contacts: []string{
			"assistive-eng@google.com",
			"angelaxiao@google.com",
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
			Value: "screenplay-c94b404a-ac8f-4119-93d5-775d59b8ac20",
		}, {
			Key:   "feature_id",
			Value: "screenplay-eec006a9-871b-4742-85c4-5979ac7323c9",
		}},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{{
			Fixture: quickanswers.Parameterize(
				quickanswers.EnabledWithBrowserFixture,
				quickanswers.VariantSimpleWord,
			),
		}, {
			Name: "lacros",
			Fixture: quickanswers.Parameterize(
				quickanswers.EnabledWithBrowserFixture,
				quickanswers.VariantSimpleWord,
			),
			ExtraSoftwareDeps: []string{"lacros"},
		}},
	})
}

// DefinitionWithSimpleWord tests Quick Answers always trigger for single word feature.
func DefinitionWithSimpleWord(ctx context.Context, s *testing.State) {
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

	// Right click the selected word and ensure the Quick Answers UI shows up with the definition result.
	quickAnswers := nodewith.ClassName("QuickAnswersView")
	definitionResult := nodewith.NameContaining("domesticated carnivorous mammal").ClassName("QuickAnswersTextLabel")
	ui := uiauto.New(tconn)
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
