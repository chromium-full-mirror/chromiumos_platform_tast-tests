// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/optin"
	"go.chromium.org/tast-tests/cros/local/arc/playstore"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         LauncherApps,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "A functional test that checks if installed app appears in Launcher",
		Contacts:     []string{"cros-arc-te@google.com", "arc-core@google.com", "jinrongwu@google.com"},
		// ChromeOS > Software > ARC++ > EngProd
		BugComponent: "b:1052117",
		Attr:         []string{"group:arc", "arc_core", "group:arc-functional"},
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_p", "chrome"},
		}, {
			Name:              "vm",
			ExtraSoftwareDeps: []string{"android_vm", "chrome"},
		}},
		Timeout: chrome.GAIALoginTimeout + arc.BootTimeout + 10*time.Minute,
		VarDeps: []string{"ui.gaiaPoolDefault"},
	})
}

func LauncherApps(ctx context.Context, s *testing.State) {

	pkgName := "com.google.android.calculator"
	appName := apps.Calculator.Name
	appID := apps.Calculator.ID

	// Setup Chrome.
	cr, err := chrome.New(
		ctx,
		chrome.GAIALoginPool(s.RequiredVar("ui.gaiaPoolDefault")),
		chrome.ARCSupported(),
		chrome.ExtraArgs(arc.DisableSyncFlags()...))
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(ctx)
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}

	if err := optin.PerformAndClose(ctx, cr, tconn); err != nil {
		s.Fatal("Failed to optin to Play Store and Close: ", err)
	}

	// Setup ARC.
	a, err := arc.New(ctx, s.OutDir(), cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to start ARC: ", err)
	}
	defer a.Close(ctx)

	d, err := a.NewUIDevice(ctx)
	if err != nil {
		s.Fatal("Failed initializing UI Automator: ", err)
	}
	defer d.Close(ctx)

	// Install app.
	s.Log("Installing app")
	if err := playstore.InstallApp(ctx, a, d, pkgName, &playstore.Options{}); err != nil {
		s.Fatal("Failed to install app: ", err)
	}

	// Check the newly downloaded app in Launcher.
	// TODO(b/210702593): Replace with LaunchAndWaitForAppOpen once fixed.
	if err := launcher.LaunchApp(tconn, appName)(ctx); err != nil {
		s.Fatal("Failed to launch: ", err)
	}

	ui := uiauto.New(tconn)
	appButton := nodewith.Name(appName).ClassName("ash/ShelfAppButton")
	if err := ui.WaitUntilExists(appButton)(ctx); err != nil {
		s.Fatal("Failed to find app in Shelf: ", err)
	}

	if err := apps.Close(ctx, tconn, appID); err != nil {
		s.Fatal("Failed to close: ", err)
	}

	// Turn off the Play Store
	if err := optin.SetPlayStoreEnabled(ctx, tconn, false); err != nil {
		s.Fatal("Failed to Turn Off Play Store: ", err)
	}

	// Verify Play Store is Off
	playStoreState, err := optin.GetPlayStoreState(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to check Google PlayStore State: ", err)
	}
	if playStoreState["enabled"] == true {
		s.Fatal("Playstore Still Enabled")
	}

	// Verify the app icon is not visible in Launcher and the app fails to launch.
	if err := launcher.LaunchApp(tconn, appName)(ctx); err == nil {
		s.Fatal("Installed app remained in launcher after play store disabled")
	}
}
