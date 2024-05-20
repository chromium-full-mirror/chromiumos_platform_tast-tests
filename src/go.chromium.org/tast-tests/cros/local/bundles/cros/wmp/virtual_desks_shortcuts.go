// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wmp

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/testing"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         VirtualDesksShortcuts,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks that virtual desks shortcuts works correctly",
		Contacts: []string{
			"chromeos-wm@google.com",
			"chromeos-consumer-engprod@google.com",
		},
		// ChromeOS > Software > Window Management > Virtual Desks
		BugComponent: "b:1238200",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		SearchFlags: []*testing.StringPair{
			{
				Key:   "feature_id",
				Value: "screenplay-e4c751d7-be37-45e5-8017-47197bb76197",
			},
			{
				Key:   "feature_id",
				Value: "screenplay-aa50f67f-f24e-4f8d-af88-1fac22e84312",
			},
			{
				Key:   "feature_id",
				Value: "screenplay-f2f7491e-e6ee-429e-9d56-aa386a2db2ca",
			},
			{
				Key:   "feature_id",
				Value: "screenplay-2e1d03c8-d145-4fe5-b68f-04301d32199b",
			},
			{
				Key:   "feature_id",
				Value: "screenplay-353dbfd4-4666-4e1f-be6c-7a210f95069d",
			}},
		Params: []testing.Param{{
			Fixture: "chromeLoggedIn",
			Val:     browser.TypeAsh,
		}, {
			Name:              "lacros",
			Fixture:           "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               browser.TypeLacros,
		}},
	})
}

// deskMiniViewFinder finds a desk mini view for a given desk.
func deskMiniViewFinder(deskName string) *nodewith.Finder {
	return nodewith.ClassName("DeskMiniView").Name(fmt.Sprintf("Desk: %s", deskName))
}

func findBrowserWindow(ctx context.Context, s *testing.State, tconn *chrome.TestConn, bt browser.Type) *ash.Window {
	window, err := ash.FindWindow(ctx, tconn, ash.BrowserTypeMatch(bt))
	if err != nil {
		s.Fatal("Failed to find browser window: ", err)
	}
	return window
}

// findDeskMiniViewLocations finds the locations of a list of mini views.
func findDeskMiniViewLocations(ctx context.Context, tconn *chrome.TestConn, miniViewFinders []*nodewith.Finder) ([]*coords.Rect, error) {
	ac := uiauto.New(tconn)

	locations := make([]*coords.Rect, len(miniViewFinders))
	for i, finder := range miniViewFinders {
		location, err := ac.Location(ctx, finder)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get location of desk mini view")
		}
		locations[i] = location
	}

	return locations, nil
}

