// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package arcent provides enterprise test related ARC utilities.
package arcent

import (
	"context"
	"fmt"
	"sort"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/playstore"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/policyutil"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// InstallTypeForceInstalled is the install type for app that is force-installed.
const InstallTypeForceInstalled = "FORCE_INSTALLED"

// InstallTypeAvailable is the install type for app that is allowed.
const InstallTypeAvailable = "AVAILABLE"

// InstallTypeBlocked is the install type for app that is blocked.
const InstallTypeBlocked = "BLOCKED"

// InstallTimeout is how long Play Store can take to install an app.
const InstallTimeout = 10 * time.Minute

// PlayStoreModeBlockList is the Play Store mode that allows any app to be installed unless it is blocked.
const PlayStoreModeBlockList = "BLACKLIST"

// PlayStoreModeAllowList is the Play Store mode that allows only allowlisted and force-installed apps.
const PlayStoreModeAllowList = "WHITELIST"

// UIAutomatorPackages is a list of packages installed by UIAutomator.
var UIAutomatorPackages = []string{"com.github.uiautomator.test", "com.github.uiautomator"}

// SetupPolicyServerWithArcApps sets up a fake policy server with ARC enabled and a list of packages with the corresponding install type.
func SetupPolicyServerWithArcApps(ctx context.Context, outDir, policyUser string, packages []string, installType, playStoreMode string) (fdms *fakedms.FakeDMS, retErr error) {
	return SetupPolicyServerWithArcAppsAffiliated(ctx, outDir, policyUser, packages, installType, playStoreMode, false /*affiliated*/)
}

// SetupPolicyServerWithArcAppsAffiliated sets up a fake policy affiliated server with ARC enabled and a list of packages with the corresponding install type.
func SetupPolicyServerWithArcAppsAffiliated(ctx context.Context, outDir, policyUser string, packages []string, installType, playStoreMode string, affiliated bool) (fdms *fakedms.FakeDMS, retErr error) {
	arcPolicy := CreateArcPolicyWithApps(packages, installType, playStoreMode)
	arcEnabledPolicy := &policy.ArcEnabled{Val: true}
	policies := []policy.Policy{arcEnabledPolicy, arcPolicy}

	return SetUpFakePolicyServer(ctx, outDir, policyUser, policies, affiliated)
}

// SetUpFakePolicyServer creates a FakeDMS that enforces the provided policies and optionally also sets up affiliation.
func SetUpFakePolicyServer(ctx context.Context, outdir, policyUser string, policies []policy.Policy, affiliated bool) (fdms *fakedms.FakeDMS, retErr error) {
	fdms, err := fakedms.New(ctx, outdir)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create fakedms")
	}
	defer func() {
		if retErr != nil {
			fdms.Stop(ctx)
		}
	}()

	blob := policy.NewBlob()
	blob.PolicyUser = policyUser
	if affiliated {
		blob.DeviceAffiliationIds = []string{"default"}
		blob.UserAffiliationIds = []string{"default"}
	}
	if err := blob.AddPolicies(policies); err != nil {
		return nil, errors.Wrap(err, "failed to add policy to policy blob")
	}
	if err := fdms.WritePolicyBlob(blob); err != nil {
		return nil, errors.Wrap(err, "failed to write policy blob to fdms")
	}

	return fdms, nil
}

// VerifyArcPolicyForceInstalled matches ArcPolicy FORCE_INSTALLED apps list with expected packages.
func VerifyArcPolicyForceInstalled(ctx context.Context, tconn *chrome.TestConn, forceInstalledPackages []string) error {
	dps, err := policyutil.PoliciesFromDUT(ctx, tconn)
	if err != nil {
		return err
	}

	expected := &policy.ArcPolicy{}
	actual, ok := dps.Chrome[expected.Name()]
	if !ok {
		return errors.New("no such a policy ArcPolicy")
	}
	value, err := expected.UnmarshalAs(actual.ValueJSON)
	if err != nil {
		return err
	}
	arcPolicyValue, ok := value.(policy.ArcPolicyValue)
	if !ok {
		return errors.Errorf("cannot extract ArcPolicyValue %s", value)
	}

	apps := arcPolicyValue.Applications
	forceInstalled := make(map[string]bool)
	for _, application := range apps {
		packageName := application.PackageName
		installType := application.InstallType
		if installType == InstallTypeForceInstalled {
			forceInstalled[packageName] = true
		}
	}
	for _, p := range forceInstalledPackages {
		if !forceInstalled[p] {
			return errors.Errorf("the next package is not FORCE_INSTALLED by policy: %s", p)
		}
		delete(forceInstalled, p)
	}

	if len(forceInstalled) != 0 {
		testing.ContextLogf(ctx, "WARNING: Extra FORCE_INSTALLED packages in ArcPolicy that can cause the test to timeout: %s", makeList(forceInstalled))
	}

	return nil
}

// makeList returns a list of keys from map.
// TODO: there's several duplication of makeList. Unify them.
func makeList(packages map[string]bool) []string {
	var packagesList []string
	for pkg := range packages {
		packagesList = append(packagesList, pkg)
	}
	sort.Strings(packagesList)
	return packagesList
}

