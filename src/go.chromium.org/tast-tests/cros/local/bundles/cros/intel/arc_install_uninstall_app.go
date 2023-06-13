// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package intel

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/optin"
	"go.chromium.org/tast-tests/cros/local/arc/playstore"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ARCInstallUninstallApp,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Platform should support ARC++ install/uninstall apps",
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		BugComponent: "b:157291",
		Attr:         []string{"group:intel-nda"},
		SoftwareDeps: []string{"android_p", "chrome"},
		Timeout:      chrome.GAIALoginTimeout + arc.BootTimeout + 10*time.Minute,
		Fixture:      "arcBootedWithPlayStore",
		VarDeps:      []string{"ui.gaiaPoolDefault"},
	})
}

func ARCInstallUninstallApp(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*arc.PreData).Chrome
	a := s.FixtValue().(*arc.PreData).ARC
	d := s.FixtValue().(*arc.PreData).UIDevice

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	pkgs := map[string]apps.App{
		"com.google.android.apps.docs.editors.docs": apps.Docs,
		"com.google.android.apps.dynamite":          apps.Chat}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	for pkg, app := range pkgs {
		s.Logf("Installing %s app", app.Name)
		if err := playstore.InstallApp(ctx, a, d, pkg, &playstore.Options{}); err != nil {
			s.Fatalf("Failed to install %s: %v ", app.Name, err)
		}

		// Check the newly downloaded app in Launcher.
		if err := launcher.LaunchApp(tconn, app.Name)(ctx); err != nil {
			s.Fatalf("Failed to launch %s: %v ", app.Name, err)
		}

		if err := apps.Close(ctx, tconn, app.ID); err != nil {
			s.Fatalf("Failed to close %s: %v ", app.Name, err)
		}

		if err := optin.ClosePlayStore(ctx, tconn); err != nil {
			s.Fatal("Failed to close Play Store: ", err)
		}
	}

	// Turn off the Play Store.
	if err := optin.SetPlayStoreEnabled(ctx, tconn, false); err != nil {
		s.Fatal("Failed to Turn Off Play Store: ", err)
	}

	// Verify Play Store is Off.
	playStoreState, err := optin.GetPlayStoreState(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to check Google Play Store State: ", err)
	}
	if playStoreState["enabled"] == true {
		s.Fatal("Failed as Play Store is still enabled")
	}

	for _, app := range pkgs {
		// Verify the app icon is not visible in Launcher and the app fails to launch.
		if err := launcher.LaunchApp(tconn, app.Name)(ctx); err == nil {
			s.Fatal("Installed app remained in launcher after play store disabled: ", err)
		}
	}

	// Turn on the Play Store.
	s.Log("Performing optin to Play Store")
	maxAttempts := 2
	if err := optin.PerformWithRetry(ctx, cr, maxAttempts); err != nil {
		s.Fatal("Failed to optin to Play Store: ", err)
	}

	if err := optin.WaitForPlayStoreShown(ctx, tconn, time.Minute); err != nil {
		s.Fatal("Failed to wait for Play Store: ", err)
	}

	// Check for ARC notification after enabling Play Store.
	const notificationWaitTime = 30 * time.Second
	const notificationID = "ARC_NOTIFICATION"
	_, err = ash.WaitForNotification(ctx, tconn, notificationWaitTime, ash.WaitIDContains(notificationID))
	if err != nil {
		s.Fatal("Installed app remained in launcher after play store disabled")
	}

	// Clear any notifications that are currently displayed.
	if err := ash.CloseNotifications(ctx, tconn); err != nil {
		s.Fatal("Failed to clear notifications: ", err)
	}
}
