// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wmp

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/ui"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/event"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         AssignToNewDesk,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Assign apps to a new desk",
		Contacts: []string{
			"chromeos-wm@google.com",
			"chromeos-consumer-engprod@google.com",
		},
		// ChromeOS > Software > Window Management > Virtual Desks
		BugComponent: "b:1238200",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome", "gaia"},
		Timeout:      2 * time.Minute,
		VarDeps:      []string{ui.GaiaPoolDefaultVarName},
		Fixture:      "loggedInToCUJUserARCSupported",
		SearchFlags: []*testing.StringPair{
			{
				Key:   "feature_id",
				Value: "screenplay-ff7bb1e3-d3d8-45b4-b48d-16163633c78d",
			},
			{
				Key:   "feature_id",
				Value: "screenplay-c7f54dee-97c4-4834-b4c5-dbee34b1cb0a",
			},
			{
				Key:   "feature_id",
				Value: "screenplay-0996004d-b395-4948-899f-1eba89966e34",
			}},
	})
}

func AssignToNewDesk(ctx context.Context, s *testing.State) {
	// Use a shortened context for test operations to reserve time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}

	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, false)
	if err != nil {
		s.Fatal("Failed to ensure clamshell mode: ", err)
	}
	defer cleanup(cleanupCtx)
	defer ash.CleanUpDesks(cleanupCtx, tconn)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_dump")

	ac := uiauto.New(tconn)

	// Create 4 new desks.
	const numNewDesks = 4
	for i := 1; i <= numNewDesks; i++ {
		if err := ash.CreateNewDesk(ctx, tconn); err != nil {
			s.Fatalf("Failed to create the Desk %d: %v", i+1, err)
		}
	}

	// Activate Desk 3.
	if err := ash.ActivateDeskAtIndex(ctx, tconn, 2); err != nil {
		s.Fatal("Failed to activate Desk 3: ", err)
	}

	// Ensure there is no window open before test starts.
	if err := ash.CloseAllWindows(ctx, tconn); err != nil {
		s.Fatal("Failed to ensure no window is open: ", err)
	}

	// Find correct Chrome browser app.
	chromeApp, err := apps.ChromeOrChromium(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to find Chrome or Chromium app: ", err)
	}

	// Install an arc app.
	a := s.FixtValue().(cuj.FixtureData).ARC

	if err := a.WaitIntentHelper(ctx); err != nil {
		s.Fatal("Failed to wait for ARC Intent Helper: ", err)
	}
	const apk = "ArcInstallAppWithAppListSortedTest.apk"
	if err := a.Install(ctx, arc.APKPath(apk)); err != nil {
		s.Fatal("Failed installing arc app: ", err)
	}
	const appName = "InstallAppWithAppListSortedMockApp"
	arcAppID, err := ash.WaitForChromeAppByNameInstalled(ctx, tconn, appName, 1*time.Minute)
	if err != nil {
		s.Fatalf("Failed to wait until %s is installed: %v", appName, err)
	}
	arcApp := apps.App{ID: arcAppID, Name: appName}

	appsList := []apps.App{chromeApp, apps.Terminal, arcApp}
	for _, app := range appsList {
		if err := apps.Launch(ctx, tconn, app.ID); err != nil {
			s.Fatalf("Failed to launch %s: %v", app.Name, err)
		}
		if err := ash.WaitForApp(ctx, tconn, app.ID, time.Minute); err != nil {
			s.Fatalf("%s did not appear in shelf after launch: %v", app.Name, err)
		}
		if _, err := ash.WaitForAppWindow(ctx, tconn, app.ID); err != nil {
			s.Fatalf("%s did not become visible: %s", app.Name, err)
		}
	}

	// Make sure all apps finished launch animation.
	if err := ac.WithInterval(2*time.Second).WithTimeout(10*time.Second).WaitUntilNoEvent(nodewith.Root(), event.LocationChanged)(ctx); err != nil {
		s.Fatal("Failed to wait for apps finishing animation: ", err)
	}

	if err := moveToAndCloseWindowsOnDesk2(ctx, tconn, ac); err != nil {
		s.Fatal("Failed to move and verify windows: ", err)
	}
}

