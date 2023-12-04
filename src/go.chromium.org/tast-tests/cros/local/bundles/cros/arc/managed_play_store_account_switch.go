// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/common/chrome/credconfig"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/arcent"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/retry"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type managedPlayStoreAccountSwitchArgs struct {
	playStoreMode        string
	accountSwitchEnabled bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ManagedPlayStoreAccountSwitch,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that we can switch to secondary account in Play Store only when mode is block list",
		Contacts:     []string{"arc-commercial@google.com", "mhasank@chromium.org"},
		// ChromeOS > Software > ARC++ > Commercial
		BugComponent: "b:157100",
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"chrome", "play_store"},
		Timeout:      15 * time.Minute,
		VarDeps: []string{
			arcent.LoginPoolVar,
			"ui.gaiaPoolDefault",
		},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.ArcEnabled{}, pci.VerifiedFunctionalityOS),
		},
		Params: []testing.Param{
			{
				Name: "blocklist",
				Val: managedPlayStoreAccountSwitchArgs{
					playStoreMode:        arcent.PlayStoreModeBlockList,
					accountSwitchEnabled: true,
				},
				ExtraSoftwareDeps: []string{"android_container", "no_qemu"},
				ExtraAttr:         []string{"informational", "group:criticalstaging"},
			},
			{
				Name: "blocklist_betty",
				Val: managedPlayStoreAccountSwitchArgs{
					playStoreMode:        arcent.PlayStoreModeBlockList,
					accountSwitchEnabled: true,
				},
				ExtraSoftwareDeps: []string{"android_container", "qemu"},
				ExtraAttr:         []string{"informational"},
			},
			{
				Name: "blocklist_vm",
				Val: managedPlayStoreAccountSwitchArgs{
					playStoreMode:        arcent.PlayStoreModeBlockList,
					accountSwitchEnabled: true,
				},
				ExtraSoftwareDeps: []string{"android_vm", "no_android_vm_t", "no_qemu"},
				ExtraAttr:         []string{"informational"},
			},
			{
				Name: "blocklist_x",
				Val: managedPlayStoreAccountSwitchArgs{
					playStoreMode:        arcent.PlayStoreModeBlockList,
					accountSwitchEnabled: true,
				},
				ExtraSoftwareDeps: []string{"android_vm_t", "no_qemu"},
				ExtraAttr:         []string{"informational"},
			},
			{
				Name: "blocklist_betty_vm",
				Val: managedPlayStoreAccountSwitchArgs{
					playStoreMode:        arcent.PlayStoreModeBlockList,
					accountSwitchEnabled: true,
				},
				ExtraSoftwareDeps: []string{"android_vm", "qemu"},
				ExtraAttr:         []string{"informational", "group:hw_agnostic"},
			},
			{
				Name: "allowlist",
				Val: managedPlayStoreAccountSwitchArgs{
					playStoreMode:        arcent.PlayStoreModeAllowList,
					accountSwitchEnabled: false,
				},
				ExtraSoftwareDeps: []string{"android_container", "no_qemu"},
				ExtraAttr:         []string{"informational"},
			},
			{
				Name: "allowlist_betty",
				Val: managedPlayStoreAccountSwitchArgs{
					playStoreMode:        arcent.PlayStoreModeAllowList,
					accountSwitchEnabled: false,
				},
				ExtraSoftwareDeps: []string{"android_container", "qemu"},
				ExtraAttr:         []string{"informational"},
			},
			{
				Name: "allowlist_vm",
				Val: managedPlayStoreAccountSwitchArgs{
					playStoreMode:        arcent.PlayStoreModeAllowList,
					accountSwitchEnabled: false,
				},
				ExtraSoftwareDeps: []string{"android_vm", "no_android_vm_t", "no_qemu"},
				ExtraAttr:         []string{"informational"},
			},
			{
				Name: "allowlist_x",
				Val: managedPlayStoreAccountSwitchArgs{
					playStoreMode:        arcent.PlayStoreModeAllowList,
					accountSwitchEnabled: false,
				},
				ExtraSoftwareDeps: []string{"android_vm_t", "no_qemu"},
				ExtraAttr:         []string{"informational"},
			},
			{
				Name: "allowlist_betty_vm",
				Val: managedPlayStoreAccountSwitchArgs{
					playStoreMode:        arcent.PlayStoreModeAllowList,
					accountSwitchEnabled: false,
				},
				ExtraSoftwareDeps: []string{"android_vm", "qemu"},
				ExtraAttr:         []string{"informational", "group:hw_agnostic"},
			}},
	})
}

