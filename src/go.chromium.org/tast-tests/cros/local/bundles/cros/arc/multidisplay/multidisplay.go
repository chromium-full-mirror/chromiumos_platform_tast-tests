// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package multidisplay consists of shared test structure for multidisplay (virtual or physical)
package multidisplay

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/arc/wm"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/media/imgcmp"
	"go.chromium.org/tast-tests/cros/local/personalization"
	"go.chromium.org/tast-tests/cros/local/screenshot"
	virtualmultidisplay "go.chromium.org/tast-tests/cros/local/virtualmultidisplay"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	dispPkg = "org.chromium.arc.testapp.multidisplay"
	dispApk = "ArcMultiDisplayTest.apk"

	settingsPkgMD = "com.android.settings"
	settingsActMD = ".Settings"
)

var defaultPollOptions = &testing.PollOptions{Timeout: 5 * time.Second}

// Power state for displays.
type displayPowerState int

// As defined in DisplayPowerState here:
// https://cs.chromium.org/chromium/src/third_party/cros_system_api/dbus/service_constants.h
const (
	displayPowerAllOn                 displayPowerState = 0
	displayPowerAllOff                displayPowerState = 1
	displayPowerInternalOffExternalOn displayPowerState = 2
	displayPowerInternalOnExternalOff displayPowerState = 3
)

type testFunc func(context.Context, *testing.State, *chrome.Chrome, *arc.ARC, virtualmultidisplay.VirtualDisplayController) error
type testEntry struct {
	name string
	fn   testFunc
}

// StableTestSet contains all tests that currently are passing normally and are safe for presubmit.
var StableTestSet = []testEntry{
	// Based on http://b/63773037 and http://b/140056612.
	{"Relayout displays", relayoutDisplays},
}

// AndroidVM contains tests that are currently only passing on android VM (not container P).
var AndroidVM = []testEntry{
	// Based on http://b/130897153.
	{"Drag a window between displays", dragWindowBetweenDisplays},
	{"Remove and re-add displays", removeAddDisplay},
	{"Rotate display", rotateDisplay},
	// Based on http://b/129564108.
	{"Launch activity on external display", launchActivityOnExternalDisplay},
	// Broken because it relies on launching on external display.
	{"Snapping", snappingOnDisplay},
	// Based on http://b/110105532.
	{"Activity is visible when other is maximized", maximizeVisibility},
}

// SharedVirtualPhysical is the implementation of the above tests.  It utilizes displayController (if present) to set the displays up when necessary, otherwise it assumes the displays are physical.
func SharedVirtualPhysical(ctx context.Context, s *testing.State, dc virtualmultidisplay.VirtualDisplayController) {
	cr := s.FixtValue().(*arc.PreData).Chrome
	a := s.FixtValue().(*arc.PreData).ARC

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// Enable a second display for the test.
	if dc != nil {
		dc.EnableDisplay(1)
	}

	displayInfos, err := WaitForDisplaysToPopulate(ctx, s, tconn, 2)
	if err != nil {
		s.Fatal("Failed to get display info: ", err)
	}

	// TODO(ruanc): This part can be removed if hardware dependency for multi-display is available.
	if len(displayInfos) != 2 {
		s.Fatalf("Not enough connected displays: got %d; want 2", len(displayInfos))
	}

	ui := uiauto.New(tconn).WithTimeout(30 * time.Second)
	if err := uiauto.Combine("Enable light mode",
		personalization.OpenPersonalizationHub(ui),
		personalization.ToggleLightMode(ui),
	)(ctx); err != nil {
		s.Fatal("Failed to enable light mode: ", err)
	}

	if err := a.Install(ctx, arc.APKPath(wm.APKNameArcWMTestApp24)); err != nil {
		s.Fatal("Failed installing app: ", err)
	}

	if err := a.Install(ctx, arc.APKPath(dispApk)); err != nil {
		s.Fatal("Failed installing app: ", err)
	}

	// Make sure the device is clamshell mode.
	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, false)
	if err != nil {
		s.Fatal("Failed to set tablet mode disabled: ", err)
	}
	defer cleanup(cleanupCtx)
	// TODO(b/187788935): GoBigSleepLint Wait for "tablet mode animation is finished" in a reliable way.
	// If an activity is launched while the tablet mode animation is active, the activity
	// will be launched in an undefined state, making the test flaky.
	if err := testing.Sleep(ctx, 5*time.Second); err != nil {
		s.Fatal("Failed to wait until tablet-mode animation finished: ", err)
	}

	for idx, test := range s.Param().([]testEntry) {
		if !runOrFatal(ctx, s, test.name, func(ctx context.Context, s *testing.State) error {
			return test.fn(ctx, s, cr, a, dc)
		}) {
			for _, info := range displayInfos {
				path := fmt.Sprintf("%s/screenshot-multi-display-failed-test-%d-%q.png", s.OutDir(), idx, info.ID)
				if err := screenshot.CaptureChromeForDisplay(ctx, cr, info.ID, path); err != nil {
					s.Logf("Failed to capture screenshot for display ID %q: %v", info.ID, err)
				}
			}
		}
	}
}

// launchActivityOnExternalDisplay launches the activity directly on the external display.
func launchActivityOnExternalDisplay(ctx context.Context, s *testing.State, cr *chrome.Chrome, a *arc.ARC, dc virtualmultidisplay.VirtualDisplayController) error {
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return err
	}

	displays, err := internalAndExternalDisplays(ctx, tconn, dc)
	if err != nil {
		return err
	}

	for _, test := range []struct {
		name    string
		actName string
	}{
		{"Launch resizeable activity on the external display", wm.ResizableUnspecifiedActivity},
		{"Launch unresizeable activity on the external display", wm.NonResizableUnspecifiedActivity},
	} {
		runOrFatal(ctx, s, test.name, func(ctx context.Context, s *testing.State) error {
			_, externalARCDisplayID, err := arcDisplayIDs(ctx, a, dc)
			if err != nil {
				return err
			}

			act, err := arc.NewActivityOnDisplay(a, wm.Pkg24, test.actName, externalARCDisplayID)
			if err != nil {
				return err
			}
			defer act.Close(ctx)
			if err := act.Start(ctx, tconn, arc.WithDisplayID(externalARCDisplayID)); err != nil {
				return err
			}
			defer act.Stop(ctx, tconn)

			return ensureWindowOnDisplay(ctx, tconn, wm.Pkg24, displays.external.ID)
		})
	}

	return nil
}