func VirtualDesksShortcuts(ctx context.Context, s *testing.State) {
	// Reserve five seconds for various cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	bt := s.Param().(browser.Type)
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, false)
	if err != nil {
		s.Fatal("Failed to ensure clamshell mode: ", err)
	}
	defer cleanup(cleanupCtx)

	defer ash.CleanUpDesks(cleanupCtx, tconn)
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	// Ensure there is no window open before test starts.
	// TODO(crbug.com/1311504): Ensure that chrome.ResetState can close lacros windows opened in the previous tests as well. Apply it to wmp test package.
	if err := ash.CloseAllWindows(ctx, tconn); err != nil {
		s.Fatal("Failed to ensure no window is open: ", err)
	}

	ac := uiauto.New(tconn)

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to create a keyboard: ", err)
	}

	// Internally, each virtual desk has a container for the windows on that desk. The container for
	// the currently active desk is visible while all other containers are invisible. When a desk is
	// activated, an animation plays out and the new desks's container window becomes visible. The
	// test uses this to know when a desk switch has completed.

	// Populate UI tree. TODO(b/258688744): Why is this needed?
	uiauto.RootDebugInfo(ctx, tconn)

	// First we go through shortcuts to create and switch desks.
	//   * Create a new desk. This places us on desk 2.
	//   * Switch back to desk 1.
	//   * Switch to desk 2.
	if err := uiauto.Combine(
		"create new virtual desk using keyboard shortcut",
		kb.AccelAction("Search+Shift+="),
		// This will wait until we have switched to desk 2.
		ash.WaitForDesk(tconn, 1),
	)(ctx); err != nil {
		s.Fatal("Failed to create desk 2: ", err)
	}

	if err := uiauto.Combine(
		"activate virtual desk on the left",
		kb.AccelAction("Search+["),
		ash.WaitForDesk(tconn, 0),
	)(ctx); err != nil {
		s.Fatal("Failed to switch to desk 1: ", err)
	}

	if err := uiauto.Combine(
		"activate virtual desk on the right",
		kb.AccelAction("Search+]"),
		ash.WaitForDesk(tconn, 1),
	)(ctx); err != nil {
		s.Fatal("Failed to switch to desk 2: ", err)
	}

	browserApp, err := apps.PrimaryBrowser(ctx, tconn)
	if err != nil {
		s.Fatal("Could not find browser app info: ", err)
	}

	// We then move the currently active window between desks. We're going to use a browser window for
	// these tests. We are on desk 2 when this sequence starts.
	//   * Open a browser and use a keyboard shortcut to move it to desk 1.
	//   * Verify that it no longer exists on desk 2.
	//   * Switch to desk 1 and verify that the browser is there.
	//   * Similar sequence to verify that the browser can be moved back to desk 2.
	if err := apps.Launch(ctx, tconn, browserApp.ID); err != nil {
		s.Fatal("Failed to open browser: ", err)
	}
	if err := ash.WaitForApp(ctx, tconn, browserApp.ID, time.Minute); err != nil {
		s.Fatal("Browser did not appear in shelf after launch: ", err)
	}
	if _, err := ash.WaitForAppWindow(ctx, tconn, browserApp.ID); err != nil {
		s.Fatal("Browser did not become visible: ", err)
	}
	if !findBrowserWindow(ctx, s, tconn, bt).OnActiveDesk {
		s.Fatal("Browser window is not on the current active desk (desk 2)")
	}

	if err := kb.Accel(ctx, "Search+Shift+["); err != nil {
		s.Fatal("Failed to move the current active window to the desk on the left: ", err)
	}
	// Wait for the window to finish animating and verify that it is no longer on the active desk.
	browserWindowID := findBrowserWindow(ctx, s, tconn, bt).ID
	ash.WaitWindowFinishAnimating(ctx, tconn, browserWindowID)
	if findBrowserWindow(ctx, s, tconn, bt).OnActiveDesk {
		s.Fatal("Browser window unexpectedly still on desk 2, expected it to be on desk 1")
	}

	// Change to the desk 1 and verify that the browser is on this desk.
	if err := ash.ActivateDeskAtIndex(ctx, tconn, 0); err != nil {
		s.Fatal("Failed to activate desk 1: ", err)
	}
	if !findBrowserWindow(ctx, s, tconn, bt).OnActiveDesk {
		s.Fatal("Browser window is not on desk 1")
	}

	// Move the browser back to desk 2 (on the right).
	if err := kb.Accel(ctx, "Search+Shift+]"); err != nil {
		s.Fatal("Failed to move app to desk: ", err)
	}
	// Wait for the window to finish animating and verify that it is no longer on the active desk.
	ash.WaitWindowFinishAnimating(ctx, tconn, browserWindowID)
	if findBrowserWindow(ctx, s, tconn, bt).OnActiveDesk {
		s.Fatal("Browser window unexpectedly still on desk 1, expected it to be on desk 2")
	}

	// Change to the desk 2 and verify that the browser is on this desk.
	if err := ash.ActivateDeskAtIndex(ctx, tconn, 1); err != nil {
		s.Fatal("Failed to activate desk 2: ", err)
	}
	if !findBrowserWindow(ctx, s, tconn, bt).OnActiveDesk {
		s.Fatal("Browser window is not on the currently active desk (desk 2)")
	}

	// Rename the desks so that we have stable desk identifiers to work with.
	// Desks with default names are otherwise renamed when reordered.
	if err := ash.SetOverviewModeAndWait(ctx, tconn, true); err != nil {
		s.Fatal("Failed to enter overview mode: ", err)
	}
	if err := uiauto.Combine(
		"rename desks",
		ac.DoDefault(nodewith.ClassName("DeskNameView").Name("Desk 1")),
		kb.TypeAction("desk one"),
		kb.AccelAction("Enter"),
		ac.DoDefault(nodewith.ClassName("DeskNameView").Name("Desk 2")),
		kb.TypeAction("desk two"),
		kb.AccelAction("Enter"),
	)(ctx); err != nil {
		s.Fatal("Failed to rename desks: ", err)
	}
	if err := ash.SetOverviewModeAndWait(ctx, tconn, false); err != nil {
		s.Fatal("Failed to exit overview mode: ", err)
	}

	// We are now going to reorder desks in overview mode.
	if err := ash.SetOverviewModeAndWait(ctx, tconn, true); err != nil {
		s.Fatal("Failed to enter overview mode: ", err)
	}
	defer ash.SetOverviewModeAndWait(cleanupCtx, tconn, false)

	// Get the locations of the mini view, prior to reordering.
	deskMiniViewFinders := []*nodewith.Finder{
		deskMiniViewFinder("desk one"),
		deskMiniViewFinder("desk two")}
	locationsPreReorder, err := findDeskMiniViewLocations(ctx, tconn, deskMiniViewFinders)
	if err != nil {
		s.Fatal("Failed to get locations prior to reordering: ", err)
	}

	if err := uiauto.Combine(
		"reorder desk mini views",
		kb.AccelAction("Tab"),
		kb.AccelAction("Tab"),
		kb.AccelAction("Ctrl+Right"),
		ac.WaitForLocation(deskMiniViewFinders[0]),
		ac.WaitForLocation(deskMiniViewFinders[1]),
	)(ctx); err != nil {
		s.Fatal("Failed to reorder desks")
	}

	// Verify that the mini views for the desks have swapped positions.
	locationsPostReorder, err := findDeskMiniViewLocations(ctx, tconn, deskMiniViewFinders)
	if err != nil {
		s.Fatal("Failed to get locations after reordering: ", err)
	}
	if locationsPostReorder[0].Left != locationsPreReorder[1].Left || locationsPostReorder[1].Left != locationsPreReorder[0].Left {
		s.Fatal("Failed to reorder desks")
	}

	if err := uiauto.Combine(
		"reorder desk mini views",
		kb.AccelAction("Ctrl+Left"),
		ac.WaitForLocation(deskMiniViewFinders[0]),
		ac.WaitForLocation(deskMiniViewFinders[1]),
	)(ctx); err != nil {
		s.Fatal("Failed to reorder desks")
	}

	// Verify that the mini views are now back to their positions prior to reordering.
	locationsPostRestore, err := findDeskMiniViewLocations(ctx, tconn, deskMiniViewFinders)
	if err != nil {
		s.Fatal("Failed to get locations after restoring desks: ", err)
	}
	if locationsPostRestore[0].Left != locationsPreReorder[0].Left || locationsPostRestore[1].Left != locationsPreReorder[1].Left {
		s.Fatal("Failed to reorder desks")
	}

	if err := ash.SetOverviewModeAndWait(ctx, tconn, false); err != nil {
		s.Fatal("Failed to exit overview mode: ", err)
	}

	// Test index-based desk switch accelerator.
	if err := uiauto.Combine(
		"switch to desk 1 with index-based accelerator",
		kb.AccelAction("Search+Shift+1"),
		ash.WaitForDesk(tconn, 0),
	)(ctx); err != nil {
		s.Fatal("Failed to switch to desk 1: ", err)
	}

	// Switch back to desk 2, where a browser window should still be open.
	if err := uiauto.Combine(
		"switch to desk 2 with index-based accelerator",
		kb.AccelAction("Search+Shift+2"),
		ash.WaitForDesk(tconn, 1),
	)(ctx); err != nil {
		s.Fatal("Failed to switch to desk 2: ", err)
	}
	if !findBrowserWindow(ctx, s, tconn, bt).OnActiveDesk {
		s.Fatal("Browser window is not on the currently active desk (desk 2)")
	}

	// Test the all desk window shortcut.
	if err := kb.Accel(ctx, "Search+Shift+A"); err != nil {
		s.Fatal("Failed to toggle all desk window state for active window: ", err)
	}

	// Switch to desk 1 and verify that the window is on desk 1.
	if err := ash.ActivateDeskAtIndex(ctx, tconn, 0); err != nil {
		s.Fatal("Failed to activate desk 1: ", err)
	}
	if !findBrowserWindow(ctx, s, tconn, bt).OnActiveDesk {
		s.Fatal("Browser window is not on the currently active desk (desk 1)")
	}

	// Switch back to desk 2 and verify that the window is still there.
	if err := ash.ActivateDeskAtIndex(ctx, tconn, 1); err != nil {
		s.Fatal("Failed to activate desk 2: ", err)
	}
	if !findBrowserWindow(ctx, s, tconn, bt).OnActiveDesk {
		s.Fatal("Browser window is not on the currently active desk (desk 2)")
	}

	// Finally, remove desk 2. We expect to be back at desk 1.
	if err := uiauto.Combine(
		"remove the current virtual desk",
		kb.AccelAction("Search+Shift+-"),
		ash.WaitForDesk(tconn, 0),
	)(ctx); err != nil {
		s.Fatal("Failed to remove desk 2: ", err)
	}
}
