// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/inputs/spellcheck"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/imesettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/inputs/fixture"
	"go.chromium.org/tast-tests/cros/local/inputs/pre"
	"go.chromium.org/tast-tests/cros/local/inputs/testserver"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SpellCheckRemoveWords,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Verify that spell check works while typing the word removed from customize spell check",
		Contacts: []string{
			"essential-inputs-gardener-oncall@google.com",
			"essential-inputs-team@google.com",
		},
		BugComponent: "b:95887",
		Attr:         []string{"group:mainline", "informational", "group:input-tools"},
		SoftwareDeps: []string{"inputs_deps", "chrome"},
		HardwareDeps: hwdep.D(pre.InputsStableModels),
		Params: []testing.Param{{
			Fixture: fixture.ClamshellNonVK,
		}, {
			Name:              "lacros",
			Fixture:           fixture.LacrosClamshellNonVK,
			ExtraSoftwareDeps: []string{"lacros"},
		}},
		Timeout: 3 * time.Minute,
	})
}

// SpellCheckRemoveWords verifies that spell check works while typing the word removed from customize spell check.
func SpellCheckRemoveWords(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(fixture.FixtData).Chrome
	tconn := s.FixtValue().(fixture.FixtData).TestAPIConn

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to take keyboard: ", err)
	}
	defer kb.Close(ctx)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	ui := uiauto.New(tconn)
	settings, err := imesettings.LaunchAtInputsSettingsPage(ctx, tconn, cr)
	if err != nil {
		s.Fatal("Failed to open the OS settings page: ", err)
	}
	defer settings.Close(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_dump_settings")

	s.Log("Enabling spelling and grammar check")
	if err := settings.SetSpellingAndGrammarCheck(cr, true)(ctx); err != nil {
		s.Fatal("Failed to toggle spelling and grammar check: ", err)
	}

	const customizedWord = "newword"
	if err := spellcheck.SetOneTimeMarkerWithWord(ctx, tconn, customizedWord); err != nil {
		s.Fatal("Failed to observe spelling marker changes: ", err)
	}

	s.Log("Adding customized word: ", customizedWord)
	if err := settings.AddCustomizedSpellCheck(cr, kb, customizedWord)(ctx); err != nil {
		s.Fatal("Failed to add customized word: ", err)
	}

	s.Log("Removing customized word: ", customizedWord)
	if err := settings.RemoveCustomizedSpellCheck()(ctx); err != nil {
		s.Fatal("Failed to click delete word button: ", err)
	}

	its, err := testserver.LaunchBrowser(ctx, s.FixtValue().(fixture.FixtData).BrowserType, cr, tconn)
	if err != nil {
		s.Fatal("Failed to launch inputs test server: ", err)
	}
	defer its.CloseAll(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_dump_browser")

	s.Logf("Typing a misspelling word %q and checks the spelling suggestion", customizedWord)
	inputField := testserver.TextInputField
	menuContainer := nodewith.Ancestor(nodewith.Role(role.Menu).HasClass("SubmenuView"))
	addToDictionary := menuContainer.Name("Add to dictionary").HasClass("MenuItemView").Role(role.MenuItem)
	if err := uiauto.Combine("check for spelling check suggestion",
		its.Clear(inputField),
		its.ClickFieldAndWaitForActive(inputField),
		kb.TypeAction(customizedWord),
		spellcheck.WaitUntilMarkerExists(tconn, customizedWord),
		its.RightClickFieldAndWaitForActive(inputField),
		ui.WaitUntilExists(addToDictionary),
	)(ctx); err != nil {
		s.Fatal("Failed to complete all actions: ", err)
	}
}
