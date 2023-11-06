// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package accountmanager provides functions to manage accounts in-session.
package accountmanager

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	androidui "go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/local/accountmanager"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ARCAccountPicker,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Verify ARC account picker behavior",
		Contacts: []string{
			"team-dent@google.com",
			"anastasiian@chromium.org",
		},
		BugComponent: "b:1279804", // ChromeOS > Software > Commercial (Enterprise) > Identity > Account Manager
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
			"group:hw_agnostic",
		},
		SoftwareDeps: []string{"chrome", "lacros"},
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_container"},
			Fixture:           "loggedInToChromeAndArcWithLacros",
		}, {
			Name:              "vm",
			ExtraSoftwareDeps: []string{"android_vm"},
			Fixture:           "loggedInToChromeAndArcWithLacros",
		}},
		VarDeps: []string{
			"accountmanager.ARCAccountPicker.username",
			"accountmanager.ARCAccountPicker.password",
		},
		Timeout: 6 * time.Minute,
	})
}

func ARCAccountPicker(ctx context.Context, s *testing.State) {
	username := s.RequiredVar("accountmanager.ARCAccountPicker.username")
	password := s.RequiredVar("accountmanager.ARCAccountPicker.password")

	// Reserve one minute for various cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Minute)
	defer cancel()

	cr := s.FixtValue().(accountmanager.FixtureData).Chrome()

	// Connect to Test API to use it with the UI library.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}

	defer func(ctx context.Context) {
		s.Log("Running test cleanup")
		if err := accountmanager.TestCleanup(ctx, tconn, cr); err != nil {
			s.Fatal("Failed to do cleanup: ", err)
		}
	}(cleanupCtx)

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "arc_account_picker")

	ui := uiauto.New(tconn).WithTimeout(time.Minute)
	a := s.FixtValue().(accountmanager.FixtureData).ARC
	defer a.DumpUIHierarchyOnError(ctx, s.OutDir(), s.HasError)

	d, err := a.NewUIDevice(ctx)
	if err != nil {
		s.Fatal("Failed initializing UI Automator: ", err)
	}
	defer d.Close(ctx)

	addAccountButton := nodewith.Name("Add Google Account").Role(role.Button)
	moreActionsButton := nodewith.Name("More actions, " + username).Role(role.Button)
	addAccountDialog := accountmanager.AddAccountDialog()
	arcToggle := nodewith.NameStartingWith("Use this account with Android apps").Role(role.ToggleButton).Ancestor(addAccountDialog)

	// Open Account Manager page in OS Settings and click Add Google Account button.
	if err := uiauto.Combine("click add Google Account button",
		accountmanager.OpenAccountManagerSettingsAction(tconn, cr),
		ui.DoDefault(addAccountButton),
		ui.WaitUntilExists(accountmanager.AddAccountDialog()),
		// Uncheck ARC toggle.
		ui.LeftClickUntil(arcToggle, accountmanager.CheckARCToggleStatusAction(tconn, browser.TypeLacros, false /*expectedVal*/)),
	)(ctx); err != nil {
		s.Fatal("Failed to click add Google Account button: ", err)
	}

	s.Log("Adding a secondary Account")
	if err := accountmanager.AddAccount(ctx, tconn, username, password); err != nil {
		s.Fatal("Failed to add a secondary Account: ", err)
	}

	if err := uiauto.Combine("confirm account addition",
		// Make sure that the settings page is focused again.
		ui.WaitUntilExists(addAccountButton),
		// Find "More actions, <email>" button to make sure that account was added.
		ui.WaitUntilExists(moreActionsButton),
		// Check that account is not present in ARC.
		accountmanager.CheckIsAccountPresentInARCAction(tconn, d,
			accountmanager.NewARCAccountOptions(username).ExpectedPresentInARC(false)),
	)(ctx); err != nil {
		s.Fatal("Failed to confirm account addition: ", err)
	}

	accountPickerItem := nodewith.NameContaining(username).Role(role.Button).Focusable().Ancestor(addAccountDialog)
	if err := uiauto.Combine("add account to ARC from account picker",
		openAddAccountDialogFromARCAction(d, tconn),
		ui.WaitUntilExists(accountPickerItem),
		// Click on account to add it to ARC.
		ui.LeftClick(accountPickerItem),
		// Check that account is present in ARC.
		accountmanager.CheckIsAccountPresentInARCAction(tconn, d,
			accountmanager.NewARCAccountOptions(username).ExpectedPresentInARC(true)),
	)(ctx); err != nil {
		s.Fatal("Failed to add account to ARC from account picker: ", err)
	}
}

// openAddAccountDialogFromARCAction returns an action that clicks 'Add account' button in ARC settings.
func openAddAccountDialogFromARCAction(d *androidui.Device, tconn *chrome.TestConn) action.Action {
	return func(ctx context.Context) error {
		if err := arc.ClickAddAccountInSettings(ctx, d, tconn); err != nil {
			return errors.Wrap(err, "failed to open Add account dialog from ARC")
		}
		return nil
	}
}