// maximizeVisibility checks whether the window is visible on one display if another window is maximized on the other display.
func maximizeVisibility(ctx context.Context, s *testing.State, cr *chrome.Chrome, a *arc.ARC, dc virtualmultidisplay.VirtualDisplayController) error {
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return err
	}

	// Set background to a color that won't interfere with the test.
	wm.SetSolidWhiteWallpaper(ctx, tconn, s)

	_, externalARCDisplayID, err := arcDisplayIDs(ctx, a, dc)
	if err != nil {
		return errors.Wrap(err, "could not get arc display ids")
	}

	// Start settings activity and set it to normal window state.
	settingsAct, err := arc.NewActivity(a, settingsPkgMD, settingsActMD)
	if err != nil {
		return errors.Wrap(err, "could not create new activity")
	}
	defer settingsAct.Close(ctx)

	if err := settingsAct.Start(ctx, tconn); err != nil {
		return errors.Wrap(err, "could not start new activity")
	}
	defer settingsAct.Stop(ctx, tconn)

	if err := ensureSetWindowState(ctx, tconn, settingsPkgMD, ash.WindowStateNormal); err != nil {
		return errors.Wrap(err, "activity in incorrect state")
	}

	// Start WM activity on the external display and set it to normal window state.
	wmAct, err := arc.NewActivityOnDisplay(a, wm.Pkg24, wm.ResizableUnspecifiedActivity, externalARCDisplayID)
	if err != nil {
		return errors.Wrap(err, "could not create activity on external display")
	}
	defer wmAct.Close(ctx)

	displays, err := internalAndExternalDisplays(ctx, tconn, dc)
	if err != nil {
		return errors.Wrap(err, "could not get internal and external displays")
	}

	if err := wmAct.Start(ctx, tconn, arc.WithDisplayID(externalARCDisplayID)); err != nil {
		if scerr := grabScreenshotOfDisplaysForFailedTests(ctx, cr, displays, "maximize-visibility"); scerr != nil {
			s.Error("Failed to start activity on the external display and could not grab screenshot")
		}
		return errors.Wrap(err, "could not start new activity on external display")
	}
	defer wmAct.Stop(ctx, tconn)

	if err := ensureWindowOnDisplay(ctx, tconn, wm.Pkg24, displays.external.ID); err != nil {
		return errors.Wrap(err, "activity not on external display")
	}

	if err := ensureSetWindowState(ctx, tconn, wm.Pkg24, ash.WindowStateNormal); err != nil {
		return errors.Wrap(err, "activity not in normal state")
	}

	// Preserve WindowInfo.
	wmWinInfo, err := ash.GetARCAppWindowInfo(ctx, tconn, wm.Pkg24)
	if err != nil {
		return errors.Wrap(err, "error getting arc window info")
	}

	settingsWinInfo, err := ash.GetARCAppWindowInfo(ctx, tconn, settingsPkgMD)
	if err != nil {
		return errors.Wrap(err, "error getting arc window info")
	}

	for _, test := range []struct {
		name       string
		maxAct     *arc.Activity
		maxPkgName string

		checkPkgName    string
		checkAppWinInfo *ash.Window
	}{
		{"Maximize the activity on primary display", settingsAct, settingsPkgMD, wm.Pkg24, wmWinInfo},
		{"Maximize the activity on external display", wmAct, wm.Pkg24, settingsPkgMD, settingsWinInfo},
	} {
		runOrFatal(ctx, s, test.name, func(ctx context.Context, s *testing.State) error {
			if err := ensureSetWindowState(ctx, tconn, test.maxPkgName, ash.WindowStateMaximized); err != nil {
				return errors.Wrap(err, "activity not in maximized state")
			}
			if err := ensureWindowStable(ctx, tconn, test.checkPkgName, test.checkAppWinInfo); err != nil {
				return errors.Wrap(err, "could not ensure window stable")
			}
			// The black window shows when the activity is not visible on Android side (see: http://b/110105532).
			if err := ensureNoBlackBkg(ctx, cr, tconn); err != nil {
				return errors.Wrap(err, "window has a black background")
			}
			// Reset maximized window to normal.
			return ensureSetWindowState(ctx, tconn, test.maxPkgName, ash.WindowStateNormal)
		})
	}

	return nil
}

// relayoutDisplays checks whether the window moves position when relayout displays.
func relayoutDisplays(ctx context.Context, s *testing.State, cr *chrome.Chrome, a *arc.ARC, dc virtualmultidisplay.VirtualDisplayController) error {
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get test api connection")
	}

	_, externalARCDisplayID, err := arcDisplayIDs(ctx, a, dc)
	if err != nil {
		return errors.Wrap(err, "failed to get first external display")
	}

	displays, err := internalAndExternalDisplays(ctx, tconn, dc)
	if err != nil {
		return errors.Wrap(err, "could not get internal/external displays")
	}

	// Start settings Activity on internal display.
	settingsAct, err := arc.NewActivity(a, settingsPkgMD, settingsActMD)
	if err != nil {
		return errors.Wrap(err, "could not create settings activity")
	}
	defer settingsAct.Close(ctx)

	if err := settingsAct.Start(ctx, tconn); err != nil {
		return errors.Wrap(err, "could not start settings activity")
	}
	defer settingsAct.Stop(ctx, tconn)
	if err := ash.WaitForVisible(ctx, tconn, settingsPkgMD); err != nil {
		return errors.Wrap(err, "settings activity not visible")
	}

	// Start wm Activity on external display.
	wmAct, err := arc.NewActivityOnDisplay(a, wm.Pkg24, wm.ResizableUnspecifiedActivity, externalARCDisplayID)
	if err != nil {
		return errors.Wrap(err, "could not create ARC activity")
	}
	defer wmAct.Close(ctx)

	if err := wmAct.Start(ctx, tconn, arc.WithDisplayID(externalARCDisplayID)); err != nil {
		return errors.Wrap(err, "could not start ARC activity")
	}
	defer wmAct.Stop(ctx, tconn)
	if err := ash.WaitForVisible(ctx, tconn, wm.Pkg24); err != nil {
		return errors.Wrap(err, "ARC activity did not become visible")
	}

	for _, test := range []struct {
		name        string
		windowState ash.WindowStateType
	}{
		{"Windows are normal", ash.WindowStateNormal},
		{"Windows are maximized", ash.WindowStateMaximized},
	} {
		runOrFatal(ctx, s, test.name, func(ctx context.Context, s *testing.State) error {
			if err := ensureSetWindowState(ctx, tconn, settingsPkgMD, test.windowState); err != nil {
				return errors.Wrap(err, "incorrect window state")
			}
			settingsWindowInfo, err := ash.GetARCAppWindowInfo(ctx, tconn, settingsPkgMD)
			if err != nil {
				return errors.Wrap(err, "could not get window info")
			}

			if err := ensureSetWindowState(ctx, tconn, wm.Pkg24, test.windowState); err != nil {
				return errors.Wrap(err, "incorrect window state")
			}
			wmWindowInfo, err := ash.GetARCAppWindowInfo(ctx, tconn, wm.Pkg24)
			if err != nil {
				return errors.Wrap(err, "could not get window info")
			}

			// Relayout external display and make sure the windows will not move their positions or show black background.
			for _, relayout := range []struct {
				name   string
				offset coords.Point
			}{
				{"Relayout external display to the left side of internal display", coords.NewPoint(-displays.external.Bounds.Width, 0)},
				{"Relayout external display to the right side of internal display", coords.NewPoint(displays.internal.Bounds.Width, 0)},
				{"Relayout external display on top of internal display", coords.NewPoint(0, -displays.external.Bounds.Height)},
				{"Relayout external display on bottom of internal display", coords.NewPoint(0, displays.internal.Bounds.Height)},
			} {
				runOrFatal(ctx, s, relayout.name, func(ctx context.Context, s *testing.State) error {
					p := display.DisplayProperties{BoundsOriginX: &relayout.offset.X, BoundsOriginY: &relayout.offset.Y}
					if err := display.SetDisplayProperties(ctx, tconn, displays.external.ID, p); err != nil {
						return errors.Wrap(err, "failed to set display properties")
					}
					if err := ensureWindowStable(ctx, tconn, settingsPkgMD, settingsWindowInfo); err != nil {
						return errors.Wrap(err, "settings window not stable")
					}
					if err := ensureWindowStable(ctx, tconn, wm.Pkg24, wmWindowInfo); err != nil {
						return errors.Wrap(err, "ARC window not stable")
					}
					return nil
				})
			}

			return nil
		})
	}

	return nil
}

