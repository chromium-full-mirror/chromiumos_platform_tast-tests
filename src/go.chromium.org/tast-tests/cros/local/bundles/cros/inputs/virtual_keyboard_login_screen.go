// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/ui"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ime"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/vkb"
	"go.chromium.org/tast-tests/cros/local/chrome/useractions"
	"go.chromium.org/tast-tests/cros/local/inputs/inputactions"
	"go.chromium.org/tast-tests/cros/local/inputs/pre"
	"go.chromium.org/tast-tests/cros/local/inputs/util"
	"go.chromium.org/tast-tests/cros/local/uidetection"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         VirtualKeyboardLoginScreen,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that the virtual keyboard works on login screen",
		Attr:         []string{"group:mainline", "informational", "group:input-tools", "group:input-tools-upstream", "group:hw_agnostic"},
		Contacts:     []string{"essential-inputs-gardener-oncall@google.com", "essential-inputs-team@google.com"},
		BugComponent: "b:95887",
		SoftwareDeps: []string{"inputs_deps", "chrome", "google_virtual_keyboard"},
		HardwareDeps: hwdep.D(pre.InputsStableModels),
		VarDeps: []string{
			ui.GaiaPoolDefaultVarName,
			"ui.signinProfileTestExtensionManifestKey",
		},
		SearchFlags: util.SearchFlagsWithIMEAndScreenPlay([]ime.InputMethod{ime.EnglishUS}, []string{"screenplay-79dbd617-95bd-484b-89fe-8921fb9178c6"}),
		Timeout:     3 * time.Minute,
		Params: []testing.Param{
			{
				Name: "tablet",
				Val:  true, // Tablet VK.
			},
			{
				Name: "clamshell",
				Val:  false, // A11y VK.
			},
		},
	})
}

func VirtualKeyboardLoginScreen(ctx context.Context, s *testing.State) {
	// Give 5 seconds to clean up and dump out UI tree.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// Use GAIA login otherwise user profile does not exist after restart UI.
	cr, err := chrome.New(ctx, chrome.GAIALoginPool(s.RequiredVar(ui.GaiaPoolDefaultVarName)))
	if err != nil {
		s.Fatal("Failed to start Chrome via GAIA login: ", err)
	}

	// Restart device and keep state to land login page.
	chromeOpts := []chrome.Option{
		chrome.NoLogin(),
		chrome.KeepState(),
		chrome.LoadSigninProfileExtension(s.RequiredVar("ui.signinProfileTestExtensionManifestKey")),
	}

	isTabletVK := s.Param().(bool)
	if isTabletVK {
		chromeOpts = append(chromeOpts, chrome.ExtraArgs("--force-tablet-mode=touch_view"), chrome.VKEnabled())
	} else {
		chromeOpts = append(chromeOpts, chrome.ExtraArgs("--force-tablet-mode=clamshell"))
	}
	cr, err = chrome.New(ctx, chromeOpts...)
	if err != nil {
		s.Fatal("Failed to start Chrome after restart: ", err)
	}

	tconn, err := cr.SigninProfileTestAPIConn(ctx)
	if err != nil {
		s.Fatal("Creating test API connection failed: ", err)
	}

	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	uc, err := inputactions.NewInputsUserContext(ctx, s, cr, tconn, nil)
	if err != nil {
		s.Fatal("Failed to create user context: ", err)
	}

	ui := uiauto.New(tconn)
	ud := uidetection.NewDefault(tconn).WithTimeout(3 * time.Second).WithScreenshotStrategy(uidetection.ImmediateScreenshot)
	vkbCtx := vkb.NewContext(cr, tconn)
	leftShiftKey := nodewith.Name("shift").Ancestor(vkb.NodeFinder.HasClass("key_pos_shift_left"))
	passwordField := nodewith.NameContaining("Password").Role(role.TextField)

	// Manually enable A11y VK and click password field to trigger VK.
	if !isTabletVK {
		if err := uiauto.Combine("trigger A11y VK",
			vkbCtx.EnableA11yVirtualKeyboard(true),
			vkbCtx.ClickUntilVKShown(passwordField),
		)(ctx); err != nil {
			s.Fatal("Failed to enable A11y VK: ", err)
		}
	}

	// Type password "x2Zg  m" to cover letters, capitals, numbers and space.
	// Note: Any keys with accent popup are not used due to b/246622721.
	passwordText := uidetection.TextBlock([]string{"x2Zg", "m"})

	if err := uiauto.UserAction(
		"VK typing input",
		uiauto.Combine(`input and verify login password`,
			ui.WaitUntilExists(passwordField.Focused().Editable()),
			vkbCtx.TapKeys([]string{"x", "2"}), // pwd: x2
			uiauto.Retry(3, uiauto.Combine(
				"press left SHIFT key and check VK shifted",
				vkbCtx.TapNode(leftShiftKey),
				vkbCtx.WaitForKeysExist([]string{"Z"}),
			)),
			vkbCtx.TapKey("Z"), // pwd: x2Z
			vkbCtx.TapKeysIgnoringCase([]string{"g", "space", "space", "m"}), // pwd: x2Zg  m
			uiauto.Retry(5, uiauto.NamedCombine(
				"Show password and validate text",
				ui.DoDefault(nodewith.Name("Show password")),
				ud.WithScreenshotResizing().WaitUntilExists(passwordText),
			)),
		),
		uc,
		&useractions.UserActionCfg{
			Attributes: map[string]string{
				useractions.AttributeFeature:      useractions.FeatureVKTyping,
				useractions.AttributeInputField:   "Password field on login page",
				useractions.AttributeTestScenario: "Use VK in password field on login page",
			},
		},
	)(ctx); err != nil {
		s.Fatal("Failed to verify VK input in password field on login page: ", err)
	}
}
