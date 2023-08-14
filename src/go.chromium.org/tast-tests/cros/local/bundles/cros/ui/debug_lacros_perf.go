// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/pointer"
	"go.chromium.org/tast-tests/cros/local/cpu"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	debugLacrosTestWaitDuration = 2 * time.Minute

	// This URL has black text on a white page. It automatically scrolls up and down.
	textScrollingURL = "https://petermcneeleychromium.github.io/small_scroll_text/index.html"

	// This URL has a background color that switches between orange and blue at 60fps.
	colorChangeURL = "https://petermcneeleychromium.github.io/color_change_60fps/index.html"

	// This URL plays a 180p video that is expanded to 720p pixels.
	videoPlaybackURL = "https://petermcneeleychromium.github.io/video_60fps_fake_720p_loop/index.html"
)

type debugLacrosTest struct {
	windowURL   string
	browserType browser.Type
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         DebugLacrosPerf,
		LacrosStatus: testing.LacrosVariantExists,
		// TODO(https://crbug.com/1401138): Evaluate the long-term utility
		Desc:         "Temporary tests to help debug lacros performance issues",
		Contacts:     []string{"cros-sw-perf@google.com", "erikchen@chromium.org"},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		SoftwareDeps: []string{"chrome"},
		Data:         []string{cujrecorder.SystemTraceConfigFile},
		Timeout:      cuj.CPUStablizationTimeout + debugLacrosTestWaitDuration,

		Params: []testing.Param{{
			Name: "text_scroll_ash",
			Val: debugLacrosTest{
				browserType: browser.TypeAsh,
				windowURL:   textScrollingURL,
			},
			Fixture: "chromeLoggedInDisableSync",
		}, {
			Name:              "text_scroll_lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val: debugLacrosTest{
				browserType: browser.TypeLacros,
				windowURL:   textScrollingURL,
			},
			Fixture: "lacrosDisableSync",
		}, {
			Name: "color_change_ash",
			Val: debugLacrosTest{
				browserType: browser.TypeAsh,
				windowURL:   colorChangeURL,
			},
			Fixture: "chromeLoggedInDisableSync",
		}, {
			Name:              "color_change_lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val: debugLacrosTest{
				browserType: browser.TypeLacros,
				windowURL:   colorChangeURL,
			},
			Fixture: "lacrosDisableSync",
		}, {
			Name: "video_playback_ash",
			Val: debugLacrosTest{
				browserType: browser.TypeAsh,
				windowURL:   videoPlaybackURL,
			},
			Fixture: "chromeLoggedInDisableSync",
		}, {
			Name:              "video_playback_lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val: debugLacrosTest{
				browserType: browser.TypeLacros,
				windowURL:   videoPlaybackURL,
			},
			Fixture: "lacrosDisableSync",
		}},
	})
}

func DebugLacrosPerf(ctx context.Context, s *testing.State) {
	// Wait for CPU to stabilize before test.
	if _, err := cpu.WaitUntilStabilized(ctx, cujrecorder.CPUCoolDownConfig()); err != nil {
		s.Log("Failed to wait for CPU to become idle: ", err)
	}

	// Ensure display on to record UI performance correctly.
	if err := power.TurnOnDisplay(ctx); err != nil {
		s.Fatal("Failed to turn on display: ", err)
	}

	debugLacrosTest := s.Param().(debugLacrosTest)

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	// Shorten context a bit to allow for cleanup.
	closeCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 2*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to the test API connection: ", err)
	}

	conn, br, closeBrowser, err := browserfixt.SetUpWithURL(ctx, cr, debugLacrosTest.browserType, debugLacrosTest.windowURL)
	if err != nil {
		s.Fatalf("Failed to open %s: %v", debugLacrosTest.windowURL, err)
	}
	bTconn, err := br.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get browser test API connection: ", err)
	}
	defer closeBrowser(closeCtx)
	defer conn.Close()

	activeWindow, err := ash.GetActiveWindow(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get the active window: ", err)
	}

	if err := ash.SetWindowStateAndWait(ctx, tconn, activeWindow.ID, ash.WindowStateMaximized); err != nil {
		s.Fatal("Failed to ensure that the window is maximized: ", err)
	}

	// Click at the center of the window to avoid showing the tab tooltip.
	pc := pointer.NewMouse(tconn)
	if err := pc.ClickAt(activeWindow.BoundsInRoot.CenterPoint())(ctx); err != nil {
		s.Fatal("Failed to click at the window: ", err)
	}

	// Recorder with no additional config; it records and reports memory usage and
	// CPU percents of browser/GPU processes.
	recorder, err := cujrecorder.NewRecorder(ctx, cr, bTconn, nil, cujrecorder.RecorderOptions{})
	if err != nil {
		s.Fatal("Failed to create a recorder: ", err)
	}
	defer func() {
		if err := recorder.Close(closeCtx); err != nil {
			s.Error("Failed to stop recorder: ", err)
		}
	}()

	if err := recorder.AddCommonMetrics(tconn, bTconn); err != nil {
		s.Fatal("Failed to add common metrics to recorder: ", err)
	}

	if err := recorder.Run(ctx, func(ctx context.Context) error {
		// See go/trace-in-cuj-tests about rules for tracing.
		if err := recorder.StartTracing(ctx, s.OutDir(), s.DataPath(cujrecorder.SystemTraceConfigFile)); err != nil {
			return errors.Wrap(err, "failed to start tracing")
		}

		s.Logf("Wait for %v to gather more data", debugLacrosTestWaitDuration)
		// GoBigSleepLint: It's used as a measurement step which is part of the performance testing logic.
		if err := testing.Sleep(ctx, debugLacrosTestWaitDuration); err != nil {
			return errors.Wrap(err, "failed to sleep")
		}

		if err := recorder.StopTracing(ctx); err != nil {
			return errors.Wrap(err, "failed to stop tracing")
		}

		return nil
	}); err != nil {
		s.Fatal("Failed to run the test scenario: ", err)
	}

	pv := perf.NewValues()
	if err = recorder.Record(ctx, pv); err != nil {
		s.Fatal("Failed to report: ", err)
	}
	if err := recorder.SaveTraceFiles(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to save trace files: ", err)
	}
	if err = pv.Save(s.OutDir()); err != nil {
		s.Error("Failed to store values: ", err)
	}
	if err := recorder.SaveHistograms(s.OutDir()); err != nil {
		s.Error("Failed to save histogram raw data: ", err)
	}
}