// removeAddDisplay checks whether the window moves to another display and shows inside of display.
// After adding the display back without changing windows, it checks whether the window restores to the previous display.
func removeAddDisplay(ctx context.Context, s *testing.State, cr *chrome.Chrome, a *arc.ARC, dc virtualmultidisplay.VirtualDisplayController) (retErr error) {
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "could not create test api connection")
	}

	_, externalARCDisplayID, err := arcDisplayIDs(ctx, a, dc)
	if err != nil {
		return errors.Wrap(err, "could not get arc display ids")
	}

	info, err := internalAndExternalDisplays(ctx, tconn, dc)
	if err != nil {
		return errors.Wrap(err, "could not get display info")
	}
	intDispInfo := info.internal
	extDispInfo := info.external

	// Start settings Activity on internal display.
	settingsAct, err := arc.NewActivity(a, settingsPkgMD, settingsActMD)
	if err != nil {
		return errors.Wrap(err, "could not create settings activity")
	}
	defer settingsAct.Close(ctx)

	if err := settingsAct.Start(ctx, tconn); err != nil {
		return errors.Wrap(err, "could not start settings activity")
	}
	defer settingsAct.Stop(ctx, tconn)
	if err := ensureActivityReady(ctx, tconn, settingsAct); err != nil {
		return errors.Wrap(err, "settings activity not ready")
	}

	// Start wm Activity on external display.
	wmAct, err := arc.NewActivityOnDisplay(a, wm.Pkg24, wm.ResizableUnspecifiedActivity, externalARCDisplayID)
	if err != nil {
		return errors.Wrap(err, "could not create ARC activity")
	}
	defer wmAct.Close(ctx)

	if err := wmAct.Start(ctx, tconn, arc.WithDisplayID(externalARCDisplayID)); err != nil {
		return errors.Wrap(err, "could not start ARC activity")
	}
	defer wmAct.Stop(ctx, tconn)
	if err := ensureActivityReady(ctx, tconn, wmAct); err != nil {
		return errors.Wrap(err, "ARC activity not ready")
	}

	// Set windows to normal window state.
	if err := ensureSetWindowState(ctx, tconn, settingsPkgMD, ash.WindowStateNormal); err != nil {
		return errors.Wrap(err, "settings activity not normal")
	}
	if err := ensureActivityReady(ctx, tconn, settingsAct); err != nil {
		return errors.Wrap(err, "settings activity not ready")
	}

	if err := ensureSetWindowState(ctx, tconn, wm.Pkg24, ash.WindowStateNormal); err != nil {
		return errors.Wrap(err, "ARC activity not normal")
	}
	if err := ensureActivityReady(ctx, tconn, wmAct); err != nil {
		return errors.Wrap(err, "ARC activity not ready")
	}

	settingsWindowInfo, err := ash.GetARCAppWindowInfo(ctx, tconn, settingsPkgMD)
	if err != nil {
		return errors.Wrap(err, "could not get settings window info")
	}

	wmWindowInfo, err := ash.GetARCAppWindowInfo(ctx, tconn, wm.Pkg24)
	if err != nil {
		return errors.Wrap(err, "could not get ARC window info")
	}

	for _, removeAdd := range []struct {
		name                 string
		power                displayPowerState
		origDispInfo         display.Info
		origDisplayVirtualID int
		destDispInfo         display.Info
		destDisplayVirtualID int

		moveAct     *arc.Activity
		moveWinInfo *ash.Window
	}{
		// When removing internal display, the window on internal display will move to the external display.
		{"Remove and add internal display", displayPowerInternalOffExternalOn, intDispInfo, 0, extDispInfo, 1, settingsAct, settingsWindowInfo},
		// When removing external display, the window on external display will move to the internal display.
		{"Remove and add external display", displayPowerInternalOnExternalOff, extDispInfo, 1, intDispInfo, 0, wmAct, wmWindowInfo},
	} {
		runOrFatal(ctx, s, removeAdd.name, func(ctx context.Context, s *testing.State) error {
			// Remove one display and the window on the removed display should move to the other display.
			if err := setDisplayPower(ctx, dc, removeAdd.power); err != nil {
				return errors.Wrap(err, "error setting power")
			}
			// TODO: Check display power state to avoid setting display power redundantly.
			defer func() {
				if err := setDisplayPower(ctx, dc, displayPowerAllOn); err != nil && retErr == nil {
					retErr = errors.Wrap(err, "during removeAddDisplay cleanup")
				}
			}()
			// Wait for display off.
			if err := waitForDisplay(ctx, tconn, dc, removeAdd.origDisplayVirtualID, removeAdd.origDispInfo.ID, false, 30*time.Second); err != nil {
				return errors.Wrap(err, "error waiting for display to be off")
			}
			// Wait for display on.
			if err := waitForDisplay(ctx, tconn, dc, removeAdd.destDisplayVirtualID, removeAdd.destDispInfo.ID, true, 30*time.Second); err != nil {
				return errors.Wrap(err, "error waiting for display to be on")
			}
			if err := ensureActivityReady(ctx, tconn, removeAdd.moveAct); err != nil {
				return errors.Wrap(err, "activity is not ready")
			}

			// Check if the window moves to required display automatically.
			newWinInfo, err := ash.GetARCAppWindowInfo(ctx, tconn, removeAdd.moveAct.PackageName())
			if err != nil {
				return errors.Wrap(err, "error getting arc window info")
			}
			if newWinInfo.DisplayID != removeAdd.destDispInfo.ID {
				return errors.Wrapf(err, "failed to move window to another display: got %s; want %s", newWinInfo.DisplayID, removeAdd.destDispInfo.ID)
			}

			if err := ensureWinBoundsInDisplay(newWinInfo.BoundsInRoot, removeAdd.destDispInfo.Bounds); err != nil {
				return errors.Wrap(err, "window bounds not in display")
			}

			// Re-add display and the window should move back to the original display.
			if err := setDisplayPower(ctx, dc, displayPowerAllOn); err != nil {
				return errors.Wrap(err, "error waiting for display to be re-added")
			}
			// Wait for display on.
			if err := waitForDisplay(ctx, tconn, dc, removeAdd.origDisplayVirtualID, removeAdd.origDispInfo.ID, true, 30*time.Second); err != nil {
				return errors.Wrap(err, "error waiting for display to be on again")
			}
			if err := ensureActivityReady(ctx, tconn, removeAdd.moveAct); err != nil {
				return errors.Wrap(err, "error waiting for display to be ready again")
			}
			var restoreWinBounds coords.Rect
			err = testing.Poll(ctx, func(ctx context.Context) error {
				restoreWinInfo, err := ash.GetARCAppWindowInfo(ctx, tconn, removeAdd.moveAct.PackageName())
				if err != nil {
					return errors.Wrap(err, "could not get arc window info")
				}
				if restoreWinInfo.DisplayID != removeAdd.moveWinInfo.DisplayID {
					return errors.Wrapf(err, "failed to restore window to original display: got %s; want %s", restoreWinInfo.DisplayID, removeAdd.moveWinInfo.DisplayID)
				}
				restoreWinBounds = restoreWinInfo.BoundsInRoot
				return nil
			}, &testing.PollOptions{Timeout: 5 * time.Second})
			if err != nil {
				return err
			}
			return ensureWinBoundsInDisplay(restoreWinBounds, removeAdd.origDispInfo.Bounds)
		})
	}
	return nil
}

