// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wmp

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

const (
	deskIconButton                   = "DeskIconButton"
	deskIconButtonExpandedStateWidth = 36
	deskBarZeroHeight                = 40
	deskBarName                      = "LegacyDeskBarView"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         OverviewDragWindowToNewDesk,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks that drag window to new desk in overview mode works correctly",
		Contacts: []string{
			"chromeos-wm@google.com",
			"chromeos-consumer-engprod@google.com",
		},
		// ChromeOS > Software > Window Management > OverviewMode
		BugComponent: "b:1252584",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SearchFlags: []*testing.StringPair{
			{
				Key:   "feature_id",
				Value: "screenplay-655469b9-efb0-4595-aba4-7d91d265b3dd",
			},
			{
				Key:   "feature_id",
				Value: "screenplay-048ac5ea-ecf6-4f52-818e-ca13507a591a",
			},
			{
				Key:   "feature_id",
				Value: "screenplay-97e91de9-7126-4997-b0f3-707c3ff48fce",
			}},
		SoftwareDeps: []string{"chrome"},
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

func OverviewDragWindowToNewDesk(ctx context.Context, s *testing.State) {
	// Reserve five seconds for various cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

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
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_dump")

	// Ensure there is no window open before test starts.
	if err := ash.CloseAllWindows(ctx, tconn); err != nil {
		s.Fatal("Failed to ensure no window is open: ", err)
	}

	blankConn, _, closeBrowser, err := browserfixt.SetUpWithURL(ctx, cr, s.Param().(browser.Type), chrome.BlankURL)
	if err != nil {
		s.Fatal("Failed to set up Chrome: ", err)
	}
	defer closeBrowser(cleanupCtx)
	defer blankConn.Close()

	// Enter overview mode.
	if err := ash.SetOverviewModeAndWait(ctx, tconn, true); err != nil {
		s.Fatal("Failed to set overview mode: ", err)
	}
	defer ash.SetOverviewModeAndWait(cleanupCtx, tconn, false)

	// 1. Test that desk bar will be transformed to expanded
	// state when dragging a window, and the desk bar never
	// goes back to zero state, even when dropping the window
	// outside of the new desk button.

	ws, err := ash.GetAllWindows(ctx, tconn)
	if len(ws) != 1 {
		s.Fatalf("Unexpected number of windows; got %d window(s), expected 1 window", len(ws))
	}
	bw := ws[0]

	// Drag the window to the new location without dropping
	// it, desk bar should be transformed to expanded state
	// immediately at the beginning of drag.
	if bw.OverviewInfo == nil {
		s.Fatal("Expected window to have overview info")
	}
	startLoc := bw.OverviewInfo.Bounds.CenterPoint()
	newLoc := coords.NewPoint(startLoc.X+10, startLoc.Y-10)
	if err := uiauto.Combine("move mouse on the chrome window and then drag the window",
		mouse.Move(tconn, startLoc, 0),
		mouse.Press(tconn, mouse.LeftButton),
		mouse.Move(tconn, newLoc, time.Second),
	)(ctx); err != nil {
		s.Fatal("Failed to drag browser to the new desk button: ", err)
	}

	ac := uiauto.New(tconn)

	// Desk bar should be at expanded state now.
	deskBarView := nodewith.ClassName(deskBarName)
	deskBarViewLoc, err := ac.Location(ctx, deskBarView)
	if err != nil {
		s.Fatal("Failed to get the location of the desk bar view: ", err)
	}
	if deskBarViewLoc.Height == deskBarZeroHeight {
		s.Fatalf("Failed to go to desk bar's expanded state, expected height to be greater than %d", deskBarZeroHeight)
	}

	// Now release the drag and verify the desk bar stays at the expanded state.
	if err := mouse.Release(tconn, mouse.LeftButton)(ctx); err != nil {
		s.Fatal("Failed to release the drag: ", err)
	}

	// Desk bar should stay at expanded state.
	deskBarViewLoc, err = ac.Location(ctx, deskBarView)
	if err != nil {
		s.Fatal("Failed to get the location of the desk bar view: ", err)
	}
	if deskBarViewLoc.Height == deskBarZeroHeight {
		s.Fatalf("Failed to keep desk bar at expanded state, expected a value greater than %d", deskBarZeroHeight)
	}

	// 2. Tests that dragging and dropping a window to the new desk button will create a new
	// desk and the window being dragged is moved to the new desk at the same time.

	newDeskButtonView := nodewith.ClassName(deskIconButton)
	newDeskButtonViewLoc, err := ac.Location(ctx, newDeskButtonView)
	if err != nil {
		s.Fatal(err, "Failed to get the location of the new desk button view")
	}

	// Drag the window to make it hover on top of the expanded new desk button for over 500ms,
	// verify that new desk button is transformed to active state from expanded state.
	if err := uiauto.Combine("drag the chrome window and make it hover on top of the new desk button for over 500ms",
		mouse.Move(tconn, startLoc, 0),
		mouse.Press(tconn, mouse.LeftButton),
		mouse.Move(tconn, newDeskButtonViewLoc.BottomRight(), time.Second),
		uiauto.Sleep(550*time.Millisecond),
	)(ctx); err != nil {
		s.Fatal("Failed to drag the window and hover it on top of the new desk button: ", err)
	}

	newDeskButtonViewLoc, err = ac.Location(ctx, newDeskButtonView)
	if err != nil {
		s.Fatal(err, "Failed to get the location of the new desk button view: ", err)
	}

	if newDeskButtonViewLoc.Width == deskIconButtonExpandedStateWidth {
		s.Fatalf("Failed to activate the new desk button, expected a value greater than %d", deskIconButtonExpandedStateWidth)
	}

	// The desk button has now expanded in width so we'll move the cursor to the centerpoint of its new location and finally release the drag.
	if err := uiauto.Combine("release the drag and wait for the animation of new desk button to be done",
		mouse.Move(tconn, newDeskButtonViewLoc.CenterPoint(), time.Second),
		mouse.Release(tconn, mouse.LeftButton),
		ac.WaitForLocation(newDeskButtonView),
	)(ctx); err != nil {
		s.Fatal("Failed to release the drag: ", err)
	}

	// Verify that the new desk button goes back to the expanded state from the active state.
	newDeskButtonViewLoc, err = ac.Location(ctx, newDeskButtonView)
	if err != nil {
		s.Fatal("Failed to get the location of the new desk button view: ", err)
	}

	if newDeskButtonViewLoc.Width != deskIconButtonExpandedStateWidth {
		s.Fatalf("Failed to go back to new desk button's expanded state, got: %d, expected: %d", newDeskButtonViewLoc.Width, deskIconButtonExpandedStateWidth)
	}

	// Verifies that a new desk is created.
	deskMiniViewsInfo, err := ash.FindDeskMiniViews(ctx, ac)
	if err != nil {
		s.Fatal("Failed to find desks: ", err)
	}
	if len(deskMiniViewsInfo) != 2 {
		s.Fatalf("Unexpected number of desks, got %d, expected %d", len(deskMiniViewsInfo), 2)
	}

	// Checks that the browser window is in the new desk. The new desk is inactive.
	ws, err = ash.GetAllWindows(ctx, tconn)
	if len(ws) != 1 {
		s.Fatalf("Unexpected number of windows; got %d window(s), expected 1 window", len(ws))
	}
	bw = ws[0]
	if bw.OnActiveDesk {
		s.Fatal("Browser window should be in the inactive desk")
	}
}
