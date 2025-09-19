// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package kiosk

import (
	"context"
	"path/filepath"

	"golang.org/x/sys/unix"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash/ashproc"
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

var (
	webKioskCrashRecoveryFeature = testing.StringPair{
		Key: "feature_id",
		// Relaunch kiosk web app after os crash.
		Value: "screenplay-86fc814e-2bdd-4680-bbb2-defed8bde33c",
	}
	chromeAppKioskCrashRecoveryFeature = testing.StringPair{
		Key: "feature_id",
		// Relaunch kiosk chrome app after os crash.
		Value: "screenplay-6ac07cf6-6fe6-49d7-9398-769574c032ba",
	}
)

// The heading of the Chrome app.
const chromeAppWindowHeading = "Simple Print Sample"

func init() {
	testing.AddTest(&testing.Test{
		Func: CrashRecovery,
		Desc: "Verifies crash recovery flow for kiosk sessions",
		Contacts: []string{
			"chromeos-kiosk-eng+TAST@google.com",
			"macinashutosh@google.com", // Test author
		},
		BugComponent: "b:892153", // ChromeOS > Software > Commercial (Enterprise) > Kiosk
		Attr: []string{
			"group:golden_tier_secondary",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
			"group:hw_agnostic",
			"group:on_flex",
		},
		// Enough time for kioskmode.New, kiosk launch, and kiosk.Close.
		Timeout:      kioskmode.SetupDuration + kioskmode.LaunchDuration + kioskmode.CleanupDuration,
		SoftwareDeps: []string{"reboot", "chrome"},
		VarDeps:      []string{"ui.signinProfileTestExtensionManifestKey"},
		Fixture:      fixture.FakeDMSEnrolled,
		Params: []testing.Param{
			{
				Name:             "auto_webapp",
				Val:              appType(webApp),
				ExtraSearchFlags: []*testing.StringPair{&webKioskCrashRecoveryFeature},
			},
			{
				Name:             "auto_chromeapp",
				Val:              appType(chromeApp),
				ExtraSearchFlags: []*testing.StringPair{&chromeAppKioskCrashRecoveryFeature},
			},
		},
	})
}

// appType enum for different app types.
type appType int

const (
	chromeApp appType = iota
	webApp
)

func CrashRecovery(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()
	appType := s.Param().(appType)
	signinTestExtensionManifestKey := s.RequiredVar("ui.signinProfileTestExtensionManifestKey")

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, kioskmode.CleanupDuration)
	defer cancel()

	kiosk, cr, err := kioskmode.New(
		ctx, fdms, signinTestExtensionManifestKey, getWebKioskModeOptions(appType)...,
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
		if err := kiosk.Close(ctx); err != nil {
			s.Error("Failed to close kiosk: ", err)
		}
	}(cleanupCtx)

	if err := kiosk.WaitLaunchLogs(ctx); err != nil {
		s.Fatal("Failed to launch Kiosk: ", err)
	}

	if err := waitForKioskAppStart(ctx, cr, appType, s.OutDir()); err != nil {
		s.Fatal("Kiosk launched but app did not start: ", err)
	}

	if err := kiosk.WaitForSplashScreenClosed(ctx); err != nil {
		s.Fatal("Kiosk app launch is not completed successfully: ", err)
	}

	// TODO(crbug.com/379867155) Remove this after chrome app kiosk crash recovery
	// is independent of extensions garbage collection.
	if appType == chromeApp {
		if err := kiosk.WaitForExtensionGarbageCollectionLogs(ctx); err != nil {
			s.Fatal("Extensions garbage collection didn't finish: ", err)
		}
	}

	// Cause crash using SIGSEGV signal to simulate a browser crash.
	s.Log("Cause a browser crash")
	proc, err := ashproc.RootWithContext(ctx)
	if err != nil {
		s.Fatal("Failed to get proc: ", err)
	}

	if err := proc.SendSignalWithContext(ctx, unix.SIGSEGV); err != nil {
		s.Fatal("Failed to crash chrome: ", err)
	}

	if err := kiosk.WaitForCrashRecoveryLogs(ctx); err != nil {
		s.Fatal("Kiosk crash recovery was not successful: ", err)
	}
}

func getWebKioskModeOptions(appType appType) []kioskmode.Option {
	var options []kioskmode.Option
	options = append(options, kioskmode.AutoLaunch(getKioskAccountId(appType)))
	return options
}

func getKioskAccountId(appType appType) string {
	switch appType {
	case webApp:
		return kioskmode.WebKioskAccountID
	case chromeApp:
		return kioskmode.KioskAppAccountID
	default:
		return ""
	}
}

func waitForKioskAppStart(ctx context.Context, cr *chrome.Chrome, appType appType, outDir string) error {
	testing.ContextLog(ctx, "Waiting until Kiosk app started")
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create Test API connection")
	}

	ui := uiauto.New(tconn)
	appWidget := nodewith.Name(getAppPageHeading(appType)).Role(role.Heading)
	if err := ui.WaitUntilExists(appWidget)(ctx); err != nil {
		faillog.DumpUITree(ctx, outDir, tconn)
		return errors.Wrap(err, "failed to find Kiosk app widget node")
	}
	return nil
}

func getAppPageHeading(appType appType) string {
	switch appType {
	case chromeApp:
		return chromeAppWindowHeading
	case webApp:
		return kioskmode.DefaultKioskWebAppHeading
	default:
		return ""
	}
}
