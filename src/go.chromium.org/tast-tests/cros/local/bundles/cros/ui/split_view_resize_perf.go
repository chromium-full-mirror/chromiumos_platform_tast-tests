// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/chrome/histogram"
	"go.chromium.org/tast-tests/cros/common/perf"
	uiperf "go.chromium.org/tast-tests/cros/local/bundles/cros/ui/perf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/pointer"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/perfutil"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/ui"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type splitViewResizeParam int

const (
	splitViewResizeClamshell splitViewResizeParam = iota
	splitViewResizeTablet
	splitViewResizeWebUI
)

func init() {
	testing.AddTest(&testing.Test{
		Func: SplitViewResizePerf,
		Desc: "Measures smoothness of resizing split view windows",
		Contacts: []string{
			"cros-sw-perf@google.com",
			"sammiequon@chromium.org",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Fixture:      "chromeLoggedIn",
		Params: []testing.Param{
			{
				Name:    "clamshell_mode",
				Val:     splitViewResizeClamshell,
				Timeout: 4 * time.Minute,
			},
			{
				Val:     splitViewResizeTablet,
				Timeout: 5 * time.Minute,
			},
		},
	})
}

func SplitViewResizePerf(ctx context.Context, s *testing.State) {
	cuj.WriteMetadataFile(ctx, s.TestName())

	// Reserve ten seconds for various cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Ensure display on to record ui performance correctly.
	if err := power.TurnOnDisplay(ctx); err != nil {
		s.Fatal("Failed to turn on display: ", err)
	}

	param := s.Param().(splitViewResizeParam)
	tabletMode := param != splitViewResizeClamshell
	webUITabStrip := param == splitViewResizeWebUI

	var cr *chrome.Chrome
	if webUITabStrip {
		var err error
		// TODO(mukai): remove this when WebUITabStrip is enabled by default.
		if cr, err = chrome.New(ctx, chrome.EnableFeatures("WebUITabStrip")); err != nil {
			s.Fatal("Failed to init: ", err)
		}
		defer cr.Close(cleanupCtx)
	} else {
		cr = s.FixtValue().(chrome.HasChrome).Chrome()
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, tabletMode)
	if err != nil {
		if tabletMode {
			s.Fatal("Failed to ensure in tablet mode: ", err)
		} else {
			s.Fatal("Failed to ensure in clamshell mode: ", err)
		}
	}
	defer cleanup(cleanupCtx)

	revertZoom, err := display.MinimizePrimaryDisplayZoomFactor(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to set the zoom factor of the primary display to minimum: ", err)
	}
	defer revertZoom(cleanupCtx, tconn)

	// Ensures landscape orientation so this test can assume that windows snap on
	// the left and right. Windows snap on the top and bottom in portrait-oriented
	// tablet mode. They snap on the left and right in portrait-oriented clamshell
	// mode, but there are active (although contentious) proposals to change that.
	orientation, err := display.GetOrientation(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to obtain the orientation info: ", err)
	}
	if orientation.Type == display.OrientationPortraitPrimary {
		info, err := display.GetPrimaryInfo(ctx, tconn)
		if err != nil {
			s.Fatal("Failed to get the primary display info: ", err)
		}
		if err = display.SetDisplayRotationSync(ctx, tconn, info.ID, display.Rotate90); err != nil {
			s.Fatal("Failed to rotate display: ", err)
		}
		defer display.SetDisplayRotationSync(cleanupCtx, tconn, info.ID, display.Rotate0)
	}

	var pc pointer.Context
	if tabletMode {
		pc, err = pointer.NewTouch(ctx, tconn)
		if err != nil {
			s.Fatal("Failed to set up the touch context: ", err)
		}
	} else {
		pc = pointer.NewMouse(tconn)
	}
	defer pc.Close(ctx)

	info, err := display.GetPrimaryInfo(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get the primary display info: ", err)
	}

	yCenter := info.WorkArea.CenterY()
	// Compute the coordinates for the actual test scenario: drag the divider
	// slightly left and then all the way right. The left snapped window should
	// shrink and then expand and become maximized.
	dragPoints := []coords.Point{
		info.WorkArea.CenterPoint(),
		coords.NewPoint(info.WorkArea.Left+info.WorkArea.Width/4, yCenter),
		coords.NewPoint(info.WorkArea.Left+info.WorkArea.Width-1, yCenter),
		info.WorkArea.CenterPoint(),
	}
	if tabletMode {
		// In tablet mode, the last point should be moved slightly right, so that
		// the split-view controller moves the divider slightly.
		dragPoints[3].X += info.WorkArea.Width / 20
	}
	ac := uiauto.New(tconn)

	// Testing 3 patterns;
	// SingleWindow: there's a single window which is snapped to the left.
	// WithOverview: one window is snapped to the left, and the right area is
	//   in the overview mode.
	// MultiWindow (tablet only): one window is snapped to the left and another
	//   window is snapped to the right.
	type testCaseSlice []struct {
		name       string
		numWindows int
		customPrep func(ctx context.Context) error
	}
	var testCases testCaseSlice
	var modeName string
	if tabletMode {
		testCases = testCaseSlice{
			{"SingleWindow", 1, func(ctx context.Context) error {
				if !webUITabStrip {
					return nil
				}
				// Click the toggle button to open the WebUI tabstrip.
				toggleButton := nodewith.Role(role.Button).NameContaining("toggle tab strip")
				if err := uiauto.Combine(
					"wait and click",
					ac.WaitForLocation(toggleButton),
					pc.Click(toggleButton),
					ac.WaitForLocation(toggleButton),
				)(ctx); err != nil {
					return err
				}
				// Disable automation features explicitly, so that further operations
				// won't be affected by accessibility events. See
				// https://crbug.com/1096719 and https://crbug.com/1111137.
				return tconn.ResetAutomation(ctx)
			}},
			{"WithOverview", 8, func(ctx context.Context) error {
				// No need to control the WebUI tabstrip, as it remains open on the
				// browser window of the left side.
				return nil
			}},
			{"MultiWindow", 8, func(ctx context.Context) error {
				// Additional preparation for the multi-window; by default the right side
				// should be in the overview mode, so here selects one of the windows.
				// First, find the window which is in the overview and obtains the center
				// point of its bounds.
				w, err := ash.FindFirstWindowInOverview(ctx, tconn)
				if err != nil {
					return err
				}
				id1 := w.ID
				if err := pc.ClickAt(w.OverviewInfo.Bounds.CenterPoint())(ctx); err != nil {
					return errors.Wrapf(err, "failed to tap the center of %d", id1)
				}
				if err := ash.WaitForCondition(ctx, tconn, func(w *ash.Window) bool {
					return w.ID == id1 && !w.IsAnimating && w.State == ash.WindowStateSecondarySnapped
				}, &testing.PollOptions{Timeout: 5 * time.Second}); err != nil {
					return errors.Wrap(err, "failed to wait for the condition")
				}

				if !webUITabStrip {
					return nil
				}

				// Open the WebUI tabstrip of the browser window of the right side.
				toggleButton := nodewith.Role(role.Button).NameContaining("toggle tab strip")
				nodes, err := ac.NodesInfo(ctx, toggleButton)
				if err != nil {
					return err
				}
				for i := range nodes {
					q := toggleButton.Nth(i)
					loc, err := ac.Location(ctx, q)
					if err != nil {
						return err
					}
					// The toggle button on the lefthand-side browser window should be
					// skipped.
					if loc.CenterPoint().X < info.Bounds.CenterPoint().X {
						continue
					}
					if err := uiauto.Combine(
						"click and wait",
						pc.ClickAt(loc.CenterPoint()),
						ac.WaitForLocation(q),
					)(ctx); err != nil {
						return err
					}
					break
				}
				// Disable automation features explicitly, so that further operations
				// won't be affected by accessibility events. See
				// https://crbug.com/1096719 and https://crbug.com/1111137.
				return tconn.ResetAutomation(ctx)
			}},
		}
		modeName = "TabletMode"
	} else {
		testCases = testCaseSlice{
			{"SingleWindow", 2, func(ctx context.Context) error {
				// Additional preparation for the empty overview grid. When we drag and
				// snap an overview item to achieve clamshell split view in the first
				// place, we need a second overview item so that overview stays nonempty;
				// otherwise overview will end. After entering clamshell split view, we
				// need to get rid of that extra overview item, but if we close it then
				// overview will end. So we have to create a second virtual desk and drag
				// the overview item thereto. This workflow may be too rare to warrant
				// performance testing, but the results also apply to the potentially
				// more frequent workflow where one display is in clamshell split view
				// with an empty overview grid while another display has overview items.
				if err := ash.CreateNewDesk(ctx, tconn); err != nil {
					return errors.Wrap(err, "failed to create a new desk")
				}
				if err := moveOverviewWindowToNextDesk(ctx, tconn, ac); err != nil {
					return errors.Wrap(err, "failed to move overview window to the next desk")
				}
				// Disable automation features explicitly, so that further operations
				// won't be affected by accessibility events. See
				// https://crbug.com/1096719 and https://crbug.com/1111137.
				return tconn.ResetAutomation(ctx)
			}},
			// 9 windows, including 1 on the extra virtual desk from the "SingleWindow" case.
			{"WithOverview", 9, func(context.Context) error { return nil }},
		}
		modeName = "ClamshellMode"
	}

	defer ash.SetOverviewModeAndWait(cleanupCtx, tconn, false)
	defer ash.CleanUpDesks(cleanupCtx, tconn)
	currentWindows := 0
	runner := perfutil.NewRunner(cr, perfutil.RunnerOptions{IgnoreFirstRun: true, DropMinMaxValues: true})
	var id0 int
	for i, testCase := range testCases {
		s.Run(ctx, testCase.name, func(ctx context.Context, s *testing.State) {
			if err := ash.CreateWindows(ctx, tconn, cr, ui.PerftestURL, testCase.numWindows-currentWindows); err != nil {
				s.Fatal("Failed to open windows: ", err)
			}
			currentWindows = testCase.numWindows

			if err := ash.SetOverviewModeAndWait(ctx, tconn, true); err != nil {
				s.Fatal("Failed to enter into the overview mode: ", err)
			}

			// In tablet-mode, this is needed only once as the left-snapped window
			// should stay as left-snapped.
			if !tabletMode || i == 0 {
				w, err := ash.FindFirstWindowInOverview(ctx, tconn)
				if err != nil {
					s.Fatal("Failed to find the window in the overview mode: ", err)
				}

				// id0 supposed to have the window id which is left-snapped.
				id0 = w.ID
				if err := ash.SetWindowStateAndWait(ctx, tconn, id0, ash.WindowStatePrimarySnapped); err != nil {
					s.Fatal("Failed to snap window: ", err)
				}
			}

			if err := testCase.customPrep(ctx); err != nil {
				s.Fatal("Failed to prepare: ", err)
			}

			const dividerSmoothnessName = "Ash.SplitViewResize.AnimationSmoothness.DividerAnimation"
			histogramNames := []string{
				fmt.Sprintf("Ash.SplitViewResize.PresentationTime.%s.%s", modeName, testCase.name),
				fmt.Sprintf("Ash.SplitViewResize.PresentationTime.MaxLatency.%s.%s", modeName, testCase.name),
			}

			// In tablet mode, there is a divider which snaps to the closest fixed ratio on release. The written histogram is a smoothness histogram.
			if modeName == "TabletMode" {
				histogramNames = append(histogramNames, dividerSmoothnessName)
			}
			gestures := []uiauto.Action{
				pc.DragTo(dragPoints[1], time.Second),
				pc.DragTo(dragPoints[2], time.Second),
				pc.DragTo(dragPoints[3], time.Second),
			}
			if !tabletMode {
				gestures = append(gestures, func(ctx context.Context) error {
					return ash.WaitForCondition(ctx, tconn, func(w *ash.Window) bool {
						return w.ID == id0 && w.BoundsInRoot.Right() == dragPoints[3].X
					}, &testing.PollOptions{Timeout: 2 * time.Second})
				})
			}
			// Note: in tablet mode, the split view divider will be still animating
			// at this point because ash.WaitForCondition does not check divider's
			// status. Still this is not a problem, as RunAndWaitAll function will
			// wait for the metrics for the divider animation which is generated
			// after the divider animation finishes.
			runner.RunMultiple(ctx, testCase.name, uiperf.Run(s,
				perfutil.RunAndWaitAll(tconn,
					uiauto.Combine("drag resizing the splitview",
						pc.Drag(dragPoints[0], gestures...),
						func(ctx context.Context) error {
							return ash.WaitForCondition(ctx, tconn, func(w *ash.Window) bool {
								return w.ID == id0 && !w.IsAnimating && w.State == ash.WindowStatePrimarySnapped
							}, &testing.PollOptions{Timeout: 2 * time.Second})
						},
					),
					histogramNames...,
				)),
				func(ctx context.Context, pv *perfutil.Values, hists []*histogram.Histogram) error {
					for _, hist := range hists {
						value, err := hist.Mean()
						if err != nil {
							return errors.Wrapf(err, "failed to get mean for histogram %s", hist.Name)
						}
						testing.ContextLog(ctx, hist.Name, " = ", value)
						unit := "ms"
						direction := perf.SmallerIsBetter
						if strings.Contains(hist.Name, "AnimationSmoothnes") {
							unit = "percent"
							direction = perf.BiggerIsBetter
						}
						pv.Append(perf.Metric{
							Name:      hist.Name,
							Unit:      unit,
							Direction: direction,
						}, value)
					}
					return nil
				})
		})
	}

	if err := runner.Values().Save(ctx, s.OutDir()); err != nil {
		s.Error("Failed saving perf data: ", err)
	}
}

func moveOverviewWindowToNextDesk(ctx context.Context, tconn *chrome.TestConn, ac *uiauto.Context) error {
	kb, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get keyboard")
	}
	defer kb.Close(ctx)

	if err := uiauto.NamedCombine("move overview window to the next desk",
		// Focus on the window in the overview grid.
		kb.AccelAction("Tab"),
		ac.WaitUntilExists(nodewith.HasClass("OverviewItemView").Focused()),
		// Move the focused window to the next desk.
		kb.AccelAction("Search+Shift+]"),
	)(ctx); err != nil {
		return err
	}

	if err := ash.WaitForAllWindowCondition(ctx, tconn, func(win *ash.Window) bool {
		return win.OverviewInfo == nil
	}); err != nil {
		return errors.Wrap(err, "failed to wait for overview grid to be empty")
	}
	return nil
}
