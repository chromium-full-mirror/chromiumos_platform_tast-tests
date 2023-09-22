// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wmp

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/pointer"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         VirtualDesksBasic,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks that virtual desks works correctly",
		Contacts: []string{
			"chromeos-wms@google.com",
			"dandersson@chromium.org",
			"chromeos-sw-engprod@google.com",
		},
		// ChromeOS > Software > Window Management > Virtual Desks
		BugComponent: "b:1238200",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		SearchFlags: []*testing.StringPair{
			{
				Key:   "feature_id",
				Value: "screenplay-c74ed558-34e5-4373-9b18-cb40269caa65",
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
			}},
		Params: []testing.Param{{
			Val: browser.TypeAsh,
		}, {
			Name:              "lacros",
			Val:               browser.TypeLacros,
			ExtraSoftwareDeps: []string{"lacros"},
		}},
		Timeout: 2 * time.Minute,
	})
}

// almostTopLeft returns a point in the window that is offset from the topleft
// position. This is a (temporary?) hack to account for uiauto.Location not
// returning proper recs for transformed windows. TODO(b/266148813) - revert
// to using CenterPoint if this gets resolved.
func almostTopLeft(rect *coords.Rect, offset int) coords.Point {
	return coords.Point{X: rect.Left + offset, Y: rect.Top + offset}
}

