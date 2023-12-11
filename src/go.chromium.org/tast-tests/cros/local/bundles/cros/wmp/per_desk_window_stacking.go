// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wmp

import (
	"context"
	"reflect"
	"sort"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
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
		Func:         PerDeskWindowStacking,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Assign windows to all desks and verify that per-desk window stacking works",
		Contacts: []string{
			"chromeos-wms@google.com",
			"dandersson@chromium.org",
			"chromeos-sw-engprod@google.com",
		},
		// ChromeOS > Software > Window Management > Virtual Desks
		BugComponent: "b:1238200",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{{
			Val: browser.TypeAsh,
		}, {
			Name:              "lacros",
			Val:               browser.TypeLacros,
			ExtraSoftwareDeps: []string{"lacros"},
		}},
		Timeout: chrome.GAIALoginTimeout + 120*time.Second,
		VarDeps: []string{"ui.gaiaPoolDefault"},
	})
}

// assignWindowToAllDesks makes the specified window show on all desks. This
// assumes that there is more than one desk since it's done by using the window
// frame context menu and it will only show when there are multiple desks.
func assignWindowToAllDesks(ctx context.Context, tconn *chrome.TestConn, ac *uiauto.Context, window *ash.Window) error {
	if err := window.ActivateWindow(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to activate window")
	}

	rightClickPoint := coords.NewPoint(window.BoundsInRoot.CenterPoint().X, window.BoundsInRoot.Top+10)

	moveMenu := nodewith.ClassName("MenuItemView").Name("Move window to desk")
	moveTarget := nodewith.ClassName("MenuItemView").Name("All desks")

	if err := uiauto.Combine(
		"assign window to all desks",
		mouse.Click(tconn, rightClickPoint, mouse.RightButton),
		ac.MouseMoveTo(moveMenu, 0),
		ac.DoDefault(moveMenu),
		ac.WaitUntilExists(moveTarget),
		ac.DoDefault(moveTarget),
	)(ctx); err != nil {
		return err
	}

	if err := ash.WaitWindowFinishAnimating(ctx, tconn, window.ID); err != nil {
		return errors.Wrap(err, "failed to wait window finish animating")
	}

	return nil
}

// launchAndWaitForApps launches the specified apps and waits for them to start.
func launchAndWaitForApps(ctx context.Context, tconn *chrome.TestConn, appList []apps.App) error {
	for _, app := range appList {
		if err := apps.Launch(ctx, tconn, app.ID); err != nil {
			return errors.Wrap(err, "failed to launch")
		}
		if err := ash.WaitForApp(ctx, tconn, app.ID, 15*time.Second); err != nil {
			return errors.Wrap(err, "app did not appear in shelf after launch")
		}
	}
	return nil
}

// containsString returns true if the string needle is found in arr.
func containsString(arr []string, needle string) bool {
	for _, line := range arr {
		if line == needle {
			return true
		}
	}
	return false
}

// bringToFront activates (raises) the first window (in this test there will only be one) of the specified app.
func bringToFront(ctx context.Context, tconn *chrome.TestConn, appID string) error {
	window, err := ash.FindWindow(ctx, tconn, func(w *ash.Window) bool {
		return w.AppID == appID
	})
	if err != nil {
		return err
	}
	return window.ActivateWindow(ctx, tconn)
}

// getRelativeStacking returns the relative stacking of a given set of app windows on
// the current desk. The topmost window will be first in the list, and so on.
func getRelativeStacking(ctx context.Context, tconn *chrome.TestConn, appIds []string) ([]string, error) {
	ws, err := ash.FindAllWindows(ctx, tconn, func(w *ash.Window) bool {
		return w.OnActiveDesk
	})
	if err != nil {
		return nil, err
	}

	type appAndOrder struct {
		AppID string
		Order int
	}

	// Collect the windows that we are interested in and sort them.
	var sorted []appAndOrder
	for _, w := range ws {
		if containsString(appIds, w.AppID) {
			sorted = append(sorted, appAndOrder{AppID: w.AppID, Order: w.StackingOrder})
		}
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Order < sorted[j].Order })

	// Finally filter out just the app IDs.
	var sortedApps []string
	for _, a := range sorted {
		sortedApps = append(sortedApps, a.AppID)
	}
	return sortedApps, nil
}

