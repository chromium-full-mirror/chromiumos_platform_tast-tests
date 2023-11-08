// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package kiosk

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfaillog"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosproc"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/kioskmode"
	"go.chromium.org/tast-tests/cros/local/screenshot"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// smokeTestParam contains the options that configure Smoke tests.
type smokeTestParam struct {
	// Whether this test uses lacros or ash.
	isLacros bool
	// Whether kiosk should auto launch or be manually launched.
	autoLaunch bool
	// Whether this test uses a web app or chrome app.
	isWebApp bool
}

var (
	launchChromeAppKioskFeature = testing.StringPair{
		Key: "feature_id",
		// Launch chrome app Kiosk.
		Value: "screenplay-5e6b8c54-2eab-4ac0-a484-b9738466bb9b",
	}
	launchWebKioskFeature = testing.StringPair{
		Key: "feature_id",
		// Launch web Kiosk.
		Value: "screenplay-d93db63f-372e-4c3b-a1ca-3f23587dbac4",
	}
	manualLaunchChromeAppKioskFeature = testing.StringPair{
		Key: "feature_id",
		// Manually launch chrome app Kiosk.
		Value: "screenplay-749437a3-b4d5-427f-abc0-4c62d397d120",
	}
	manualLaunchWebKioskFeature = testing.StringPair{
		Key: "feature_id",
		// Manually launch web Kiosk.
		Value: "screenplay-cf0d13cd-2203-406c-a2df-1f4ad502d9e7",
	}
	autoLaunchKioskFeature = testing.StringPair{
		Key: "feature_id",
		// Auto launch Kiosk.
		Value: "screenplay-90708eaa-cdde-4060-8ccb-eeb305485531",
	}
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Smoke,
		Desc:         "Verifies core flows of Kiosk sessions",
		LacrosStatus: testing.LacrosVariantExists,
		Contacts: []string{
			"chromeos-kiosk-eng+TAST@google.com",
			"edmanp@google.com", // Test author
		},
		BugComponent: "b:892153", // ChromeOS > Software > Commercial (Enterprise) > Kiosk
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
		},
		// Enough time for kioskmode.New, kiosk launch, and kiosk.Close.
		Timeout:      kioskmode.SetupDuration + kioskmode.LaunchDuration + kioskmode.CleanupDuration,
		SoftwareDeps: []string{"reboot", "chrome", "lacros"},
		VarDeps:      []string{"ui.signinProfileTestExtensionManifestKey"},
		Fixture:      fixture.FakeDMSEnrolled,
		Params: []testing.Param{
			{
				Name:             "ash_manual_chromeapp",
				Val:              smokeTestParam{isLacros: false, autoLaunch: false, isWebApp: false},
				ExtraSearchFlags: []*testing.StringPair{&launchChromeAppKioskFeature, &manualLaunchChromeAppKioskFeature},
			},
			{
				Name:             "ash_manual_webapp",
				Val:              smokeTestParam{isLacros: false, autoLaunch: false, isWebApp: true},
				ExtraSearchFlags: []*testing.StringPair{&launchWebKioskFeature, &manualLaunchWebKioskFeature},
			},
			{
				Name:             "ash_auto_chromeapp",
				Val:              smokeTestParam{isLacros: false, autoLaunch: true, isWebApp: false},
				ExtraSearchFlags: []*testing.StringPair{&launchChromeAppKioskFeature, &autoLaunchKioskFeature},
			},
			{
				Name:             "ash_auto_webapp",
				Val:              smokeTestParam{isLacros: false, autoLaunch: true, isWebApp: true},
				ExtraSearchFlags: []*testing.StringPair{&launchWebKioskFeature, &autoLaunchKioskFeature},
			},
			{
				Name:             "lacros_manual_chromeapp",
				Val:              smokeTestParam{isLacros: true, autoLaunch: false, isWebApp: false},
				ExtraSearchFlags: []*testing.StringPair{&launchChromeAppKioskFeature, &manualLaunchChromeAppKioskFeature},
			},
			{
				Name:             "lacros_manual_webapp",
				Val:              smokeTestParam{isLacros: true, autoLaunch: false, isWebApp: true},
				ExtraSearchFlags: []*testing.StringPair{&launchWebKioskFeature, &manualLaunchWebKioskFeature},
			},
			{
				Name:             "lacros_auto_chromeapp",
				Val:              smokeTestParam{isLacros: true, autoLaunch: true, isWebApp: false},
				ExtraSearchFlags: []*testing.StringPair{&launchChromeAppKioskFeature, &autoLaunchKioskFeature},
			},
			{
				Name:             "lacros_auto_webapp",
				Val:              smokeTestParam{isLacros: true, autoLaunch: true, isWebApp: true},
				ExtraSearchFlags: []*testing.StringPair{&launchWebKioskFeature, &autoLaunchKioskFeature},
			},
		},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.LacrosAvailability{}, pci.VerifiedFunctionalityOS),
		},
	})
}

func (param smokeTestParam) appAccountID() string {
	if param.isWebApp {
		return kioskmode.WebKioskAccountID
	}
	return kioskmode.KioskAppAccountID
}

// appButtonName returns the name shown in the "Apps" button in the login screen.
func (param smokeTestParam) appButtonName() string {
	if param.isWebApp {
		return kioskmode.DefaultKioskWebAppTitle
	}
	return kioskmode.KioskAppBtnName
}

// The heading of the Chrome app.
const chromeAppHeading = "Simple Print Sample"

