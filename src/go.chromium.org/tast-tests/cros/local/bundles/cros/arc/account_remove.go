// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/ui"
	arcCommon "go.chromium.org/tast-tests/cros/common/arc"
	"go.chromium.org/tast-tests/cros/common/chrome/credconfig"
	"go.chromium.org/tast-tests/cros/common/dma"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	uiCommon "go.chromium.org/tast-tests/cros/common/ui"
	"go.chromium.org/tast-tests/cros/local/accountmanager"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/arcent"
	"go.chromium.org/tast-tests/cros/local/arc/optin"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast/core/errors"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type accountRemoveTestArgs struct {
	primaryAccountPool   string
	secondaryAccountPool string
	optin                bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         AccountRemove,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that primary managed account cannot be removed from ARC",
		Contacts:     []string{"arc-commercial@google.com", "mhasank@chromium.org"},
		// ChromeOS > Software > ARC++ > Commercial > Tast Tests
		BugComponent: "b:1487630",
		Attr:         []string{"group:mainline", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome", "play_store"},
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
				Name: "managed",
				Val: accountRemoveTestArgs{
					primaryAccountPool:   arcCommon.ManagedAccountPoolVarName,
					secondaryAccountPool: uiCommon.GaiaPoolDefaultVarName,
					optin:                false,
				},
				ExtraSoftwareDeps: []string{"android_container"},
				ExtraAttr:         []string{"informational"},
			},
			{
				Name: "managed_vm",
				Val: accountRemoveTestArgs{
					primaryAccountPool:   arcCommon.ManagedAccountPoolVarName,
					secondaryAccountPool: uiCommon.GaiaPoolDefaultVarName,
					optin:                false,
				},
				ExtraSoftwareDeps: []string{"android_vm"},
				ExtraAttr:         []string{"informational"},
			},
			{
				Name: "unmanaged",
				Val: accountRemoveTestArgs{
					primaryAccountPool:   uiCommon.GaiaPoolDefaultVarName,
					secondaryAccountPool: arcCommon.ManagedAccountPoolVarName,
					optin:                true,
				},
				ExtraSoftwareDeps: []string{"android_container", "gaia"},
				ExtraAttr:         []string{"informational"},
			},
			{
				Name: "unmanaged_vm",
				Val: accountRemoveTestArgs{
					primaryAccountPool:   uiCommon.GaiaPoolDefaultVarName,
					secondaryAccountPool: arcCommon.ManagedAccountPoolVarName,
					optin:                true,
				},
				ExtraSoftwareDeps: []string{"android_vm", "gaia"},
				ExtraAttr:         []string{"informational"},
			}},
	})
}

