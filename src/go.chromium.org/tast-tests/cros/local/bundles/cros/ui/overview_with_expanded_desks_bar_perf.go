// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"fmt"
	"time"

	uiperf "go.chromium.org/tast-tests/cros/local/bundles/cros/ui/perf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/event"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
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
		Func:         OverviewWithExpandedDesksBarPerf,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Measures the performance of entering and exiting overview with the desks bar expanded",
		Contacts: []string{
			"chromeos-wm@google.com",
			"dandersson@google.com",
			"sammiequon@google.com",
			"yongshun@google.com",
		},
		// ChromeOS > Software > Window Management > OverviewMode
		BugComponent: "b:1252584",
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

func OverviewWithExpandedDesksBarPerf(ctx context.Context, s *testing.State) {
	// Reserve five seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// Ensure display on to record ui performance correctly.
	if err := power.TurnOnDisplay(ctx); err != nil {
		s.Fatal("Failed to turn on display: ", err)
	}

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	blankConn, br, closeBrowser, err := browserfixt.SetUpWithURL(ctx, cr, s.Param().(browser.Type), chrome.BlankURL)
	if err != nil {
		s.Fatal("Failed to set up the browser: ", err)
	}
	defer closeBrowser(cleanupCtx)
	defer blankConn.Close()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	// Remove any desks we may have created during the test.
	defer ash.CleanUpDesks(cleanupCtx, tconn)

	const numWindows = 16
	if err := ash.CreateWindows(ctx, tconn, br, ui.PerftestURL, numWindows-1); err != nil {
		s.Fatal("Failed to open browser windows: ", err)
	}

	runner := perfutil.NewRunner(cr.Browser(), perfutil.RunnerOptions{IgnoreFirstRun: true, DropMinMaxValues: true})

	// Maximize the active window so that the desks bar is created before the
	// overview window animation is started. If the screen isn't covered by
	// window(s), the desks bar will be created after the overview animation
	// is completed.
	activeWindow, err := ash.GetActiveWindow(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get active window: ", err)
	}
	if err := ash.SetWindowStateAndWait(ctx, tconn, activeWindow.ID, ash.WindowStateMaximized); err != nil {
		s.Fatal("Failed to maximize active window: ", err)
	}

	if err := runOverviewWithExpandedDesksBarSubtest(ctx, s, tconn, runner, 2 /* desks */); err != nil {
		s.Fatal("Test case with 2 desks failed: ", err)
	}

	if err := runOverviewWithExpandedDesksBarSubtest(ctx, s, tconn, runner, 8 /* desks */); err != nil {
		s.Fatal("Test case with 8 desks failed: ", err)
	}

	if err := runner.Values().Save(ctx, s.OutDir()); err != nil {
		s.Error("Failed saving perf data: ", err)
	}
}

// runOverviewWithExpandedDesksBarSubtest creates the number of desks needed and
// then enters and exits overview multiple times.
func runOverviewWithExpandedDesksBarSubtest(ctx context.Context, s *testing.State,
	tconn *chrome.TestConn, runner *perfutil.Runner, desks int) error {
	for {
		dc, err := ash.GetDeskCount(ctx, tconn)
		if err != nil {
			return errors.Wrap(err, "failed to count desks")
		}

		if dc > desks {
			return errors.Wrap(err, "too many desks")
		}

		if dc == desks {
			break
		}

		if err = ash.CreateNewDesk(ctx, tconn); err != nil {
			return errors.Wrap(err, "failed to create new desk")
		}
	}

	ui := uiauto.New(tconn)
	if err := ui.WithTimeout(time.Minute).WaitUntilNoEvent(nodewith.Root(), event.LocationChanged)(ctx); err != nil {
		s.Log("Failed to wait for overview stabilization: ", err)
	}

	name := fmt.Sprintf("%ddesks", desks)
	runner.RunMultiple(ctx, name, uiperf.Run(s,
		perfutil.RunAndWaitAll(tconn,
			func(ctx context.Context) error {
				if err := ash.SetOverviewModeAndWait(ctx, tconn, true); err != nil {
					return errors.Wrap(err, "failed to enter into the overview mode")
				}
				if err := ash.SetOverviewModeAndWait(ctx, tconn, false); err != nil {
					return errors.Wrap(err, "failed to exit from the overview mode")
				}
				return nil
			},
			"Ash.Overview.AnimationSmoothness.Enter.SingleClamshellMode",
			"Ash.Overview.AnimationSmoothness.Exit.SingleClamshellMode",
			"Ash.Overview.Enter.PresentationTime",
			"Ash.Overview.Exit.PresentationTime",
		)),
		perfutil.StoreAllWithHeuristics(name))

	return nil
}
