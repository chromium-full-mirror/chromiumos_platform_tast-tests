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
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/checked"
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
		Func:         AssignToAllDesks,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Assign apps to all desks",
		Contacts: []string{
			"chromeos-wm@google.com",
			"chromeos-consumer-engprod@google.com",
		},
		// ChromeOS > Software > Window Management > Virtual Desks
		BugComponent: "b:1238200",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome", "gaia"},
		Timeout:      120 * time.Second,
		VarDeps:      []string{ui.GaiaPoolDefaultVarName},
		Params: []testing.Param{{
			Val:     browser.TypeAsh,
			Fixture: "loggedInToCUJUserARCSupported",
		}, {
			Name:              "lacros",
			Val:               browser.TypeLacros,
			Fixture:           "loggedInToCUJUserARCSupportedLacros",
			ExtraSoftwareDeps: []string{"lacros"},
		}},
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

func AssignToAllDesks(ctx context.Context, s *testing.State) {
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

	chromeApp, err := apps.PrimaryBrowser(ctx, tconn)
	if err != nil {
		s.Fatal("Could not find the Chrome app: ", err)
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

	// Create 4 desks.
	const numNewDesks = 4
	for i := 1; i <= numNewDesks; i++ {
		if err := ash.CreateNewDesk(ctx, tconn); err != nil {
			s.Fatalf("Failed to create the Desk %d: %v", i+1, err)
		}
		// Active the new created desk.
		if err = ash.ActivateDeskAtIndex(ctx, tconn, i); err != nil {
			s.Fatalf("Failed to activate desk with index %d: %v", i, err)
		}
	}

	// Activate Desk 1 to launch and show windows on it.
	if err = ash.ActivateDeskAtIndex(ctx, tconn, 0); err != nil {
		s.Fatalf("Failed to activate desk with index %d: %v", 0, err)
	}

	// Ensure there is no window open before test starts.
	if err := ash.CloseAllWindows(ctx, tconn); err != nil {
		s.Fatal("Failed to ensure no window is open before creating test windows: ", err)
	}

	appsList := []apps.App{chromeApp, apps.Terminal, arcApp}
	bt := s.Param().(browser.Type)
	for _, app := range appsList {
		if bt == browser.TypeLacros && app == chromeApp {
			cr.Browser().NewConn(ctx, chrome.BlankURL)
			continue
		}
		if err := apps.Launch(ctx, tconn, app.ID); err != nil {
			s.Fatalf("Failed to launch %s: %v", app.Name, err)
		}
		if err := ash.WaitForApp(ctx, tconn, app.ID, time.Minute); err != nil {
			s.Fatalf("%s did not appear in shelf after launch: %v", app.Name, err)
		}
		if _, err := ash.WaitForAppWindow(ctx, tconn, app.ID); err != nil {
			s.Fatalf("%s did not produce a visible window after launch: %v", app.Name, err)
		}
	}

	// 5. Assign window to all desks.
	if err := assignWindowsToDesks(ctx, tconn, ac, true); err != nil {
		s.Fatal("Failed to assign window to all desks: ", err)
	}
	// 6. Verify window shows up on all desks.
	if err := verifyWindowsOnDesks(ctx, tconn, ac, true); err != nil {
		s.Fatal("Failed to verify the window on all desks: ", err)
	}

	// 7. Create two more desks and verify the windows also can show up on the new created desks.
	for i := 0; i <= 2; i++ {
		if err := ash.CreateNewDesk(ctx, tconn); err != nil {
			s.Fatalf("Failed to create the Desk %d: %v", i, err)
		}
	}
	if err := verifyWindowsOnDesks(ctx, tconn, ac, true); err != nil {
		s.Fatal("Failed to verify the window on all desks: ", err)
	}

	// 8. Activate Desk 4 and close it.
	if err := ash.ActivateDeskAtIndex(ctx, tconn, 3); err != nil {
		s.Fatal("Failed to activate Desk 4: ", err)
	}
	// After removing Desk 4, Desk 3 should be the active desk.
	if err := ash.RemoveActiveDesk(ctx, tconn); err != nil {
		s.Fatal("Failed to remove active Desk 4: ", err)
	}
	if err := verifyWindowsOnDesks(ctx, tconn, ac, true); err != nil {
		s.Fatal("Failed to verify the window on all desks: ", err)
	}
	// Check if Desk 3 is the active desk.
	info, err := ash.GetDesksInfo(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get the desk info: ", err)
	}
	if info.ActiveDeskIndex != 2 {
		s.Fatalf("After removing Desk 4, Desk 3 should be the active desk; while Desk %d is the current desk", info.ActiveDeskIndex+1)
	}

	// 9. Close the windows got on Desk 3 and verify no windows in system.
	ws, err := ash.FindAllWindows(ctx, tconn, func(w *ash.Window) bool {
		return w.OnActiveDesk
	})
	if err != nil {
		s.Fatal("Failed to find all windows on active Desk 3: ", err)
	}
	if len(ws) != 3 {
		s.Fatalf("Unexpected number of windows found; want: 3, got: %d", len(ws))
	}

	if err := ash.CloseAllWindows(ctx, tconn); err != nil {
		s.Fatal("Failed to close any existing windows: ", err)
	}
	ws, err = ash.GetAllWindows(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to find all windows: ", err)
	}
	if len(ws) != 0 {
		s.Fatalf("Unexpected number of windows found; want: 0, got: %d", len(ws))
	}

	// 10. Re-open the 3 kinds of windows and assign them to all desks. Then re-assign them to a specific desk.
	for _, app := range appsList {
		// For the lacros browser, we don't need to do the launch step.
		if bt == browser.TypeLacros && app == chromeApp {
			cr.Browser().NewConn(ctx, chrome.BlankURL)
			continue
		}
		if err := apps.Launch(ctx, tconn, app.ID); err != nil {
			s.Fatalf("Failed to launch %s: %v", app.Name, err)
		}
		if err := ash.WaitForApp(ctx, tconn, app.ID, time.Minute); err != nil {
			s.Fatalf("%s did not appear in shelf after launch: %v", app.Name, err)
		}
		if _, err := ash.WaitForAppWindow(ctx, tconn, app.ID); err != nil {
			s.Fatalf("%s did not produce a visible window after launch: %v", app.Name, err)
		}
	}
	ws, err = ash.GetAllWindows(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to find all windows: ", err)
	}
	if len(ws) != 3 {
		s.Fatalf("Unexpected number of windows found; want: 3, got: %d", len(ws))
	}

	// Assign window to all desks and verify it shows up on all desks.
	if err := assignWindowsToDesks(ctx, tconn, ac, true); err != nil {
		s.Fatal("Failed to assign windows to all desks: ", err)
	}
	if err := verifyWindowsOnDesks(ctx, tconn, ac, true); err != nil {
		s.Fatal("Failed to verify the window on all desks: ", err)
	}
	// Re-assign it to Desk 2 and verify it only shows up on Desk 2.
	if err := assignWindowsToDesks(ctx, tconn, ac, false); err != nil {
		s.Fatal("Failed to assign windows to Desk 2: ", err)
	}
	if err := verifyWindowsOnDesks(ctx, tconn, ac, false); err != nil {
		s.Fatal("Failed to verify the window only on Desk 2: ", err)
	}
}

// isMenuItemChecked returns true if `menuItem` is checked.
func isMenuItemChecked(ctx context.Context, ac *uiauto.Context, menuItem *nodewith.Finder) (bool, error) {
	nodeInfo, err := ac.NodesInfo(ctx, menuItem)
	if err != nil {
		return false, err
	}
	if len(nodeInfo) != 1 {
		return false, errors.Errorf("expected exactly one entry from NodesInfo, got %d", len(nodeInfo))
	}

	return nodeInfo[0].Checked == checked.True, nil
}

// assignWindowsToDesks assigns windows to all desks or Desk 2 based on `onAllDesks`.
func assignWindowsToDesks(ctx context.Context, tconn *chrome.TestConn, ac *uiauto.Context, onAllDesks bool) error {
	dc, err := ash.GetDeskCount(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to count desks")
	}

	// We must have at least two desks. Then, the `MenuItemView` can be shown up when right-clicking on the caption bar of the window.
	if dc == 1 {
		return errors.Wrapf(err, "unexpected number of desks: got %v, want more than 1", dc)
	}

	ws, err := ash.GetAllWindows(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to find all windows")
	}
	numWindows := 3
	if len(ws) != 3 {
		return errors.Wrapf(err, "unexpected number of windows found; want: %d, got: %d", numWindows, len(ws))
	}

	var moveTarget *nodewith.Finder
	// Assign window to all desks or Desk 2.
	if onAllDesks {
		moveTarget = nodewith.ClassName("MenuItemView").Name("All desks")
	} else {
		moveTarget = nodewith.ClassName("MenuItemView").Name("Desk 2")
	}

	for i := 0; i < numWindows; i++ {
		// Bring the window to the front.
		if err := ws[i].ActivateWindow(ctx, tconn); err != nil {
			return errors.Wrap(err, "failed to activate window")
		}

		// Right click on the top of the window.
		rightClickPoint := coords.NewPoint(ws[i].BoundsInRoot.Left+ws[i].BoundsInRoot.Width/2, ws[i].BoundsInRoot.Top+ws[i].CaptionHeight/2)
		if err := mouse.Click(tconn, rightClickPoint, mouse.RightButton)(ctx); err != nil {
			return errors.Wrap(err, "failed to right click the top of the window")
		}

		// Move mouse to the move window to desk menu item.
		moveWindowToDeskMenuItem := nodewith.ClassName("MenuItemView").Name("Move window to desk")

		// For the arc app, when right-clicking on the caption bar of it, the `moveTarget` will be shown up, so we don't need to do the step below.
		if err := ac.Exists(moveTarget)(ctx); err != nil {
			if err := uiauto.Combine(
				"move cursor to menu and wait for submenu",
				ac.MouseMoveTo(moveWindowToDeskMenuItem, 0),
				ac.DoDefault(moveWindowToDeskMenuItem),
				ac.WaitUntilExists(moveTarget),
			)(ctx); err != nil {
				return errors.Wrapf(err, "failed to get window menu for window name: %s", ws[i].Name)
			}
		}

		// If the menu item is already checked, the we must not check it again, since this toggles the item. See b/276296010 for more info.
		checked, err := isMenuItemChecked(ctx, ac, moveTarget)
		if err != nil {
			return errors.Wrap(err, "failed to determine if the menu item is checked")
		}
		if checked {
			continue
		}

		if err := ac.DoDefault(moveTarget)(ctx); err != nil {
			return errors.Wrap(err, "failed to move the window to Desk 2")
		}
		if err := ash.WaitWindowFinishAnimating(ctx, tconn, ws[i].ID); err != nil {
			return errors.Wrap(err, "failed to wait window finish animating")
		}
	}

	return nil
}

// verifyWindowsOnDesks verifies windows showing up on all desks or only on Desk 2 based on `onAllDesks`.
func verifyWindowsOnDesks(ctx context.Context, tconn *chrome.TestConn, ac *uiauto.Context, onAllDesks bool) error {
	info, err := ash.GetDesksInfo(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to get the desk info")
	}
	numDesks := info.NumDesks
	preActiveDeskIndex := info.ActiveDeskIndex

	for i := 0; i < numDesks; i++ {
		// To avoid the case we re-activate the Desk 1.
		if (i == 0 && preActiveDeskIndex != 0) || i > 0 {
			if err := ash.ActivateDeskAtIndex(ctx, tconn, i); err != nil {
				return errors.Wrapf(err, "failed to activate Desk %v", i+1)
			}
		}

		if err := ash.WaitUntilDesksFinishAnimating(ctx, tconn); err != nil {
			return errors.Wrap(err, "failed to wait for desks to finish animating")
		}

		count, err := ash.CountVisibleWindows(ctx, tconn)
		if err != nil {
			return errors.Wrapf(err, "failed to find the window on Desk %v", i+1)
		}

		if onAllDesks {
			if count != 3 {
				return errors.Wrap(err, "failed to find 3 kinds of windows on all desks")
			}
		} else {
			// Any desks except Desk 2 shouldn't own any windows.
			if i != 1 && count != 0 {
				return errors.Wrap(err, "unexpected found windows on desk except for Desk 2")
			}
			// Desk 2 should have 3 kinds of windows.
			if i == 1 && count != 3 {
				return errors.Wrapf(err, "failed to find windows on Desk 2; want: 3, got: %d", count)
			}
		}
	}

	// Activate the desk at the start of this function.
	if err := ash.ActivateDeskAtIndex(ctx, tconn, preActiveDeskIndex); err != nil {
		return errors.Wrapf(err, "failed to activate Desk %v", preActiveDeskIndex+1)
	}

	return nil
}