// dragWindowBetweenDisplays verifies the behavior of dragging an ARC window between displays.
func dragWindowBetweenDisplays(ctx context.Context, s *testing.State, cr *chrome.Chrome, a *arc.ARC, dc virtualmultidisplay.VirtualDisplayController) error {
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return err
	}

	internalARCDisplayID, externalARCDisplayID, err := arcDisplayIDs(ctx, a, dc)
	if err != nil {
		return err
	}

	// Setup display layout.
	disp, err := internalAndExternalDisplays(ctx, tconn, dc)
	if err != nil {
		return err
	}
	zero := 0
	p := display.DisplayProperties{BoundsOriginX: &disp.internal.Bounds.Width, BoundsOriginY: &zero}
	if err := display.SetDisplayProperties(ctx, tconn, disp.external.ID, p); err != nil {
		return err
	}
	// Poll is required as completion of display.SetDisplayProperties does not
	// ensure display.GetInfo returns new info.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		disp, err = internalAndExternalDisplays(ctx, tconn, dc)
		if err != nil {
			return err
		}
		if disp.external.Bounds.Left != disp.internal.Bounds.Width || disp.external.Bounds.Top != 0 {
			return errors.New("display origin has not been updated")
		}
		return nil
	}, &testing.PollOptions{Timeout: 5 * time.Second}); err != nil {
		return err
	}

	// Raw mouse API.
	m, err := input.Mouse(ctx)
	if err != nil {
		return err
	}
	defer m.Close(ctx)

	version, err := arc.SDKVersion()
	if err != nil {
		s.Fatal("Failed to get ARC version: ", err)
	}
	// In ARC R, screen size (in pixels) may change when window move to another display.
	allowScreenSizeConfigChange := version >= arc.SDKR

	type shouldMoveFlag bool
	const (
		shouldMove    shouldMoveFlag = true
		shouldNotMove shouldMoveFlag = false
	)
	for _, param := range []struct {
		// Activity package and class.
		resizeability        resizeability
		configChangeHandling configChangeHandling
		// Initial state of the window being dragged.
		winState ash.WindowStateType
		// Display where activity should be placed after the drag operation.
		shouldMove shouldMoveFlag
		// Expected config set to be changed.
		wantCC []configChangeEvent
	}{
		{resizeable, handling, ash.WindowStateNormal, shouldMove, []configChangeEvent{{
			handled: true,
			density: true,
		}}},
		// TODO(b/161859617): Drag maximized window is disabled on ARC currently.
		// {resizeable, handling, ash.WindowStateMaximized, shouldMove, []configChangeEvent{{
		// 	handled:            true,
		// 	density:            true,
		// 	screenSize:         true,
		// 	smallestScreenSize: true,
		// 	orientation:        true,
		// }}},
		{resizeable, relaunching, ash.WindowStateNormal, shouldMove, []configChangeEvent{{
			handled: false,
			density: true,
		}}},
		// TODO(b/161859617): Drag maximized window is disabled on ARC currently.
		// {resizeable, relaunching, ash.WindowStateMaximized, shouldMove, []configChangeEvent{{
		// 	handled:            false,
		// 	density:            true,
		// 	screenSize:         true,
		// 	smallestScreenSize: true,
		// 	orientation:        true,
		// }}},
		// {nonResizeable, handling, ash.WindowStateMaximized, shouldNotMove, nil},
		// NonResizable apps can never be maximized, no need for that case.
	} {
		for _, dir := range []struct {
			// Display where drag operation starts.
			srcDisp     int
			srcDispType arc.DisplayType
			// Display where drag operation ends.
			dstDisp     int
			dstDispType arc.DisplayType
		}{
			{internalARCDisplayID, arc.InternalDisplay, externalARCDisplayID, arc.ExternalDisplay},
			{externalARCDisplayID, arc.ExternalDisplay, internalARCDisplayID, arc.InternalDisplay},
		} {
			name := fmt.Sprintf(
				"%s %s from %s to %s",
				param.winState, testActivitySimpleName(param.resizeability, param.configChangeHandling), dir.srcDispType, dir.dstDispType)
			runOrFatal(ctx, s, name, func(ctx context.Context, s *testing.State) error {
				act := testappActivity{ctx, tconn, a, param.resizeability, param.configChangeHandling, nil}
				defer act.close(ctx)

				if err := act.launch(ctx, dir.srcDisp); err != nil {
					return err
				}

				if param.resizeability != sizeCompat {
					if err := act.setWindowState(param.winState); err != nil {
						return err
					}
				}

				if err := deleteConfigurationChanges(ctx, a); err != nil {
					return err
				}

				win, err := act.findWindow()
				if err != nil {
					return err
				}

				// This test assumes the external display is always to the RIGHT orientation of the primary internal parent display.
				srcDispBnds := disp.displayInfo(dir.srcDispType).Bounds
				dstDispBnds := disp.displayInfo(dir.dstDispType).Bounds

				cursor := cursorOnDisplay{internalARCDisplayID, arc.InternalDisplay}
				defer cursor.moveTo(ctx, tconn, m, internalARCDisplayID, arc.InternalDisplay, disp)
				if err := cursor.moveTo(ctx, tconn, m, dir.srcDisp, dir.srcDispType, disp); err != nil {
					return err
				}

				var winPt coords.Point
				if dir.srcDispType == arc.ExternalDisplay {
					winPt = coords.NewPoint(dstDispBnds.Width+win.BoundsInRoot.Left+win.BoundsInRoot.Width/2, win.BoundsInRoot.Top+win.CaptionHeight/4)
				} else {
					winPt = coords.NewPoint(win.BoundsInRoot.Left+win.BoundsInRoot.Width/2, win.BoundsInRoot.Top+win.CaptionHeight/4)
				}
				if err := mouse.Move(tconn, winPt, 0)(ctx); err != nil {
					return err
				}

				if err := mouse.Press(tconn, mouse.LeftButton)(ctx); err != nil {
					return err
				}

				if err := cursor.moveTo(ctx, tconn, m, dir.dstDisp, dir.dstDispType, disp); err != nil {
					return err
				}

				var dstPt coords.Point
				if dir.dstDispType == arc.ExternalDisplay {
					dstPt = coords.NewPoint(srcDispBnds.Width+dstDispBnds.Width/2, dstDispBnds.Height/2)
				} else {
					dstPt = coords.NewPoint(dstDispBnds.Width/2, dstDispBnds.Height/2)
				}
				if err := mouse.Move(tconn, dstPt, time.Second)(ctx); err != nil {
					return err
				}

				if err := mouse.Release(tconn, mouse.LeftButton)(ctx); err != nil {
					return err
				}

				sourceDispID := disp.displayInfo(dir.srcDispType).ID
				wantDispID := disp.displayInfo(dir.dstDispType).ID

				err = testing.Poll(ctx, func(ctx context.Context) error {
					win, err := act.findWindow()
					if err != nil {
						return err
					}
					if win.DisplayID == wantDispID {
						return nil
					}
					if win.DisplayID == sourceDispID {
						return &activityStayingError{win.DisplayID}
					}
					return testing.PollBreak(errors.Errorf("Display is moved to unexpected display: got %s; want %s", win.DisplayID, wantDispID))
				}, &testing.PollOptions{Timeout: 2 * time.Second})

				if param.shouldMove {
					if err != nil {
						return errors.Wrap(err, "should have moved")
					}
				} else {
					if err == nil {
						return errors.New("Activity is unexpectedly moved to the destination display")
					}
					var notMoved *activityStayingError
					if !errors.As(err, &notMoved) {
						return err
					}
				}

				isVirtual := dc != nil
				if !isVirtual {
					// TODO(b/285232830) When we are migrated to VKMS and can support config change checking on virtual
					// displays.  This can be used to detect differing densities etc.
					//
					// To do this, we'll need to create virtual display with different
					// density.  It might be worth it to add test cases with separate and
					// without separate densities.  After this is done we can test both
					// and that the flag is not set when the density is the same.
					if ccList, err := queryConfigurationChanges(ctx, a); err != nil {
						return err
					} else if !isConfigurationChangeMatched(ccList, param.wantCC, allowScreenSizeConfigChange) {
						return errors.Errorf("unexpected config change: got %+v; want %+v", ccList, param.wantCC)
					}
				}

				return nil
			})
		}
	}

	return nil
}

// rotateDisplay verifies the behavior of rotating a display.
func rotateDisplay(ctx context.Context, s *testing.State, cr *chrome.Chrome, a *arc.ARC, dc virtualmultidisplay.VirtualDisplayController) error {
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return err
	}

	internalARCDisplayID, externalARCDisplayID, err := arcDisplayIDs(ctx, a, dc)
	if err != nil {
		return err
	}

	// Setup display layout.
	displays, err := internalAndExternalDisplays(ctx, tconn, dc)
	if err != nil {
		return err
	}

	for _, param := range []struct {
		name        string
		displayID   int
		displayType arc.DisplayType
		windowState ash.WindowStateType
		wantCC      []configChangeEvent
	}{
		{"rotate-internal-normal", internalARCDisplayID, arc.InternalDisplay, ash.WindowStateNormal, nil},
		{"rotate-internal-maximimized", internalARCDisplayID, arc.InternalDisplay, ash.WindowStateMaximized, []configChangeEvent{
			{handled: true, screenSize: true, orientation: true},
		}},
		{"rotate-external-normal", externalARCDisplayID, arc.ExternalDisplay, ash.WindowStateNormal, nil},
		{"rotate-external-maximimized", externalARCDisplayID, arc.ExternalDisplay, ash.WindowStateMaximized, []configChangeEvent{
			{handled: true, screenSize: true, orientation: true},
		}},
	} {
		runOrFatal(
			ctx,
			s,
			fmt.Sprintf("%s on %s display", param.windowState, param.displayType),
			func(ctx context.Context, s *testing.State) error {
				act := testappActivity{ctx, tconn, a, resizeable, handling, nil}
				if err := act.launch(ctx, param.displayID); err != nil {
					return errors.Wrap(err, "could not launch activity on display")
				}
				defer act.close(ctx)

				if err := ensureWindowOnDisplay(ctx, tconn, act.activity.PackageName(), displays.displayInfo(param.displayType).ID); err != nil {
					return errors.Wrap(err, "Activity launched on wrong display")
				}

				if err := act.setWindowState(param.windowState); err != nil {
					return errors.Wrap(err, "could not set window state")
				}

				if err := deleteConfigurationChanges(ctx, a); err != nil {
					return errors.Wrap(err, "could not delete configuration changes")
				}

				var displayInfoID string
				if param.displayType == arc.InternalDisplay {
					displayInfoID = displays.internal.ID
				} else {
					displayInfoID = displays.external.ID
				}

				currentRot := currentRotation{ctx, tconn, displayInfoID, 0}
				if err := currentRot.read(); err != nil {
					return errors.Wrap(err, "could not read rotation")
				}
				defer currentRot.setTo(currentRot.degree)
				if err := currentRot.setTo((currentRot.degree + 90) % 360); err != nil {
					return errors.Wrap(err, "could not set rotation")
				}

				ccList, err := queryConfigurationChanges(ctx, a)
				if err != nil {
					return errors.Wrap(err, "could not query config changes")
				}

				version, err := arc.SDKVersion()
				if err != nil {
					return errors.Wrap(err, "unable to get sdk version")
				}

				// TODO(b/304444756): Due to a change in T and up, only consider the
				// first event (yes, this is a hack, but what we're hacking doesn't have
				// to do with multidisplay).
				if version >= arc.SDKT && len(ccList) >= 1 {
					ccList = append([]configChangeEvent(nil), ccList[:(len(ccList)-1)]...)
				}

				if !reflect.DeepEqual(ccList, param.wantCC) {
					if scerr := grabScreenshotOfDisplaysForFailedTests(ctx, cr, displays, param.name); scerr != nil {
						s.Error("Failed to grab screenshot of displays")
					}
					return errors.Errorf("unexpected config change: got %+v; want %+v", ccList, param.wantCC)
				}

				return nil
			})
	}

	return nil
}

