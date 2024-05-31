// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wmp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/dma"
	"go.chromium.org/tast-tests/cros/common/ui"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/optin"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/event"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/pointer"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         OverviewCloseAllUndo,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks the action to close all windows and desks can be canceled",
		Contacts: []string{
			"chromeos-wm@google.com",
			"chromeos-consumer-engprod@google.com",
		},
		// ChromeOS > Software > Window Management > Virtual Desks
		BugComponent: "b:1238200",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome", "android_vm", "gaia"},
		Params: []testing.Param{{
			Val: browser.TypeAsh,
		}, {
			Name:              "lacros",
			Val:               browser.TypeLacros,
			ExtraSoftwareDeps: []string{"lacros"},
		}},
		Timeout: chrome.GAIALoginTimeout + arc.BootTimeout + 120*time.Second,
		SearchFlags: []*testing.StringPair{
			{
				Key:   "feature_id",
				Value: "screenplay-a8ad8d9d-ed34-48e1-a984-25570a099f75",
			},
			{
				Key:   "feature_id",
				Value: "screenplay-04939a9c-e24a-427a-9548-2994f88ad7c1",
			}},
		VarDeps: []string{ui.GaiaPoolDefaultVarName},
	})
}

func OverviewCloseAllUndo(ctx context.Context, s *testing.State) {
	// Reserves five seconds for various cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	bt := s.Param().(browser.Type)
	cr, _, closeBrowser, err := browserfixt.SetUpWithNewChrome(ctx, bt, lacrosfixt.NewConfig(),
		chrome.GAIALoginPool(dma.CredsFromPool(ui.GaiaPoolDefaultVarName)),
		chrome.EnableFeatures("DesksCloseAll"),
		chrome.ARCSupported(),
		chrome.ExtraArgs(arc.DisableSyncFlags()...))
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)
	defer closeBrowser(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, false)
	if err != nil {
		s.Fatal("Failed to ensure clamshell mode: ", err)
	}
	defer cleanup(cleanupCtx)

	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	// Ensure there is no window open before test starts. (Except we should have one lacros window open in lacros)
	if err := ash.CloseAllButOneLacrosWindow(ctx, tconn); err != nil {
		s.Fatal("Failed to ensure no unexpected windows are open: ", err)
	}

	ac := uiauto.New(tconn)

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to create a keyboard: ", err)
	}
	defer kb.Close(ctx)

	pc := pointer.NewMouse(tconn)
	defer pc.Close(cleanupCtx)

	// Sets up for launching ARC apps.
	if err := optin.PerformAndClose(ctx, cr, tconn); err != nil {
		s.Fatal("Failed to optin to Play Store and Close: ", err)
	}

	// Sets up ARC.
	a, err := arc.New(ctx, s.OutDir(), cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to start ARC: ", err)
	}
	defer a.Close(cleanupCtx)

	if err := a.WaitIntentHelper(ctx); err != nil {
		s.Fatal("Failed to wait for ARC Intent Helper: ", err)
	}

	// Enters overview mode.
	if err := ash.SetOverviewModeAndWait(ctx, tconn, true); err != nil {
		s.Fatal("Failed to set overview mode: ", err)
	}
	defer ash.SetOverviewModeAndWait(cleanupCtx, tconn, false)
	defer ash.CleanUpDesks(cleanupCtx, tconn)

	newDeskButton := nodewith.ClassName("ZeroStateIconButton")
	desk2NameView := nodewith.ClassName("DeskNameView").Name("Desk 2")
	desk2Name := "BusyDesk"
	if err := uiauto.Combine(
		"create a new desk by clicking new desk button",
		ac.DoDefault(newDeskButton),
		// The focus on the new desk should be on the desk name field.
		ac.WaitUntilExists(desk2NameView.Focused()),
		kb.TypeAction(desk2Name),
		kb.AccelAction("Enter"),
	)(ctx); err != nil {
		s.Fatal("Failed to change the name of the second desk: ", err)
	}

	// Exits overview mode.
	if err := ash.SetOverviewModeAndWait(ctx, tconn, false); err != nil {
		s.Fatal("Failed to exit overview mode: ", err)
	}

	// If we are in lacros-chrome, then browserfixt.SetUp has already opened a
	// blank browser window in the first desk. In that case, we want to move the
	// already-existing browser window over to the second desk with a keyboard
	// shortcut and wait for the window to finish moving.
	if bt == browser.TypeLacros {
		if err := ash.MoveActiveWindowToAdjacentDesk(ctx, tconn, ash.WindowMovementDirectionRight); err != nil {
			s.Fatal("Failed to move lacros window to desk 2: ", err)
		}
	}

	// Activates the second desk and launch app windows on it.
	if err := ash.ActivateDeskAtIndex(ctx, tconn, 1); err != nil {
		s.Fatal("Failed to activate desk 2: ", err)
	}

	// Opens PlayStore, Chrome and Files. As mentioned above, if we are in
	// lacros-chrome we will already have a chrome window, so if that is the case
	// then we can skip opening another browser window.
	for _, app := range []apps.App{apps.PlayStore, apps.Chrome, apps.FilesSWA} {
		if bt == browser.TypeLacros && app == apps.Chrome {
			continue
		}

		if err := apps.Launch(ctx, tconn, app.ID); err != nil {
			s.Fatalf("Failed to open %s: %v", app.Name, err)
		}
		if err := ash.WaitForApp(ctx, tconn, app.ID, time.Minute); err != nil {
			s.Fatalf("%s did not appear in shelf after launch: %s", app.Name, err)
		}

		// Waits for the launched app window to become visible.
		if err := ash.WaitForCondition(ctx, tconn, func(w *ash.Window) bool {
			return w.IsVisible && strings.Contains(w.Title, app.Name)
		}, &testing.PollOptions{Timeout: 30 * time.Second}); err != nil {
			s.Fatalf("%v app window not visible after launching: %v", app.Name, err)
		}
	}

	if err := ac.WithInterval(2*time.Second).WaitUntilNoEvent(nodewith.Root(), event.LocationChanged)(ctx); err != nil {
		s.Fatal("Failed to wait for app launch events to be completed: ", err)
	}

	// Enters overview mode.
	if err := ash.SetOverviewModeAndWait(ctx, tconn, true); err != nil {
		s.Fatal("Failed to set overview mode: ", err)
	}

	desk2DeskMiniView := nodewith.ClassName("DeskMiniView").Name(fmt.Sprintf("Desk: %s", desk2Name))

	// Gets the location of the second desk mini view.
	desk2DeskMiniViewLoc, err := ac.Location(ctx, desk2DeskMiniView)
	if err != nil {
		s.Fatalf("Failed to get the mini view location of desk %s: %v", desk2Name, err)
	}

	// Moves the mouse to second desk mini view and hover.
	if err := mouse.Move(tconn, desk2DeskMiniViewLoc.CenterPoint(), 100*time.Millisecond)(ctx); err != nil {
		s.Fatal("Failed to hover at the second desk mini view: ", err)
	}

	// Finds the "Close All" button.
	closeAllButton := nodewith.ClassName("CloseButton").Name("Close desk and windows")

	// Closes a desk and windows on it.
	if err := pc.Click(closeAllButton)(ctx); err != nil {
		s.Fatal("Failed to close all windows on a desk: ", err)
	}

	// Exits overview mode.
	if err := ash.SetOverviewModeAndWait(ctx, tconn, false); err != nil {
		s.Fatal("Failed to exit overview mode: ", err)
	}

	// Waits for the CloseAll toast to show up.
	if err := ac.WaitUntilExists(nodewith.ClassName("ToastOverlay"))(ctx); err != nil {
		s.Fatal("Failed to wait for CloseAll toast: ", err)
	}

	// There should be 0 visible windows since all windows are closed.
	wc, err := ash.CountVisibleWindows(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to count windows: ", err)
	}
	if wc != 0 {
		s.Fatalf("Unexpected number of visible windows: got %v, want 0", wc)
	}

	// There should be one desk at this point.
	dc, err := ash.GetDeskCount(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to count desks: ", err)
	}

	if dc != 1 {
		s.Fatalf("Unexpected number of desks: got %v, want 1", dc)
	}

	// Finds the "Undo" button on the CloseAll toast.
	undoButton := nodewith.ClassName("PillButton").Name("Undo")

	// Clicks on the "Undo" button in the toast and waits for the toast to disappear.
	if err := uiauto.Combine(
		"Click on 'Undo' to dismiss the toast",
		ac.DoDefault(undoButton),
		ac.WaitUntilGone(nodewith.ClassName("ToastOverlay")),
	)(ctx); err != nil {
		s.Fatal("Failed to dismiss the CloseAll toast: ", err)
	}

	// There should still be 3 windows since "Close All" action was canceled.
	wc, err = ash.CountVisibleWindows(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to count windows: ", err)
	}

	if wc != 3 {
		s.Fatalf("Unexpected number of windows: got %v, want 3", wc)
	}

	// There should be two desks remaining.
	dc, err = ash.GetDeskCount(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to count desks: ", err)
	}

	if dc != 2 {
		s.Fatalf("Unexpected number of desks: got %v, want 2", dc)
	}
}
