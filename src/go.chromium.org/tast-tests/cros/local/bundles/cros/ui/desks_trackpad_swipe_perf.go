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
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/event"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
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
		Func:         DesksTrackpadSwipePerf,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Measures the performance of using the trackpad to change desks",
		Contacts: []string{
			"chromeos-wms@google.com",
			"chromeos-sw-engprod@google.com",
			"dandersson@google.com",
			"richui@google.com",
		},
		//  ChromeOS > Software > Window Management > Virtual Desks
		BugComponent: "b:1238200",
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(
			hwdep.InternalDisplay(),
			// Due to the varying sizes of touchpads on different models, it is hard to have one good swipe
			// motion reliably pass all models. Since this test is for collecting performance metrics, having it work
			// on most boards is good enough.
			hwdep.SkipOnModel("kohaku", "morphius", "samus"),
		),
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

func DesksTrackpadSwipePerf(ctx context.Context, s *testing.State) {
	// Reserve five seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
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

	// Add an extra desk and remove it at the end of the test.
	if err = ash.CreateNewDesk(ctx, tconn); err != nil {
		s.Fatal("Failed to create a new desk: ", err)
	}
	defer ash.CleanUpDesks(cleanupCtx, tconn)

	// Create a virtual trackpad.
	tpw, err := input.Trackpad(ctx)
	if err != nil {
		s.Fatal("Failed to create a trackpad device: ", err)
	}
	defer tpw.Close(ctx)

	tw, err := tpw.NewMultiTouchWriter(4)
	if err != nil {
		s.Fatal("Failed to create a multi touch writer: ", err)
	}
	defer tw.Close()

	histogramNames := []string{
		"Ash.Desks.AnimationSmoothness.DeskEndGesture",
		"Ash.Desks.PresentationTime.UpdateGesture",
		"Ash.Desks.PresentationTime.UpdateGesture.MaxLatency",
	}

	runner := perfutil.NewRunner(cr.Browser(), perfutil.RunnerOptions{IgnoreFirstRun: true, DropMinMaxValues: true})

	// Subtest 1: Desks switch with zero windows, not in overview mode.
	if err := runDesksTrackpadSwipeSubtest(ctx, s, tconn, runner, tpw, tw, false /* inOverview */, "NotInOverview" /* testName */, histogramNames); err != nil {
		s.Fatal("Test case with 2 desks, no windows and no overview failed: ", err)
	}

	// Subtest 2: Desks switch with 12 windows, in overview mode. We add 12
	// windows since windows affect overview performance directly, and
	// indirectly by adding extra mirrored layers to the desk bar.
	const numWindows = 12
	blankConn, br, closeBrowser, err := browserfixt.SetUpWithURL(ctx, cr, s.Param().(browser.Type), chrome.BlankURL)
	if err != nil {
		s.Fatal("Failed to set up the browser: ", err)
	}
	defer closeBrowser(cleanupCtx)
	defer blankConn.Close()

	if err := ash.CreateWindows(ctx, tconn, br, ui.PerftestURL, numWindows-1); err != nil {
		s.Fatal("Failed to create browser windows: ", err)
	}

	// Enter overview mode.
	if err := ash.SetOverviewModeAndWait(ctx, tconn, true); err != nil {
		s.Fatal("Failed to enter overview mode: ", err)
	}
	defer ash.SetOverviewModeAndWait(cleanupCtx, tconn, false)

	ac := uiauto.New(tconn)
	if err := ac.WithTimeout(time.Minute).WaitUntilNoEvent(nodewith.Root(), event.LocationChanged)(ctx); err != nil {
		s.Log("Failed to wait for overview stabilization: ", err)
	}

	// We exit and re-enter overview mode behind the scenes and are interested
	// in the presentation time of this operation in addition to the previous
	// metrics.
	histogramNames = append(histogramNames, "Ash.Overview.Enter.PresentationTime",
		"Ash.Overview.Exit.PresentationTime")

	if err := runDesksTrackpadSwipeSubtest(ctx, s, tconn, runner, tpw, tw, true /* inOverview */, "InOverview" /* testName */, histogramNames); err != nil {
		s.Fatal("Test case with 2 desks, 12 windows and overview failed: ", err)
	}

	if err := runner.Values().Save(ctx, s.OutDir()); err != nil {
		s.Error("Failed saving perf data: ", err)
	}
}

