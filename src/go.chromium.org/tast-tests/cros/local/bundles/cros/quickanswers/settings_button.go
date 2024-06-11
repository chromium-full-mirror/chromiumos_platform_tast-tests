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
		Func:         SettingsButton,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test Quick Answers settings button",
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
		SoftwareDeps: []string{"chrome", "gaia"},
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
		}}})
}

// SettingsButton tests Quick Answers settings button.
func SettingsButton(ctx context.Context, s *testing.State) {
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

	// Right click the selected units and ensure the Quick Answers UI shows up
	// with the settings button and the conversion result in pounds.
	ui := uiauto.New(tconn)
	quickAnswers := nodewith.ClassName("QuickAnswersView")
	settingsButton := nodewith.ClassName("ImageButton").Name("Open Quick Answers settings")
	unitConversionResult := quickanswers.ResultTextContains("110.231")
	if err := uiauto.Combine("Show context menu",
		ui.RightClick(queryFinder),
		ui.WaitUntilExists(quickAnswers),
		ui.WaitUntilExists(settingsButton),
		ui.WaitUntilExists(unitConversionResult))(ctx); err != nil {
		s.Fatal("Quick Answers card not showing up: ", err)
	}

	// Click on the settings button and ensure the OS settings subpage show up.
	OsSettingsQuickAnswersToggleButton := nodewith.NameContaining("Quick answers").Role(role.ToggleButton)
	if err := uiauto.Combine("Click settings button",
		ui.LeftClick(settingsButton),
		ui.WaitUntilExists(OsSettingsQuickAnswersToggleButton))(ctx); err != nil {
		s.Fatal("Quick Answers settings subpage not showing up: ", err)
	}
}
