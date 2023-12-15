// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package peripherals

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

// settingsTestParams contains all the data needed to run a single test iteration.
type settingsTestParams struct {
	appID                 string
	menuLabel             string
	settingsRevampEnabled bool
	settingsPage          string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         LaunchAppFromSettings,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Peripherals app can be found and launched from the settings",
		Contacts: []string{
			"cros-peripherals@google.com",
			"michaelcheco@google.com",
			"jimmyxgong@google.com",
			"ashleydp@google.com",
		},
		// ChromeOS > Software > System Services > Peripherals
		BugComponent: "b:1150827",
		Attr:         []string{"group:mainline", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{
			{
				Name: "diagnostics",
				Val: settingsTestParams{
					appID:                 apps.Diagnostics.ID,
					menuLabel:             apps.Diagnostics.Name,
					settingsRevampEnabled: false,
					settingsPage:          "help", // URL for About ChromeOS page
				},
			},
			{
				Name: "diagnostics_revamp_enabled",
				Val: settingsTestParams{
					appID:                 apps.Diagnostics.ID,
					menuLabel:             apps.Diagnostics.Name + " Test your battery, CPU, memory, connectivity, and more",
					settingsRevampEnabled: true,
					settingsPage:          "help", // URL for About ChromeOS page
				},
			},
			{
				Name: "scan",
				Val: settingsTestParams{
					appID:                 apps.Scan.ID,
					menuLabel:             apps.Scan.Name + " Scan documents and images",
					settingsRevampEnabled: false,
					settingsPage:          "osPrinting", // URL for Print and scan page
				},
				ExtraAttr: []string{"group:paper-io", "paper-io_scanning"},
			},
			{
				Name: "scan_revamp_enabled",
				Val: settingsTestParams{
					appID:                 apps.Scan.ID,
					menuLabel:             apps.Scan.Name + " Scan documents and images",
					settingsRevampEnabled: true,
					settingsPage:          "device", // URL for Print and scan page
				},
				ExtraAttr: []string{"group:paper-io", "paper-io_scanning"},
			},
			{
				Name: "print_management",
				Val: settingsTestParams{
					appID:                 apps.PrintManagement.ID,
					menuLabel:             apps.PrintManagement.Name + " View and manage print jobs",
					settingsRevampEnabled: false,
					settingsPage:          "osPrinting", // URL for Print and scan page
				},
			},
			{
				Name: "print_management_revamp_enabled",
				Val: settingsTestParams{
					appID:                 apps.PrintManagement.ID,
					menuLabel:             apps.PrintManagement.Name + " View and manage print jobs",
					settingsRevampEnabled: true,
					settingsPage:          "cupsPrinters", // URL for Print and scan page
				},
			},
		},
	})
}

// LaunchAppFromSettings verifies launching an app from the settings.
func LaunchAppFromSettings(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	params := s.Param().(settingsTestParams)

	const settingsRevampFeature = "OsSettingsRevampWayfinding"

	var arg chrome.Option
	if params.settingsRevampEnabled {
		arg = chrome.EnableFeatures(settingsRevampFeature)
	} else {
		arg = chrome.DisableFeatures(settingsRevampFeature)
	}
	cr, err := chrome.New(ctx, arg)

	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx) // Close our own chrome instance.

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	if err := tconn.Call(ctx, nil, "tast.promisify(chrome.autotestPrivate.waitForSystemWebAppsInstall)"); err != nil {
		s.Fatal("Failed to wait for all system web apps to be installed: ", err)
	}

	ui := uiauto.New(tconn)
	entryFinder := nodewith.Name(params.menuLabel).Role(role.Link).Ancestor(ossettings.WindowFinder)
	if _, err := ossettings.LaunchAtPageURL(ctx, tconn, cr, params.settingsPage, ui.Exists(entryFinder)); err != nil {
		s.Fatal("Failed to launch Settings page: ", err)
	}

	if err := ui.LeftClick(entryFinder)(ctx); err != nil {
		s.Fatal("Failed to click entry: ", err)
	}

	err = ash.WaitForApp(ctx, tconn, params.appID, time.Minute)
	if err != nil {
		s.Fatal("Could not find app in shelf after launch: ", err)
	}
}
