// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wmp

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/pointer"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SplitChromeTabsTabletMode,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks that Chrome tabs are draggable to split screen",
		Contacts: []string{
			"chromeos-wm-corexp@google.com",
			"chromeos-sw-engprod@google.com",
			"sophiewen@chromium.org",
		},
		BugComponent: "b:1253115",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      2 * time.Minute,
		Params: []testing.Param{{
			Val:     browser.TypeAsh,
			Fixture: "chromeLoggedIn",
		}, {
			Name:              "lacros",
			Val:               browser.TypeLacros,
			Fixture:           "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
		}},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-433ab5fd-64cf-4f9d-8318-5acf68163ad4",
		}},
	})
}

func SplitChromeTabsTabletMode(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	bt := s.Param().(browser.Type)

	// Reserve ten seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	br, closeBrowser, err := browserfixt.SetUp(ctx, cr, bt)
	if err != nil {
		s.Fatal("Failed to set up browser: ", err)
	}
	defer closeBrowser(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// Open a Chrome window with three new tabs.
	url := "https://chrome://version"
	numNewTabs := 3
	// Lacro creates an extra blank tab in `browserfixt.SetUp`.
	if bt == browser.TypeLacros {
		numNewTabs--
	}
	for i := 0; i < numNewTabs; i++ {
		conn, err := br.NewConn(ctx, url)
		if err != nil {
			s.Fatalf("Failed to open new tab with url %q: %v", url, err)
		}
		defer conn.Close()
	}

	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, true)
	if err != nil {
		s.Fatal("Failed to ensure tablet mode: ", err)
	}
	defer cleanup(cleanupCtx)

	pc, err := pointer.NewTouch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to create a touch controller: ", err)
	}
	defer pc.Close(ctx)

	// Open the Chrome browser tab strip.
	tabStripButton := nodewith.Role(role.Button).HasClass("WebUITabCounterButton").First()
	if err := pc.Click(tabStripButton)(ctx); err != nil {
		s.Fatal("Failed to tap the tab strip button: ", err)
	}

	expectedNumWindows := 2
	if err := dragToSnap(ctx, tconn, pc, expectedNumWindows, ash.WindowStateSecondarySnapped); err != nil {
		s.Fatal("Failed to drag to snap right: ", err)
	}

	if err := dragToSnap(ctx, tconn, pc, expectedNumWindows+1, ash.WindowStatePrimarySnapped); err != nil {
		s.Fatal("Failed to drag to snap left: ", err)
	}
}

func dragToSnap(ctx context.Context, tconn *chrome.TestConn, pc pointer.Context, expectedNumWindows int, snappedState ash.WindowStateType) error {
	// Get the first tab thumbnail in the tab strip.
	tabRect, err := uiauto.New(tconn).Location(ctx, nodewith.Role(role.Tab).First())
	if err != nil {
		return errors.Wrap(err, "failed to get the tab thumbnail")
	}

	info, err := display.GetPrimaryInfo(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to get the primary display info")
	}
	snapPoint := coords.NewPoint(info.WorkArea.Right()-1, info.WorkArea.CenterY())
	if snappedState == ash.WindowStatePrimarySnapped {
		snapPoint = coords.NewPoint(info.WorkArea.Left, info.WorkArea.CenterY())
	}

	// Drag the first tab in the window and snap it to the side. Add sleep
	// to long press the tab thumbnail to be able to grab it.
	if err := pc.Drag(tabRect.CenterPoint(),
		action.Sleep(time.Second),
		pc.DragTo(snapPoint, 3*time.Second),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to drag the tab")
	}

	// Lacros needs to wait for both the dragged tab to be snapped in its new
	// window and the remaining tab to become its own window. Since the
	// remaining window does not exist until after the dragged tab is snapped
	// and therefore cannot be accessed, wait for it to be created by checking
	// `ash.GetAllWindows`.
	var ws []*ash.Window
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		var err error
		ws, err = ash.GetAllWindows(ctx, tconn)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to get windows"))
		}
		if len(ws) != expectedNumWindows {
			return errors.Errorf("unexpected number of windows; got %d, want %d", len(ws), expectedNumWindows)
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: 5 * time.Second}); err != nil {
		return errors.Wrap(err, "failed to wait for tab to become new window")
	}

	if ws[0].State != snappedState {
		return errors.New("failed to snap the tab")
	}

	return nil
}
