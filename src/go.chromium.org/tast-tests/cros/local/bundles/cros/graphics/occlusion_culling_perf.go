// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package graphics

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/pointer"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type occlusionCullingPerfParam struct {
	// Maximum number (exclusive) of quads one draw quad can be split into
	// during occlusion culling.
	quadSplitsLimit int
}

type corner int

const (
	cornerUpperLeft  corner = iota
	cornerUpperRight corner = iota
	cornerLowerRight corner = iota
	cornerLowerLeft  corner = iota
)

const (
	numBackgroundWindows = 6
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         OcclusionCullingPerf,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Measures performance of occlusion culling of ChromeOS",
		Contacts: []string{
			"cros-sw-perf@google.com",
			"zoraiznaeem@chromium.org",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Attr:         []string{"group:cuj"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "gpuWatchHangs",
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Timeout:      5 * time.Minute,
		Params: []testing.Param{
			{
				Name: "4_quads_per_split",
				Val:  occlusionCullingPerfParam{quadSplitsLimit: 5},
			},
			{
				Name: "6_quads_per_split",
				Val:  occlusionCullingPerfParam{quadSplitsLimit: 7},
			},
			{
				Name: "8_quads_per_split",
				Val:  occlusionCullingPerfParam{quadSplitsLimit: 9},
			},
			{
				Name: "10_quads_per_split",
				Val:  occlusionCullingPerfParam{quadSplitsLimit: 11},
			},
			{
				Name: "12_quads_per_split",
				Val:  occlusionCullingPerfParam{quadSplitsLimit: 13},
			},
			{
				Name: "14_quads_per_split",
				Val:  occlusionCullingPerfParam{quadSplitsLimit: 15},
			},
		},
	})
}