// moveToAndCloseWindowsOnDesk2 moves windows to Desk 2 from Desk 3, and verifies windows showing up on Desk 2; finally,
// we closed all of the windows on Desk 2.
func moveToAndCloseWindowsOnDesk2(ctx context.Context, tconn *chrome.TestConn, ac *uiauto.Context) error {
	// Check if current desk is Desk 3.
	info, err := ash.GetDesksInfo(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to get the desk info")
	}
	if info.ActiveDeskIndex != 2 {
		return errors.Errorf("Active desk should be Desk 3; while current desk is Desk %d", info.ActiveDeskIndex+1)
	}

	numWindows := 3
	ws, err := findNumWindows(ctx, tconn, numWindows)
	if err != nil {
		return errors.Wrap(err, "failed to find windows")
	}

	for i := 0; i < numWindows; i++ {
		if err := clickToMoveWindow(ctx, tconn, ac, ws[i]); err != nil {
			return errors.Wrapf(err, "failed to click and move %v", ws[i].Name)
		}
	}

	// Activate Desk 2.
	if err := ash.ActivateDeskAtIndex(ctx, tconn, 1); err != nil {
		return errors.Wrap(err, "failed to activate Desk 2")
	}

	// Make sure the desk animiation is finished.
	if err := ash.WaitUntilDesksFinishAnimating(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to wait for desk finishing animation after activating Desk 2")
	}

	// Check if there are total 3 windows on Desk 2.
	ws, err = findNumWindows(ctx, tconn, numWindows)
	if err != nil {
		return errors.Wrap(err, "failed to find windows")
	}

	// Close all windows on Desk 2.
	for i := 0; i < len(ws); i++ {
		if err := ws[i].CloseWindow(ctx, tconn); err != nil {
			return errors.Wrapf(err, "failed to close the window with the name %v", ws[i].Name)
		}
	}

	return nil
}

// findNumWindows finds the number of `numWindows` windows.
func findNumWindows(ctx context.Context, tconn *chrome.TestConn, numWindows int) ([]*ash.Window, error) {
	ws, err := ash.FindAllWindows(ctx, tconn, func(w *ash.Window) bool {
		return w.OnActiveDesk
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to find all windows on Desk 3")
	}
	if len(ws) != numWindows {
		return ws, errors.Errorf("unexpected number of windows found; want: %d, got: %d", numWindows, len(ws))
	}
	return ws, nil
}

// clickToMoveWindow clicks on the top of the window and assigns it from Desk 3 to Desk 2.
func clickToMoveWindow(ctx context.Context, tconn *chrome.TestConn, ac *uiauto.Context, w *ash.Window) error {
	// Right click on the top of the window.
	rightClickPoint := coords.NewPoint(w.BoundsInRoot.CenterPoint().X, w.BoundsInRoot.Top+10)
	if err := mouse.Click(tconn, rightClickPoint, mouse.RightButton)(ctx); err != nil {
		return errors.Wrap(err, "failed to right click the top of the window")
	}

	moveWindowToDeskMenuItem := nodewith.ClassName("MenuItemView").Name("Move window to desk")
	moveToDesk2 := nodewith.ClassName("MenuItemView").Name("Desk 2")

	if err := uiauto.Combine(
		"move mouse to and click Desk 2",
		ac.MouseMoveTo(moveWindowToDeskMenuItem, 0),
		ac.DoDefault(moveWindowToDeskMenuItem),
		ac.WaitUntilExists(moveToDesk2),
		ac.DoDefault(moveToDesk2),
	)(ctx); err != nil {
		return err
	}

	if err := ash.WaitWindowFinishAnimating(ctx, tconn, w.ID); err != nil {
		return errors.Wrap(err, "failed to wait window finish animating")
	}
	return nil
}
