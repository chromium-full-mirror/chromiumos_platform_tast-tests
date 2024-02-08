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
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/ctxutil"
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
		// Read-only permissions is currently only enabled on ARC-T.
		SoftwareDeps: []string{"chrome", "android_vm_t"},
		Fixture:      "arcBootedWithPlayStore",
		Timeout:      10 * time.Minute,
	})
}

// AppDetails tests presence of App Details in the OS Settings App
// Management UI.
func AppDetails(ctx context.Context, s *testing.State) {
	const (
		testAppID       = "ibiognfelmneebngbnbeonnllapmffmb"
		testAppName     = "Jitsi Meet"
		testPackageName = "org.jitsi.meet"
	)

	cr := s.FixtValue().(*arc.PreData).Chrome
	arcDevice := s.FixtValue().(*arc.PreData).ARC
	uiAutomator := s.FixtValue().(*arc.PreData).UIDevice

	// Give 5 seconds to clean up and dump out UI tree.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}

	ui := uiauto.New(tconn).WithTimeout(30 * time.Second)

	s.Log("Installing app")
	if err := playstore.InstallApp(ctx, arcDevice, uiAutomator, testPackageName, &playstore.Options{TryLimit: -1}); err != nil {
		s.Fatal("Failed to install the app: ", err)
	}

	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	if err := playstore.VerifyPlayStoreWindowPresent(ctx, tconn, 5*time.Second); err != nil {
		s.Fatal("Failed to ensure Play Store window is present: ", err)
	}

	appTitle := uiAutomator.Object(androidui.ClassName("android.widget.TextView"), androidui.TextMatches("(?i)"+testAppName), androidui.Enabled(true))
	if err := appTitle.WaitForExists(ctx, 5*time.Second); err != nil {
		s.Fatal("Failed to find text: ", err)
	}

	// Close Play Store.
	if err := optin.ClosePlayStore(ctx, tconn); err != nil {
		s.Fatal("Failed to close Play Store: ", err)
	}

	appHeader := nodewith.Name(testAppName).Role(role.Heading).Ancestor(ossettings.WindowFinder)

	osSettings, err := ossettings.LaunchAtAppMgmtPage(ctx, tconn, cr, testAppID, ui.Exists(appHeader))
	if err != nil {
		s.Fatal("Failed to open OS Settings: ", err)
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
		s.Fatal("Failed to find text: ", err)
	}

	if err := osSettings.LeftClick(nodewith.Name("Google Play Store").First())(ctx); err != nil {
		s.Fatal("Failed to open ARC settings: ", err)
	}

}
