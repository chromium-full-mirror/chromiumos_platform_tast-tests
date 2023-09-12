// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/audio/crastestclient"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/ui/windowarrangementcuj"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/pointer"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	localPerf "go.chromium.org/tast-tests/cros/local/perf"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         WindowArrangementCUJ,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Measures the performance of critical user journey for window arrangements",
		Contacts: []string{
			"cros-sw-perf@google.com",
			"yichenz@chromium.org",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Attr:         []string{"group:cuj"},
		SoftwareDeps: []string{"chrome", "chrome_internal"},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Timeout:      15 * time.Minute,
		Data:         []string{"shaka_720.webm", "pip.html", cujrecorder.SystemTraceConfigFile},
		Params: []testing.Param{
			{
				Name: "clamshell_mode",
				Val: windowarrangementcuj.TestParam{
					BrowserType: browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUser",
			},
			{
				Name:              "tablet_mode",
				ExtraHardwareDeps: hwdep.D(hwdep.TouchScreen()),
				Val: windowarrangementcuj.TestParam{
					BrowserType: browser.TypeAsh,
					Tablet:      true,
				},
				Fixture: "loggedInToCUJUser",
			},
			{
				Name: "lacros",
				Val: windowarrangementcuj.TestParam{
					BrowserType: browser.TypeLacros,
				},
				Fixture:           "loggedInToCUJUserLacros",
				ExtraSoftwareDeps: []string{"lacros"},
			},
			{
				Name:              "tablet_mode_lacros",
				ExtraHardwareDeps: hwdep.D(hwdep.TouchScreen()),
				Val: windowarrangementcuj.TestParam{
					BrowserType: browser.TypeLacros,
					Tablet:      true,
				},
				Fixture:           "loggedInToCUJUserLacros",
				ExtraSoftwareDeps: []string{"lacros"},
			},
			// TODO(b/292249282): Remove when Vulkan is launched on brya, volteer, and skyrim
			{
				Name: "vulkan",
				Val: windowarrangementcuj.TestParam{
					BrowserType: browser.TypeAsh,
				},
				Fixture:           "loggedInToCUJUserVulkan",
				ExtraHardwareDeps: hwdep.D(hwdep.Model("redrix", "drobit", "frostflow")),
			},
		},
	})
}

