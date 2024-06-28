// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package quickanswers

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/quickanswers"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         EnableFromSettings,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test enabling Quick Answers from settings",
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
		SoftwareDeps: []string{"chrome", "gaia"},
		Params: []testing.Param{{
			Fixture: quickanswers.Parameterize(
				quickanswers.BaseFixture,
				quickanswers.VariantNotEnabled,
			),
		}, {
			Name: "lacros",
			Fixture: quickanswers.Parameterize(
				quickanswers.BaseLacrosFixture,
				quickanswers.VariantNotEnabled,
			),
			ExtraSoftwareDeps: []string{"lacros"},
		}},
	})
}

const (
	searchSettingsShortURL = "osSearch/search"
	queryWord              = "dog"
)

func EnableFromSettings(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	ui := uiauto.New(tconn)
	quickAnswersToggleButton := nodewith.Name("Quick answers").Role(role.ToggleButton)
	osSettings, err := ossettings.LaunchAtPageURL(
		ctx, tconn, cr, searchSettingsShortURL, ui.Exists(quickAnswersToggleButton))
	if err != nil {
		// If waiting quickAnswersToggleButton failed, it might mean UI tree has changed.
		// Take a screenshot with ui tree.
		defer faillog.DumpUITreeWithScreenshotOnError(
			ctx, s.OutDir(), s.HasError, cr, "os_settings")

		s.Fatal("Failed to open Quick Answers settings page: ", err)
	}

	if err := ui.LeftClick(quickAnswersToggleButton)(ctx); err != nil {
		// If clicking quickAnswersToggleButton failed, it might mean UI tree has changed.
		// Take a screenshot with ui tree.
		defer faillog.DumpUITreeWithScreenshotOnError(
			ctx, s.OutDir(), s.HasError, cr, "os_settings")

		s.Fatal("Failed to enable Quick Answers: ", err)
	}

	if err := osSettings.Close(ctx); err != nil {
		s.Fatal("Failed to close os settings: ", err)
	}

	bt := s.FixtValue().(quickanswers.HasBrowserType).BrowserType()
	conn, _, closeBrowser, err := browserfixt.SetUpWithURL(
		ctx, cr, bt, quickanswers.BuildDataURL(queryWord))
	// defer is last-in-first-out. Execution order needs to be:
	// ui tree dump -> close browser -> close conn.
	defer conn.Close()
	defer closeBrowser(ctx)
	defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "browser_ui")
	if err != nil {
		s.Fatal("Failed to open a browser: ", err)
	}

	query, err := quickanswers.SelectQueryWord(ctx, tconn, queryWord)
	if err != nil {
		s.Fatal("Failed to select a query word: ", err)
	}

	quickAnswers := nodewith.ClassName("QuickAnswersView")
	definitionResult := quickanswers.ResultTextContains("domesticated carnivorous mammal")
	if err := uiauto.Combine("confirm Quick Answers is working",
		ui.WaitUntilExists(query),
		ui.RightClick(query),
		ui.WaitUntilExists(quickAnswers),
		ui.WaitUntilExists(definitionResult))(ctx); err != nil {
		s.Fatal("Failed to confirm Quick Answers is working: ", err)
	}
}
