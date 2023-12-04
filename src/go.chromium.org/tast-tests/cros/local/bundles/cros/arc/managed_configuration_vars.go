// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/common/chrome/credconfig"
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

func init() {
	testing.AddTest(&testing.Test{
		Func:         ManagedConfigurationVars,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that managed configuration variables are replaced for force-installed apps",
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
				ExtraSoftwareDeps: []string{"android_container", "no_qemu"},
			},
			{
				Name:              "betty",
				ExtraSoftwareDeps: []string{"android_container", "qemu"},
				ExtraAttr:         []string{"informational"},
			},
			{
				Name:              "vm",
				ExtraSoftwareDeps: []string{"android_vm", "no_android_vm_t", "no_qemu"},
				ExtraAttr:         []string{"informational"},
			},
			{
				Name:              "x",
				ExtraSoftwareDeps: []string{"android_vm_t", "no_qemu"},
				ExtraAttr:         []string{"informational"},
			},
			{
				Name:              "betty_vm",
				ExtraSoftwareDeps: []string{"android_vm", "qemu"},
				ExtraAttr:         []string{"informational", "group:hw_agnostic"},
			}},
	})
}

func ManagedConfigurationVars(ctx context.Context, s *testing.State) {
	const (
		bootTimeout           = 4 * time.Minute
		testPackage           = "com.google.android.gm"
		appMainActivity       = "com.google.android.gm.ui.MailActivityGmail"
		gotItButtonResourceID = "com.google.android.gm:id/welcome_tour_got_it"
		layoutTitleResourceID = "com.google.android.gm:id/suc_layout_title"
	)

	rl := &retry.Loop{Attempts: 1,
		MaxAttempts: 2,
		DoRetries:   true,
		Fatalf:      s.Fatalf,
		Logf:        s.Logf}

	packages := []string{testPackage}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Minute)
	defer cancel()

	if err := testing.Poll(ctx, func(ctx context.Context) (retErr error) {
		creds, err := credconfig.PickRandomCreds(s.RequiredVar(arcent.LoginPoolVar))
		if err != nil {
			rl.Exit("get login creds", err)
		}
		login := chrome.GAIALogin(creds)

		arcPolicy := arcent.CreateArcPolicyWithApps(packages, arcent.InstallTypeForceInstalled, arcent.PlayStoreModeBlockList)
		arcPolicy.Val.Applications[0].ManagedConfiguration = map[string]interface{}{
			"email_address":     "${USER_EMAIL}",
			"exchange_username": "${USER_EMAIL}",
			"exchange_host":     "outlook.office365.com",
		}
		arcEnabledPolicy := &policy.ArcEnabled{Val: true}
		policies := []policy.Policy{arcEnabledPolicy, arcPolicy}

		fdms, err := policyutil.SetUpFakePolicyServer(ctx, s.OutDir(), creds.User, policies)
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

		installCtx, cancel := context.WithTimeout(ctx, arcent.InstallTimeout)
		defer cancel()
		if err := a.WaitForPackages(installCtx, packages); err != nil {
			return rl.Exit("wait for packages", err)
		}

		d, err := a.NewUIDevice(ctx)
		if err != nil {
			return rl.Exit("initialize UI Automator", err)
		}
		defer d.Close(cleanupCtx)

		act, err := arc.NewActivity(a, testPackage, appMainActivity)
		if err != nil {
			return rl.Exit("create main activity", err)
		}
		if err := act.Start(ctx, tconn); err != nil {
			return rl.Exit("start main activity", err)
		}

		gotItButton := d.Object(ui.ResourceID(gotItButtonResourceID))
		if err := gotItButton.WaitForExists(ctx, 5*time.Second); err != nil {
			return rl.Exit("waiting for 'Got it' button", err)
		}

		if err := gotItButton.Click(ctx); err != nil {
			return rl.Exit("click 'Got it' button", err)
		}

		layoutTitle := d.Object(ui.ResourceID(layoutTitleResourceID))
		if err := layoutTitle.WaitForExists(ctx, 10*time.Second); err != nil {
			return rl.Exit("waiting for login screen", err)
		}

		removeZeroWidthSpace := func(str string) string {
			return strings.ReplaceAll(str, "\u200B", "")
		}

		if layoutTitleText, err := layoutTitle.GetText(ctx); err != nil {
			return rl.Exit("get expected login email", err)
		} else if email := removeZeroWidthSpace(layoutTitleText); email != creds.User {
			return rl.Exit(fmt.Sprintf("match expected login email. Found '%s'", email), nil)
		}

		return nil
	}, nil); err != nil {
		s.Fatal("Managed configuration variables test failed: ", err)
	}
}