// CreateArcPolicyWithApps creates a policy with specified packages with given install type.
func CreateArcPolicyWithApps(packages []string, installType, playStoreMode string) *policy.ArcPolicy {
	var appsInPolicy []policy.Application
	for _, packageName := range packages {
		appsInPolicy = append(appsInPolicy, policy.Application{
			PackageName: packageName,
			InstallType: installType,
		})
	}

	// Make UIAutomator packages always available otherwise they will be removed by ARC DPC.
	for _, packageName := range UIAutomatorPackages {
		appsInPolicy = append(appsInPolicy, policy.Application{
			PackageName: packageName,
			InstallType: InstallTypeAvailable,
		})
	}

	arcPolicy := &policy.ArcPolicy{
		Val: &policy.ArcPolicyValue{
			Applications:            appsInPolicy,
			PlayStoreMode:           playStoreMode,
			DpsInteractionsDisabled: true,
		},
	}

	return arcPolicy
}

// EnsurePlayStoreEmpty ensures that the asset browser displays empty screen.
func EnsurePlayStoreEmpty(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome, a *arc.ARC, d *ui.Device, outDir string, runID int) (retErr error) {
	return EnsurePlayStoreState(ctx, tconn, cr, a, d, outDir, runID, true)
}

// EnsurePlayStoreNotEmpty ensures that the asset browser does not display empty screen.
func EnsurePlayStoreNotEmpty(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome, a *arc.ARC, d *ui.Device, outDir string, runID int) (retErr error) {
	return EnsurePlayStoreState(ctx, tconn, cr, a, d, outDir, runID, false)
}

// EnsurePlayStoreState ensures that the asset browser has expected state.
func EnsurePlayStoreState(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome, a *arc.ARC, d *ui.Device, outDir string, runID int, shouldBeEmpty bool) (retErr error) {
	const (
		searchBarTextStart = "Search.*apps.*"
		emptyPlayStoreText = "No results found."
	)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	attempts := 0

	assertState := func(ctx context.Context, isEmpty bool, message string) error {
		if isEmpty == shouldBeEmpty {
			return nil
		}

		faillog.SaveScreenshotToFileOnError(cleanupCtx, cr, outDir, func() bool { return true }, fmt.Sprintf("play_store_%d_%d.png", runID, attempts))

		testing.ContextLog(ctx, message)
		return errors.New(message)
	}

	return testing.Poll(ctx, func(ctx context.Context) error {
		// if GMS Core updates after launch, it can cause Play Store to be closed so we have to
		// launch it again.
		act, err := playstore.LaunchAssetBrowserActivity(ctx, tconn, a)
		if err != nil {
			return err
		}
		defer act.Close(cleanupCtx)

		err = testing.Poll(ctx, func(ctx context.Context) error {
			attempts++

			if running, err := act.IsRunning(ctx); err != nil {
				return testing.PollBreak(err)
			} else if !running {
				testing.ContextLog(ctx, "Play Store closed")
				return testing.PollBreak(errors.New("Play Store closed"))
			}

			if err := playstore.FindAndDismissErrorDialog(ctx, d); err != nil {
				return testing.PollBreak(err)
			}

			// This is to ensure that we're looking at a normal asset browser UI.
			if err := d.Object(ui.TextMatches(searchBarTextStart)).Exists(ctx); err != nil {
				testing.ContextLog(ctx, "Search bar is missing")
				return errors.Wrap(err, "Search bar is missing")
			}

			// Play Store is considered empty if shows the message when there are no available apps.
			if err := d.Object(ui.Text(emptyPlayStoreText)).Exists(ctx); err == nil {
				return assertState(ctx, true, "Play Store is empty")
			}

			// Play Store is considered to be not empty when an app is found.
			if IsAnyAppInCatalog(ctx, d) {
				return assertState(ctx, false, "App found")
			}

			// Play Store is considered to be empty when we didn't find an app blurb or app card.
			return assertState(ctx, true, "no app in the catalog")
		}, &testing.PollOptions{Interval: 5 * time.Second, Timeout: 30 * time.Second})

		if err != nil {
			playstore.Close(ctx, a)
		}
		return err
	}, &testing.PollOptions{Interval: 5 * time.Second})
}

// IsAnyAppInCatalog finds an app icon in Play Store catalog view.
func IsAnyAppInCatalog(ctx context.Context, d *ui.Device) bool {
	selectors := [][]ui.SelectorOption{
		{ui.DescriptionStartsWith("Image of app or game"), ui.ClassName("android.view.View")},
		{ui.ResourceID("com.android.vending:id/mini_blurb"), ui.ClassName("android.widget.FrameLayout")},
		{ui.ResourceID("com.android.vending:id/play_card"), ui.ClassName("android.view.ViewGroup")},
		{ui.DescriptionStartsWith("Install"), ui.ClassName("android.view.View")}, // 36.7.21-21
		{ui.Text("Games"), ui.ClassName("android.widget.TextView")},              // 36.7.21-21
		{ui.Text("Apps"), ui.ClassName("android.widget.TextView")},               // 36.7.21-21
	}

	for _, selector := range selectors {
		if err := d.Object(selector...).Exists(ctx); err == nil {
			return true
		}
	}

	return false
}
