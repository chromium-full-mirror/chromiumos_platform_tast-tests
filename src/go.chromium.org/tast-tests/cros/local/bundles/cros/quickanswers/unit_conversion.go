// Copyright 2021 The ChromiumOS Authors
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
		Func:         UnitConversion,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test Quick Answers unit conversion feature",
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
			Value: "screenplay-c94b404a-ac8f-4119-93d5-775d59b8ac20",
		}},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{{
			Fixture: quickanswers.Parameterize(
				quickanswers.EnabledWithBrowserFixture,
				quickanswers.VariantUnitConversion,
			),
		}, {
			Name: "lacros",
			Fixture: quickanswers.Parameterize(
				quickanswers.EnabledWithBrowserLacrosFixture,
				quickanswers.VariantUnitConversion,
			),
			ExtraSoftwareDeps: []string{"lacros"},
		}},
	})
}

// UnitConversion tests Quick Answers unit conversion feature.
func UnitConversion(ctx context.Context, s *testing.State) {
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

	// Right click the selected units and ensure the Quick Answers UI shows up with the conversion result in pounds.
	ui := uiauto.New(tconn)
	quickAnswers := nodewith.ClassName("QuickAnswersView")
	unitConversionResult := nodewith.NameContaining("110.231").ClassName("QuickAnswersTextLabel")
	if err := uiauto.Combine("Show context menu",
		ui.RightClick(queryFinder),
		ui.WaitUntilExists(quickAnswers),
		ui.WaitUntilExists(unitConversionResult))(ctx); err != nil {
		s.Fatal("Quick Answers result not showing up: ", err)
	}

	// Dismiss the context menu and ensure the Quick Answers UI also dismiss.
	if err := uiauto.Combine("Dismiss context menu",
		ui.LeftClick(queryFinder),
		ui.WaitUntilGone(quickAnswers))(ctx); err != nil {
		s.Fatal("Quick Answers result not dismissed: ", err)
	}
}