// runDesksTrackpadSwipeSubtest does trackpad swipes to trigger continuous desk
// animation multiple times.
func runDesksTrackpadSwipeSubtest(ctx context.Context, s *testing.State,
	tconn *chrome.TestConn, runner *perfutil.Runner, tpw *input.TrackpadEventWriter, tw *input.TouchEventWriter, inOverview bool, testName string,
	histogramNames []string) error {
	// Perform a four finger horizontal swipe on the trackpad. The vertical location is always vertically
	// centered on the trackpad. The fingers are spaced horizontally on the trackpad by 1/16th of the trackpad
	// width. Wait for desks to finish animating after the swipe.
	fingerSpacing := tpw.Width() / 16
	doTrackpadFourFingerSwipeScroll := func(ctx context.Context, x0, x1 input.TouchCoord) error {
		y := tpw.Height() / 2
		if err := tw.Swipe(ctx, x0, y, x1, y, fingerSpacing, 0, 4, time.Second); err != nil {
			return errors.Wrap(err, "failed to perform four finger scroll")
		}
		if err := tw.End(); err != nil {
			return errors.Wrap(err, "failed to finish trackpad scroll")
		}
		if err := ash.WaitUntilDesksFinishAnimating(ctx, tconn); err != nil {
			return errors.Wrap(err, "failed to wait for desks to finish animating")
		}
		return nil
	}

	// This is supposedly the distance (in trackpad units) spanned by the 4 fingers on the trackpad. It should
	// be fingerSpacing * 3 because if there are 4 fingers then there are only 3 spaces between fingers. We
	// still use fingerSpacing * 4, just to keep new performance data consistent with old performance data.
	fingerDistance := fingerSpacing * 4

	runner.RunMultiple(ctx, testName, uiperf.Run(s,
		perfutil.RunAndWaitAll(tconn,
			func(ctx context.Context) error {
				// Do a big swipe going right. This will continuously shift to the next desk on the right.
				// Note: The swipe should end at tpw.Width()-1-fingerDistance because a finger at tpw.Width() is
				// off the trackpad. However, fingerDistance is already more than it should be, so this works out.
				// Note that due to the presentation time of overview mode, we may not drag enough to move to the next
				// desk, but it is enough to partial show the next desk in the desk animation, and record overview
				// presentation time as a result. (i.e. at this time we commonly reach 1s of exit/enter presentation
				// time; the trackpad swipe as a result may already be halfway done)
				if err := doTrackpadFourFingerSwipeScroll(ctx, 0, tpw.Width()-fingerDistance); err != nil {
					return errors.Wrap(err, "failed to perform four finger scroll")
				}

				if inOverview {
					// In overview, swipe back so we don't have to enter overview again.
					if err := doTrackpadFourFingerSwipeScroll(ctx, tpw.Width()-fingerDistance, 0); err != nil {
						return errors.Wrap(err, "failed to perform four finger scroll back to starting desk")
					}
				} else {
					// Activate the desk at index 0 for the next run. Note that we could also swipe back like in
					// overview subtest, but that would affect the metrics already collected and displayed on Crosbolt.
					if err := ash.ActivateDeskAtIndex(ctx, tconn, 0); err != nil {
						return errors.Wrap(err, "failed to activate the first desk")
					}
				}

				return nil
			},
			histogramNames...,
		)),
		perfutil.StoreAllWithHeuristics(testName))

	return nil
}
