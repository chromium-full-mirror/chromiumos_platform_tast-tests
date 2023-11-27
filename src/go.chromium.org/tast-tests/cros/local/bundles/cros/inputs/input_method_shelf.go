// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ime"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/imesettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/useractions"
	"go.chromium.org/tast-tests/cros/local/inputs/inputactions"
	"go.chromium.org/tast-tests/cros/local/inputs/pre"
	"go.chromium.org/tast-tests/cros/local/inputs/util"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         InputMethodShelf,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that user can toggle shelf option and switch inut method",
		Contacts:     []string{"essential-inputs-gardener-oncall@google.com", "essential-inputs-team@google.com"},
		BugComponent: "b:95887",
		Attr:         []string{"group:mainline", "group:input-tools"},
		SoftwareDeps: []string{"inputs_deps", "chrome", "chrome_internal"},
		SearchFlags:  util.IMESearchFlags([]ime.InputMethod{ime.DefaultInputMethod, ime.JapaneseWithUSKeyboard}),
		Timeout:      3 * time.Minute,
		Params: []testing.Param{{
			ExtraHardwareDeps: hwdep.D(pre.InputsStableModels),
			ExtraAttr:         []string{"group:input-tools-upstream"},
		}, {
			Name:              "informational",
			ExtraHardwareDeps: hwdep.D(pre.InputsUnstableModels),
			ExtraAttr:         []string{"informational"},
		}},
	})
}

func InputMethodShelf(ctx context.Context, s *testing.State) {
	inputMethod := ime.JapaneseWithUSKeyboard

	cr, err := chrome.New(ctx, chrome.EnableFeatures("LanguageSettingsUpdate2"))
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(ctx)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	uc, err := inputactions.NewInputsUserContext(ctx, s, cr, tconn, nil)
	if err != nil {
		s.Fatal("Failed to initiate inputs user context: ", err)
	}

	settings, err := imesettings.LaunchAtInputsSettingsPage(ctx, tconn, cr)
	if err != nil {
		s.Fatal("Failed to launch OS settings and land at inputs setting page: ", err)
	}

	ui := uiauto.New(tconn)
	imeMenuTrayButtonFinder := nodewith.Name("IME menu button").Role(role.Button)
	jpOptionFinder := nodewith.Name("Japanese with US keyboard").Role(role.CheckBox)
	usOptionFinder := nodewith.Name("English (US)").Role(role.CheckBox)

	validateShelfDisabledByDefaultAction := uiauto.Combine("validate that setting is disabled by default",
		// Show input options in shelf should be disabled by default.
		settings.WaitUntilToggleOption(cr, string(imesettings.ShowInputOptionsInShelf), false),
		// IME tray should be hidden by default.
		ui.WaitUntilGone(imeMenuTrayButtonFinder),
	)

	if err := uiauto.UserAction(
		"Verify IME shelf is disabled by default",
		validateShelfDisabledByDefaultAction,
		uc,
		&useractions.UserActionCfg{
			Attributes: map[string]string{
				useractions.AttributeFeature: useractions.FeatureIMEManagement,
			},
			Tags: []useractions.ActionTag{useractions.ActionTagOSSettings, useractions.ActionTagIMEShelf},
		},
	)(ctx); err != nil {
		s.Fatal("Failed to validate that input options in shelf is disabled by default: ", err)
	}

	activateAction := uiauto.Combine("input options in shelf is enabled automatically by adding second IME",
		// Add second IME.
		inputMethod.Install(tconn),
		// Show input options in shelf is enabled automatically.
		settings.WaitUntilToggleOption(cr, string(imesettings.ShowInputOptionsInShelf), true),
		// IME tray should be displayed after adding second IME.
		ui.WaitUntilExists(imeMenuTrayButtonFinder),
	)

	if err := uiauto.UserAction(
		"input options in shelf is enabled by adding second IME",
		activateAction,
		uc,
		&useractions.UserActionCfg{
			Attributes: map[string]string{
				useractions.AttributeFeature: useractions.FeatureIMEManagement,
			},
			Tags: []useractions.ActionTag{useractions.ActionTagOSSettings, useractions.ActionTagIMEShelf},
		},
	)(ctx); err != nil {
		s.Fatal("Failed to validate that input options in shelf is enabled by adding second IME: ", err)
	}

	changeIMEAction := uiauto.Combine("change input method via IME tray",
		// Select JP input method from IME tray.
		ui.LeftClickUntil(imeMenuTrayButtonFinder, ui.WithTimeout(3*time.Second).WaitUntilExists(jpOptionFinder)),
		ui.LeftClick(jpOptionFinder),
		func(ctx context.Context) error {
			fullyQualifiedIMEID, err := inputMethod.FullyQualifiedIMEID(ctx, tconn)
			if err != nil {
				return errors.Wrapf(err, "failed to get fully qualified IME ID of %q", inputMethod)
			}
			return ime.WaitForInputMethodMatches(ctx, tconn, fullyQualifiedIMEID, 10*time.Second)
		},
		// Select US input method from IME tray.
		ui.LeftClick(imeMenuTrayButtonFinder),
		ui.LeftClick(usOptionFinder),
		func(ctx context.Context) error {
			return ime.WaitForInputMethodMatches(ctx, tconn, ime.ChromeIMEPrefix+ime.EnglishUS.ID, 10*time.Second)
		},
	)

	if err := uiauto.UserAction(
		"Switch input method via IME tray",
		changeIMEAction,
		uc,
		&useractions.UserActionCfg{
			Attributes: map[string]string{
				useractions.AttributeFeature: useractions.FeatureIMEManagement,
			},
			Tags: []useractions.ActionTag{useractions.ActionTagIMEShelf},
		},
	)(ctx); err != nil {
		s.Fatal("Failed to change input method via IME tray: ", err)
	}

	toggleOffAction := uiauto.Combine("toggle off the option in OS setting",
		// Toggle off the option. IME tray should be gone.
		settings.ToggleShowInputOptionsInShelf(cr, false),
		ui.WaitUntilGone(imeMenuTrayButtonFinder),
	)

	if err := uiauto.UserAction(
		"Disable IME tray in OS settings",
		toggleOffAction,
		uc,
		&useractions.UserActionCfg{
			Attributes: map[string]string{
				useractions.AttributeFeature: useractions.FeatureIMEManagement,
			},
			Tags: []useractions.ActionTag{useractions.ActionTagOSSettings, useractions.ActionTagIMEShelf},
		},
	)(ctx); err != nil {
		s.Fatal("Failed to disable IME tray in OS settings: ", err)
	}
}
