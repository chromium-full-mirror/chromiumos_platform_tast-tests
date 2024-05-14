// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package videoconferencing

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/videoconferencing/fakevctab"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/vctray"
	"go.chromium.org/tast-tests/cros/local/videoconferencing/fixture"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         TrayReturnToAppVirtualDesktop,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks VC tray returns to app in a virtual desktop",
		Contacts: []string{
			"chrome-knowledge-eng@google.com",
			"xiuwen@google.com",
		},
		BugComponent: "b:187682",
		Timeout:      3 * time.Minute,
		Attr: []string{
			"group:camera_dependent",
			"group:video_conference",
			"video_conference_cq_critical",
			"group:cbx", "cbx_feature_enabled", "cbx_unstable",
		},
		TestBedDeps:  []string{tbdep.Cbx(true)},
		SoftwareDeps: []string{"chrome"},
		Data: []string{
			"vc_tester/popup.html",
			"vc_tester/popup.js",
		},
		Fixture: fixture.LoggedInWithFakeHALAndEffectsEnabled,
		SearchFlags: []*testing.StringPair{
			{
				// VC features on virtual desktop.
				Key:   "feature_id",
				Value: "screenplay-9a4e65ac-ea29-4367-8053-a6683af49bf1",
			},
		},
	})
}

func TrayReturnToAppVirtualDesktop(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}

	if err := ash.CreateNewDesk(ctx, tconn); err != nil {
		s.Fatal("Failed to create new desk: ", err)
	}
	defer ash.CleanUpDesks(cleanupCtx, tconn)

	if err := ash.ActivateDeskAtIndex(ctx, tconn, 1); err != nil {
		s.Fatal("Failed to activate new desk: ", err)
	}

	vcTray := vctray.New(ctx, tconn)

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_dump")

	browserType := s.FixtValue().(fixture.FixtData).BrowserType()

	// Open an empty chrome tab.
	conn, br, closeBrowser, err := browserfixt.SetUpWithURL(ctx, cr, browserType, chrome.NewTabURL)
	if err != nil {
		s.Fatal("Failed to launch browser: ", err)
	}
	defer closeBrowser(cleanupCtx)
	defer conn.Close()
	defer conn.CloseTarget(cleanupCtx)

	// Open VcTester tab.
	vcTabFullURL := fakevctab.SetupServerAndPermission(ctx, br, s)
	tabUI, err := fakevctab.LaunchTab(ctx, tconn, br, vcTabFullURL)
	if err != nil {
		s.Fatal("Failed to open tab: ", err)
	}

	// Turn on camera to trigger the tray.
	if err := uiauto.Combine("activate camera",
		tabUI.StartVideo,
		vcTray.WaitUntilState(vctray.DevCamera, vctray.DeviceInUse),
	)(ctx); err != nil {
		s.Fatal("Failed to verify that tab triggers vcTray by camera: ", err)
	}

	// Get current window.
	tabWindow, err := ash.GetActiveWindow(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get active window: ", err)
	}

	// Return to original desk.
	if err := ash.ActivateDeskAtIndex(ctx, tconn, 0); err != nil {
		s.Fatal("Failed to activate new desk: ", err)
	}

	// Apply return to app from vcTray.
	if err := vcTray.ReturnToAppForWindow("VcTester", tabWindow, tconn)(ctx); err != nil {
		s.Fatal("Failed to verify return to app: ", err)
	}

	// Get current active window and verify that it is the original tabWindow.
	newActiveWindow, err := ash.GetActiveWindow(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get active window: ", err)
	}
	if newActiveWindow.ID != tabWindow.ID || newActiveWindow.State != tabWindow.State {
		s.Fatalf("Failed to restore window(expected: %v, actual: %v)", tabWindow, newActiveWindow)
	}

	// Close the tab.
	if err := tabUI.CloseTab(ctx); err != nil {
		s.Fatal("Failed to close tab: ", err)
	}
}
