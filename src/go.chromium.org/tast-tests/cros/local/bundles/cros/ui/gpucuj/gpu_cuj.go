// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package gpucuj tests GPU CUJ tests on ChromeOS Chrome.
package gpucuj

import (
	"context"
	"math"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast-tests/cros/local/cpu"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// TestType describes the type of GpuCUJ test to run.
type TestType string

// TestParams holds parameters describing how to run a GpuCUJ test.
type TestParams struct {
	TestType TestType
	Rot90    bool // Whether to rotate the screen 90 or not.
}

const (
	// TestTypeMaximized is a simple test of performance with a maximized window opening various web content.
	// This is useful for tracking the performance w.r.t hardware overlay forwarding of video or WebGL content.
	TestTypeMaximized TestType = "maximized"
	// TestTypeThreeDot is a test of performance while showing the three-dot context menu. This is intended to track the
	// performance impact of potential double composition of the context menu and hardware overlay usage.
	TestTypeThreeDot TestType = "threedot"
	// TestTypeResize is a test of performance during a drag-resize operation.
	TestTypeResize TestType = "resize"
	// TestTypeMoveOcclusion is a test of performance of gradual occlusion via drag-move of web content. This is useful for tracking impact
	// of hardware overlay forwarding and clipping (due to occlusion) of tiles optimisations.
	TestTypeMoveOcclusion TestType = "moveocclusion"
	// TestTypeMoveOcclusionWithCrosWindow is a test similar to TestTypeMoveOcclusion but always occludes using a ChromeOS chrome window.
	TestTypeMoveOcclusionWithCrosWindow TestType = "moveocclusion_withcroswindow"

	// testDuration indicates how long histograms should be sampled for during performance tests.
	testDuration time.Duration = 20 * time.Second
	// dragMoveOffsetDP indicates the offset from the top-left of a Chrome window to drag to ensure we can drag move it.
	dragMoveOffsetDP int = 5
	// insetSlopDP indicates how much to inset the work area (display area) to avoid window snapping to the
	// edges of the screen interfering with drag-move and drag-resize of windows.
	insetSlopDP int = 40
)

type page struct {
	name string
	// url indicates a web page to navigate to as part of a GpuCUJ test. If url begins with a '/' it is instead
	// interpreted as a path to a local data file, which will be accessed via a local HTTP server.
	url string
}

var pageSet = []page{
	{
		name: "scroll60fps", // Manual Scrolling 60 fps. This is for testing delegated compositing scroll performance.
		url:  "/continuous_scroll_60fps.html",
	},
	{
		name: "tiles60fps", // Gradient updated at 60 fps. This is for testing delegated compositing tile performance.
		url:  "/gradient_color_60fps.html",
	},
	{
		name: "webgl60fps", // Simplest small Webgl canvas at 60fps. This is for testing wayland overlay forwarding performance.
		url:  "/webgl_small_60fps.html",
	},
	{
		name: "aquarium", // WebGL Aquarium. This page is for testing WebGL.
		url:  "https://webglsamples.org/aquarium/aquarium.html",
	},
	{
		name: "poster", // Poster Circle. This page is for testing compositor performance.
		url:  "https://webkit.org/blog-files/3d-transforms/poster-circle.html",
	},
	{
		name: "video", // Static video. This page is for testing video playback.
		url:  "/video.html",
	},
	{
		name: "wikipedia", // Wikipedia. This page is for testing conventional web-pages.
		url:  "https://en.wikipedia.org/wiki/Cat",
	},
}

func toggleThreeDotMenu(ctx context.Context) error {
	// Open the three-dot menu via keyboard shortcut.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		return err
	}
	defer kb.Close(ctx)

	// Press Alt+F to open three-dot menu.
	if err := kb.Accel(ctx, "Alt+F"); err != nil {
		return err
	}

	return nil
}

func setWindowBounds(ctx context.Context, tconn *chrome.TestConn, windowID int, to coords.Rect) error {
	w, err := ash.GetWindow(ctx, tconn, windowID)
	if err != nil {
		return err
	}

	info, err := display.GetPrimaryInfo(ctx, tconn)
	if err != nil {
		return err
	}

	b, d, err := ash.SetWindowBounds(ctx, tconn, w.ID, to, info.ID)
	if err != nil {
		return err
	}
	if b != to {
		return errors.Errorf("unable to set window bounds; got: %v, want: %v", b, to)
	}
	if d != info.ID {
		return errors.Errorf("unable to set window display; got: %v, want: %v", d, info.ID)
	}
	return nil
}

