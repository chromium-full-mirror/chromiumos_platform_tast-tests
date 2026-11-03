// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	arcCommon "go.chromium.org/tast-tests/cros/common/arc"
	"go.chromium.org/tast-tests/cros/common/chrome/credconfig"
	"go.chromium.org/tast-tests/cros/common/dma"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/arcent"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/retry"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ManagedAutoLaunchApps,
		Desc:         "Checks that the Android app configured by the AutoLaunchAndroidApps policy is launched automatically after sign-in",
		Contacts:     []string{"arc-commercial@google.com", "fahadmansoor@google.com"},
		BugComponent: "b:1487630", // ChromeOS > Software > ARC++ > Commercial > Tast Tests
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome", "play_store", "gaia"},
		Timeout:      15 * time.Minute,
		VarDeps: []string{
			arcCommon.ManagedAccountPoolVarName,
		},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.AutoLaunchAndroidApps{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.ArcEnabled{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.ArcPolicy{}, pci.VerifiedFunctionalityOS),
			// Only set so that ARC starts without the Terms of Service step.
			pci.SearchFlag(&policy.ArcBackupRestoreServiceEnabled{}, pci.Served),
			pci.SearchFlag(&policy.ArcGoogleLocationServicesEnabled{}, pci.Served),
			pci.SearchFlag(&policy.GoogleLocationServicesEnabled{}, pci.Served),
		},
		Params: []testing.Param{
			{
				Name:              "vm",
				ExtraSoftwareDeps: []string{"android_vm", "no_qemu"},
			},
			{
				Name:              "betty_vm",
				ExtraSoftwareDeps: []string{"android_vm", "qemu"},
				ExtraAttr:         []string{"group:hw_agnostic"},
			},
		},
	})
}

// ManagedAutoLaunchApps logs in with a managed account whose policy enables
// ARC, force-installs an app through ArcPolicy and configures the same app in
// AutoLaunchAndroidApps. It then checks that the app is launched automatically
// once it is installed, without launching it twice.
func ManagedAutoLaunchApps(ctx context.Context, s *testing.State) {
	// Keep is not preinstalled, so the app is installed after sign-in and the
	// auto-launch has to wait for it. Play allows installing it for the test
	// accounts.
	const testPackage = "com.google.android.keep"

	// Signing in and starting ARC can fail for reasons unrelated to
	// auto-launch, so the test is retried once in that case.
	rl := &retry.Loop{
		Attempts:    1,
		MaxAttempts: 2,
		DoRetries:   true,
		Errorf:      s.Errorf,
		Logf:        s.Logf,
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Minute)
	defer cancel()

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return runAutoLaunchAttempt(ctx, cleanupCtx, rl, s.OutDir(), s.HasError, testPackage)
	}, nil); err != nil {
		s.Fatal("Auto-launch test failed: ", err)
	}
}

