// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	arcCommon "go.chromium.org/tast-tests/cros/common/arc"
	"go.chromium.org/tast-tests/cros/common/chrome/credconfig"
	"go.chromium.org/tast-tests/cros/common/dma"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	uiCommon "go.chromium.org/tast-tests/cros/common/ui"
	"go.chromium.org/tast-tests/cros/local/accountmanager"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/optin"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/retry"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type managedSecondaryAccountBlockArgs struct {
	managed     bool
	browserType browser.Type
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ManagedSecondaryAccountBlock,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks that enterprise secondary account is not available in ARC",
		Contacts:     []string{"arc-commercial@google.com", "mhasank@chromium.org"},
		// ChromeOS > Software > ARC++ > Commercial > Tast Tests
		BugComponent: "b:1487630",
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"chrome", "play_store", "gaia", "android_vm"},
		Timeout:      15 * time.Minute,
		VarDeps: []string{
			arcCommon.ManagedAccountPoolVarName,
			uiCommon.GaiaPoolDefaultVarName,
		},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.ArcEnabled{}, pci.VerifiedFunctionalityOS),
		},
		Params: []testing.Param{
			{
				Name: "managed_lacros",
				Val: managedSecondaryAccountBlockArgs{
					managed:     true,
					browserType: browser.TypeLacros,
				},
				ExtraAttr: []string{"informational"},
			},
			{
				Name: "unmanaged_lacros",
				Val: managedSecondaryAccountBlockArgs{
					managed:     false,
					browserType: browser.TypeLacros,
				},
				ExtraAttr: []string{"informational"},
			},
			{
				Name: "managed_ash",
				Val: managedSecondaryAccountBlockArgs{
					managed:     true,
					browserType: browser.TypeAsh,
				},
				ExtraAttr: []string{"informational"},
			},
			{
				Name: "unmanaged_ash",
				Val: managedSecondaryAccountBlockArgs{
					managed:     false,
					browserType: browser.TypeAsh,
				},
				ExtraAttr: []string{"informational"},
			}},
	})
}

// ManagedSecondaryAccountBlock verifies that enterprise secondary account is not available in ARC.
func ManagedSecondaryAccountBlock(ctx context.Context, s *testing.State) {
	args := s.Param().(managedSecondaryAccountBlockArgs)

	rl := &retry.Loop{Attempts: 1,
		MaxAttempts: 2,
		DoRetries:   true,
		Errorf:      s.Errorf,
		Logf:        s.Logf}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Minute)
	defer cancel()

	if err := testing.Poll(ctx, func(ctx context.Context) (retErr error) {
		creds, err := credconfig.PickNRandomCreds(dma.CredsFromPool(uiCommon.GaiaPoolDefaultVarName), 2 /*n*/)
		if err != nil {
			return rl.Exit("get login creds", err)
		}

		primaryUser := creds[0]
		secondaryUser := creds[1]
		if args.managed {
			secondaryUser, err = credconfig.PickRandomCreds(dma.CredsFromPool(arcCommon.ManagedAccountPoolVarName))
			if err != nil {
				return rl.Exit("get secondary user creds", err)
			}
		}

		cr, err := browserfixt.NewChrome(ctx, args.browserType, lacrosfixt.NewConfig(), chrome.GAIALogin(primaryUser),
			chrome.ARCSupported(),
			chrome.UnRestrictARCCPU(),
			chrome.EnableFeatures("SecondaryAccountAllowedInArcPolicy"),
			chrome.ExtraArgs(arc.DisableSyncFlags()...))
		if err != nil {
			return rl.Retry("start Chrome", err)
		}
		defer cr.Close(cleanupCtx)

		tconn, err := cr.TestAPIConn(ctx)
		if err != nil {
			return rl.Retry("create test API connection", err)
		}

		s.Log("Performing optin")
		if err := optin.PerformAndClose(ctx, cr, tconn); err != nil {
			return rl.Retry("perform optin", err)
		}

		a, err := arc.New(ctx, s.OutDir(), cr.NormalizedUser())
		if err != nil {
			return rl.Retry("start ARC", err)
		}
		defer a.Close(cleanupCtx)

		ui := uiauto.New(tconn).WithTimeout(time.Minute)

		// Open Account Manager page in OS Settings and click Add Google Account button.
		addAccountButton := nodewith.Name("Add Google Account").Role(role.Button)
		if err := uiauto.Combine("Click Add Google Account button",
			accountmanager.OpenAccountManagerSettingsAction(tconn, cr),
			ui.DoDefault(addAccountButton),
			ui.WaitUntilExists(accountmanager.AddAccountDialog()),
		)(ctx); err != nil {
			return rl.Exit("click Add Google Account button", err)
		}

		s.Log("Adding a secondary Account")
		if err := accountmanager.AddAccount(ctx, tconn, secondaryUser.User, secondaryUser.Pass); err != nil {
			return rl.Exit("add a secondary Account", err)
		}

		// Make sure that the settings page is focused again.
		if err := ui.WaitUntilExists(addAccountButton)(ctx); err != nil {
			return rl.Exit("find Add Google Account button", err)
		}

		moreActionsButton := nodewith.Name("More actions, " + secondaryUser.User).Role(role.Button)
		// Find "More actions, <email>" button to make sure that account was added.
		if err := ui.WaitUntilExists(moreActionsButton)(ctx); err != nil {
			return rl.Exit("find More actions button", err)
		}

		d, err := a.NewUIDevice(ctx)
		if err != nil {
			return rl.Exit("initialize UI Automator", err)
		}
		defer d.Close(cleanupCtx)

		s.Log("Checking for account presence in ARC")
		// Managed accounts should not be present in ARC but unmanaged should be.
		if err := accountmanager.CheckIsAccountPresentInARCAction(tconn, d,
			accountmanager.NewARCAccountOptions(secondaryUser.User).ExpectedPresentInARC(!args.managed))(ctx); err != nil {
			return rl.Exit("check that account is present in ARC", err)
		}

		return nil
	}, nil); err != nil {
		s.Fatal("Secondary account state check failed: ", err)
	}
}
