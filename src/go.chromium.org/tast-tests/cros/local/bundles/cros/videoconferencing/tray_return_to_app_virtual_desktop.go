// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package videoconferencing

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/camera/cca"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/vctray"
	"go.chromium.org/tast-tests/cros/local/videoconferencing/fixture"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         TrayReturnToAppVirtualDesktop,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks VC tray returns to app in a virtual desktop",
		Contacts: []string{
			"chrome-knowledge-eng@google.com",
			"shengjun@chromium.org",
		},
		BugComponent: "b:187682",
		Timeout:      3 * time.Minute,
		Attr: []string{
			"group:camera_dependent",
			"group:video_conference",
			"video_conference_cq_critical",
			"group:cbx", "cbx_feature_enabled", "cbx_unstable",
		},
		SoftwareDeps: []string{"chrome", "camera_feature_effects"},
		HardwareDeps: hwdep.D(hwdep.SkipOnModel("betty")),
		Fixture:      fixture.LoggedInWithFakeHALAndEffectsEnabled,
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

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_dump")

	if err := apps.Launch(ctx, tconn, apps.Camera.ID); err != nil {
		s.Fatal("Failed to launch Camera app: ", err)
	}
	defer apps.Close(cleanupCtx, tconn, apps.Camera.ID)

	// Wait for Camera activated, otherwise vcTray is not triggered.
	if err := uiauto.New(tconn).WaitUntilExists(cca.A11yCanvasNode)(ctx); err != nil {
		s.Fatal("Camera is not working appropriately: ", err)
	}

	appWindow, err := ash.GetActiveWindow(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get active window: ", err)
	}

	// Save current window state and use it for return verification.
	appWindowState := appWindow.State

	// Return to original desk.
	if err := ash.ActivateDeskAtIndex(ctx, tconn, 0); err != nil {
		s.Fatal("Failed to activate new desk: ", err)
	}

	vcTray := vctray.New(ctx, tconn)

	if err := uiauto.Combine("return to app via mcpanel",
		vcTray.ExpandPanel,
		vcTray.ReturnToApp(appWindow.Title),
	)(ctx); err != nil {
		s.Fatal("Failed to return to app: ", err)
	}

	newActiveWindow, err := ash.GetActiveWindow(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get active window: ", err)
	}

	if newActiveWindow.ID != appWindow.ID || newActiveWindow.State != appWindowState {
		s.Fatalf("Failed to restore window(expected: %v, actual: %v)", appWindow, newActiveWindow)
	}
}
