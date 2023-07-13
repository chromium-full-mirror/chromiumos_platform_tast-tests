// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/arcent"
	"go.chromium.org/tast-tests/cros/local/arc/unicorn"
	"go.chromium.org/tast-tests/cros/local/retry"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         UnicornBlockedApps,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks if blocked apps cannot be installed from Child Account",
		Contacts:     []string{"arc-commercial@google.com", "mhasank@chromium.org"},
		// ChromeOS > Software > ARC++ > Commercial
		BugComponent: "b:157100",
		Attr:         []string{"group:mainline", "group:arc-functional", "group:hw_agnostic"},
		SoftwareDeps: []string{
			"chrome",
			"chrome_internal",
			"play_store",
		},
		Timeout: 15 * time.Minute,
		VarDeps: []string{unicorn.ParentUserVar, unicorn.ParentPasswordVar, unicorn.ChildUserVar, unicorn.ChildPasswordVar},
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_container"},
			ExtraAttr:         []string{"informational"},
		}, {
			Name:              "vm",
			ExtraSoftwareDeps: []string{"android_vm"},
			ExtraAttr:         []string{"informational"},
		}},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.ArcEnabled{}, pci.VerifiedFunctionalityOS),
		},
	})
}

func UnicornBlockedApps(ctx context.Context, s *testing.State) {
	const (
		bootTimeout         = 4 * time.Minute
		provisioningTimeout = 3 * time.Minute
		blockedPackage      = "com.google.android.apps.youtube.creator"
	)

	rl := &retry.Loop{Attempts: 1,
		MaxAttempts: 2,
		DoRetries:   true,
		Fatalf:      s.Fatalf,
		Logf:        s.Logf}

	childUser := s.RequiredVar(unicorn.ChildUserVar)
	arcPolicy := arcent.CreateArcPolicyWithApps([]string{blockedPackage}, arcent.InstallTypeBlocked, arcent.PlayStoreModeBlockList)
	arcEnabledPolicy := &policy.ArcEnabled{Val: true}
	policies := []policy.Policy{arcEnabledPolicy, arcPolicy}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Minute)
	defer cancel()

	fdms, err := unicorn.SetUpFakePolicyServer(ctx, s.OutDir(), childUser, policies)
	if err != nil {
		s.Fatal("Failed to setup fake policy server: ", err)
	}
	defer fdms.Stop(cleanupCtx)

	if err := testing.Poll(ctx, func(ctx context.Context) (retErr error) {
		cr, err := unicorn.StartChromeWithARC(ctx,
			childUser,
			s.RequiredVar(unicorn.ChildPasswordVar),
			s.RequiredVar(unicorn.ParentUserVar),
			s.RequiredVar(unicorn.ParentPasswordVar),
			fdms.URL)
		if err != nil {
			return rl.Retry("start Chrome", err)
		}
		defer cr.Close(cleanupCtx)

		tconn, err := cr.TestAPIConn(ctx)
		if err != nil {
			return rl.Retry("create test API Connection", err)
		}

		a, err := arc.NewWithTimeout(ctx, s.OutDir(), bootTimeout)
		if err != nil {
			return rl.Retry("connect to ARC", err)
		}
		defer a.Close(cleanupCtx)

		if err := arcent.ConfigureProvisioningLogs(ctx, a); err != nil {
			return rl.Exit("configure provisioning logs", err)
		}

		if err := a.WaitForProvisioning(ctx, provisioningTimeout); err != nil {
			return rl.Retry("wait for provisioning", err)
		}

		defer a.DumpUIHierarchyOnError(ctx, s.OutDir(), func() bool {
			return s.HasError() || retErr != nil
		})

		defer arcent.DumpBugReportOnError(cleanupCtx, func() bool {
			return s.HasError() || retErr != nil
		}, a, filepath.Join(s.OutDir(), "bugreport.zip"))

		d, err := a.NewUIDevice(ctx)
		if err != nil {
			return rl.Exit("initialize UI Automator", err)
		}
		defer d.Close(cleanupCtx)

		// Blocked app should either not install or immediately uninstall after installation.
		if err := arcent.ValidateBlockedAppInstall(ctx, tconn, a, d, blockedPackage, 5*time.Minute); err != nil {
			return rl.Exit("verify blocked app uninstall", err)
		}

		return nil
	}, nil); err != nil {
		s.Fatal("Failed to verify blocked apps flow: ", err)
	}
}