func AccountRemove(ctx context.Context, s *testing.State) {
	const (
		bootTimeout      = 4 * time.Minute
		defaultUITimeout = 1 * time.Minute
		settingPkgName   = "com.android.settings"
		settingActName   = ".Settings"
	)

	args := s.Param().(accountRemoveTestArgs)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	primary, err := credconfig.PickRandomCreds(dma.CredsFromPool(args.primaryAccountPool))
	if err != nil {
		s.Fatal("Failed to get login creds: ", err)
	}

	fdms, err := arcent.SetupPolicyServerWithArcApps(ctx, s.OutDir(), primary.User, []string{}, arcent.InstallTypeAvailable, arcent.PlayStoreModeAllowList)
	if err != nil {
		s.Fatal("Failed to setup fake policy server: ", err)
	}
	defer fdms.Stop(cleanupCtx)

	cr, err := chrome.New(
		ctx,
		chrome.GAIALogin(primary),
		chrome.ARCSupported(),
		chrome.UnRestrictARCCPU(),
		chrome.DMSPolicy(fdms.URL),
		chrome.ExtraArgs(arc.DisableSyncFlags()...))
	if err != nil {
		s.Fatal("Failed to connect to Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	if args.optin {
		if err := optin.PerformWithRetry(ctx, cr, 2 /*maxAttempts*/); err != nil {
			s.Fatal("Failed to optin to Play Store")
		}
	}

	a, err := arc.NewWithTimeout(ctx, s.OutDir(), bootTimeout, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to start ARC by policy: ", err)
	}
	defer a.Close(cleanupCtx)

	if err := arcent.WaitForProvisioning(ctx, a, 2 /*attempts*/); err != nil {
		s.Fatal("Failed to wait for provisioning: ", err)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}

	d, err := a.NewUIDevice(ctx)
	if err != nil {
		s.Fatal("Failed to initialize UI Automator: ", err)
	}
	defer d.Close(cleanupCtx)

	secondary, err := credconfig.PickRandomCreds(dma.CredsFromPool(args.secondaryAccountPool))
	if err != nil {
		s.Fatal("Failed to get secondary user creds: ", err)
	}

	s.Log("Adding secondary account")
	if err := arcent.AddSecondaryAccount(ctx, tconn, cr, d, secondary.User, secondary.Pass); err != nil {
		s.Fatal("Failed to add secondary account: ", err)
	}

	recorder := uiauto.CreateAndStartScreenRecorder(ctx, tconn)
	defer uiauto.StopAndSaveOnError(cleanupCtx, recorder, filepath.Join(s.OutDir(), "recording.webm"), s.HasError)

	s.Log("Verifying the primary account removal")
	if err := verifyAccountRemovable(ctx, tconn, a, d, primary.User, false /*removable*/); err != nil {
		s.Fatal("Failed to verify primary account is not removable: ", err)
	}
	s.Log("Verifying the secondary account removal")
	if err := verifyAccountRemovable(ctx, tconn, a, d, secondary.User, true /*removable*/); err != nil {
		s.Fatal("Failed to verify secondary account is removable: ", err)
	}
}

func verifyAccountRemovable(ctx context.Context, tconn *chrome.TestConn, a *arc.ARC, d *ui.Device, email string, removable bool) error {
	const (
		removeButtonClassName  = "android.widget.Button"
		removeButtonResourceID = "com.android.settings:id/button"
		removeButtonText       = "(?i)Remove account"

		confirmButtonClassName  = removeButtonClassName
		confirmButtonResourceID = "android:id/button1"
		confirmButtonText       = removeButtonText

		notAllowedClassName  = "android.widget.TextView"
		notAllowedResourceID = "android:id/message"
		notAllowedText       = "(?i).*allowed by your admin"

		settingPkgName = "com.android.settings"
		settingActName = ".Settings"

		defaultUITimeout = 5 * time.Second
	)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 2*time.Second)
	defer cancel()

	act, err := arc.NewActivity(a, settingPkgName, settingActName)
	if err != nil {
		return errors.Wrap(err, "failed to launch Android Settings")
	}
	defer act.Close(cleanupCtx)
	if err := act.StartWithDefaultOptions(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to start activity")
	}
	defer act.Stop(cleanupCtx, tconn)

	if err := accountmanager.CheckIsAccountPresentInARCAction(tconn, d,
		accountmanager.NewARCAccountOptions(email).ExpectedPresentInARC(true))(ctx); err != nil {
		return errors.Wrap(err, "failed to check that account is present in ARC")
	}

	account := d.Object(ui.ClassName("android.widget.TextView"),
		ui.ResourceID("android:id/title"),
		ui.Text(email))
	if err := account.Click(ctx); err != nil {
		return errors.Wrap(err, "failed to open account page for "+email)
	}

	removeButton := d.Object(ui.ClassName(removeButtonClassName),
		ui.ResourceID(removeButtonResourceID),
		ui.TextMatches(removeButtonText))
	if err := removeButton.WaitForExists(ctx, defaultUITimeout); err != nil {
		return errors.Wrap(err, "failed to find remove account option")
	}
	if err := removeButton.Click(ctx); err != nil {
		return errors.Wrap(err, "failed to click remove account option")
	}

	confirmButton := d.Object(ui.ClassName(confirmButtonClassName),
		ui.ResourceID(confirmButtonResourceID),
		ui.TextMatches(confirmButtonText))
	if err := confirmButton.WaitForExists(ctx, defaultUITimeout); err != nil {
		return errors.Wrap(err, "failed to find remove confirmation")
	}
	if err := confirmButton.Click(ctx); err != nil {
		return errors.Wrap(err, "failed to click remove confirmation")
	}

	err = d.Object(ui.ClassName(notAllowedClassName),
		ui.ResourceID(notAllowedResourceID),
		ui.TextMatches(notAllowedText)).WaitForExists(ctx, defaultUITimeout)
	removed := err != nil

	if removed != removable {
		return errors.Errorf("expected account to be removable: %t but it was removed: %t", removable, removed)
	}

	return nil
}