// snappingOnDisplay verifies snapping behavior (Alt + '['/']') on internal/external displays.
func snappingOnDisplay(ctx context.Context, s *testing.State, cr *chrome.Chrome, a *arc.ARC, dc virtualmultidisplay.VirtualDisplayController) error {
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return err
	}

	disp, err := internalAndExternalDisplays(ctx, tconn, dc)
	if err != nil {
		return err
	}

	keyboard, err := input.Keyboard(ctx)
	if err != nil {
		return err
	}
	defer keyboard.Close(ctx)

	for _, dispType := range []arc.DisplayType{arc.InternalDisplay, arc.ExternalDisplay} {
		d := disp.displayInfo(dispType)
		u := d.WorkArea.Width / 2
		testing.ContextLogf(ctx, "Testing snapping on display %s, resolution is %d x %d, snap width is %d", d.ID, d.Bounds.Width, d.Bounds.Height, u)
		for _, state := range []ash.WindowStateType{ash.WindowStateNormal, ash.WindowStateMaximized} {
			for _, param := range []struct {
				name     string
				accel    string
				wantBnds coords.Rect
			}{
				{"left", "Alt+[", coords.Rect{Left: 0, Top: 0, Width: u, Height: d.WorkArea.Height}},
				{"right", "Alt+]", coords.Rect{Left: u, Top: 0, Width: u, Height: d.WorkArea.Height}},
			} {
				runOrFatal(
					ctx, s, fmt.Sprintf("%s window to %s on %s display", state, param.name, dispType),
					func(ctx context.Context, s *testing.State) error {

						internalARCDisplayID, externalARCDisplayID, err := arcDisplayIDs(ctx, a, dc)
						if err != nil {
							return err
						}
						var arcDisplayID int
						if dispType == arc.InternalDisplay {
							arcDisplayID = internalARCDisplayID
						} else {
							arcDisplayID = externalARCDisplayID
						}

						act := testappActivity{ctx, tconn, a, resizeable, handling, nil}
						defer act.close(ctx)
						if err := act.launch(ctx, arcDisplayID); err != nil {
							return err
						}
						if err := ensureWindowOnDisplay(ctx, tconn, act.activity.PackageName(), d.ID); err != nil {
							return errors.Wrap(err, "Activity launched on wrong display")
						}

						if err := act.setWindowState(state); err != nil {
							return err
						}

						if err := keyboard.Accel(ctx, param.accel); err != nil {
							return err
						}

						err = testing.Poll(ctx, func(ctx context.Context) error {
							win, err := act.findWindow()
							if err != nil {
								return testing.PollBreak(err)
							}
							if !reflect.DeepEqual(win.BoundsInRoot, param.wantBnds) {
								return errors.Errorf("unexpected snapped window bounds: got %+v; want %+v",
									win.BoundsInRoot, param.wantBnds)
							}
							return nil
						}, &testing.PollOptions{Timeout: time.Second})

						if err != nil {
							if scerr := grabScreenshotOfDisplaysForFailedTests(ctx, cr, disp, param.name); scerr != nil {
								s.Error("Failed to grab screenshot of displays")
							}
						}

						return err
					})
			}
		}
	}
	return nil
}

// Helper functions.

// currentRotation remembers the current display rotation so that it gets back
// to the original rotation after sub-test completes.
type currentRotation struct {
	ctx    context.Context
	tconn  *chrome.TestConn
	id     string
	degree int
}

// read reads the current display rotation via Chrome API.
func (current *currentRotation) read() error {
	info, err := display.GetInfo(current.ctx, current.tconn)
	if err != nil {
		return err
	}

	for _, i := range info {
		if i.ID == current.id {
			current.degree = i.Rotation
			return nil
		}
	}

	return errors.Errorf("display %s not found", current.id)
}

// setTo changes the display rotation and waits until the change is effective.
func (current *currentRotation) setTo(degree int) error {
	if current.degree == degree {
		return nil
	}

	if err := display.SetDisplayProperties(
		current.ctx, current.tconn, current.id,
		display.DisplayProperties{Rotation: &degree}); err != nil {
		return err
	}

	// Poll is required as completion of display.SetDisplayProperties does not
	// ensure display.GetInfo returns new info.
	return testing.Poll(current.ctx, func(ctx context.Context) error {
		if err := current.read(); err != nil {
			return testing.PollBreak(err)
		}
		if current.degree != degree {
			return errors.Errorf("display rotation has not been updated: got %d; want %d", current.degree, degree)
		}
		return nil
	}, &testing.PollOptions{Timeout: 5 * time.Second})
}

// See go/arc-wm-r-spec for details.
type resizeability string

const (
	// Resizeable
	resizeable resizeability = "Resizeable"
	// Non-resizeable
	nonResizeable resizeability = "NonResizeable"
	// Non-resizeable + specifying orientation
	sizeCompat resizeability = "SizeCompat"
)

// Whether activity is expected to handle config changes, or it's going to relaunch.
type configChangeHandling string

const (
	handling    configChangeHandling = "Handling"
	relaunching configChangeHandling = "Relaunching"
)

// configChangeEvent is an entry of config change event.
type configChangeEvent struct {
	// True if config change is handled by Activity.
	handled bool
	// Config set.
	screenSize, density, orientation, fontScale, smallestScreenSize bool
}

// URI for logged config changes.
const configChangesURI = "content://org.chromium.arc.testapp.multidisplay/configChanges"

// Parser for config changes.
type configChangeParser struct {
	pattern *regexp.Regexp
}

var ccParser = configChangeParser{regexp.MustCompile("Row: [0-9]+ activityId=([0-9]+), " +
	"handled=(true|false), density=(true|false), " +
	"fontScale=(true|false), orientation=(true|false), screenLayout=(?:true|false), " +
	"screenSize=(true|false), smallestScreenSize=(true|false)")}

// parse parses the output of `content query` command.
func (parser *configChangeParser) parse(line string) (int32, configChangeEvent, error) {
	s := parser.pattern.FindStringSubmatch(line)
	if s == nil {
		return 0, configChangeEvent{}, errors.Errorf("unexpected line format %q", line)
	}

	actID, err := strconv.ParseInt(s[1], 10, 32)
	if err != nil {
		return 0, configChangeEvent{}, err
	}

	var c configChangeEvent
	for i, config := range []*bool{&c.handled, &c.density, &c.fontScale, &c.orientation, &c.screenSize, &c.smallestScreenSize} {
		*config, err = strconv.ParseBool(s[i+2])
		if err != nil {
			return 0, configChangeEvent{}, err
		}
	}

	return int32(actID), c, nil
}

