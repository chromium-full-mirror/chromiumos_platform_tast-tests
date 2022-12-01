// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"path/filepath"
	"time"

	"chromiumos/tast/common/android/ui"
	"chromiumos/tast/common/policy"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/arc"
	"chromiumos/tast/local/arc/arcent"
	"chromiumos/tast/local/arc/unicorn"
	"chromiumos/tast/local/retry"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         UnicornBlockedApps,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks if blocked apps cannot be installed from Child Account",
		Contacts:     []string{"arc-commercial@google.com", "mhasank@chromium.org"},
		// ChromeOS > Software > ARC++ > Commercial
		BugComponent: "b:157100",
		Attr:         []string{"group:mainline", "informational", "group:arc-functional", "group:hw_agnostic"},
		SoftwareDeps: []string{
			"chrome",
			"chrome_internal",
			"play_store",
		},
		Timeout: 7 * time.Minute,
		VarDeps: []string{unicorn.ParentUserVar, unicorn.ParentPasswordVar, unicorn.ChildUserVar, unicorn.ChildPasswordVar},
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_p"},
		}, {
			Name:              "vm",
			ExtraSoftwareDeps: []string{"android_vm"},
		}},
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
	arcPolicy := arcent.CreateArcPolicyWithApps([]string{blockedPackage}, arcent.InstallTypeBlocked)
	arcPolicy.Val.PlayStoreMode = arcent.PlayStoreModeBlockList
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

		d, err := a.NewUIDevice(ctx)
		if err != nil {
			return rl.Exit("initialize UI Automator", err)
		}
		defer d.Close(cleanupCtx)

		defer arcent.DumpBugReportOnError(cleanupCtx, func() bool {
			return retErr != nil
		}, a, filepath.Join(s.OutDir(), "bugreport.zip"))

		if err := arcent.PollAppPageState(ctx, tconn, a, blockedPackage, func(ctx context.Context) error {
			installButton, err := arcent.WaitForInstallButton(ctx, d)
			if err != nil {
				return errors.Wrap(err, "failed to find the install button")
			}

			enabled, err := installButton.IsEnabled(ctx)
			if err != nil {
				return errors.Wrap(err, "failed to check the install button state")
			}

			if !enabled {
				testing.ContextLog(ctx, "Install button is disabled")
				return nil
			}

			if err := validateAutoUninstall(ctx, a, installButton, blockedPackage); err != nil {
				testing.PollBreak(err)
			}

			testing.ContextLog(ctx, "Blocked app uninstalled")
			return nil
		}, time.Minute); err != nil {
			return rl.Exit("verify blocked app uninstall", err)
		}

		return nil
	}, nil); err != nil {
		s.Fatal("Failed to verify blocked apps flow: ", err)
	}
}

func validateAutoUninstall(ctx context.Context, a *arc.ARC, installButton *ui.Object, blockedPackage string) error {
	testing.ContextLog(ctx, "Install button is enabled. Attempting install")
	if err := installButton.Click(ctx); err != nil {
		return errors.Wrap(err, "failed to click the install button")
	}

	if err := a.WaitForPackages(ctx, []string{blockedPackage}); err != nil {
		return errors.Wrap(err, "package installation failed")
	}

	testing.ContextLog(ctx, "Waiting for package to uninstall")
	if err := arcent.WaitForUninstall(ctx, a, blockedPackage); err != nil {
		return errors.Wrap(err, "package not uninstalled")
	}

	return nil
}