func VirtualDesksBasic(ctx context.Context, s *testing.State) {
	// Reserve five seconds for various cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	bt := s.Param().(browser.Type)
	cr, _, closeBrowser, err := browserfixt.SetUpWithNewChrome(ctx, bt, lacrosfixt.NewConfig())
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

	defer ash.CleanUpDesks(cleanupCtx, tconn)
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
	pc := pointer.NewMouse(tconn)
	defer pc.Close(cleanupCtx)

	// Opens Files and Chrome. (In lacros we should already have an open browser window)
	if bt != browser.TypeLacros {
		browserApp, err := apps.PrimaryBrowser(ctx, tconn)
		if err != nil {
			s.Fatal("Could not find browser app info: ", err)
		}
		if err := apps.Launch(ctx, tconn, browserApp.ID); err != nil {
			s.Fatal("Failed to open browser window: ", err)
		}
		if err := ash.WaitForApp(ctx, tconn, browserApp.ID, time.Minute); err != nil {
			s.Fatal("Browser window did not appear in shelf after launch: ", err)
		}
	}
	if err := apps.Launch(ctx, tconn, apps.FilesSWA.ID); err != nil {
		s.Fatal("Failed to open Files app: ", err)
	}
	if err := ash.WaitForApp(ctx, tconn, apps.FilesSWA.ID, time.Minute); err != nil {
		s.Fatal("Files app did not appear in shelf after launch: ", err)
	}

	// Enters overview mode.
	if err := ash.SetOverviewModeAndWait(ctx, tconn, true); err != nil {
		s.Fatal("Failed to set overview mode: ", err)
	}
	defer ash.SetOverviewModeAndWait(cleanupCtx, tconn, false)

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_dump")

	// Creates new desk.
	addDeskButton := nodewith.ClassName("ZeroStateIconButton")
	newDeskNameView := nodewith.ClassName("DeskNameView").Name("Desk 2")
	newDeskName := "new desk"
	newDeskMiniView :=
		nodewith.ClassName("DeskMiniView").Name(fmt.Sprintf("Desk: %s", newDeskName))
	if err := uiauto.Combine(
		"create a new desk",
		ac.DoDefault(addDeskButton),
		// The focus on the new desk should be on the desk name field.
		ac.WaitUntilExists(newDeskNameView.Focused()),
		kb.TypeAction(newDeskName),
		kb.AccelAction("Enter"),
	)(ctx); err != nil {
		s.Fatal("Failed to create a new desk: ", err)
	}
	// Verifies that there are 2 desks.
	deskMiniViewsInfo, err := ash.FindDeskMiniViews(ctx, ac)
	if err != nil {
		s.Fatal("Failed to find desks: ", err)
	}
	if len(deskMiniViewsInfo) != 2 {
		s.Fatalf("Got %v desks, want 2 desks", len(deskMiniViewsInfo))
	}

	// Reorders desks by drag and drop.
	firstDeskMiniViewLoc, secondDeskMiniViewLoc := deskMiniViewsInfo[0].Location, deskMiniViewsInfo[1].Location
	if err := pc.Drag(
		firstDeskMiniViewLoc.CenterPoint(),
		pc.DragTo(secondDeskMiniViewLoc.CenterPoint(), 3*time.Second))(ctx); err != nil {
		s.Fatal("Failed to drag and drop desks: ", err)
	}
	// The new desk should be the first desk on the list.
	newDeskLoc, err := ac.Location(ctx, newDeskMiniView)
	if err != nil {
		s.Fatal("Failed to get the location of the new desk mini view: ", err)
	}
	if *newDeskLoc != firstDeskMiniViewLoc {
		s.Fatal("New desk is not the first desk")
	}

	// Drags Files App into the new desk.
	filesAppWindowView := nodewith.HasClass("BrowserFrame").Name("Files - My files")
	filesAppWindowViewLoc, err := ac.Location(ctx, filesAppWindowView)
	if err != nil {
		s.Fatal("Failed to get the location of the Files app: ", err)
	}
	// Dragging a window in overview in portrait orientation causes a bar to
	// appear that pushes the desk mini views downward. We want to start drag and
	// then find the new positions of those mini views to account for this
	// situation.
	filesAppWindowViewAdjustedLoc := almostTopLeft(filesAppWindowViewLoc, 50)
	mc := pointer.NewMouseController(tconn)
	defer mc.Close(ctx)
	if err := mc.Press(ctx, filesAppWindowViewAdjustedLoc); err != nil {
		s.Fatal("Failed to press on overview window view: ", err)
	}
	windowDragMidpoint := coords.Point{X: filesAppWindowViewAdjustedLoc.X, Y: filesAppWindowViewAdjustedLoc.Y + 10}
	if err := mc.Move(ctx, filesAppWindowViewAdjustedLoc, windowDragMidpoint, 3*time.Second); err != nil {
		s.Fatal("Failed to move the overview window view: ", err)
	}
	deskMiniViewsInfo, err = ash.FindDeskMiniViews(ctx, ac)
	if err != nil {
		s.Fatal("Failed to find new desk mini view locations: ", err)
	}
	if err := mc.Move(ctx, windowDragMidpoint, deskMiniViewsInfo[0].Location.CenterPoint(), 2*time.Second); err != nil {
		s.Fatal("Failed to move the overview window view to the desk preview: ", err)
	}
	if err := mc.Release(ctx); err != nil {
		s.Fatal("Failed to drop the overview window view onto the desk preview: ", err)
	}
	// Checks that Files App is in the new desk. The new desk is inactive.
	if err := ash.ForEachWindow(ctx, tconn, func(w *ash.Window) error {
		if (w.Title == "Files - My files") && w.OnActiveDesk == true {
			return errors.New("Files app should be in the inactive desk")
		}
		if (w.Title == "Chrome - New Tab") && w.OnActiveDesk == false {
			return errors.New("Chrome app should be in the active desk")
		}
		return nil
	}); err != nil {
		s.Fatal("Failed to verify the desk of the app: ", err)
	}

	// Moves the mouse to first desk mini view and hovers to show the close
	// buttons.
	if err := mouse.Move(tconn, firstDeskMiniViewLoc.CenterPoint(), 100*time.Millisecond)(ctx); err != nil {
		s.Fatal("Failed to hover at the second desk mini view: ", err)
	}

	// Delete the new desk.
	closeDeskButton := nodewith.HasClass("CloseButton").NameStartingWith("Combine").Ancestor(newDeskMiniView)
	if err := uiauto.Combine(
		"Delete a new desk",
		ac.DoDefault(closeDeskButton),
		ac.WaitUntilGone(newDeskMiniView),
	)(ctx); err != nil {
		s.Fatal("Failed to delete the new desk: ", err)
	}
	// There should still be 2 visible windows. Deleting the new desk won't delete the
	// Files app in it.
	windowCount, err := ash.CountVisibleWindows(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to count visible windows: ", err)
	}
	if windowCount != 2 {
		s.Fatalf("Expected 2 visible windows, got %v instead", windowCount)
	}
}