// queryConfigurationChanges obtains the history of configuration change from test app.
func queryConfigurationChanges(ctx context.Context, a *arc.ARC) ([]configChangeEvent, error) {
	bytes, err := a.Command(ctx, "content", "query", "--uri", configChangesURI).Output(testexec.DumpLogOnError)
	if err != nil {
		return nil, err
	}
	perAct := make(map[int32][]configChangeEvent)
	lines := strings.Split(string(bytes), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		// "No result found." taken from here: https://source.corp.google.com/android/frameworks/base/cmds/content/src/com/android/commands/content/Content.java;l=657
		if line == "No result found." {
			return nil, nil
		}
		actID, c, err := ccParser.parse(line)
		if err != nil {
			return nil, err
		}
		perAct[actID] = append(perAct[actID], c)
	}

	if len(perAct) > 1 {
		return nil, errors.Errorf("there must be at most one activity generating config changes: got %d; want 1", len(perAct))
	}

	for _, cc := range perAct {
		return cc, nil
	}

	return nil, nil
}

// deleteConfigurationChanges deletes recorded config changes.
func deleteConfigurationChanges(ctx context.Context, a *arc.ARC) error {
	return a.Command(ctx, "content", "delete", "--uri", configChangesURI).Run(testexec.DumpLogOnError)
}

// ensureWindowOnDisplay checks whether a window is on a certain display.
func ensureWindowOnDisplay(ctx context.Context, tconn *chrome.TestConn, pkgName, dispID string) error {
	windowInfo, err := ash.GetARCAppWindowInfo(ctx, tconn, pkgName)
	if err != nil {
		return err
	}
	if windowInfo.DisplayID != dispID {
		return errors.Errorf("invalid display ID: got %q; want %q", windowInfo.DisplayID, dispID)
	}
	return nil
}

// ensureSetWindowState checks whether the window is in requested window state. If not, make sure to set window state to the requested window state.
func ensureSetWindowState(ctx context.Context, tconn *chrome.TestConn, pkgName string, expectedState ash.WindowStateType) error {
	if state, err := ash.GetARCAppWindowState(ctx, tconn, pkgName); err != nil {
		return err
	} else if state == expectedState {
		return nil
	}
	windowEventMap := map[ash.WindowStateType]ash.WMEventType{
		ash.WindowStateNormal:     ash.WMEventNormal,
		ash.WindowStateMaximized:  ash.WMEventMaximize,
		ash.WindowStateMinimized:  ash.WMEventMinimize,
		ash.WindowStateFullscreen: ash.WMEventFullscreen,
	}
	wmEvent, ok := windowEventMap[expectedState]
	if !ok {
		return errors.Errorf("didn't find the event for window state: %q", expectedState)
	}
	state, err := ash.SetARCAppWindowState(ctx, tconn, pkgName, wmEvent)
	if err != nil {
		return errors.Wrapf(err, "failed to set arc app window state %s", wmEvent)
	}
	if state != expectedState {
		return errors.Errorf("unexpected window state: got %s; want %s", state, expectedState)
	}
	if err := ash.WaitForARCAppWindowState(ctx, tconn, pkgName, expectedState); err != nil {
		return errors.Wrapf(err, "failed to wait for activity to enter %v state", expectedState)
	}
	return nil
}

// ensureWindowStable checks whether the window moves its position.
func ensureWindowStable(ctx context.Context, tconn *chrome.TestConn, pkgName string, expectedWindowInfo *ash.Window) error {
	windowInfo, err := ash.GetARCAppWindowInfo(ctx, tconn, pkgName)
	if err != nil {
		return errors.Wrapf(err, "failed to get window info for window: %q", pkgName)
	}
	if !reflect.DeepEqual(windowInfo.BoundsInRoot, expectedWindowInfo.BoundsInRoot) || windowInfo.DisplayID != expectedWindowInfo.DisplayID {
		return errors.Errorf("window moves: got bounds %+v (displayID %q); expected bounds %+v (displayID %q)", windowInfo.BoundsInRoot, windowInfo.DisplayID, expectedWindowInfo.BoundsInRoot, expectedWindowInfo.DisplayID)
	}
	return nil
}

// ensureNoBlackBkg checks whether there is black background.
func ensureNoBlackBkg(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn) error {
	infos, err := display.GetInfo(ctx, tconn)
	if err != nil {
		return err
	}

	for _, info := range infos {
		img, err := grabScreenshotForDisplay(ctx, cr, info.ID)
		if err != nil {
			return err
		}
		blackPixels := imgcmp.CountPixels(img, color.RGBA{0, 0, 0, 255})
		rect := img.Bounds()
		totalPixels := (rect.Max.Y - rect.Min.Y) * (rect.Max.X - rect.Min.X)
		percent := blackPixels * 100 / totalPixels
		testing.ContextLogf(ctx, "Black pixels = %d / %d (%d%%) on display %q", blackPixels, totalPixels, percent, info.ID)

		// "5 percent" is arbitrary.
		if percent > 5 {
			// Save image with black pixels.
			dir, ok := testing.ContextOutDir(ctx)
			if !ok {
				return errors.New("failed to get directory for saving files")
			}
			path := fmt.Sprintf("%s/no-black-background-screenshot-failed-%s.png", dir, info.ID)
			fd, err := os.Create(path)
			if err != nil {
				return errors.Wrap(err, "failed to create screenshot")
			}
			defer fd.Close()
			if err := png.Encode(fd, img); err != nil {
				return errors.Wrap(err, "failed to save screenshot in PNG format")
			}

			testing.ContextLogf(ctx, "Image containing the black pixels: %s", path)
			return errors.Errorf("test failed: contains %d / %d (%d%%) black pixels", blackPixels, totalPixels, percent)
		}
	}
	return nil
}

// ensureWinBoundsInDisplay checks whether the window bounds are inside of display bounds.
func ensureWinBoundsInDisplay(winBounds, displayBounds coords.Rect) error {
	// Convert local window bounds to global window bounds.
	winBounds.Left += displayBounds.Left
	winBounds.Top += displayBounds.Top

	if winBounds.Left < displayBounds.Left || winBounds.Top < displayBounds.Top ||
		winBounds.Left+winBounds.Width > displayBounds.Left+displayBounds.Width ||
		winBounds.Top+winBounds.Height > displayBounds.Top+displayBounds.Height {
		return errors.Errorf("window bounds is out of display bounds: window bounds %+v, display bounds %+v", winBounds, displayBounds)
	}
	return nil
}

// waitForStopAnimating waits until Ash window stops animation.
func waitForStopAnimating(ctx context.Context, tconn *chrome.TestConn, pkgName string, timeout time.Duration) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		info, err := ash.GetARCAppWindowInfo(ctx, tconn, pkgName)
		if err != nil {
			return err
		}
		if info.IsAnimating {
			return errors.New("the window is still animating")
		}
		return nil
	}, &testing.PollOptions{Timeout: timeout})
}

// waitForDisplay waits until a display on or off.
// Virtual displays are always identified by their index, not the display ID, for activation.
func waitForDisplay(ctx context.Context, tconn *chrome.TestConn, dc virtualmultidisplay.VirtualDisplayController, displayVirtualID int, dispID string, isOn bool, timeout time.Duration) error {
	if dc != nil {
		// Ensure that configfs has the requested display set accordingly before polling.
		if e, err := dc.DisplayEnabled(displayVirtualID); err != nil {
			return errors.Wrapf(err, "error checking if display enabled: %d", displayVirtualID)
		} else if e != isOn {
			return errors.Wrapf(err, "expected display on: %t but was %t", isOn, e)
		}
	}

	return testing.Poll(ctx, func(ctx context.Context) error {
		infos, err := display.GetInfo(ctx, tconn)
		if err != nil {
			return err
		}
		if isOn {
			found := false
			for _, info := range infos {
				if info.ID == dispID {
					found = true
					break
				}
			}
			if !found {
				return errors.Errorf("failed to set display %s power to %t", dispID, isOn)
			}
		} else {
			for _, info := range infos {
				if info.ID == dispID {
					return errors.Errorf("failed to set display %s power to %t", dispID, isOn)
				}
			}
		}
		return nil
	}, &testing.PollOptions{Timeout: timeout})
}

