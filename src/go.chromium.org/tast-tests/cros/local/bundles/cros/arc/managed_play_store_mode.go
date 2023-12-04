// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

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

type managedPlayStoreModeArgs struct {
	playStoreMode string
	shouldBeEmpty bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ManagedPlayStoreMode,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that apps are shown/hidden according to playStoreMode",
		Contacts:     []string{"arc-commercial@google.com", "mhasank@chromium.org"},
		// ChromeOS > Software > ARC++ > Commercial
		BugComponent: "b:157100",
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"chrome", "play_store"},
		Timeout:      15 * time.Minute,
		VarDeps: []string{
			arcent.LoginPoolVar,
		},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.ArcEnabled{}, pci.VerifiedFunctionalityOS),
		},
		Params: []testing.Param{
			{
				Name: "allowlist",
				Val: managedPlayStoreModeArgs{
					playStoreMode: arcent.PlayStoreModeAllowList,
					shouldBeEmpty: true,
				},
				ExtraSoftwareDeps: []string{"android_container", "no_qemu"},
			},
			{
				Name: "allowlist_vm",
				Val: managedPlayStoreModeArgs{
					playStoreMode: arcent.PlayStoreModeAllowList,
					shouldBeEmpty: true,
				},
				ExtraSoftwareDeps: []string{"android_vm", "no_android_vm_t", "no_qemu"},
				ExtraAttr:         []string{"informational"},
			},
			{
				Name: "allowlist_x",
				Val: managedPlayStoreModeArgs{
					playStoreMode: arcent.PlayStoreModeAllowList,
					shouldBeEmpty: true,
				},
				ExtraSoftwareDeps: []string{"android_vm_t", "no_qemu"},
				ExtraAttr:         []string{"informational"},
			},
			{
				Name: "allowlist_betty",
				Val: managedPlayStoreModeArgs{
					playStoreMode: arcent.PlayStoreModeAllowList,
					shouldBeEmpty: true,
				},
				ExtraSoftwareDeps: []string{"android_container", "qemu"},
				ExtraAttr:         []string{"informational"},
			},
			{
				Name: "allowlist_vm_betty",
				Val: managedPlayStoreModeArgs{
					playStoreMode: arcent.PlayStoreModeAllowList,
					shouldBeEmpty: true,
				},
				ExtraSoftwareDeps: []string{"android_vm", "qemu"},
				ExtraAttr:         []string{"informational", "group:hw_agnostic"},
			},
			{
				Name: "blocklist",
				Val: managedPlayStoreModeArgs{
					playStoreMode: arcent.PlayStoreModeBlockList,
					shouldBeEmpty: false,
				},
				ExtraSoftwareDeps: []string{"android_container", "no_qemu"},
			},
			{
				Name: "blocklist_vm",
				Val: managedPlayStoreModeArgs{
					playStoreMode: arcent.PlayStoreModeBlockList,
					shouldBeEmpty: false,
				},
				ExtraSoftwareDeps: []string{"android_vm", "no_android_vm_t", "no_qemu"},
				ExtraAttr:         []string{"informational"},
			},
			{
				Name: "blocklist_x",
				Val: managedPlayStoreModeArgs{
					playStoreMode: arcent.PlayStoreModeBlockList,
					shouldBeEmpty: false,
				},
				ExtraSoftwareDeps: []string{"android_vm_t", "no_qemu"},
				ExtraAttr:         []string{"informational"},
			},
			{
				Name: "blocklist_betty",
				Val: managedPlayStoreModeArgs{
					playStoreMode: arcent.PlayStoreModeBlockList,
					shouldBeEmpty: false,
				},
				ExtraSoftwareDeps: []string{"android_container", "qemu"},
				ExtraAttr:         []string{"informational"},
			},
			{
				Name: "blocklist_vm_betty",
				Val: managedPlayStoreModeArgs{
					playStoreMode: arcent.PlayStoreModeBlockList,
					shouldBeEmpty: false,
				},
				ExtraSoftwareDeps: []string{"android_vm", "qemu"},
				ExtraAttr:         []string{"informational", "group:hw_agnostic"},
			}},
	})
}

// ManagedPlayStoreMode verifies that apps are shown/hidden according to playStoreMode.
func ManagedPlayStoreMode(ctx context.Context, s *testing.State) {
	const (
		bootTimeout      = 4 * time.Minute
		defaultUITimeout = 1 * time.Minute
	)

	args := s.Param().(managedPlayStoreModeArgs)

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

		if err := arcent.ConfigureProvisioningLogs(ctx, a); err != nil {
			return rl.Exit("configure provisioning logs", err)
		}

		defer arcent.DumpBugReportOnError(cleanupCtx, func() bool {
			return s.HasError() || retErr != nil
		}, a, filepath.Join(s.OutDir(), fmt.Sprintf("bugreport_%d.zip", rl.Attempts)))

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

		if err := arcent.EnsurePlayStoreState(ctx, tconn, cr, a, d, s.OutDir(), rl.Attempts, args.shouldBeEmpty); err != nil {
			return rl.Exit("verify Play Store state", err)
		}

		return nil
	}, nil); err != nil {
		s.Fatal("Play Store mode test failed: ", err)
	}
}