func reduceDisplayZoomFactor(ctx context.Context, tconn *chrome.TestConn) (
	func(context.Context, *chrome.TestConn) error,
	error,
) {
	info, err := display.GetPrimaryInfo(ctx, tconn)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get the primary display info")
	}

	displayZoomFactors := info.AvailableDisplayZoomFactors
	if len(displayZoomFactors) == 0 {
		return nil, errors.New("failed to get AvailableDisplayZoomFactors")
	}
	originalZoom := info.DisplayZoomFactor
	newZoom := 0.0
	// Get the first zoom factor less than the original one.
	for i := len(displayZoomFactors) - 1; i >= 0; i-- {
		if displayZoomFactors[i] < originalZoom {
			newZoom = displayZoomFactors[i]
			break
		}
	}
	if newZoom == 0 {
		return nil, errors.Errorf("invalid AvailableDisplayZoomFactors: want array with at least one value less than '%.2f', got %v", originalZoom, displayZoomFactors)
	}

	if err := display.SetDisplayProperties(ctx, tconn, info.ID, display.DisplayProperties{DisplayZoomFactor: &newZoom}); err != nil {
		return nil, errors.Wrapf(err, "failed to set zoom factor of primary display to %f", newZoom)
	}
	return func(ctx context.Context, tconn *chrome.TestConn) error {
		if err := display.SetDisplayProperties(ctx, tconn, info.ID, display.DisplayProperties{DisplayZoomFactor: &originalZoom}); err != nil {
			return errors.Wrapf(err, "failed to revert zoom factor of primary display to %f", originalZoom)
		}
		return nil
	}, nil
}

// testInvocation describes a particular test run. A test run involves running a particular scenario
// (e.g. moveocclusion). This structure holds the necessary data to do this.
type testInvocation struct {
	pv       *perf.Values
	scenario TestType
	page     page
	metrics  *metricsRecorder
	traceDir string
}