func WindowArrangementCUJ(ctx context.Context, s *testing.State) {
	const (
		timeout  = 10 * time.Second
		duration = 2 * time.Second
	)

	// Shorten context a bit to allow for cleanup.
	closeCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 2*time.Second)
	defer cancel()

	pv, err := localPerf.CaptureDeviceSnapshot(ctx, "Initial")
	if err != nil {
		s.Fatal("Failed to capture device snapshot: ", err)
	}

	// Ensure display on to record ui performance correctly.
	if err := power.TurnOnDisplay(ctx); err != nil {
		s.Fatal("Failed to turn on display: ", err)
	}

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	testParam := s.Param().(windowarrangementcuj.TestParam)
	tabletMode := testParam.Tablet

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test api: ", err)
	}

	srv := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer srv.Close()
	pipVideoTestURL := srv.URL + "/pip.html"

	revertZoom, err := display.MinimizePrimaryDisplayZoomFactor(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to set the zoom factor of the primary display to minimum: ", err)
	}
	defer revertZoom(closeCtx, tconn)

	info, err := display.GetPrimaryInfo(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get the primary display info: ", err)
	}
	displayID := info.ID

	// Set to clamshell mode first and ensure landscape orientation is
	// observed. This ensures that when windowarrangementcuj.combineTabs
	// switches to clamshell mode, it won't cause the display to rotate.
	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, false)
	if err != nil {
		s.Fatal("Failed to ensure clamshell mode: ", err)
	}
	defer cleanup(closeCtx)
	orientation, err := display.GetOrientation(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to obtain the orientation info: ", err)
	}
	if orientation.Type == display.OrientationPortraitPrimary {
		if err := display.SetDisplayRotationSync(ctx, tconn, displayID, display.Rotate90); err != nil {
			s.Fatal("Failed to rotate display: ", err)
		}
		defer display.SetDisplayRotationSync(closeCtx, tconn, displayID, display.Rotate0)
	}

	if tabletMode {
		if err := ash.SetTabletModeEnabled(ctx, tconn, true); err != nil {
			s.Fatal("Failed to switch from clamshell to tablet: ", err)
		}
		// Defer a change back to clamshell mode so that the above deferred
		// call to display.SetDisplayRotationSync will still revert the
		// display orientation associated with clamshell mode.
		defer ash.SetTabletModeEnabled(closeCtx, tconn, false)

		// Ensure landscape orientation in tablet mode too.
		orientation, err := display.GetOrientation(ctx, tconn)
		if err != nil {
			s.Fatal("Failed to obtain the orientation info: ", err)
		}
		if orientation.Type == display.OrientationPortraitPrimary {
			if err := display.SetDisplayRotationSync(ctx, tconn, displayID, display.Rotate90); err != nil {
				s.Fatal("Failed to rotate display: ", err)
			}
			defer display.SetDisplayRotationSync(closeCtx, tconn, displayID, display.Rotate0)
		}
	}

	connNoPiP, br, closeBrowser, err := browserfixt.SetUpWithURL(ctx, cr, testParam.BrowserType, chrome.BlankURL)
	if err != nil {
		s.Fatal("Failed to setup Chrome: ", err)
	}
	defer closeBrowser(closeCtx)
	defer connNoPiP.Close()

	bTconn, err := br.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to browser test API connection: ", err)
	}

	tabChecker, err := cuj.NewTabCrashChecker(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to create TabCrashChecker: ", err)
	}

	// Set up the cujrecorder.Recorder: In clamshell mode, this test will measure the combinations of
	// input latency of tab dragging and of window resizing and of split view resizing, and
	// also the percent of dropped frames of video; In tablet mode, this test will measure
	// the combinations of input latency of tab dragging and of input latency of split view
	// resizing and the percent of dropped frames of video.
	var configs []cujrecorder.MetricConfig
	if !tabletMode {
		configs = append(configs, cujrecorder.NewLatencyMetricConfig("Ash.InteractiveWindowResize.TimeToPresent"))
	} else {
		configs = append(configs,
			cujrecorder.NewLatencyMetricConfig("Ash.TabDrag.PresentationTime.TabletMode"),
			cujrecorder.NewLatencyMetricConfig("Ash.SplitViewResize.PresentationTime.TabletMode.SingleWindow"),
			cujrecorder.NewLatencyMetricConfig("Ash.SplitViewResize.PresentationTime.TabletMode.WithOverview"),
			cujrecorder.NewLatencyMetricConfig("Ash.SplitViewResize.PresentationTime.TabletMode.MultiWindow"))
	}

	recorder, err := cujrecorder.NewRecorder(ctx, cr, bTconn, nil, cujrecorder.RecorderOptions{})
	if err != nil {
		s.Fatal("Failed to create a recorder: ", err)
	}

	if err := recorder.AddCollectedMetrics(bTconn, testParam.BrowserType, configs...); err != nil {
		s.Fatal("Failed to add metrics to recorder: ", err)
	}

	if err := recorder.AddCommonMetrics(tconn, bTconn); err != nil {
		s.Fatal("Failed to add common metrics to recorder: ", err)
	}

	// Take a screenshot every 13 seconds up to a maximum of 5
	// screenshots, to capture the first window arrangement cycle and
	// the beginning of the second cycle.
	if err := recorder.AddScreenshotRecorder(ctx, 13*time.Second, 5); err != nil {
		s.Log("Failed to add screenshot recorder: ", err)
	}

	defer recorder.Close(closeCtx)

	if err := crastestclient.Mute(ctx); err != nil {
		s.Fatal("Failed to mute audio: ", err)
	}
	defer crastestclient.Unmute(closeCtx)

	defer faillog.DumpUITreeOnError(closeCtx, s.OutDir(), s.HasError, tconn)

	if err := connNoPiP.Navigate(ctx, pipVideoTestURL); err != nil {
		s.Fatal("Failed to load pip.html: ", err)
	}
	// Close the browser window at the end of the test. If it is left playing a video, it
	// will cause the test server's Close() function to block for a few minutes.
	defer connNoPiP.CloseTarget(closeCtx)

	if err := webutil.WaitForQuiescence(ctx, connNoPiP, timeout); err != nil {
		s.Fatal("Failed to wait for pip.html to achieve quiescence: ", err)
	}

	connPiP, err := br.NewConn(ctx, pipVideoTestURL)
	if err != nil {
		s.Fatal("Failed to load pip.html: ", err)
	}
	defer connPiP.Close()
	// Close the browser window at the end of the test. If it is left playing a video, it
	// will cause the test server's Close() function to block for a few minutes.
	defer connPiP.CloseTarget(closeCtx)

	if err := webutil.WaitForQuiescence(ctx, connPiP, timeout); err != nil {
		s.Fatal("Failed to wait for pip.html to achieve quiescence: ", err)
	}

	ui := uiauto.New(tconn)

	// The second tab enters the system PiP mode.
	webview := nodewith.ClassName("ContentsWebView").Role(role.WebView)
	pipButton := nodewith.Name("Enter Picture-in-Picture").Role(role.Button).Ancestor(webview)
	if err := action.Combine(
		"focus the PIP button (to ensure it is in view) and left-click on it",
		ui.FocusAndWait(pipButton),
		ui.LeftClick(pipButton),
	)(ctx); err != nil {
		s.Fatal("Failed to click the pip button: ", err)
	}
	if err := webutil.WaitForQuiescence(ctx, connPiP, timeout); err != nil {
		s.Fatal("Failed to wait for quiescence: ", err)
	}

	// Check if "Picture in picture" window is displayed after clicking.
	pipWindow := nodewith.Name("Picture in picture").Role(role.Window).ClassName("Widget").First()
	if err := ui.WaitUntilExists(pipWindow)(ctx); err != nil {
		s.Fatal("Failed to wait for pip to be active: ", err)
	}

	// For clamshell variants, activate the tab on the left.
	if !tabletMode {
		var tabs []map[string]interface{}
		if err := bTconn.Call(ctx, &tabs,
			"tast.promisify(chrome.tabs.query)",
			map[string]interface{}{},
		); err != nil {
			s.Fatal("Failed to fetch browser tabs: ", err)
		}
		if len(tabs) != 2 {
			s.Errorf("Unexpected number of browser tabs; got %d, want 2", len(tabs))
		}
		if err := bTconn.Call(ctx, nil,
			"tast.promisify(chrome.tabs.update)",
			int(tabs[0]["id"].(float64)),
			map[string]interface{}{"active": true},
		); err != nil {
			s.Fatal("Failed to activate first tab: ", err)
		}
	}

	var pc pointer.Context
	if !tabletMode {
		pc = pointer.NewMouse(tconn)
	} else {
		pc, err = pointer.NewTouch(ctx, tconn)
		if err != nil {
			s.Fatal("Failed to create a touch controller: ", err)
		}
	}
	defer pc.Close(ctx)

	var f func(ctx context.Context) error
	traceRecorded := false
	if !tabletMode {
		f = func(ctx context.Context) error {
			// See go/trace-in-cuj-tests about rules for tracing.
			if !traceRecorded {
				if err := recorder.StartTracing(ctx, s.OutDir(), s.DataPath(cujrecorder.SystemTraceConfigFile)); err != nil {
					return errors.Wrap(err, "failed to start tracing")
				}
				traceRecorded = true
				defer recorder.StopTracing(ctx)
			}

			return windowarrangementcuj.RunClamShell(ctx, closeCtx, tconn, ui, pc)
		}
	} else {
		f = func(ctx context.Context) error {
			return windowarrangementcuj.RunTablet(ctx, closeCtx, br, tconn, ui, pc)
		}
	}

	// Run the recorder.
	if err := recorder.RunFor(ctx, f, 10*time.Minute); err != nil {
		s.Fatal("Failed to conduct the recorder task: ", err)
	}

	// Check if there is any tab crashed.
	if err := tabChecker.Check(ctx); err != nil {
		s.Fatal("Tab renderer crashed: ", err)
	}

	// Store perf metrics.
	if err := recorder.Record(ctx, pv); err != nil {
		s.Fatal("Failed to record the data: ", err)
	}
	if err := recorder.SaveTraceFiles(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to save trace files: ", err)
	}
	if err := pv.Save(s.OutDir()); err != nil {
		s.Fatal("Failed to save the perf data: ", err)
	}
}