func OcclusionCullingPerf(ctx context.Context, s *testing.State) {
	// Reserve five seconds for various cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	params := s.Param().(occlusionCullingPerfParam)

	// Create a chrome instance and enable rounded-display feature. Also specify
	// the radii of display via the command-line flag.
	cr, err := chrome.New(
		ctx, chrome.ExtraArgs(formatQuadSplitLimitAsSwitch(params.quadSplitsLimit)))

	if err != nil {
		s.Fatal("Failed to create chrome: ", err)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	// Ensure clamshell mode is enabled.
	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, false)
	if err != nil {
		s.Fatal("Failed to ensure clamshell mode: ", err)
	}
	defer cleanup(cleanupCtx)

	// Ensure landscape orientation.
	orientation, err := display.GetOrientation(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to obtain the orientation info: ", err)
	}

	if orientation.Type == display.OrientationPortraitPrimary {
		info, err := display.GetInternalInfo(ctx, tconn)
		if err != nil {
			s.Fatal("Failed to get the internal display info: ", err)
		}
		if err := display.SetDisplayRotationSync(ctx, tconn, info.ID, display.Rotate90); err != nil {
			s.Fatal("Failed to rotate display: ", err)
		}
		defer display.SetDisplayRotationSync(cleanupCtx, tconn, info.ID, display.Rotate0)
	}

	pc := pointer.NewMouse(tconn)
	defer pc.Close(cleanupCtx)

	displayInfo, err := display.GetInternalInfo(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get display info: ", err)
	}

	workArea := displayInfo.WorkArea
	displayBounds := displayInfo.Bounds
	s.Log("Display bounds: ", displayBounds)

	br := cr.Browser()

	// Create background browser windows.
	for i := 0; i < numBackgroundWindows; i++ {
		_, err = br.NewConn(ctx, chrome.NewTabURL, browser.WithNewWindow())
		if err != nil {
			s.Fatal("Failed to launch: ", err)
		}
	}

	ws, err := ash.GetAllWindows(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get the window list: ", err)
	}

	if len(ws) != numBackgroundWindows {
		s.Fatalf("Unexpected number of windows: got %d; want %d", len(ws), numBackgroundWindows)
	}

	//  The background windows are arranged in the following pattern:
	//
	//          +-------------------------------------------------+
	//          | Level 0 window                                  |
	//          |                                                 |
	//          |       +---------------------------------+       |
	//          |       |  Level 1 window                 |       |
	//          |       |                                 |       |
	//          |       |      +-------------------+      |       |
	//          +-------|      | Level 2 window    |      |-------+
	//                  |      |                   |      |
	//                  |      |                   |      |
	//                  +------|                   |------+
	//                         |                   |
	//                         |                   |
	//                         +-------------------+
	//                                   .
	//                                   .
	//                                   .

	initialWindowWidth := float64(workArea.Width) * 0.70
	initialWindowHeight := float64(workArea.Height) * 0.40

	initialTop := float64(workArea.Height) * 0.1
	initialLeft := float64(workArea.Width) * 0.1

	deltaWidthPercent := 0.1

	deltaXPercent := 0.05
	deltaY := initialWindowHeight * 0.25

	level := 0

	// Arrange background windows.
	for index := numBackgroundWindows - 1; index >= 0; index-- {
		// Each window is |deltaWidthPercent| percent smaller than the window above
		// its level.
		windowWidth := initialWindowWidth * (1 - (deltaWidthPercent * float64(level)))
		windowHeight := initialWindowHeight

		top := initialTop + deltaY*float64(level)
		// Each window is moved left |deltaXPercent| percent compare to the window
		//  above its level.
		left := initialLeft + initialWindowWidth*float64(level)*(deltaXPercent)

		bounds := coords.NewRectLTRB(int(left), int(top), int(left+windowWidth), int(top+windowHeight))
		setWindowBounds(ctx, tconn, ws[index].ID, bounds)
		level++
	}

	// Open a Files window that is dragged across the display.
	files, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch Files app: ", err)
	}

	w, err := ash.WaitForAnyWindowWithTitle(ctx, tconn, "Files - My files")
	if err != nil {
		s.Fatal("Failed to wait for Files window: ", err)
	}

	filesWidth := float64(workArea.Width) * 0.50
	filesHeight := float64(workArea.Height) * 0.50
	bounds := coords.NewRectLTRB(0, 0, int(filesWidth), int(filesHeight))
	setWindowBounds(ctx, tconn, w.ID, bounds)

	titleBar, err := files.Info(ctx, nodewith.ClassName("WebAppFrameToolbarView"))
	if err != nil {
		s.Log(uiauto.RootDebugInfo(ctx, tconn))
		s.Fatal("Failed to obtain Files app info: ", err)
	}
	startDragPt := titleBar.Location.CenterPoint()

	recorder, err := cujrecorder.NewRecorder(ctx, cr, tconn, nil, cujrecorder.RecorderOptions{})
	if err != nil {
		s.Fatal("Failed to create a recorder: ", err)
	}
	defer recorder.Close(cleanupCtx)

	if err := recorder.AddCommonMetrics(tconn, tconn); err != nil {
		s.Fatal("Failed to add common metrics to recorder: ", err)
	}

	const mouseMoveDuration = 3 * time.Second

	if err := recorder.Run(ctx, func(ctx context.Context) error {
		// 1. Drag files window around the periphery of the display.
		if err := pc.Drag(startDragPt,
			pc.DragTo(dragCoordinates(cornerUpperRight, displayInfo), mouseMoveDuration),
			pc.DragTo(dragCoordinates(cornerLowerRight, displayInfo), mouseMoveDuration),
			pc.DragTo(dragCoordinates(cornerLowerLeft, displayInfo), mouseMoveDuration),
			pc.DragTo(dragCoordinates(cornerUpperLeft, displayInfo), mouseMoveDuration))(ctx); err != nil {
			return errors.Wrap(err, "failed to drag the window")
		}

		// 2. Enter and exit overview mode.
		if err := ash.SetOverviewModeAndWait(ctx, tconn, true); err != nil {
			return errors.Wrap(err, "failed to enter overview mode")
		}
		if err := ash.SetOverviewModeAndWait(ctx, tconn, false); err != nil {
			return errors.Wrap(err, "failed to exit overview mode")
		}

		return nil
	}); err != nil {
		s.Fatal("Failed to run the test scenario: ", err)
	}

	pv := perf.NewValues()
	if err := recorder.Record(ctx, pv); err != nil {
		s.Fatal("Failed to report: ", err)
	}
	if err := pv.Save(s.OutDir()); err != nil {
		s.Error("Failed to store values: ", err)
	}
}

func setWindowBounds(ctx context.Context, tconn *chrome.TestConn, id int, bounds coords.Rect) error {
	displayInfo, err := display.GetInternalInfo(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to get display info")
	}

	if err := ash.SetWindowStateAndWait(ctx, tconn, id, ash.WindowStateNormal); err != nil {
		return errors.Wrap(err, "failed to set window state normal")
	}

	if _, _, err := ash.SetWindowBounds(ctx, tconn, id, bounds, displayInfo.ID); err != nil {
		return errors.Wrap(err, "failed to set window bounds")
	}

	return nil
}

func formatQuadSplitLimitAsSwitch(limit int) string {
	return fmt.Sprintf("--draw-quad-split-limit=%d", limit)
}

// dragCoordinates return coordinates to drag a window in a |corner| of the
// display referenced by |info|.
func dragCoordinates(displayCorner corner, info *display.Info) coords.Point {
	displayBounds := info.Bounds

	dw := float64(displayBounds.Width) * 0.3
	dh := float64(displayBounds.Height) * 0.3

	displayBounds = displayBounds.WithInset(int(dw), int(dh))

	switch displayCorner {
	case cornerUpperLeft:
		return displayBounds.TopLeft()
	case cornerUpperRight:
		return displayBounds.TopRight()
	case cornerLowerRight:
		return displayBounds.BottomRight()
	case cornerLowerLeft:
		return displayBounds.BottomLeft()
	}

	return coords.NewPoint(0, 0)
}