// ManagedPlayStoreAccountSwitch verifies that we can switch to secondary account in Play Store only when mode is block list.
func ManagedPlayStoreAccountSwitch(ctx context.Context, s *testing.State) {
	const (
		bootTimeout      = 4 * time.Minute
		defaultUITimeout = 1 * time.Minute
	)

	args := s.Param().(managedPlayStoreAccountSwitchArgs)

	rl := &retry.Loop{Attempts: 1,
		MaxAttempts: 2,
		DoRetries:   true,
		Fatalf:      s.Fatalf,
		Logf:        s.Logf}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Minute)
	defer cancel()

	if err := testing.Poll(ctx, func(ctx context.Context) (retErr error) {
		creds, err := credconfig.PickRandomCreds(s.RequiredVar(arcent.LoginPoolVar))
		if err != nil {
			rl.Exit("get login creds", err)
		}
		login := chrome.GAIALogin(creds)

		fdms, err := arcent.SetupPolicyServerWithArcApps(ctx, s.OutDir(), creds.User, []string{}, arcent.InstallTypeAvailable, args.playStoreMode)
		if err != nil {
			rl.Exit("setup fake policy server", err)
		}
		defer fdms.Stop(cleanupCtx)

		cr, err := chrome.New(
			ctx,
			login,
			chrome.ARCSupported(),
			chrome.UnRestrictARCCPU(),
			chrome.DMSPolicy(fdms.URL),
			chrome.ExtraArgs(arc.DisableSyncFlags()...))
		if err != nil {
			return rl.Retry("connect to Chrome", err)
		}
		defer cr.Close(cleanupCtx)

		tconn, err := cr.TestAPIConn(ctx)
		if err != nil {
			return rl.Retry("create test API connection", err)
		}

		a, err := arc.NewWithTimeout(ctx, s.OutDir(), bootTimeout, cr.NormalizedUser())
		if err != nil {
			return rl.Retry("start ARC by policy", err)
		}
		defer a.Close(cleanupCtx)

		if err := arcent.WaitForProvisioning(ctx, a, rl.Attempts); err != nil {
			return rl.Retry("wait for provisioning", err)
		}

		defer a.DumpUIHierarchyOnError(cleanupCtx, s.OutDir(), func() bool {
			return s.HasError() || retErr != nil
		})

		d, err := a.NewUIDevice(ctx)
		if err != nil {
			return rl.Exit("initialize UI Automator", err)
		}
		defer d.Close(cleanupCtx)

		secondaryUser, err := credconfig.PickRandomCreds(s.RequiredVar("ui.gaiaPoolDefault"))
		if err != nil {
			rl.Exit("get secondary user creds", err)
		}

		if err := arcent.AddSecondaryAccount(ctx, tconn, cr, d, secondaryUser.User, secondaryUser.Pass); err != nil {
			rl.Exit("add secondary account", err)
		}

		if args.accountSwitchEnabled {
			if err := arc.SwitchPlayStoreAccount(ctx, d, tconn, secondaryUser.User); err != nil {
				return rl.Exit("switch to secondary account in Play Store", err)
			}
			return nil
		}

		if err := arc.OpenPlayStoreAccountSettings(ctx, d, tconn); err != nil {
			return rl.Exit("open account settings", err)
		}
		accountNameButton := d.Object(ui.ClassName("android.widget.TextView"), ui.Text(secondaryUser.User))
		if err := accountNameButton.WaitUntilGone(ctx, 30*time.Second); err != nil {
			return rl.Exit("ensure that secondary account is not shown in Play Store", err)
		}

		return nil
	}, nil); err != nil {
		s.Fatal("Play Store mode account switch test failed: ", err)
	}
}
