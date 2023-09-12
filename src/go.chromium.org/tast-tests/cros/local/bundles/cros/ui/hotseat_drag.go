// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"time"

	uiperf "go.chromium.org/tast-tests/cros/local/bundles/cros/ui/perf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/perfutil"
	"go.chromium.org/tast-tests/cros/local/ui"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         HotseatDrag,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Measures the presentation time of dragging the hotseat in tablet mode",
		Contacts: []string{
			"cros-system-ui-eng@google.com",
			"tbarzic@chromium.org",
			"cros-shelf-prod-notifications@google.com",
		},
		// ChromeOS > Software > System UI Surfaces > Shelf
		BugComponent: "b:1288352",
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
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

func HotseatDrag(ctx context.Context, s *testing.State) {
	// Reserve a few seconds for various cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr := s.FixtValue().(*chrome.Chrome)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	orientation, err := display.GetOrientation(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to obtain the display rotation: ", err)
	}

	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, true)
	if err != nil {
		s.Fatal("Failed to ensure in tablet mode: ", err)
	}
	defer cleanup(cleanupCtx)

	// Prepare the touch screen as this test requires touch scroll events.
	tsw, err := input.Touchscreen(ctx)
	if err != nil {
		s.Fatal("Failed to create touch screen event writer: ", err)
	}
	defer tsw.Close(ctx)

	if err = tsw.SetRotation(-orientation.Angle); err != nil {
		s.Fatal("Failed to set rotation: ", err)
	}

	stw, err := tsw.NewSingleTouchWriter()
	if err != nil {
		s.Fatal("Failed to create single touch writer: ", err)
	}
	defer stw.Close()

	// Open a browser window depending on the given browser type.
	conn, _, closeBrowser, err := browserfixt.SetUpWithURL(ctx, cr, s.Param().(browser.Type), ui.PerftestURL)
	if err != nil {
		s.Fatal("Failed to open browser window: ", err)
	}
	defer closeBrowser(cleanupCtx)
	defer conn.Close()

	// Note that ash-chrome `cr` and `tconn` is passed in to take traces and metrics from ash-chrome.
	if perfutil.RunMultipleAndSave(ctx, s.OutDir(), cr.Browser(), uiperf.Run(s, perfutil.RunAndWaitAll(tconn, func(ctx context.Context) error {
		ws, err := ash.GetAllWindows(ctx, tconn)
		if err != nil {
			s.Fatal("Failed to obtain the window list: ", err)
		}
		if len(ws) == 0 {
			s.Fatal("Failed to find any windows")
		}

		startX := tsw.Width() / 2
		startY := tsw.Height() - 1

		endX := tsw.Width() / 2
		// Scroll 1/4th of the screen to guarantee the hotseat is dragged the full
		// amount.
		endY := tsw.Height() * 3 / 4

		if err := stw.Swipe(ctx, startX, startY, endX, endY, time.Second); err != nil {
			return errors.Wrap(err, "failed to execute a swipe gesture")
		}

		if err := stw.End(); err != nil {
			return errors.Wrap(err, "failed to finish the swipe gesture")
		}
		if err := ash.WaitWindowFinishAnimating(ctx, tconn, ws[0].ID); err != nil {
			return errors.Wrap(err, "failed to wait")
		}

		return ash.SetOverviewModeAndWait(ctx, tconn, false)
	},
		"Ash.HotseatTransition.Drag.PresentationTime",
		"Ash.HotseatTransition.Drag.PresentationTime.MaxLatency")),
		perfutil.StoreLatency,
		perfutil.RunnerOptions{IgnoreFirstRun: true, DropMinMaxValues: true},
	); err != nil {
		s.Fatal("Failed to run or save: ", err)
	}
}
