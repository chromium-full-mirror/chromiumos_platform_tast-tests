// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/arcent"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/retry"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

const (
	managed3pEmmAccountVar = "arc.managed3pEmmAccount"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ManagedPlayStore3pEmm,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that Play Store mode is loaded correctly for 3p EMM customer",
		Contacts:     []string{"arc-commercial@google.com", "mhasank@chromium.org"},
		// ChromeOS > Software > ARC++ > Commercial > Tast Tests
		BugComponent: "b:1487630",
		Attr:         []string{"group:mainline", "group:arc-functional"},
		SoftwareDeps: []string{"chrome", "play_store"},
		Timeout:      15 * time.Minute,
		VarDeps:      []string{managed3pEmmAccountVar},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.ArcEnabled{}, pci.VerifiedFunctionalityOS),
		},
		Params: []testing.Param{
			{
				ExtraSoftwareDeps: []string{"android_container", "no_qemu"},
			},
			{
				Name:              "vm",
				ExtraAttr:         []string{"informational"},
				ExtraSoftwareDeps: []string{"android_vm", "no_android_vm_t", "no_qemu"},
			},
			{
				Name:              "x",
				ExtraSoftwareDeps: []string{"android_vm_t", "no_qemu"},
				ExtraAttr:         []string{"informational"},
			}},
	})
}

// ManagedPlayStore3pEmm verifies that Play Store mode is loaded correctly for 3p EMM customer.
func ManagedPlayStore3pEmm(ctx context.Context, s *testing.State) {
	const (
		bootTimeout      = 4 * time.Minute
		defaultUITimeout = 1 * time.Minute
	)

	rl := &retry.Loop{Attempts: 1,
		MaxAttempts: 2,
		DoRetries:   true,
		Fatalf:      s.Fatalf,
		Logf:        s.Logf}

	arcPolicy := arcent.CreateArcPolicyWithApps([]string{}, arcent.InstallTypeAvailable, arcent.PlayStoreModeBlockList)
	arcPolicy.Val.DpsInteractionsDisabled = true
	arcEnabledPolicy := &policy.ArcEnabled{Val: true}
	policies := []policy.Policy{arcEnabledPolicy, arcPolicy}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Minute)
	defer cancel()

	managed3pEmmAccount := strings.Split(s.RequiredVar(managed3pEmmAccountVar), ":")
	login := chrome.GAIALogin(chrome.Creds{
		User: managed3pEmmAccount[0],
		Pass: managed3pEmmAccount[1],
	})

	if err := testing.Poll(ctx, func(ctx context.Context) (retErr error) {
		fdms, err := policyutil.SetUpFakePolicyServer(ctx, s.OutDir(), managed3pEmmAccount[0], policies)
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

		d, err := a.NewUIDevice(ctx)
		if err != nil {
			return rl.Exit("initialize UI Automator", err)
		}
		defer d.Close(cleanupCtx)

		if err := arcent.EnsurePlayStoreNotEmpty(ctx, tconn, cr, a, d, s.OutDir(), rl.Attempts); err != nil {
			return rl.Exit("verify Play Store is not empty", err)
		}

		return nil
	}, nil); err != nil {
		s.Fatal("Play Store 3p EMM test failed: ", err)
	}
}