// runAutoLaunchAttempt signs in with a managed account whose policy
// auto-launches pkg, waits for pkg to be installed and checks that it is
// launched once. Failures are reported through rl, which decides whether the
// attempt is retried.
func runAutoLaunchAttempt(ctx, cleanupCtx context.Context, rl *retry.Loop, outDir string, hasError func() bool, pkg string) (retErr error) {
	const arcBootTimeout = 4 * time.Minute

	creds, err := credconfig.PickRandomCreds(dma.CredsFromPool(arcCommon.ManagedAccountPoolVarName))
	if err != nil {
		return rl.Exit("get login creds", err)
	}

	fdms, err := arcent.SetUpFakePolicyServer(ctx, outDir, creds.User, autoLaunchPolicies(pkg), false /*affiliated*/)
	if err != nil {
		return rl.Exit("set up fake policy server", err)
	}
	defer fdms.Stop(cleanupCtx)

	cr, err := chrome.New(
		ctx,
		chrome.GAIALogin(creds),
		chrome.ARCSupported(),
		chrome.UnRestrictARCCPU(),
		chrome.DMSPolicy(fdms.URL),
		chrome.ExtraArgs(arc.DisableSyncFlags()...),
	)
	if err != nil {
		return rl.Retry("connect to Chrome", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return rl.Retry("create test API connection", err)
	}

	if err := policyutil.Verify(ctx, tconn, []policy.Policy{
		&policy.ArcEnabled{Val: true},
		&policy.AutoLaunchAndroidApps{Val: []string{pkg}},
	}); err != nil {
		return rl.Exit("verify policies", err)
	}

	a, err := arc.NewWithTimeout(ctx, outDir, arcBootTimeout, cr.NormalizedUser())
	if err != nil {
		return rl.Retry("start ARC by policy", err)
	}
	defer a.Close(cleanupCtx)

	if err := arcent.ConfigureProvisioningLogs(ctx, a); err != nil {
		return rl.Exit("configure provisioning logs", err)
	}
	if err := arcent.WaitForProvisioning(ctx, a, rl.Attempts); err != nil {
		return rl.Retry("wait for provisioning", err)
	}

	defer arcent.DumpBugReportOnError(cleanupCtx, func() bool {
		return hasError() || retErr != nil
	}, a, filepath.Join(outDir, fmt.Sprintf("bugreport_%d.zip", rl.Attempts)))

	if err := waitForAppInstall(ctx, a, pkg); err != nil {
		return rl.Exit("wait for the app to be installed", err)
	}
	if err := verifyAppLaunchedOnce(ctx, tconn, outDir, pkg); err != nil {
		return rl.Exit("verify that the app is auto-launched once", err)
	}
	return nil
}

// autoLaunchPolicies returns the user policies that enable ARC, force-install
// pkg and auto-launch it. Backup & restore and location services are disabled
// so that ARC starts without asking the user to accept the Terms of Service,
// also when the user signs in through OOBE. The policies have to be served
// together, since setting a policy blob replaces the previous one.
func autoLaunchPolicies(pkg string) []policy.Policy {
	packages := []string{pkg}
	return []policy.Policy{
		&policy.ArcEnabled{Val: true},
		arcent.CreateArcPolicyWithApps(packages, arcent.InstallTypeForceInstalled, arcent.PlayStoreModeAllowList),
		&policy.ArcBackupRestoreServiceEnabled{Val: 0},
		&policy.ArcGoogleLocationServicesEnabled{Val: 0},
		&policy.GoogleLocationServicesEnabled{Val: 0},
		&policy.AutoLaunchAndroidApps{Val: packages},
	}
}

// waitForAppInstall waits for CloudDPC to install pkg after sign-in.
func waitForAppInstall(ctx context.Context, a *arc.ARC, pkg string) error {
	testing.ContextLog(ctx, "Waiting for the app to be installed: ", pkg)
	ctx, cancel := context.WithTimeout(ctx, arcent.InstallTimeout)
	defer cancel()
	return a.WaitForPackages(ctx, []string{pkg})
}

// verifyAppLaunchedOnce waits for the window of pkg to be shown, and checks
// that pkg has only been launched once.
func verifyAppLaunchedOnce(ctx context.Context, tconn *chrome.TestConn, outDir, pkg string) error {
	const (
		launchTimeout = 2 * time.Minute
		// How long to wait for a second launch once the window is shown. A
		// second launch would most likely be triggered by the same app updates
		// as the first one, so it would show up shortly after it.
		relaunchWaitTime = 10 * time.Second
	)

	testing.ContextLog(ctx, "Waiting for the auto-launched app window: ", pkg)
	windowCtx, cancel := context.WithTimeout(ctx, launchTimeout)
	defer cancel()
	if err := ash.WaitForVisible(windowCtx, tconn, pkg); err != nil {
		return errors.Wrap(err, "failed to wait for the app window")
	}

	testing.ContextLog(ctx, "Checking that the app is not launched again")
	// GoBigSleepLint: Nothing signals that the app won't be launched again, so
	// give a second launch some time to show up before counting the launches.
	if err := testing.Sleep(ctx, relaunchWaitTime); err != nil {
		return errors.Wrap(err, "failed to wait for a second launch")
	}
	n, err := countLaunches(outDir, pkg)
	if err != nil {
		return errors.Wrap(err, "failed to count the app launches")
	}
	if n != 1 {
		return errors.Errorf("app launched %d times; want 1", n)
	}
	return nil
}

// countLaunches returns the number of times pkg has been launched the way the
// launcher does it, according to the activity starts that ActivityTaskManager
// logs. Unlike the number of tasks, this also counts launches that only bring
// the existing task of pkg to the front. It reads the logcat output that
// arc.NewWithTimeout saves to outDir, which unlike the logcat ring buffers has
// every line since ARC started.
func countLaunches(outDir, pkg string) (int, error) {
	logcat, err := os.ReadFile(filepath.Join(outDir, "logcat.txt"))
	if err != nil {
		return 0, errors.Wrap(err, "failed to read the saved logcat")
	}
	// For example: ActivityTaskManager: START u0
	// {act=android.intent.action.MAIN cat=[android.intent.category.LAUNCHER]
	// flg=0x10200000 cmp=com.example.app/.MainActivity} from uid 1000
	launch := regexp.MustCompile(`ActivityTaskManager: START u\d+ \{act=android\.intent\.action\.MAIN cat=\[android\.intent\.category\.LAUNCHER\] .*cmp=` + regexp.QuoteMeta(pkg) + `/`)
	return len(launch.FindAll(logcat, -1)), nil
}
