// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	arcui "go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome/apps"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ChromeIntentPicker,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Verify Chrome Intent Picker can launch ARC app by visiting URL",
		Contacts: []string{
			"chromeos-apps-foundation-team@google.com",
			"mxcai@chromium.org",
		},
		BugComponent: "b:1203766",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_container"},
			Fixture:           "arcBooted",
			Val:               browser.TypeAsh,
		}, {
			Name:              "vm",
			ExtraSoftwareDeps: []string{"android_vm"},
			Fixture:           "arcBooted",
			Val:               browser.TypeAsh,
		}, {
			Name:              "lacros",
			ExtraSoftwareDeps: []string{"android_container", "lacros"},
			Fixture:           "lacrosWithArcBooted",
			Val:               browser.TypeLacros,
		}, {
			Name:              "lacros_vm",
			ExtraSoftwareDeps: []string{"android_vm", "lacros"},
			Fixture:           "lacrosWithArcBooted",
			Val:               browser.TypeLacros,
		}},
		Timeout: 10 * time.Minute,
	})
}

const (
	arcChromeIntentPickerUITimeout    = 15 * time.Second
	arcChromeIntentPickerPollInterval = 2 * time.Second
)

func ChromeIntentPicker(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*arc.PreData).Chrome
	arcDevice := s.FixtValue().(*arc.PreData).ARC
	uiAutomator := s.FixtValue().(*arc.PreData).UIDevice

	const (
		appName        = "Intent Picker Test App"
		intentActionID = "org.chromium.arc.testapp.chromeintentpicker:id/intent_action"
		expectedAction = "android.intent.action.VIEW"
	)

	// Give 5 seconds to clean up and dump out UI tree.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// Setup Test API Connection.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	// Setup ARC and Installs APK.
	if err := arcDevice.WaitIntentHelper(ctx); err != nil {
		s.Fatal("Failed waiting for intent helper: ", err)
	}

	if err := arcDevice.Install(ctx, arc.APKPath("ArcChromeIntentPickerTest.apk")); err != nil {
		s.Fatal("Failed installing the APK: ", err)
	}

	// Open chrome://app-service-internals to and download the installed apps.
	// This is called for both succeeded cases and failed cases to compare.
	// The whole process should not fail the test case.
	// TODO (crbug/1416707): to remove this part once the bug crbug/1416707 is resolved.
	defer func() {
		if err := apps.DownloadAppServiceInternals(ctx, cr, s.OutDir()); err != nil {
			s.Log("Failed to download app service internal: ", err)
		}
	}()

	br, closeBrowser, err := browserfixt.SetUp(ctx, cr, s.Param().(browser.Type))
	if err != nil {
		s.Fatal("Failed to open browser: ", err)
	}
	defer closeBrowser(cleanupCtx)

	// Navigate to URL which ArcChromeIntentPickerTest app has associated an intent.
	conn, err := br.NewConn(ctx, "https://www.google.com")
	if err != nil {
		s.Fatal("Failed to create renderer: ", err)
	}
	defer conn.Close()

	// Locate and left click on the Intent Picker button in Chrome omnibox.
	intentPicker := nodewith.Name("Open in app").Role(role.Button)
	openButton := nodewith.Name("Open").Role(role.Button)
	ui := uiauto.New(tconn).WithInterval(arcChromeIntentPickerPollInterval)
	if err := uiauto.Combine("",
		ui.LeftClick(intentPicker),
		ui.LeftClick(openButton))(ctx); err != nil {
		s.Fatal("Failed to click intent picker button: ", err)
	}

	// Wait for the android intent to show in the Android test app.
	intentActionField := uiAutomator.Object(arcui.ID(intentActionID), arcui.Text(expectedAction))
	if err := intentActionField.WaitForExists(ctx, arcChromeIntentPickerUITimeout); err != nil {
		s.Fatalf("Failed waiting for intent action %q to appear in ARC window: %v", expectedAction, err)
	}
}
