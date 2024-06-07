// Copyright 2021 The ChromiumOS Authors
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
	"go.chromium.org/tast-tests/cros/local/arc/arcent"
	"go.chromium.org/tast-tests/cros/local/arc/optin"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/retry"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ManagedProvisioning,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "A functional test that verifies provisioning flow for managed user",
		Contacts: []string{
			"arc-commercial@google.com",
			"mhasank@chromium.org",
			"yaohuali@google.com",
		},
		// ChromeOS > Software > ARC++ > Commercial > Tast Tests
		BugComponent: "b:1487630",
		Attr:         []string{"group:mainline", "group:arc-functional"},
		VarDeps: []string{
			arcCommon.ManagedAccountPoolVarName,
		},
		SoftwareDeps: []string{
			"chrome",
			"chrome_internal",
			"play_store",
			"gaia",
		},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.ArcEnabled{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.ArcPolicy{}, pci.VerifiedFunctionalityOS),
		},
		Params: []testing.Param{
			{
				Val:               chrome.FieldTrialConfigDefault,
				ExtraSoftwareDeps: []string{"android_container"},
			},
			{
				Name:              "fieldtrial_testing_config_off",
				Val:               chrome.FieldTrialConfigDisable,
				ExtraSoftwareDeps: []string{"android_container"},
				ExtraAttr:         []string{"informational"},
			},
			{
				Name:              "fieldtrial_testing_config_on",
				Val:               chrome.FieldTrialConfigEnable,
				ExtraSoftwareDeps: []string{"android_container"},
				ExtraAttr:         []string{"informational", "group:chrome_uprev_cbx"},
			},
			{
				Name:              "vm",
				Val:               chrome.FieldTrialConfigDefault,
				ExtraSoftwareDeps: []string{"android_vm", "no_qemu"},
			},
			{
				Name:              "fieldtrial_testing_config_off_vm",
				Val:               chrome.FieldTrialConfigDisable,
				ExtraSoftwareDeps: []string{"android_vm", "no_qemu"},
				ExtraAttr:         []string{"informational"},
			},
			{
				Name:              "fieldtrial_testing_config_on_vm",
				Val:               chrome.FieldTrialConfigEnable,
				ExtraSoftwareDeps: []string{"android_vm", "no_qemu"},
				ExtraAttr:         []string{"informational", "group:chrome_uprev_cbx"},
			},
			{
				Name:              "betty_vm",
				Val:               chrome.FieldTrialConfigDefault,
				ExtraSoftwareDeps: []string{"android_vm", "qemu"},
				ExtraAttr:         []string{"informational", "group:hw_agnostic"},
			}},
		Timeout: 7 * time.Minute,
	})
}

// ManagedProvisioning verifies that ARC can successfully provision with a managed account.
func ManagedProvisioning(ctx context.Context, s *testing.State) {
	const (
		bootTimeout         = 4 * time.Minute
		provisioningTimeout = 3 * time.Minute
	)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Minute)
	defer cancel()

	rl := &retry.Loop{Attempts: 1,
		MaxAttempts: 2,
		DoRetries:   true,
		Errorf:      s.Errorf,
		Logf:        s.Logf}

	arcPolicy := &policy.ArcPolicy{
		Val: &policy.ArcPolicyValue{
			Applications:            []policy.Application{},
			PlayStoreMode:           arcent.PlayStoreModeAllowList,
			DpsInteractionsDisabled: true,
		},
	}
	arcEnabledPolicy := &policy.ArcEnabled{Val: true, Stat: policy.StatusSet}
	policies := []policy.Policy{arcEnabledPolicy, arcPolicy}

	if err := testing.Poll(ctx, func(ctx context.Context) (retErr error) {
		creds, err := credconfig.PickRandomCreds(dma.CredsFromPool(arcCommon.ManagedAccountPoolVarName))
		if err != nil {
			return rl.Exit("get login creds", err)
		}

		fdms, err := policyutil.SetUpFakePolicyServer(ctx, s.OutDir(), creds.User, policies)
		if err != nil {
			return rl.Exit("setup fake policy server", err)
		}
		defer fdms.Stop(cleanupCtx)

		cr, err := chrome.New(ctx,
			chrome.GAIALogin(creds),
			chrome.DMSPolicy(fdms.URL),
			chrome.ARCSupported(),
			chrome.UnRestrictARCCPU(),
			chrome.FieldTrialConfig(s.Param().(chrome.FieldTrialConfigMode)),
			chrome.ExtraArgs(arc.DisableSyncFlags()...))
		if err != nil {
			return rl.Retry("connect to Chrome", err)
		}
		defer cr.Close(cleanupCtx)

		s.Log("Waiting for managed provisioning")

		a, err := arc.NewWithTimeout(ctx, s.OutDir(), bootTimeout, cr.NormalizedUser())
		if err != nil {
			return rl.Retry("start ARC by policy", err)
		}
		defer a.Close(cleanupCtx)

		if err := a.WaitForProvisioning(ctx, provisioningTimeout); err != nil {
			if err := optin.DumpLogCat(cleanupCtx, ""); err != nil {
				s.Logf("WARNING: Failed to dump logcat: %s", err)
			}
			return rl.Exit("wait for provisioning", err)
		}

		return nil
	}, nil); err != nil {
		s.Fatal("Managed provisioning failed: ", err)
	}
}
