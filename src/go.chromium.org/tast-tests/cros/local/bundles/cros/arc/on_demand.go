// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/chrome/credconfig"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/optin"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         OnDemand,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests that ARC is started in an on-demand manner",
		Contacts: []string{
			"arcvm-eng@google.com",
			"hashimoto@google.com",
			"yuholong@google.com",
		},
		// ChromeOS > Software > ARC++ > ARCVM
		BugComponent: "b:883059",
		Attr:         []string{"group:mainline", "group:arc-functional", "group:hw_agnostic", "informational"},
		VarDeps:      []string{"arc.managedAccountPool"},
		SoftwareDeps: []string{"chrome", "android_vm"},
		Timeout:      (chrome.ManagedUserLoginTimeout + arc.BootTimeout) * 2,
	})
}

// getChromeOptions returns chrome options to login as a managed user.
func getChromeOptions(creds chrome.Creds, fdms *fakedms.FakeDMS) []chrome.Option {
	return []chrome.Option{
		chrome.GAIALogin(creds),
		chrome.ARCSupported(),
		chrome.UnRestrictARCCPU(),
		chrome.DMSPolicy(fdms.URL),
		chrome.ExtraArgs(append(arc.DisableSyncFlags(), "--enable-features=ArcOnDemand")...),
	}
}

// performInitialBoot performs initial boot and waits for Play Store to get ready.
func performInitialBoot(ctx context.Context, outDir string, creds chrome.Creds, fdms *fakedms.FakeDMS) error {
	cr, err := chrome.New(ctx, getChromeOptions(creds, fdms)...)
	if err != nil {
		return errors.Wrap(err, "failed to connect to Chrome")
	}
	defer cr.Close(ctx)

	a, err := arc.New(ctx, outDir, cr.NormalizedUser())
	if err != nil {
		return errors.Wrap(err, "failed to connect to ARC")
	}
	defer a.Close(ctx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create test connection")
	}

	// Wait for chrome to receive package info from ARC.
	// ARC on Demand depends on the ARC package list held by chrome being fresh.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return tconn.Eval(ctx, `tast.promisify(chrome.autotestPrivate.getArcPackage.bind(this,"com.android.vending"))()`, nil)
	}, &testing.PollOptions{}); err != nil {
		return errors.Wrap(err, "failed to wait for ARC pacakge list refreshed")
	}
	return nil
}

// performRegularBoot performs a regular boot (i.e. logging in as an existing user) and verifies that ARC starts in an on-demand manner.
func performRegularBoot(ctx context.Context, outDir string, creds chrome.Creds, fdms *fakedms.FakeDMS) error {
	cr, err := chrome.New(ctx, append(getChromeOptions(creds, fdms), chrome.KeepState())...)
	if err != nil {
		return errors.Wrap(err, "failed to connect to Chrome")
	}
	defer cr.Close(ctx)

	// With ARC on Demand, ARC shouldn't be running on a regular boot without any app installed.
	var errArcIsNotRunning = errors.New("ARC is not running")
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		result, err := arc.InitExists()
		if err != nil {
			return testing.PollBreak(err)
		}
		if !result {
			return errArcIsNotRunning
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second}); !errors.Is(err, errArcIsNotRunning) {
		return errors.Wrap(err, "ARC is already running when it's not expected to be running")
	}

	// Launch Play Store. This results in starting ARC.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create test connection")
	}
	if err := apps.Launch(ctx, tconn, apps.PlayStore.ID); err != nil {
		return errors.Wrap(err, "failed to launch Play Store")
	}

	// Verify that we can connect to ARC and Play Store is ready.
	a, err := arc.New(ctx, outDir, cr.NormalizedUser())
	if err != nil {
		return errors.Wrap(err, "failed to connect to ARC")
	}
	defer a.Close(ctx)

	if err := optin.WaitForPlayStoreReady(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to wait for Play Store ready")
	}
	return nil
}

func OnDemand(ctx context.Context, s *testing.State) {
	// Run this test as a managed user.
	// ARC on Demand is enabled only for managed users. See go/arc-on-demand-v1.
	creds, err := credconfig.PickRandomCreds(s.RequiredVar("arc.managedAccountPool"))
	if err != nil {
		s.Fatal("Failed to get credentials: ", err)
	}
	fdms, err := policyutil.SetUpFakePolicyServer(ctx, s.OutDir(), creds.User,
		[]policy.Policy{&policy.ArcEnabled{Val: true}})
	if err != nil {
		s.Fatal("Failed to set up policy server: ", err)
	}
	defer fdms.Stop(ctx)

	s.Log("Performing initial boot to prepare for a regular boot")
	if err := performInitialBoot(ctx, s.OutDir(), creds, fdms); err != nil {
		s.Fatal("Initial boot failed: ", err)
	}

	s.Log("Performing regular boot to verify ARC on Demand behavior")
	if err := performRegularBoot(ctx, s.OutDir(), creds, fdms); err != nil {
		s.Fatal("Regular boot failed: ", err)
	}
}