// ensureActivityReady waits until given activity is ready.
func ensureActivityReady(ctx context.Context, tconn *chrome.TestConn, act *arc.Activity) error {
	if err := ash.WaitForVisible(ctx, tconn, act.PackageName()); err != nil {
		return err
	}
	if err := waitForStopAnimating(ctx, tconn, act.PackageName(), 10*time.Second); err != nil {
		return err
	}
	return nil
}

// grabScreenshotForDisplay takes a screenshot for a given displayID and returns an image.Image.
func grabScreenshotForDisplay(ctx context.Context, cr *chrome.Chrome, displayID string) (image.Image, error) {
	fd, err := ioutil.TempFile("", "screenshot")
	if err != nil {
		return nil, errors.Wrap(err, "error opening screenshot file")
	}
	defer os.Remove(fd.Name())
	defer fd.Close()

	if err := screenshot.CaptureChromeForDisplay(ctx, cr, displayID, fd.Name()); err != nil {
		return nil, errors.Wrap(err, "failed to capture screenshot")
	}

	img, _, err := image.Decode(fd)
	if err != nil {
		return nil, errors.Wrap(err, "error decoding image")
	}
	return img, nil
}

func grabScreenshotOfDisplaysForFailedTests(ctx context.Context, cr *chrome.Chrome, disp displayLayout, fileName string) error {
	for _, dispType := range []arc.DisplayType{arc.InternalDisplay, arc.ExternalDisplay} {
		d := disp.displayInfo(dispType)
		// Save image with failed snap.
		img, err := grabScreenshotForDisplay(ctx, cr, d.ID)
		if err != nil {
			return errors.Wrap(err, "could not grab display screenshot")
		}
		dir, ok := testing.ContextOutDir(ctx)
		if !ok {
			return errors.New("failed to get directory for saving files")
		}
		path := fmt.Sprintf("%s/snapping-%s-screenshot-failed-%s.png", dir, fileName, d.ID)
		if err := writeScreenshotPng(path, img); err != nil {
			return errors.Wrap(err, "could not write screenshot png file")
		}

		testing.ContextLogf(ctx, "Image containing the failed snap: %s", path)
	}

	return nil
}

func writeScreenshotPng(path string, img image.Image) error {
	fd, err := os.Create(path)
	if err != nil {
		return errors.Wrap(err, "failed to create screenshot")
	}
	defer fd.Close()
	if err := png.Encode(fd, img); err != nil {
		return errors.Wrap(err, "failed to save screenshot in PNG format")
	}

	return nil
}

// setDisplayPower sets the display power by a given power state, uses dc for virtual displays/tests.
func setDisplayPower(ctx context.Context, dc virtualmultidisplay.VirtualDisplayController, power displayPowerState) error {
	if dc == nil {
		return setDisplayPowerPhysical(ctx, power)
	}

	return setDisplayPowerVirtual(ctx, dc, power)

}

func setDisplayPowerVirtual(ctx context.Context, dc virtualmultidisplay.VirtualDisplayController, power displayPowerState) error {
	internalDisplayID, err := dc.InternalDisplayID()
	if err != nil {
		return errors.Wrap(err, "could not get internal display ID")
	}
	externalDisplayIDs, err := dc.ExternalDisplayIDs()
	if err != nil {
		return errors.Wrap(err, "could not get external display IDs")
	}

	switch power {
	case displayPowerAllOn:
		dc.EnableDisplay(internalDisplayID)
		for _, d := range externalDisplayIDs {
			if err := dc.EnableDisplay(d); err != nil {
				return errors.Wrapf(err, "could not enable display %d", d)
			}
		}
	case displayPowerAllOff:
		dc.DisableDisplay(internalDisplayID)
		for _, d := range externalDisplayIDs {
			if err := dc.DisableDisplay(d); err != nil {
				return errors.Wrapf(err, "could not disable display %d", d)
			}
		}
	case displayPowerInternalOffExternalOn:
		testing.ContextLogf(ctx, "Disabling 'internal' display ID %d", internalDisplayID)
		if err := dc.DisableDisplay(internalDisplayID); err != nil {
			return errors.Wrap(err, "could not disable 'internal' display 0")
		}

		for _, d := range externalDisplayIDs {
			testing.ContextLogf(ctx, "Enabling 'external' display ID %d", d)
			if err := dc.EnableDisplay(d); err != nil {
				return errors.Wrapf(err, "could not enable external display %d", d)
			}
		}
	case displayPowerInternalOnExternalOff:
		testing.ContextLogf(ctx, "Enabling 'internal' display ID %d", internalDisplayID)
		if err := dc.EnableDisplay(internalDisplayID); err != nil {
			return errors.Wrap(err, "could not enable 'internal' display 0")
		}

		for _, d := range externalDisplayIDs {
			testing.ContextLogf(ctx, "Disabling 'external' display ID %d", d)
			if err := dc.DisableDisplay(d); err != nil {
				return errors.Wrapf(err, "could not disable external display %d", d)
			}
		}
	}

	return nil
}

func setDisplayPowerPhysical(ctx context.Context, power displayPowerState) error {
	const (
		dbusName      = "org.chromium.DisplayService"
		dbusPath      = "/org/chromium/DisplayService"
		dbusInterface = "org.chromium.DisplayServiceInterface"

		setPowerMethod = "SetPower"
	)

	if power < displayPowerAllOn || power > displayPowerInternalOnExternalOff {
		return errors.Errorf("incorrect power value: got %d, want [%d - %d]", power, displayPowerAllOn, displayPowerInternalOnExternalOff)
	}

	_, obj, err := dbusutil.Connect(ctx, dbusName, dbus.ObjectPath(dbusPath))
	if err != nil {
		return errors.Wrapf(err, "failed to connect to %s", dbusName)
	}

	return obj.CallWithContext(ctx, dbusInterface+"."+setPowerMethod, 0, power).Err
}

// displayLayout is a pair of internal and external display.Info.
type displayLayout struct {
	internal display.Info
	external display.Info
}

// displayInfo returns display.Info by display type.
func (layout *displayLayout) displayInfo(displayType arc.DisplayType) *display.Info {
	if displayType == arc.InternalDisplay {
		return &layout.internal
	} else if displayType == arc.ExternalDisplay {
		return &layout.external
	}
	panic("Out of index")
}

// internalAndExternalDisplays returns internal and external display info.
func internalAndExternalDisplays(ctx context.Context, tconn *chrome.TestConn, dc virtualmultidisplay.VirtualDisplayController) (result displayLayout, err error) {
	infos, err := display.GetInfo(ctx, tconn)
	if err != nil {
		return result, err
	}

	var foundInt, foundExt bool
	for _, info := range infos {
		if info.IsInternal {
			result.internal = info
			foundInt = true
		} else if !foundExt {
			// Get the first external display info.
			result.external = info
			foundExt = true
		}
	}

	if !foundInt || !foundExt {
		err = errors.Wrap(err, "not enough displays: need at least one internal display and one external display")
		return result, err
	}

	return result, err
}

func arcDisplayIDs(ctx context.Context, a *arc.ARC, dc virtualmultidisplay.VirtualDisplayController) (internalARCDisplayID, externalARCDisplayID int, err error) {
	externalARCDisplayID, err = arc.FirstDisplayIDByType(ctx, a, arc.ExternalDisplay)
	if err != nil {
		return -1, -1, err
	}
	internalARCDisplayID, err = arc.FirstDisplayIDByType(ctx, a, arc.InternalDisplay)
	if err != nil {
		return -1, -1, err
	}

	return internalARCDisplayID, externalARCDisplayID, nil
}

// testappActivity provides activity-related operations ensuring state changes complete when returning from the function.
type testappActivity struct {
	ctx           context.Context
	tconn         *chrome.TestConn
	a             *arc.ARC
	resizeability resizeability
	ccHandling    configChangeHandling
	activity      *arc.Activity
}

// activityName returns an activity class name including package name.
func (act *testappActivity) activityName() string {
	return fmt.Sprintf("%s.%s", dispPkg, testActivitySimpleName(act.resizeability, act.ccHandling))
}

