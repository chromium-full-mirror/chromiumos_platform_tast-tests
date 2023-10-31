// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/pointer"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         WindowStateTransitionsCUJ,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Measures the performance of critical user journey for window state transitions",
		Contacts: []string{
			"cros-sw-perf@google.com",
			"ramsaroop@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		SoftwareDeps: []string{"chrome"},
		Data:         []string{"animation.html", "animation.js", cujrecorder.SystemTraceConfigFile},
		Timeout:      15 * time.Minute,
		Vars:         []string{"record"},
		Params: []testing.Param{{
			Val:     browser.TypeAsh,
			Fixture: "loggedInToCUJUser",
		}, {
			Name:              "lacros",
			Val:               browser.TypeLacros,
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           "loggedInToCUJUserLacros",
		}},
	})
}

func WindowStateTransitionsCUJ(ctx context.Context, s *testing.State) {
	// Reserve ten seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, false)
	if err != nil {
		s.Fatal("Failed to ensure clamshell mode: ", err)
	}
	defer cleanup(cleanupCtx)

	// Ensure landscape orientation so that we will be able to set the window bounds to
	// half the work area dimensions without infringing the minimum size of the window.
	orientation, err := display.GetOrientation(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to obtain the orientation info: ", err)
	}
	if orientation.Type == display.OrientationPortraitPrimary {
		info, err := display.GetPrimaryInfo(ctx, tconn)
		if err != nil {
			s.Fatal("Failed to get the primary display info: ", err)
		}
		if err := display.SetDisplayRotationSync(ctx, tconn, info.ID, display.Rotate90); err != nil {
			s.Fatal("Failed to rotate display: ", err)
		}
		defer display.SetDisplayRotationSync(cleanupCtx, tconn, info.ID, display.Rotate0)
	}

	info, err := display.GetPrimaryInfo(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get the primary display info: ", err)
	}

	pc := pointer.NewMouse(tconn)
	defer pc.Close(ctx)

	srv := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer srv.Close()
	animationURL := srv.URL + "/animation.html"

	conn, br, cleanup, err := browserfixt.SetUpWithURL(ctx, cr, s.Param().(browser.Type), animationURL)
	if err != nil {
		s.Fatal("Failed to launch browser: ", err)
	}
	defer cleanup(cleanupCtx)
	defer conn.Close()

	bTconn, err := br.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get browser TestAPIConn: ", err)
	}

	// Verify that there is only one window, and get its ID.
	ws, err := ash.GetAllWindows(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get the windows: ", err)
	}
	if len(ws) != 1 {
		s.Fatalf("Unexpected number of windows: got %d; want 1", len(ws))
	}
	wID := ws[0].ID

	// Set the window bounds for the "Normal" state.
	wState, err := ash.SetWindowState(ctx, tconn, wID, ash.WMEventNormal, true)
	if err != nil {
		s.Fatal("Failed to set window state to \"Normal\": ", err)
	}
	if wState != ash.WindowStateNormal {
		s.Fatalf("Unexpected window state: got %q; want \"Normal\"", wState)
	}
	desiredBounds := info.WorkArea.WithResizeAboutCenter(info.WorkArea.Width/2, info.WorkArea.Height/2)
	bounds, displayID, err := ash.SetWindowBounds(ctx, tconn, wID, desiredBounds, info.ID)
	if err != nil {
		s.Error("Failed to set the window bounds: ", err)
	}
	if displayID != info.ID {
		s.Errorf("Unexpected display ID for window: got %q; want %q", displayID, info.ID)
	}
	if bounds != desiredBounds {
		s.Errorf("Unexpected window bounds: got %v; want %v", bounds, desiredBounds)
	}

	// Maximize the window. The performance measurement will begin
	// with the window maximized.
	if err := ash.SetWindowStateAndWait(ctx, tconn, wID, ash.WindowStateMaximized); err != nil {
		s.Fatal("Failed to set window state to \"Maximized\": ", err)
	}

	// Get the window's maximized bounds, and use them to create an
	// action that drags from the top to unmaximize and maximize.
	w, err := ash.GetWindow(ctx, tconn, wID)
	if err != nil {
		s.Fatal("Failed to get window info: ", err)
	}
	top := w.BoundsInRoot.TopCenter()
	const (
		// dragDuration is the duration of each straight-
		// line part of the drag.
		dragDuration = 250 * time.Millisecond
		// holdDuration is how long the drag pauses after
		// going back to the top, to maximize the window.
		holdDuration = time.Second
	)
	dragUnmaximizeAndMaximize := pc.Drag(
		top,
		pc.DragTo(top.Add(coords.NewPoint(0, 100)), dragDuration),
		pc.DragTo(top, dragDuration),
		action.Sleep(holdDuration),
	)

	// Create and configure the metrics recorder.
	recorder, err := cujrecorder.NewRecorder(ctx, cr, bTconn, nil, cujrecorder.RecorderOptions{})
	if err != nil {
		s.Fatal("Failed to create the recorder: ", err)
	}
	defer recorder.Close(cleanupCtx)
	if err := recorder.AddCommonMetrics(tconn, bTconn); err != nil {
		s.Fatal("Failed to add CUJ common metrics to the recorder: ", err)
	}
	if err := recorder.AddCollectedMetrics(tconn, browser.TypeAsh,
		cujrecorder.NewSmoothnessMetricConfig("Ash.Window.AnimationSmoothness.CrossFade"),
		cujrecorder.NewSmoothnessMetricConfig("Ash.Window.AnimationSmoothness.CrossFade.DragMaximize"),
		cujrecorder.NewSmoothnessMetricConfig("Ash.Window.AnimationSmoothness.CrossFade.DragUnmaximize"),
		cujrecorder.NewSmoothnessMetricConfig("Ash.Window.AnimationSmoothness.Minimize"),
		cujrecorder.NewSmoothnessMetricConfig("Ash.Window.AnimationSmoothness.Unminimize"),
	); err != nil {
		s.Fatal("Failed to add window animation smoothness metrics to the recorder: ", err)
	}

	// Conduct the performance measurement.
	if err := recorder.RunFor(ctx, func(ctx context.Context) error {
		if err := dragUnmaximizeAndMaximize(ctx); err != nil {
			return errors.Wrap(err, "failed to drag to unmaximize and maximize the window")
		}
		if err := ash.WaitWindowFinishAnimating(ctx, tconn, wID); err != nil {
			return errors.Wrap(err, "failed to wait for the window animation")
		}
		for _, wState := range []ash.WindowStateType{
			ash.WindowStateNormal,
			ash.WindowStateMinimized,
			ash.WindowStateNormal,
			ash.WindowStateMaximized,
		} {
			if err := ash.SetWindowStateAndWait(ctx, tconn, wID, wState); err != nil {
				return errors.Wrapf(err, "failed to set window state to %q", wState)
			}
		}
		return nil
	}, 10*time.Minute); err != nil {
		s.Fatal("Failed to conduct the performance measurement: ", err)
	}

	// Report the results.
	pv := perf.NewValues()
	if err := recorder.Record(ctx, pv); err != nil {
		s.Fatal("Failed to record the performance data: ", err)
	}
	if err := recorder.SaveTraceFiles(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to save trace files: ", err)
	}
	if err := pv.Save(s.OutDir()); err != nil {
		s.Error("Failed to save the performance data: ", err)
	}
}
