// Copyright 2022 The ChromiumOS Authors
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
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/retry"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

const (
	arcEnabled  = true
	arcDisabled = false
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ManagedBoot,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that ARC is booted when policy is set",
		Contacts:     []string{"arc-commercial@google.com", "mhasank@chromium.org"},
		// ChromeOS > Software > ARC++ > Commercial > Tast Tests
		BugComponent: "b:1487630",
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"chrome", "play_store"},
		Timeout:      chrome.LoginTimeout + arc.BootTimeout + 30*time.Second,
		VarDeps: []string{
			arcCommon.ManagedAccountPoolVarName,
		},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.ArcEnabled{}, pci.VerifiedFunctionalityOS),
		},
		Params: []testing.Param{
			{
				Name:              "disabled",
				Val:               arcDisabled,
				ExtraSoftwareDeps: []string{"android_container", "no_qemu"},
			},
			{
				Name:              "disabled_betty",
				Val:               arcDisabled,
				ExtraSoftwareDeps: []string{"android_container", "qemu"},
				ExtraAttr:         []string{"informational"},
			},
			{
				Name:              "disabled_vm",
				Val:               arcDisabled,
				ExtraSoftwareDeps: []string{"android_vm", "no_android_vm_t", "no_qemu"},
			},
			{
				Name:              "disabled_x",
				Val:               arcDisabled,
				ExtraSoftwareDeps: []string{"android_vm_t", "no_qemu"},
				ExtraAttr:         []string{"informational"},
			},
			{
				Name:              "disabled_betty_vm",
				Val:               arcDisabled,
				ExtraAttr:         []string{"informational", "group:hw_agnostic"},
				ExtraSoftwareDeps: []string{"android_vm", "qemu"},
			},
			{
				Name:              "enabled",
				Val:               arcEnabled,
				ExtraSoftwareDeps: []string{"android_container", "no_qemu"},
			},
			{
				Name:              "enabled_betty",
				Val:               arcEnabled,
				ExtraSoftwareDeps: []string{"android_container", "qemu"},
				ExtraAttr:         []string{"informational"},
			},
			{
				Name:              "enabled_vm",
				Val:               arcEnabled,
				ExtraSoftwareDeps: []string{"android_vm", "no_android_vm_t", "no_qemu"},
				ExtraAttr:         []string{"informational"},
			},
			{
				Name:              "enabled_x",
				Val:               arcEnabled,
				ExtraSoftwareDeps: []string{"android_vm_t", "no_qemu"},
				ExtraAttr:         []string{"informational"},
			},
			{
				Name:              "enabled_betty_vm",
				Val:               arcEnabled,
				ExtraSoftwareDeps: []string{"android_vm", "qemu"},
				ExtraAttr:         []string{"informational", "group:hw_agnostic"},
			}},
	})
}

// ManagedBoot verifies that ARC boots when enabled in policy and does not boot when disabled in policy.
func ManagedBoot(ctx context.Context, s *testing.State) {
	expectEnabled := s.Param().(bool)

	rl := &retry.Loop{Attempts: 1,
		MaxAttempts: 2,
		DoRetries:   true,
		Errorf:      s.Errorf,
		Logf:        s.Logf}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Minute)
	defer cancel()

	if err := testing.Poll(ctx, func(ctx context.Context) (retErr error) {
		creds, err := credconfig.PickRandomCreds(dma.CredsFromPool(arcCommon.ManagedAccountPoolVarName))
		if err != nil {
			return rl.Exit("get login creds", err)
		}
		login := chrome.GAIALogin(creds)

		policies := []policy.Policy{&policy.ArcEnabled{Val: expectEnabled}}
		fdms, err := policyutil.SetUpFakePolicyServer(ctx, s.OutDir(), creds.User, policies)
		if err != nil {
			return rl.Exit("setup fake policy server", err)
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
			return rl.Exit("create test API Connection", err)
		}

		// Ensure chrome://policy shows correct ArcEnabled value.
		if err := policyutil.Verify(ctx, tconn, []policy.Policy{&policy.ArcEnabled{Val: expectEnabled}}); err != nil {
			return rl.Exit("verify ArcEnabled", err)
		}

		// Wait for ARC to boot. It should succeed only if enabled by policy.
		a, err := arc.New(ctx, s.OutDir(), cr.NormalizedUser())
		if err == nil {
			defer a.Close(ctx)
			if !expectEnabled {
				s.Fatal("Started ARC while blocked by user policy")
			}
		} else if expectEnabled {
			return rl.Exit("start ARC by user policy", err)
		}

		return nil
	}, nil); err != nil {
		s.Fatal("Managed boot test failed: ", err)
	}
}