// runTest runs the common part of the GpuCUJ performance test.
func runTest(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn, invoc *testInvocation) error {
	cooldownConfig := cpu.DefaultCoolDownConfig(cpu.CoolDownPreserveUI)
	// Reduce the timeout to avoid test timeouts.
	cooldownConfig.PollTimeout = 2 * time.Minute
	if _, err := cpu.WaitUntilStabilized(ctx, cooldownConfig); err != nil {
		testing.ContextLog(ctx, "Failed to wait for CPU to stabilize: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()
	conn, err := cr.NewConn(ctx, invoc.page.url)
	if err != nil {
		return errors.Wrapf(err, "failed to open %s", invoc.page)
	}
	defer conn.Close()
	defer conn.CloseTarget(cleanupCtx)

	scenario := invoc.scenario
	// Setup extra window for multi-window tests.
	if scenario == TestTypeMoveOcclusion || scenario == TestTypeMoveOcclusionWithCrosWindow {
		connBlank, err := cr.NewConn(ctx, chrome.BlankURL, browser.WithNewWindow())
		if err != nil {
			return errors.Wrap(err, "failed to open new tab")
		}
		defer connBlank.Close()
		defer connBlank.CloseTarget(cleanupCtx)
	}

	w, err := ash.WaitForAnyWindowWithoutTitle(ctx, tconn, "about:blank")
	if err != nil {
		return err
	}

	info, err := display.GetPrimaryInfo(ctx, tconn)
	if err != nil {
		return err
	}

	perfFn := func(ctx context.Context) error {
		// GoBigSleepLint: sleep to collect metrics.
		return testing.Sleep(ctx, testDuration)
	}
	if invoc.scenario == TestTypeResize {
		// Restore window.
		if err := ash.SetWindowStateAndWait(ctx, tconn, w.ID, ash.WindowStateNormal); err != nil {
			return errors.Wrap(err, "failed to restore non-blank window")
		}

		// Create a landscape rectangle. Avoid snapping by insetting by insetSlopDP.
		ms := math.Min(float64(info.WorkArea.Width), float64(info.WorkArea.Height))
		sb := coords.NewRect(info.WorkArea.Left, info.WorkArea.Top, int(ms), int(ms*0.6)).WithInset(insetSlopDP, insetSlopDP)
		if err := setWindowBounds(ctx, tconn, w.ID, sb); err != nil {
			return errors.Wrap(err, "failed to set window initial bounds")
		}

		perfFn = func(ctx context.Context) error {
			// End bounds are just flipping the rectangle.
			// TODO(crbug.com/1067535): Subtract -1 to ensure drag-resize occurs for now.
			start := coords.NewPoint(sb.Left+sb.Width-1, sb.Top+sb.Height-1)
			end := coords.NewPoint(sb.Left+sb.Height, sb.Top+sb.Width)

			if err := mouse.Drag(tconn, start, end, testDuration)(ctx); err != nil {
				return errors.Wrap(err, "failed to drag resize")
			}
			return nil
		}
	} else if invoc.scenario == TestTypeMoveOcclusion || invoc.scenario == TestTypeMoveOcclusionWithCrosWindow {
		wb, err := ash.WaitForAnyWindowWithTitle(ctx, tconn, "about:blank")
		if err != nil {
			return err
		}

		// Restore windows.
		if err := ash.SetWindowStateAndWait(ctx, tconn, w.ID, ash.WindowStateNormal); err != nil {
			return errors.Wrap(err, "failed to restore non-blank window")
		}

		if err := ash.SetWindowStateAndWait(ctx, tconn, wb.ID, ash.WindowStateNormal); err != nil {
			return errors.Wrap(err, "failed to restore blank window")
		}

		// Set content window to take up the left half of the screen in landscape, or top half in portrait.
		isp := info.WorkArea.Width < info.WorkArea.Height
		sbl := coords.NewRect(info.WorkArea.Left, info.WorkArea.Top, info.WorkArea.Width/2, info.WorkArea.Height)
		if isp {
			sbl = coords.NewRect(info.WorkArea.Left, info.WorkArea.Top, info.WorkArea.Width, info.WorkArea.Height/2)
		}
		sbl = sbl.WithInset(insetSlopDP, insetSlopDP)
		if err := setWindowBounds(ctx, tconn, w.ID, sbl); err != nil {
			return errors.Wrap(err, "failed to set non-blank window initial bounds")
		}

		// Set the occluding window to take up the right side of the screen in landscape, or bottom half in portrait.
		sbr := sbl.WithOffset(sbl.Width, 0)
		if isp {
			sbr = sbl.WithOffset(0, sbl.Height)
		}
		if err := setWindowBounds(ctx, tconn, wb.ID, sbr); err != nil {
			return errors.Wrap(err, "failed to set blank window initial bounds")
		}
		perfFn = func(ctx context.Context) error {
			// Drag from not occluding to completely occluding.
			start := coords.NewPoint(sbr.Left+dragMoveOffsetDP, sbr.Top+dragMoveOffsetDP)
			end := coords.NewPoint(sbl.Left+dragMoveOffsetDP, sbl.Top+dragMoveOffsetDP)

			if err := mouse.Drag(tconn, start, end, testDuration)(ctx); err != nil {
				return errors.Wrap(err, "failed to drag move")
			}
			return nil
		}
	} else {
		// Maximize window.
		if err := ash.SetWindowStateAndWait(ctx, tconn, w.ID, ash.WindowStateMaximized); err != nil {
			return errors.Wrap(err, "failed to maximize window")
		}
	}

	// Open the threedot menu if indicated.
	if invoc.scenario == TestTypeThreeDot {
		if err := toggleThreeDotMenu(ctx); err != nil {
			return errors.Wrap(err, "failed to open three dot menu")
		}
		defer toggleThreeDotMenu(cleanupCtx)
	}

	// GoBigSleepLint: sleep for three seconds after loading pages / setting up the environment.
	// Loading a page can cause some transient spikes in activity or similar
	// 'unstable' state. Unfortunately there's no clear condition to wait for like
	// there is before the test starts (CPU activity and temperature). Wait three
	// seconds before measuring performance stats to try to reduce the variance.
	// Three seconds seems to work for most of the pages we're using (checked via
	// manual inspection).
	testing.Sleep(ctx, 3*time.Second)

	return runHistogram(ctx, tconn, cr, invoc, perfFn)
}

// SetupPerfTest sets up the environment for a performance test.
// It disables Wi-Fi, Night Light, and enables Do Not Disturb mode.
// It returns a cleanup function that should be deferred to restore the original state.
func SetupPerfTest(ctx context.Context, tconn *chrome.TestConn, name string) (runCleanup func(context.Context) error, retErr error) {
	var cleanups []func(context.Context) error
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	runCleanup = func(ctx context.Context) error {
		var errs []error
		for i := len(cleanups) - 1; i >= 0; i-- {
			if err := cleanups[i](ctx); err != nil {
				errs = append(errs, err)
			}
		}
		return errors.Join(errs...)
	}

	// Set-up environment to be more consistent.
	sup, supCleanup := setup.New(name)
	cleanups = append(cleanups, supCleanup)
	defer func(ctx context.Context) {
		if retErr != nil {
			if err := runCleanup(ctx); err != nil {
				testing.ContextLog(ctx, "Failed to run cleanup: ", err)
			}
		}
	}(cleanupCtx)

	sup.Add(setup.PowerTest(ctx, tconn,
		setup.PowerTestOptions{Wifi: setup.DoNotChangeWifiInterfaces, NightLight: setup.DisableNightLight},
		setup.NewBatteryDischarge(false /*discharge*/, true /*ignoreErr*/, setup.DefaultDischargeThreshold),
	))

	if err := sup.Check(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to setup power test environment")
	}

	if err := quicksettings.SetDoNotDisturb(ctx, tconn, true); err != nil {
		return nil, errors.Wrap(err, "failed to enable do not disturb")
	}
	cleanupDoNotDisturb := func(ctx context.Context) error {
		return quicksettings.SetDoNotDisturb(ctx, tconn, false)
	}
	cleanups = append(cleanups, cleanupDoNotDisturb)

	// Disable automation feature for performance test.
	// ResetAutomation should be already called previously, but automation is implicitly enabled by
	// quicksettings.SetDoNotDisturb, so we ensure it is disabled by calling ResetAutomation again.
	// TODO(b/199815100): Call private API to block accessibility features here.
	if err := tconn.ResetAutomation(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to reset the automation feature")
	}

	return runCleanup, nil
}

// Run runs a GpuCUJ performance test.
func Run(ctx context.Context, cr *chrome.Chrome, params TestParams, serverURL, outDir string) error {
	// Use a shorter timeout to avoid hanging when connecting to the test API.
	sCtx, sCancel := context.WithTimeout(ctx, 30*time.Second)
	defer sCancel()
	tconn, err := cr.TestAPIConn(sCtx)
	if err != nil {
		return errors.Wrap(err, "failed to connect to test API")
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 20*time.Second)
	defer cancel()

	tabletCleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, false)
	if err != nil {
		return errors.Wrap(err, "failed to ensure tablet mode is disabled")
	}
	defer tabletCleanup(cleanupCtx)

	cleanup, err := SetupPerfTest(ctx, tconn, "GpuCUJ")
	if err != nil {
		return errors.Wrap(err, "failed to setup GpuCUJ test")
	}
	defer func(ctx context.Context) {
		if err := cleanup(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to run cleanup: ", err)
		}
	}(cleanupCtx)

	infos, err := display.GetInfo(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to get display info")
	}
	if len(infos) != 1 {
		return errors.New("failed to find unique display")
	}
	info := infos[0]
	if params.Rot90 {
		rot := 90
		if err := display.SetDisplayProperties(ctx, tconn, info.ID, display.DisplayProperties{Rotation: &rot}); err != nil {
			return errors.Wrap(err, "failed to rotate display")
		}
		defer func(ctx context.Context) {
			// Restore the initial rotation.
			if err := display.SetDisplayProperties(ctx, tconn, info.ID, display.DisplayProperties{Rotation: &info.Rotation}); err != nil {
				testing.ContextLog(ctx, "Failed to restore original display rotation: ", err)
			}
		}(cleanupCtx)
	} else if params.TestType == TestTypeMoveOcclusion || params.TestType == TestTypeMoveOcclusionWithCrosWindow {
		// According to b/320175701, the display width of the devices that failing
		// the tests are 1080px.
		const smallDisplayWidth = 1080
		// In the 'MoveOcclusion' scenario, when in landscape mode on devices with
		// small displays like 1080x675, the desired window width might be smaller
		// than the minimum width of the Chrome window. Check the width and reduce
		// the display zoom factor down by 1 if it's smaller than 'smallDisplayWidth'.
		if info.Bounds.Width <= smallDisplayWidth {
			testing.ContextLogf(ctx, "The width(%d) of the display is equal or smaller than %d px, set the display factor down by 1", info.Bounds.Width, smallDisplayWidth)

			revertZoom, err := reduceDisplayZoomFactor(ctx, tconn)
			if err != nil {
				return errors.Wrap(err, "failed to set the zoom factor of the primary display")
			}
			defer func(ctx context.Context) {
				// Restore the display zoom factor.
				if err := revertZoom(ctx, tconn); err != nil {
					testing.ContextLog(ctx, "Failed to restore original display zoom factor: ", err)
				}
			}(cleanupCtx)
		}
	}

	pv := perf.NewValues()
	m := metricsRecorder{buckets: make(map[statBucketKey][]float64), metricMap: make(map[string]metricInfo)}
	for _, page := range pageSet {
		if page.url[0] == '/' {
			page.url = serverURL + page.url
		}

		if err := runTest(ctx, cr, tconn, &testInvocation{
			pv:       pv,
			scenario: params.TestType,
			page:     page,
			metrics:  &m,
			traceDir: outDir,
		}); err != nil {
			return errors.Wrap(err, "failed to run cros test")
		}
	}

	if err := m.computeStatistics(ctx, pv); err != nil {
		return errors.Wrap(err, "could not compute derived statistics")
	}

	if err := pv.Save(outDir); err != nil {
		return errors.Wrap(err, "failed to save perf data")
	}
	return nil
}
