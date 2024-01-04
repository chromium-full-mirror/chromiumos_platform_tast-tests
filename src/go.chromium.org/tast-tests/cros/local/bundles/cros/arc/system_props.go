// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/chrome/credconfig"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/arcent"
	"go.chromium.org/tast-tests/cros/local/arc/optin"
	"go.chromium.org/tast-tests/cros/local/chrome"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type systemPropsTestArgs struct {
	accountPool string
	optin       bool
}

const systemPropsVar = "arc.SystemProps.Props"

func init() {
	testing.AddTest(&testing.Test{
		Func:         SystemProps,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that all ARC system props are set",
		Contacts:     []string{"arc-commercial@google.com", "mhasank@chromium.org"},
		// ChromeOS > Software > ARC++ > Commercial > Tast Tests
		BugComponent: "b:1487630",
		Attr:         []string{"group:mainline", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome", "play_store", "android_vm"},
		Timeout:      15 * time.Minute,
		VarDeps: []string{
			arcent.LoginPoolVar,
			optin.LoginPoolVar,
			systemPropsVar,
		},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.ArcEnabled{}, pci.VerifiedFunctionalityOS),
		},
		Params: []testing.Param{
			{
				Name: "managed_vm",
				Val: systemPropsTestArgs{
					accountPool: arcent.LoginPoolVar,
					optin:       false,
				},
				ExtraAttr: []string{"informational"},
			},
			{
				Name: "unmanaged_vm",
				Val: systemPropsTestArgs{
					accountPool: optin.LoginPoolVar,
					optin:       true,
				},
				ExtraAttr: []string{"informational"},
			}},
	})
}

func SystemProps(ctx context.Context, s *testing.State) {
	const (
		bootTimeout      = 4 * time.Minute
		defaultUITimeout = 1 * time.Minute
	)

	args := s.Param().(systemPropsTestArgs)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	primary, err := credconfig.PickRandomCreds(s.RequiredVar(args.accountPool))
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

	systemPropsText := s.RequiredVar(systemPropsVar)
	for _, systemProp := range strings.Split(systemPropsText, "\n") {
		systemProp = strings.TrimSpace(systemProp)
		if val, err := a.GetProp(ctx, systemProp); err != nil {
			s.Fatal("Failed to read system prop "+systemProp, err)
		} else if len(val) == 0 {
			s.Errorf("System property %q is not set", systemProp)
		}
	}
}