// appPageHeading returns the title in the app page as found in the accessibility tree.
func (param smokeTestParam) appPageHeading() string {
	if param.isWebApp {
		return kioskmode.DefaultKioskWebAppHeading
	}
	return chromeAppHeading
}

// kioskModeOptions returns the option slice to configure this test parameter.
func (param smokeTestParam) kioskModeOptions(signinProfileTestExtensionManifestKey string) []kioskmode.Option {
	var options []kioskmode.Option

	if param.isLacros {
		options = append(options,
			kioskmode.PublicAccountPolicies(
				param.appAccountID(),
				[]policy.Policy{&policy.LacrosAvailability{Val: "lacros_only"}}),
			// On some platforms, ash-chrome is launched with --disallow-lacros temporarily.
			// To forcibly run the tests for those devices, too, add overriding flag here.
			kioskmode.ExtraChromeOptions(
				chrome.ExtraArgs("--disable-disallow-lacros")))
	}

	if param.autoLaunch {
		options = append(options, kioskmode.AutoLaunch(param.appAccountID()))
	} else {
		options = append(options, kioskmode.ExtraChromeOptions(
			chrome.LoadSigninProfileExtension(signinProfileTestExtensionManifestKey),
		))
	}

	return options
}

// launchKioskAppManually clicks and launches the Kiosk app corresponding to
// this `param` under the "Apps" button from the login screen.
func launchKioskAppManually(ctx context.Context, cr *chrome.Chrome, param smokeTestParam) error {
	testing.ContextLog(ctx, "Launching Kiosk app manually")
	tconn, err := cr.SigninProfileTestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get Test API connection")
	}

	if err := kioskmode.LaunchAppManually(ctx, tconn, param.appButtonName()); err != nil {
		return errors.Wrap(err, "failed to start Kiosk app from Sign-in screen")
	}
	return nil
}

func waitUntilKioskAppStarted(ctx context.Context, cr *chrome.Chrome, param smokeTestParam, outDir string) error {
	testing.ContextLog(ctx, "Waiting until Kiosk app started")
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create Test API connection")
	}

	ui := uiauto.New(tconn)
	appWidget := nodewith.Name(param.appPageHeading()).Role(role.Heading)
	if err := ui.WaitUntilExists(appWidget)(ctx); err != nil {
		faillog.DumpUITree(ctx, outDir, tconn)
		return errors.Wrap(err, "failed to find Kiosk app widget node")
	}
	return nil
}

func verifyLacrosIsRunning(ctx context.Context, cr *chrome.Chrome) error {
	testing.ContextLog(ctx, "Verifying lacros is running")
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create Test API connection")
	}

	if _, err = lacrosproc.Root(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to get lacros process")
	}
	return nil
}

func saveLacrosFaillog(ctx context.Context, cr *chrome.Chrome) error {
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create Test API connection")
	}
	lacrosfaillog.Save(ctx, tconn)
	return nil
}

func saveLaunchDurationMetrics(launchDuration time.Duration, outDir string) error {
	pv := perf.NewValues()
	pv.Set(perf.Metric{
		Name:      "launch_time",
		Unit:      "s",
		Direction: perf.SmallerIsBetter,
		Multiple:  false,
	}, launchDuration.Seconds())
	return pv.Save(outDir)
}

func Smoke(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()
	param := s.Param().(smokeTestParam)
	signinTestExtensionManifestKey := s.RequiredVar("ui.signinProfileTestExtensionManifestKey")
	startTime := time.Now()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, kioskmode.CleanupDuration)
	defer cancel()

	kiosk, cr, err := kioskmode.New(
		ctx, fdms, signinTestExtensionManifestKey, param.kioskModeOptions(signinTestExtensionManifestKey)...,
	)
	if err != nil {
		s.Fatal("Failed to create Chrome in Kiosk mode: ", err)
	}
	defer func(ctx context.Context) {
		if s.HasError() {
			if err := screenshot.Capture(ctx, filepath.Join(s.OutDir(), s.TestName()+".png")); err != nil {
				s.Error("Failed to take screenshot: ", err)
			}
		}
		if param.isLacros {
			if err := saveLacrosFaillog(ctx, cr); err != nil {
				s.Error("Failed to save lacros logs: ", err)
			}
		}
		if err := kiosk.Close(ctx); err != nil {
			s.Error("Failed to close kiosk: ", err)
		}
	}(cleanupCtx)

	if !param.autoLaunch {
		if err := launchKioskAppManually(ctx, cr, param); err != nil {
			s.Fatal("Could not manual launch Kiosk app: ", err)
		}
	}

	if err := kiosk.WaitLaunchLogs(ctx); err != nil {
		s.Fatal("Failed to launch Kiosk: ", err)
	}

	if param.isLacros {
		s.Log("http://b/281993208 sleep 3 seconds before checking UI tree in lacros")
		// GoBigSleepLint: TODO(b/281993208) lacros needs some time before we can check the UI tree.
		if err := testing.Sleep(ctx, 3*time.Second); err != nil {
			s.Fatal("Failed to sleep before checking UI tree in lacros: ", err)
		}
	}

	if err := waitUntilKioskAppStarted(ctx, cr, param, s.OutDir()); err != nil {
		s.Fatal("Kiosk launched but app did not start: ", err)
	}

	if param.isLacros {
		if err := verifyLacrosIsRunning(ctx, cr); err != nil {
			s.Fatal("Could not verify lacros is running: ", err)
		}
	}

	if err := saveLaunchDurationMetrics(time.Since(startTime), s.OutDir()); err != nil {
		s.Error("Failed to save perf metrics: ", err)
	}
}
