// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wmp

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/wmp/wmputils"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/pointer"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

const (
	// deskBarZeroStateHeight is the height of desk bar when it's at zero state.
	deskBarZeroStateHeight  = 40
	zeroStateIconButtonName = "ZeroStateIconButton"
	deskBarViewName         = "LegacyDeskBarView"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         OverviewDragWindowToNewDesk,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks that drag window to new desk in overview mode works correctly",
		Contacts: []string{
			"chromeos-wmp@google.com",
			"chromeos-sw-engprod@google.com",
			"conniekxu@chromium.org",
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
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	// Ensure there is no window open before test starts.
	if err := ash.CloseAllWindows(ctx, tconn); err != nil {
		s.Fatal("Failed to ensure no window is open: ", err)
	}

	ac := uiauto.New(tconn)

	pc := pointer.NewMouse(tconn)
	defer pc.Close(ctx)

	// Open a browser window.
	browserApp, err := apps.PrimaryBrowser(ctx, tconn)
	if err != nil {
		s.Fatal("Could not find browser app info: ", err)
	}
	if err := apps.Launch(ctx, tconn, browserApp.ID); err != nil {
		s.Fatal("Failed to launch chrome: ", err)
	}
	if err := ash.WaitForApp(ctx, tconn, browserApp.ID, time.Minute); err != nil {
		s.Fatal("Browser did not appear in shelf after launch: ", err)
	}
	// Ensure that there is only one open window that is the primary browser. Wait for the browser to be visible to avoid a race that may cause test flakiness.
	bt := s.Param().(browser.Type)
	bw, err := wmputils.EnsureOnlyBrowserWindowOpen(ctx, tconn, bt)
	if err != nil {
		s.Fatal("Expected the window to be fullscreen but got: ", err)
	}

	// Enter overview mode.
	if err := ash.SetOverviewModeAndWait(ctx, tconn, true); err != nil {
		s.Fatal("Failed to set overview mode: ", err)
	}
	defer ash.SetOverviewModeAndWait(cleanupCtx, tconn, false)

	// 1. Tests that desk bar will be transformed to expanded state when dragging a window
	// towards and close enough to the new desk button. And then dropping the window outside
	// of the new desk button will let desk bar go back to zero state.

	newDeskButtonView := nodewith.ClassName(zeroStateIconButtonName)
	newDeskButtonViewLoc, err := ac.Location(ctx, newDeskButtonView)
	if err != nil {
		s.Fatal(err, "failed to get the location of new desk button view")
	}

	ws, err := ash.GetAllWindows(ctx, tconn)
	if len(ws) != 1 {
		s.Fatalf("Got %d window(s), Expected 1 window", len(ws))
	}
	bw = ws[0]

	// Drag the window towoard to the new desk button without dropping it. Since it's close
	// enough to the new desk button, the desk bar view should be transformed to its expanded
	// state.
	if err := uiauto.Combine("move mouse on the chrome window and then drag the window to the new desk button",
		mouse.Move(tconn, bw.BoundsInRoot.CenterPoint(), 0),
		mouse.Press(tconn, mouse.LeftButton),
		mouse.Move(tconn, newDeskButtonViewLoc.CenterPoint(), 2*time.Second),
	)(ctx); err != nil {
		s.Fatal("Failed to drag browser to the new desk button")
	}

	// Desk bar should be at expanded state now.
	deskBarView := nodewith.ClassName(deskBarViewName)
	deskBarViewLoc, err := ac.Location(ctx, deskBarView)
	if err != nil {
		s.Fatal("Failed to get the location of the desk bar view: ", err)
	}
	if deskBarViewLoc.Height == deskBarZeroStateHeight {
		s.Fatalf("Failed to go to desk bar's expanded state, got: %v, expected: >%v", deskBarViewLoc.Height, deskBarZeroStateHeight)
	}

	// Continue dragging the window to the outside of the new desk button and then release mouse
	// which will drop the window. Since the window is dropped outside of the new desk button,
	// it will fall back to the current desk and the desk bar should be back to zero state.
	if err := uiauto.Combine("drag the window to the outside of the new desk button and then release mouse",
		mouse.Move(tconn, newDeskButtonViewLoc.CenterPoint().Add(coords.NewPoint(100, 100)), time.Second),
		mouse.Release(tconn, mouse.LeftButton),
	)(ctx); err != nil {
		s.Fatal("Failed to drag browser to the new desk button")
	}

	// Desk bar should be transformed back to the zero state.
	deskBarView = nodewith.ClassName(deskBarViewName)
	deskBarViewLoc, err = ac.Location(ctx, deskBarView)
	if err != nil {
		s.Fatal("Failed to get the location of the desk bar view: ", err)
	}
	if deskBarViewLoc.Height != deskBarZeroStateHeight {
		s.Fatal("Failed to go back to desk bar's zero state")
	}

	// 2. Tests that dragging and dropping a window to the new desk button will create a new
	// desk and the window being dragged is moved to the new desk at the same time.

	// Drag browser window to the new desk button.
	newDeskButtonView = nodewith.ClassName(zeroStateIconButtonName)
	newDeskButtonViewLoc, err = ac.Location(ctx, newDeskButtonView)
	if err != nil {
		s.Fatal(err, "Failed to get the location of the new desk button view")
	}

	if err := pc.Drag(
		bw.BoundsInRoot.CenterPoint(),
		pc.DragTo(newDeskButtonViewLoc.CenterPoint(), 2*time.Second))(ctx); err != nil {
		s.Fatal("Failed to drag browser window into the new desk button: ", err)
	}

	// Verifies that a new desk is created.
	deskMiniViewsInfo, err := ash.FindDeskMiniViews(ctx, ac)
	if err != nil {
		s.Fatal("Failed to find desks: ", err)
	}
	if len(deskMiniViewsInfo) != 2 {
		s.Fatalf("Got %v desks, want 2 desks", len(deskMiniViewsInfo))
	}

	// Checks that the browser window is in the new desk. The new desk is inactive.
	ws, err = ash.GetAllWindows(ctx, tconn)
	if len(ws) != 1 {
		s.Fatalf("Got %d window(s), Expected 1 window", len(ws))
	}
	bw = ws[0]
	if bw.OnActiveDesk == true {
		s.Fatal("Browser window should be in the inactive desk")
	}
}
