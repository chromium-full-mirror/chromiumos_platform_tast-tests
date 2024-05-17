// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package arc supports interacting with the ARC framework, which is used to run Android applications on Chrome OS.
package arc

import (
	"context"
	"time"

	androidui "go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/optin"
	"go.chromium.org/tast-tests/cros/local/arc/playstore"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         AppDetails,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies App Details in the OS Settings App Management UI",
		Contacts: []string{
			"chromeos-apps-foundation-team@google.com",
			"sharminzaman@google.com",
		},
		BugComponent: "b:1203766",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome", "android_vm_t"},
		Fixture:      "arcBootedWithPlayStore",
		Timeout:      10 * time.Minute,
	})
}

// AppDetails tests presence of App Details in the OS Settings App
// Management UI.
func AppDetails(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*arc.PreData).Chrome
	arcDevice := s.FixtValue().(*arc.PreData).ARC

	// Give 5 seconds to clean up and dump out UI tree.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	ui := uiauto.New(tconn).WithTimeout(30 * time.Second)
	uiAutomator := s.FixtValue().(*arc.PreData).UIDevice
	if err := webAppDetails(ctx, cleanupCtx, uiAutomator, cr, arcDevice, tconn, ui); err != nil {
		s.Fatal("webAppDetails subtest failed with err: ", err)
	}

	if err := androidAppDetails(ctx, cleanupCtx, cr, arcDevice, tconn, ui); err != nil {
		s.Fatal("androidAppDetails subtest failed with err: ", err)
	}
}

func webAppDetails(ctx, cleanupCtx context.Context, d *androidui.Device, cr *chrome.Chrome, arcDevice *arc.ARC, tconn *chrome.TestConn, ui *uiauto.Context) error {
	const (
		testAppID       = "ibiognfelmneebngbnbeonnllapmffmb"
		testAppName     = "Jitsi Meet"
		testPackageName = "org.jitsi.meet"
	)
	testing.ContextLog(ctx, "Installing app")
	if err := playstore.InstallApp(ctx, arcDevice, d, testPackageName, &playstore.Options{TryLimit: -1}); err != nil {
		return errors.Wrap(err, "failed to install the app")
	}

	if err := playstore.VerifyPlayStoreWindowPresent(ctx, tconn, 5*time.Second); err != nil {
		return errors.Wrap(err, "failed to ensure Play Store window is present")
	}

	appTitle := d.Object(androidui.ClassName("android.widget.TextView"), androidui.TextMatches("(?i)"+testAppName), androidui.Enabled(true))
	if err := appTitle.WaitForExists(ctx, 5*time.Second); err != nil {
		return errors.Wrap(err, "failed to find text")
	}

	// Close Play Store.
	if err := optin.ClosePlayStore(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to close Play Store")
	}

	appHeader := nodewith.Name(testAppName).Role(role.Heading).Ancestor(ossettings.WindowFinder)

	osSettings, err := ossettings.LaunchAtAppMgmtPage(ctx, tconn, cr, testAppID, ui.Exists(appHeader))
	if err != nil {
		return errors.Wrap(err, "failed to open OS Settings")
	}

	defer osSettings.Close(cleanupCtx)

	appTypeFinder := nodewith.Name("Web App installed from Google Play Store").Role(role.Link)
	storageFinder := nodewith.Name("Storage").Role(role.StaticText)
	appSizeFinder := nodewith.NameStartingWith("App size: ").Role(role.StaticText)
	dataSizeFinder := nodewith.NameStartingWith("Data stored in app: ").Role(role.StaticText)

	if err := uiauto.Combine("check app details headings",
		osSettings.Exists(appTypeFinder),
		osSettings.Exists(storageFinder),
		osSettings.Exists(appSizeFinder),
		osSettings.Exists(dataSizeFinder),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to find text")
	}

	if err := osSettings.LeftClick(nodewith.Name("Google Play Store").First())(ctx); err != nil {
		return errors.Wrap(err, "failed to open ARC settings")
	}
	return nil
}

func androidAppDetails(ctx, cleanupCtx context.Context, cr *chrome.Chrome, arcDevice *arc.ARC, tconn *chrome.TestConn, ui *uiauto.Context) error {
	const (
		testAppID   = "klecagmlhgcabnacehjbiphlagbojhio"
		testAppName = "ARCInputOverlayTest"
		testApkName = "ArcInputOverlayTest.apk"
	)

	testing.ContextLog(ctx, "Installing app")
	if err := arcDevice.Install(ctx, arc.APKPath(testApkName)); err != nil {
		return errors.Wrap(err, "failed to install app")
	}

	appHeader := nodewith.Name(testAppName).Role(role.Heading).Ancestor(ossettings.WindowFinder)
	osSettings, err := ossettings.LaunchAtAppMgmtPage(ctx, tconn, cr, testAppID, ui.Exists(appHeader))
	if err != nil {
		return errors.Wrap(err, "failed to open OS Settings")
	}

	defer osSettings.Close(cleanupCtx)

	appTypeFinder := nodewith.Name("Android App installed from Google Play Store").Role(role.Link)
	storageFinder := nodewith.Name("Storage").Role(role.StaticText)
	appSizeFinder := nodewith.NameStartingWith("App size: ").Role(role.StaticText)
	dataSizeFinder := nodewith.NameStartingWith("Data stored in app: ").Role(role.StaticText)
	versionFinder := nodewith.NameStartingWith("Version: ").Role(role.StaticText)

	if err := uiauto.Combine("check app details headings",
		osSettings.Exists(appTypeFinder),
		osSettings.Exists(storageFinder),
		osSettings.Exists(appSizeFinder),
		osSettings.Exists(dataSizeFinder),
		osSettings.Exists(versionFinder),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to find text")
	}

	if err := osSettings.LeftClick(nodewith.Name("Google Play Store").First())(ctx); err != nil {
		return errors.Wrap(err, "failed to open ARC settings")
	}
	return nil
}
