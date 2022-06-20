// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package accountmanager provides functions to manage accounts in-session.
package accountmanager

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/tape"
	"go.chromium.org/tast-tests/cros/local/accountmanager"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mapui"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/testing"

	"go.chromium.org/tast/core/ctxutil"
)

const addAccountOSSettingsTimeout = 7 * time.Minute

func init() {
	testing.AddTest(&testing.Test{
		Func:         AddAccountOSSettings,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Verify that a secondary account can be added and removed from OS Settings",
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
			"group:crosbolt",
			"crosbolt_nightly",
			"group:hw_agnostic",
		},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_p"},
			Fixture:           "loggedInToChromeAndArc",
			Val:               browser.TypeAsh,
		}, {
			Name:              "vm",
			ExtraSoftwareDeps: []string{"android_vm"},
			Fixture:           "loggedInToChromeAndArc",
			Val:               browser.TypeAsh,
		}, {
			Name:              "lacros",
			ExtraSoftwareDeps: []string{"android_p", "lacros"},
			Fixture:           "loggedInToChromeAndArcWithLacros",
			Val:               browser.TypeLacros,
		}, {
			Name:              "vm_lacros",
			ExtraSoftwareDeps: []string{"android_vm", "lacros"},
			Fixture:           "loggedInToChromeAndArcWithLacros",
			Val:               browser.TypeLacros,
		}},
		VarDeps: []string{tape.ServiceAccountVar},
		Timeout: addAccountOSSettingsTimeout,
	})
}

