// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/pointer"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type roundedDisplayPerfParam struct {
	roundedDisplay    bool
	backgroundWindows bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         RoundedDisplayPerf,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Measures performance of rounded display",
		Contacts: []string{
			"chromeos-perfmetrics-eng@google.com",
			"yichenz@chromium.org",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Attr:         []string{"group:crosbolt", "crosbolt_nightly"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Timeout:      5 * time.Minute,
		Params: []testing.Param{{
			Name: "rounded_display_on",
			Val:  roundedDisplayPerfParam{roundedDisplay: true, backgroundWindows: true},
		}, {
			Name: "rounded_display_off",
			Val:  roundedDisplayPerfParam{roundedDisplay: false, backgroundWindows: true},
		}, {
			Name: "rounded_display_on_no_background_windows",
			Val:  roundedDisplayPerfParam{roundedDisplay: true, backgroundWindows: false},
		}, {
			Name: "rounded_display_off_no_background_windows",
			Val:  roundedDisplayPerfParam{roundedDisplay: false, backgroundWindows: false},
		}},
	})
}

func RoundedDisplayPerf(ctx context.Context, s *testing.State) {
	const mouseMoveDuration = 3 * time.Second

	// Reserve five seconds for various cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	param := s.Param().(roundedDisplayPerfParam)
	var opts []chrome.Option
	if param.roundedDisplay {
		opts = append(opts, chrome.EnableFeatures("kRoundedDisplay"))
	}
	cr, err := chrome.New(ctx, opts...)
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
	defer pc.Close(ctx)

	displayInfo, err := display.GetInternalInfo(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get display info: ", err)
	}
	workArea := displayInfo.WorkArea
	displayBounds := displayInfo.Bounds
	s.Log("Display bounds: ", displayBounds)

	if param.backgroundWindows {
		// Open 4 app windows to cover the whole screen.
		for i := 0; i < 4; i++ {
			if err := apps.Launch(ctx, tconn, apps.Gallery.ID); err != nil {
				s.Fatal("Failed to launch: ", err)
			}
		}
		ws, err := ash.GetAllWindows(ctx, tconn)
		if err != nil {
			s.Fatal("Failed to get the window list: ", err)
		}
		if len(ws) != 4 {
			s.Fatalf("Unexpected number of windows: got %d; want 4", len(ws))
		}
		windowBounds := []coords.Rect{
			// First two windows are overlapped.
			coords.NewRectLTRB(workArea.Right()*1/4, workArea.Top, workArea.Right()*3/4, workArea.CenterY()),
			coords.NewRectLTRB(workArea.CenterX(), workArea.Top, workArea.Right(), workArea.CenterY()),
			coords.NewRectLTRB(workArea.Left, workArea.Bottom()*3/4, workArea.Right()/4, workArea.Bottom()),
			coords.NewRectLTRB(workArea.CenterX(), workArea.CenterY(), workArea.Right(), workArea.Bottom()),
		}
		for i, bound := range windowBounds {
			if err := ash.SetWindowStateAndWait(ctx, tconn, ws[i].ID, ash.WindowStateNormal); err != nil {
				s.Fatal("Failed to set window state normal: ", err)
			}
			if _, _, err := ash.SetWindowBounds(ctx, tconn, ws[i].ID, bound, displayInfo.ID); err != nil {
				s.Fatal("Failed to set window bounds: ", err)
			}
		}
	}

	// Open a Files window.
	files, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch Files app: ", err)
	}
	defer files.Close(cleanupCtx)
	titleBar, err := files.Info(ctx, nodewith.ClassName("WebAppFrameToolbarView"))
	if err != nil {
		s.Log(uiauto.RootDebugInfo(ctx, tconn))
		s.Fatal("Failed to obtain Files app info: ", err)
	}
	startDragPt := titleBar.Location.CenterPoint()

	w, err := ash.WaitForAnyWindowWithTitle(ctx, tconn, "Files - My files")
	if err != nil {
		s.Fatal("Failed to wait for Files window: ", err)
	}
	wID := w.ID

	recorder, err := cujrecorder.NewRecorder(ctx, cr, tconn, nil, cujrecorder.RecorderOptions{})
	if err != nil {
		s.Fatal("Failed to create a recorder: ", err)
	}
	defer recorder.Close(cleanupCtx)
	if err := recorder.AddCommonMetrics(tconn, tconn); err != nil {
		s.Fatal("Failed to add common metrics to recorder: ", err)
	}

	if err := recorder.Run(ctx, func(ctx context.Context) error {
		// 1. Drag a window around the periphery of the display.
		if err := pc.Drag(startDragPt,
			pc.DragTo(displayBounds.TopRight(), mouseMoveDuration),
			pc.DragTo(displayBounds.BottomRight(), mouseMoveDuration),
			pc.DragTo(displayBounds.BottomLeft(), mouseMoveDuration),
			pc.DragTo(displayBounds.TopLeft(), mouseMoveDuration),
			pc.DragTo(displayBounds.TopRight(), mouseMoveDuration))(ctx); err != nil {
			return errors.Wrap(err, "failed to drag the window")
		}

		// 2. Enter and exit overview mode.
		if err := ash.SetOverviewModeAndWait(ctx, tconn, true); err != nil {
			return errors.Wrap(err, "failed to enter overview mode")
		}
		if err := ash.SetOverviewModeAndWait(ctx, tconn, false); err != nil {
			return errors.Wrap(err, "failed to exit overview mode")
		}

		// 3. Maximize the window.
		if err := ash.SetWindowStateAndWait(ctx, tconn, wID, ash.WindowStateMaximized); err != nil {
			return errors.Wrap(err, "failed to set window state to Maximized")
		}

		// 4. Move the cursor around edge of the display.
		if err := uiauto.Combine(
			"move the cursor around the periphery of the internal display",
			mouse.Move(tconn, displayBounds.BottomRight(), mouseMoveDuration),
			mouse.Move(tconn, displayBounds.BottomLeft(), mouseMoveDuration),
			mouse.Move(tconn, displayBounds.TopLeft(), mouseMoveDuration),
			mouse.Move(tconn, displayBounds.TopRight(), mouseMoveDuration),
		)(ctx); err != nil {
			return errors.Wrap(err, "failed to move the cursor")
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
