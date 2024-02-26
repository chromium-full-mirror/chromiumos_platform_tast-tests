// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"time"

	uiperf "go.chromium.org/tast-tests/cros/local/bundles/cros/ui/perf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
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
		Func:         TabletTransitionPerf,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Measures the animation smoothess of animating to and from tablet mode",
		Contacts:     []string{"cros-sw-perf@google.com", "chromeos-wm-corexp@google.com", "sammiequon@chromium.org"},
		// ChromeOS > Software > Window Management > TabletMode
		BugComponent: "b:1253116",
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Timeout:      3 * time.Minute,
	})
}

func TabletTransitionPerf(ctx context.Context, s *testing.State) {
	// Reserve five seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// Ensure display on to record ui performance correctly.
	if err := power.TurnOnDisplay(ctx); err != nil {
		s.Fatal("Failed to turn on display: ", err)
	}

	cr, err := chrome.New(ctx)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	const numWindows = 8
	if err := ash.CreateWindows(ctx, tconn, cr, ui.PerftestURL, numWindows); err != nil {
		s.Fatal("Failed to create windows: ", err)
	}

	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, false)
	if err != nil {
		s.Fatal("Failed to ensure in clamshell mode: ", err)
	}
	defer cleanup(cleanupCtx)

	// The top window (first window in the list returned by |ash.GetAllWindow|) needs to be normal window state otherwise no animation will occur.
	windows, err := ash.GetAllWindows(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to obtain the window list: ", err)
	}

	if err := ash.SetWindowStateAndWait(ctx, tconn, windows[0].ID, ash.WindowStateNormal); err != nil {
		s.Fatalf("Failed to set the window (%d): %v", windows[0].ID, err)
	}

	if err := perfutil.RunMultipleAndSave(ctx, s.OutDir(), cr.Browser(), uiperf.Run(s, perfutil.RunAndWaitAll(tconn, func(ctx context.Context) error {
		if err := ash.SetTabletModeEnabled(ctx, tconn, true); err != nil {
			return errors.Wrap(err, "failed to enable tablet mode")
		}

		// Wait for the top window to finish animating before changing states.
		if err := ash.WaitWindowFinishAnimating(ctx, tconn, windows[0].ID); err != nil {
			return errors.Wrap(err, "failed to wait for top window animation")
		}

		if err := ash.SetTabletModeEnabled(ctx, tconn, false); err != nil {
			return errors.Wrap(err, "failed to disable tablet mode")
		}

		if err := ash.WaitWindowFinishAnimating(ctx, tconn, windows[0].ID); err != nil {
			return errors.Wrap(err, "failed to wait for top window animation")
		}

		return nil
	},
		"Ash.TabletMode.AnimationSmoothness.Enter",
		"Ash.TabletMode.AnimationSmoothness.Exit")),
		perfutil.StoreSmoothness,
		perfutil.RunnerOptions{IgnoreFirstRun: true, DropMinMaxValues: true},
	); err != nil {
		s.Fatal("Failed to run or save: ", err)
	}
}
