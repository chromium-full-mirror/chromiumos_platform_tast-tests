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
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/event"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/quickanswers"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SettingsButton,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test Quick Answers settings button",
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
		}}})
}

// SettingsButton tests Quick Answers settings button.
func SettingsButton(ctx context.Context, s *testing.State) {
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

	// Open page with source units on it.
	conn, err := br.NewConn(ctx, "https://google.com/search?q=50+kg")
	if err != nil {
		s.Fatal("Failed to create new Chrome connection: ", err)
	}
	defer conn.Close()
	defer conn.CloseTarget(ctx)

	ui := uiauto.New(tconn)
	// Wait for the source units to appear.
	units := nodewith.Name("50 kg").Role(role.StaticText).First()
	if err := ui.WaitUntilExists(units)(ctx); err != nil {
		s.Fatal("Failed to wait for units to load: ", err)
	}

	// Select the units and setup watcher to wait for text selection event.
	if err := ui.WaitForEvent(nodewith.Root(),
		event.TextSelectionChanged,
		ui.Select(units, 0 /*startOffset*/, units, 5 /*endOffset*/))(ctx); err != nil {
		s.Fatal("Failed to select units: ", err)
	}

	// Right click the selected units and ensure the Quick Answers UI shows up
	// with the settings button and the conversion result in pounds.
	quickAnswers := nodewith.ClassName("QuickAnswersView")
	settingsButton := nodewith.ClassName("ImageButton").Name("Open Quick Answers settings")
	unitConversionResult := nodewith.NameContaining("110.231").ClassName("QuickAnswersTextLabel")
	if err := uiauto.Combine("Show context menu",
		ui.RightClick(units),
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