func PerDeskWindowStacking(ctx context.Context, s *testing.State) {
	// Use a shortened context for test operations to reserve time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	bt := s.Param().(browser.Type)
	cr, _, closeBrowser, err := browserfixt.SetUpWithNewChrome(ctx, bt, lacrosfixt.NewConfig(),
		chrome.GAIALoginPool(s.RequiredVar("ui.gaiaPoolDefault")),
		chrome.ExtraArgs("--enable-features=EnablePerDeskZOrder"))
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)
	defer closeBrowser(cleanupCtx)

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

	// Ensure there is no window open besides the browser window.
	if err := ash.CloseAllWindowsMatching(ctx, tconn, func(window *ash.Window) bool {
		return window.AppID != chromeApp.ID
	}); err != nil {
		s.Fatal("Failed to close non-browser windows: ", err)
	}

	// The apps that we will launch and then make all-desk windows.
	appsList := []apps.App{apps.Terminal}
	if bt == browser.TypeAsh {
		appsList = append(appsList, chromeApp)
	}
	if err := launchAndWaitForApps(ctx, tconn, appsList); err != nil {
		s.Fatal("Failed to launch apps: ", err)
	}

	// Create a second desk.
	if err := ash.CreateNewDesk(ctx, tconn); err != nil {
		s.Fatal("Failed to create a second desk: ", err)
	}

	allWindows, err := ash.GetAllWindows(ctx, tconn)
	if err != nil {
		s.Fatal("GetAllWindows failed: ", err)
	}
	if len(allWindows) != 2 {
		s.Fatalf("Expected 2 windows, got %d", len(allWindows))
	}

	// Assign windows to all desks.
	for _, w := range allWindows {
		if err := assignWindowToAllDesks(ctx, tconn, ac, w); err != nil {
			s.Fatal("Failed to assign window to all desks: ", err)
		}
	}

	// Launch a couple of more apps that will not be assigned to all desks.
	if err := launchAndWaitForApps(ctx, tconn, []apps.App{apps.Help, apps.FilesSWA}); err != nil {
		s.Fatal("Failed to launch apps: ", err)
	}

	deskApps := [][]string{
		[]string{chromeApp.ID, apps.FilesSWA.ID, apps.Terminal.ID, apps.Help.ID},
		[]string{chromeApp.ID, apps.Terminal.ID}}

	// This will hold the stacking of apps on the two desks.
	deskStack := make([][]string, 2)

	deskIndex := 0

	// Grab the initial state of the desks.
	for i := 0; i != 2; i++ {
		if deskStack[deskIndex], err = getRelativeStacking(ctx, tconn, deskApps[deskIndex]); err != nil {
			s.Fatal("Failed to get stacking: ", err)
		}
		deskIndex = 1 - deskIndex
		if err := ash.ActivateDeskAtIndex(ctx, tconn, deskIndex); err != nil {
			s.Fatalf("Failed to activate desk %d: %v", deskIndex+1, err)
		}
	}

	// Now we know how the windows are stacked on each of the two desks. It's a
	// mix of all-desk windows and normal windows. We're now going to go back and
	// forth between the desks and raise windows to the front. When we switch
	// desks we'll check that the stacking order of the desk has not been affected
	// by the other desk.
	for i := 0; i != 12; i++ {
		apps := deskApps[deskIndex]
		expected := deskStack[deskIndex]

		// First verify that the stacking order is where we left it.
		actual, err := getRelativeStacking(ctx, tconn, apps)
		if !reflect.DeepEqual(actual, expected) {
			s.Fatalf("Stacking unexpectedly changed on desk %d, expected:%v, actual:%v", deskIndex+1, expected, actual)
		}

		// Raise one of the windows to the front.
		if err := bringToFront(ctx, tconn, apps[(i/2)%len(apps)]); err != nil {
			s.Fatal("Failed to raise window: ", err)
		}
		// Grab the updated stack.
		if deskStack[deskIndex], err = getRelativeStacking(ctx, tconn, apps); err != nil {
			s.Fatal("Failed to get stacking: ", err)
		}

		// Activate the other desk.
		deskIndex = 1 - deskIndex
		if err := ash.ActivateDeskAtIndex(ctx, tconn, deskIndex); err != nil {
			s.Fatalf("Failed to activate desk %d: %v", deskIndex+1, err)
		}
	}
}