func AddAccountOSSettings(ctx context.Context, s *testing.State) {
	// Reserve one minute for various cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Minute)
	defer cancel()

	cr := s.FixtValue().(accountmanager.FixtureData).Chrome()

	// Setup the browser.
	br, closeBrowser, err := browserfixt.SetUp(ctx, cr, s.Param().(browser.Type))
	if err != nil {
		s.Fatal("Failed to setup chrome: ", err)
	}
	defer closeBrowser(cleanupCtx)

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

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "add_account_os_settings")

	ui := uiauto.New(tconn).WithTimeout(time.Minute)
	a := s.FixtValue().(accountmanager.FixtureData).ARC
	defer a.DumpUIHierarchyOnError(ctx, s.OutDir(), s.HasError)

	arcDevice, err := a.NewUIDevice(ctx)
	if err != nil {
		s.Fatal("Failed to initialize UI Automator: ", err)
	}
	defer arcDevice.Close(ctx)

	timeout := int32(addAccountOSSettingsTimeout.Seconds())
	accManager, acc, err := tape.NewOwnedTestAccountManager(ctx, []byte(s.RequiredVar(tape.ServiceAccountVar)), false /*lock*/, tape.WithTimeout(timeout), tape.WithPoolID(tape.AccountManager))
	if err != nil {
		s.Fatal("Failed to create an account manager and lease an account: ", err)
	}
	defer accManager.CleanUp(cleanupCtx)

	// Open Account Manager page in OS Settings and click Add Google Account button.
	addAccountButton := mapui.OSSettingsAddGoogleAccountButton
	if err := uiauto.Combine("Click Add Google Account button",
		accountmanager.OpenAccountManagerSettingsAction(tconn, cr),
		ui.LeftClickUntil(addAccountButton, ui.Exists(accountmanager.AddAccountDialog())),
	)(ctx); err != nil {
		s.Fatal("Failed to click Add Google Account button: ", err)
	}

	// ARC toggle should be checked.
	if err := accountmanager.CheckARCToggleStatus(ctx, tconn, s.Param().(browser.Type), true); err != nil {
		s.Fatal("Failed to check ARC toggle status: ", err)
	}

	s.Log("Adding a secondary Account")
	if err := accountmanager.AddAccount(ctx, tconn, acc.Username, acc.Password); err != nil {
		s.Fatal("Failed to add a secondary Account: ", err)
	}
	accountAddedStart := time.Now()

	// Make sure that the settings page is focused again.
	if err := ui.WaitUntilExists(addAccountButton)(ctx); err != nil {
		s.Fatal("Failed to find Add Google Account button: ", err)
	}
	// Find "More actions, <email>" button to make sure that account was added.
	moreActionsButton := nodewith.Name("More actions, " + acc.Username).Role(role.Button)
	if err := ui.WaitUntilExists(moreActionsButton)(ctx); err != nil {
		s.Fatal("Failed to find More actions button: ", err)
	}

	// Check that account is present in ARC.
	s.Log("Verifying that account is present in ARC")
	if err := accountmanager.OpenARCAccountsInARCSettings(ctx, tconn, arcDevice); err != nil {
		s.Fatal("Failed to open ARC accounts list: ", err)
	}
	arcCheckStart := time.Now()
	// Note: the method will return as soon as account appears in ARC.
	if err := accountmanager.CheckIsAccountPresentInARC(ctx, tconn, arcDevice,
		accountmanager.NewARCAccountOptions(acc.Username).ExpectedPresentInARC(true)); err != nil {
		s.Fatal("Failed to check that account is present in ARC: ", err)
	}
	saveARCAccountAdditionTime(time.Since(arcCheckStart), time.Since(accountAddedStart), s)

	// Check that account is present in OGB.
	s.Log("Verifying that account is present in OGB")
	secondaryAccountListItem := nodewith.NameContaining(acc.Username).Role(role.Link)
	if err := accountmanager.CheckOneGoogleBar(ctx, tconn, br, ui.WaitUntilExists(secondaryAccountListItem)); err != nil {
		s.Fatal("Failed to check that account is present in OGB: ", err)
	}

	if err := accountmanager.RemoveAccountFromOSSettings(ctx, tconn, cr, acc.Username); err != nil {
		s.Fatal("Failed to remove account from OS Settings: ", err)
	}

	if err := ui.WaitUntilGone(moreActionsButton)(ctx); err != nil {
		s.Fatal("Failed to remove account: ", err)
	}

	// Check that account is not present in OGB anymore.
	s.Log("Verifying that account is not present in OGB")
	if err := accountmanager.CheckOneGoogleBar(ctx, tconn, br, ui.WaitUntilGone(secondaryAccountListItem)); err != nil {
		s.Fatal("Failed to remove account from OGB: ", err)
	}

	// Check that account is not present in ARC.
	s.Log("Verifying that account is not present in ARC")
	if err := accountmanager.CheckIsAccountPresentInARCAction(tconn, arcDevice,
		accountmanager.NewARCAccountOptions(acc.Username).ExpectedPresentInARC(false).PreviouslyPresentInARC(true))(ctx); err != nil {
		s.Fatal("Failed to check that account is NOT present in ARC: ", err)
	}
}

// saveARCAccountAdditionTime saves ARC account addition duration metrics.
// `duration` is the elapsed time since ARC account list was opened and until
// the account appeared in the list.
// `totalDuration` is the total elapsed time since account was added and until
// the account appeared in the ARC account list. This equals to `duration` +
// the time needed to open ARC Settings and navigating to ARC account list.
func saveARCAccountAdditionTime(duration, totalDuration time.Duration, s *testing.State) {
	pv := perf.NewValues()
	pv.Set(perf.Metric{
		Name:      "AddAccountOSSettings.ARCAccountAdditionTime",
		Unit:      "seconds",
		Direction: perf.SmallerIsBetter,
	}, float64(duration.Seconds()))
	pv.Set(perf.Metric{
		Name:      "AddAccountOSSettings.ARCTotalAccountAdditionTime",
		Unit:      "seconds",
		Direction: perf.SmallerIsBetter,
	}, float64(totalDuration.Seconds()))

	s.Log("AddAccountOSSettings.ARCAccountAdditionTime: ", duration.Seconds())
	s.Log("AddAccountOSSettings.ARCTotalAccountAdditionTime: ", totalDuration.Seconds())

	if err := pv.Save(s.OutDir()); err != nil {
		s.Error("Failed saving perf data: ", err)
	}
}
