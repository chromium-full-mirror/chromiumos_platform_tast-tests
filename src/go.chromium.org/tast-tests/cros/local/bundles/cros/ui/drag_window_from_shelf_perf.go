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
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/perfutil"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/ui"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: DragWindowFromShelfPerf,
		Desc: "Measures the presentation time of dragging a window from the shelf in tablet mode",
		Contacts: []string{
			"chromeos-wm@google.com",
			"cros-sw-perf@google.com",
			"tbarzic@chromium.org",
			"xdai@chromium.org",
		},
		// ChromeOS > Software > GestureNav
		BugComponent: "b:1253088",
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Fixture:      "chromeLoggedIn",
	})
}

func DragWindowFromShelfPerf(ctx context.Context, s *testing.State) {
	cuj.WriteMetadataFile(ctx, s.TestName())

	// Reserve a few seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
	defer cancel()

	// Ensure display on to record ui performance correctly.
	if err := power.TurnOnDisplay(ctx); err != nil {
		s.Fatal("Failed to turn on display: ", err)
	}

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

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

	const numWindows = 8
	const url = ui.PerftestURL

	// Open a first browser window.
	conn, err := cr.NewConn(ctx, url)
	if err != nil {
		s.Fatal("Failed to open the browser: ", err)
	}
	if err := conn.Close(); err != nil {
		s.Fatalf("Failed to close connection to url %v: %v", url, err)
	}

	// Open the rest of the windows.
	if err := ash.CreateWindows(ctx, tconn, cr, url, numWindows-1); err != nil {
		s.Fatal("Failed to open browser windows: ", err)
	}

	if err := perfutil.RunMultipleAndSave(ctx, s.OutDir(), cr, uiperf.Run(s, perfutil.RunAndWaitAll(tconn, func(ctx context.Context) error {
		if err := ash.DragToShowOverview(ctx, tsw, stw, tconn); err != nil {
			return errors.Wrap(err, "failed to drag from bottom of the screen to show overview")
		}
		// Clear the overview mode state so that the next drag can enter into the
		// overview mode.
		return ash.SetOverviewModeAndWait(ctx, tconn, false)
	},
		"Ash.DragWindowFromShelf.PresentationTime",
		"Ash.DragWindowFromShelf.PresentationTime.MaxLatency",
		"Ash.Overview.Enter.PresentationTime2.UserInitiatedTablet",
		"Ash.Overview.Exit.PresentationTime2")),
		perfutil.StoreLatency,
		perfutil.RunnerOptions{IgnoreFirstRun: true, DropMinMaxValues: true},
	); err != nil {
		s.Fatal("Failed to run or save: ", err)
	}
}