// testActivitySimpleName returns an activity class name without package name.
func testActivitySimpleName(res resizeability, cc configChangeHandling) string {
	return fmt.Sprintf("%s%sActivity", res, cc)
}

// launch issues commands to launch an activity, then wait until launch completes.
func (act *testappActivity) launch(ctx context.Context, displayID int) error {
	innerAct, err := arc.NewActivityOnDisplay(act.a, dispPkg, act.activityName(), displayID)
	if err != nil {
		return err
	}

	err = innerAct.Start(act.ctx, act.tconn, arc.WithDisplayID(displayID))
	if err != nil {
		innerAct.Close(ctx)
		return err
	}
	act.activity = innerAct

	return ensureActivityReady(act.ctx, act.tconn, innerAct)
}

// close cleans up internal resources of activity including stopping the activity.
func (act *testappActivity) close(ctx context.Context) error {
	if act.activity == nil {
		return nil
	}
	act.activity.Close(ctx)
	if err := act.activity.Stop(act.ctx, act.tconn); err != nil {
		return err
	}
	act.activity = nil
	return nil
}

// setWindowState issues command to set window state, then wait until the new state is applied.
func (act *testappActivity) setWindowState(state ash.WindowStateType) error {
	return ensureSetWindowState(act.ctx, act.tconn, dispPkg, state)
}

// findWindow returns an only window which shares the same package name with activity.
func (act *testappActivity) findWindow() (*ash.Window, error) {
	windows, err := ash.GetAllWindows(act.ctx, act.tconn)
	if err != nil {
		return nil, err
	}
	var win *ash.Window
	for _, window := range windows {
		if window.ARCPackageName == dispPkg {
			if win != nil {
				return nil, errors.Errorf("found multiple windows for %q", dispPkg)
			}
			win = window
		}
	}
	if win == nil {
		return nil, errors.Errorf("window not found for %q", dispPkg)
	}
	return win, nil
}

// cursorOnDisplay remembers which display the mouse cursor is on.
type cursorOnDisplay struct {
	currentDisp     int
	currentDispType arc.DisplayType
}

// moveTo moves mouse cursor across displays.
// mouse.Move does not move the cursor out side of the display. To overcome the limitation, this method place a mouse cursor around display edge by mouse.Move, then moves cursor by raw input.MouseEventWriter to cross display boundary.
func (cursor *cursorOnDisplay) moveTo(ctx context.Context, tconn *chrome.TestConn, m *input.MouseEventWriter, dstDisp int, dstDispType arc.DisplayType, layout displayLayout) error {
	// Validates display layout
	intBnds := layout.internal.Bounds
	extBnds := layout.external.Bounds
	if intBnds.Left != 0 || intBnds.Top != 0 || extBnds.Left != intBnds.Width || extBnds.Top != 0 {
		wantIntBnds := coords.NewRect(0, 0, intBnds.Width, intBnds.Height)
		wantExtBnds := coords.NewRect(intBnds.Width, 0, extBnds.Width, extBnds.Height)
		return errors.Errorf("moveTo method assumes external display is placed on the right edge of the default display; got: (intDisp %q extDisp %q), want: (intDisp %q extDisp %q)", intBnds, extBnds, wantIntBnds, wantExtBnds)
	}

	if cursor.currentDisp == dstDisp {
		return nil
	}

	var start coords.Point
	var delta coords.Point
	const coordsMargin = 100
	if cursor.currentDispType == arc.InternalDisplay && dstDispType == arc.ExternalDisplay {
		start = coords.NewPoint(layout.internal.Bounds.Width-coordsMargin, coordsMargin)
		delta = coords.NewPoint(1, 0)
	} else if cursor.currentDispType == arc.ExternalDisplay && dstDispType == arc.InternalDisplay {
		start = coords.NewPoint(coordsMargin, coordsMargin)
		delta = coords.NewPoint(-1, 0)
	} else {
		return errors.Errorf("unexpected display: current %d, destination %d", cursor.currentDisp, dstDisp)
	}
	if err := mouse.Move(tconn, start, 0)(ctx); err != nil {
		return err
	}
	for i := 0; i < coordsMargin*2; i++ {
		if err := m.Move(int32(delta.X), int32(delta.Y)); err != nil {
			return err
		}
		// GoBigSleepLint need to wait for a moment before the cursor position is
		// stabilized in animations/callbacks/etc.
		testing.Sleep(ctx, 5*time.Millisecond)
	}
	cursor.currentDisp = dstDisp
	cursor.currentDispType = dstDispType
	return nil
}

var classNameReg = regexp.MustCompile("[^.]+$")

// simpleClassName removes package from class name
func simpleClassName(act string) string {
	return classNameReg.FindString(act)
}

// runOrFatal runs body as subtest, then invokes s.Fatal if it returns an error
func runOrFatal(ctx context.Context, s *testing.State, name string, body func(context.Context, *testing.State) error) bool {
	return s.Run(ctx, name, func(ctx context.Context, s *testing.State) {
		if err := body(ctx, s); err != nil {
			s.Fatal("subtest failed: ", err)
		}
	})
}

// activityStayingError warns the activity keeps staying at the original display.
type activityStayingError struct {
	displayID string
}

// Error is for error interface.
func (e *activityStayingError) Error() string {
	return fmt.Sprintf("activity still stays at the source display %s", e.displayID)
}

// isConfigurationChangeMatched returns true if two configuration change is matched.
func isConfigurationChangeMatched(lhs, rhs []configChangeEvent, allowScreenSizeChange bool) bool {
	if len(lhs) != len(rhs) || len(lhs) > 1 {
		return false
	}
	if len(lhs) > 0 {
		L := lhs[0]
		R := rhs[0]
		if L.handled != R.handled || (L.density != R.density) || L.orientation != R.orientation || L.fontScale != R.fontScale {
			return false
		}
		if !allowScreenSizeChange && (L.screenSize != R.screenSize || L.smallestScreenSize != R.smallestScreenSize) {
			return false
		}
	}
	return true
}

// WaitForDisplaysToPopulate waits for n displays to populate and returns the display.Infos.
func WaitForDisplaysToPopulate(ctx context.Context, s *testing.State, tconn *chrome.TestConn, n int) ([]display.Info, error) {
	displayInfo, err := WaitForCondition(ctx, tconn, func(dis []display.Info) bool {
		return len(dis) == n
	}, defaultPollOptions)

	if err != nil {
		return nil, errors.Wrapf(err, "expected display info to have %d entries but had %d", n, len(displayInfo))
	}

	if err := LogDisplayInformation(ctx, displayInfo); err != nil {
		s.Error("Error logging display info: ", err)
	}

	return displayInfo, nil
}

// WaitForCondition wait for a specific window state to exist.
func WaitForCondition(ctx context.Context, tconn *chrome.TestConn, predicate func(dis []display.Info) bool, pollOptions *testing.PollOptions) ([]display.Info, error) {
	var dis []display.Info
	err := testing.Poll(ctx, func(ctx context.Context) error {
		var err error
		dis, err = display.GetInfo(ctx, tconn)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to get the display info"))
		}
		if predicate(dis) {
			return nil
		}
		return errors.New("display infos do not satisfy the condition")
	}, pollOptions)

	return dis, err
}

// LogDisplayInformation logs all the display info returned by
func LogDisplayInformation(ctx context.Context, displayInfos []display.Info) error {
	dir, ok := testing.ContextOutDir(ctx)
	if !ok {
		return errors.New("failed to get directory for saving files")
	}

	path := filepath.Join(dir, "display_information.txt")
	testing.ContextLogf(ctx, "Display information logged to %s:", path)

	f, err := os.Create(path)
	if err != nil {
		return errors.Wrap(err, "could not create file to log display info")
	}
	defer f.Close()

	for _, displayInfo := range displayInfos {
		b, err := json.MarshalIndent(displayInfo, "", "  ")
		if err != nil {
			testing.ContextLog(ctx, "Warning: cannot json serialize display info: ", err)
			continue
		}

		if _, err := f.Write(b); err != nil {
			return errors.Wrap(err, "could not write to display log file")
		}
	}

	return nil
}
