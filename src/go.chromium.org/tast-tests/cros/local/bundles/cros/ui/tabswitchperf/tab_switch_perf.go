// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package tabswitchperf contains the test code for TabSwitchPerf. The test is
// extracted into this package to be shared between TabSwitchPerfRecorder and
// TabSwitchPerf.
//
// Steps to update the test:
//  1. Make changes in this package.
//  2. "tast run $IP ui.TabSwitchPerfRecorder" to record the contents.
//     Look for the recorded wpr archive in /tmp/tab_switch_perf.wprgo.
//  3. Update the recorded wpr archive to cloud storage under
//     gs://chromiumos-test-assets-public/tast/cros/ui/
//     It is recommended to add a date suffix to make it easier to change.
//  4. Update "tab_switch_perf.wprgo.external" file under ui/data.
//  5. "tast run $IP ui.TabSwitchPerf" locally to make sure tests works
//     with the new recorded contents.
//  6. Submit the changes here with updated external data reference.
package tabswitchperf

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	sim "go.chromium.org/tast-tests/cros/local/chrome/cuj/inputsimulations"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/event"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	"go.chromium.org/tast-tests/cros/local/input"
	localPerf "go.chromium.org/tast-tests/cros/local/perf"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// WPRArchiveName is used as the external file name of the wpr archive for
	// TabSwitchPerf and as the output filename under "/tmp" for
	// TabSwitchPerfRecorder.
	WPRArchiveName = "tab_switch_perf.wprgo"
)

// TabSwitchParam holds parameters of TabSwitchPerf test variations.
type TabSwitchParam struct {
	BrowserType browser.Type // Chrome type.
}

// tabSwitchVariables holds all the necessary variables used by the test.
type tabSwitchVariables struct {
	param    TabSwitchParam // Test Parameters
	webPages []webPage      // List of sites to visit

	cr                 *chrome.Chrome
	br                 *browser.Browser
	closeBrowser       func(context.Context) error
	tconn              *chrome.TestConn
	bTconn             *chrome.TestConn
	recorder           *cujrecorder.Recorder
	outDir             string
	perfettoConfigPath string
}

// webPage holds the info used to visit new sites in the test.
type webPage struct {
	name       string // Display Name of the Website
	startURL   string // Base URL to the Website
	urlPattern string // RegExp Pattern to Open Relevant Links on the Website
}

// coreTestDuration is a minimum duration for the core part of the test.
// The actual test duration could be longer because of various setup.
const coreTestDuration = 3 * time.Minute

func runSetup(ctx context.Context, s *testing.State) (*tabSwitchVariables, error) {
	vars := tabSwitchVariables{
		param:              s.Param().(TabSwitchParam),
		webPages:           getTestWebpages(),
		cr:                 s.FixtValue().(chrome.HasChrome).Chrome(),
		outDir:             s.OutDir(),
		perfettoConfigPath: s.DataPath(cujrecorder.SystemTraceConfigFile),
	}

	var err error
	vars.br, vars.closeBrowser, err = browserfixt.SetUp(ctx, vars.cr, vars.param.BrowserType)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open the browser")
	}

	vars.bTconn, err = vars.br.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get browser TestAPIConn")
	}

	vars.recorder, err = cujrecorder.NewRecorder(ctx, vars.cr, vars.bTconn, nil, cujrecorder.RecorderOptions{
		Mode:              cujrecorder.Benchmark,
		CooldownBeforeRun: true,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create a recorder")
	}
	metricsSuccessfullyAdded := false
	defer func(ctx context.Context) {
		if metricsSuccessfullyAdded {
			return
		}
		vars.closeBrowser(ctx)
		vars.recorder.Close(ctx)
	}(ctx)

	vars.tconn, err = vars.cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get ash-chrome test connection")
	}

	if err := vars.recorder.AddCommonMetrics(vars.tconn, vars.bTconn); err != nil {
		s.Fatal("Failed to add common metrics to the recorder: ", err)
	}

	// Add an empty screenshot recorder.
	if err := vars.recorder.AddScreenshotRecorder(ctx, 0, 0); err != nil {
		s.Log("Failed to add screenshot recorder: ", err)
	}

	metricsSuccessfullyAdded = true
	return &vars, nil
}

func getTestWebpages() []webPage {
	CNN := webPage{
		name:       "CNN",
		startURL:   "https://cnn.com",
		urlPattern: `^.*://www.cnn.com/\d{4}/\d{2}/\d{2}/`,
	}

	Reddit := webPage{
		name:       "Reddit",
		startURL:   "https://reddit.com",
		urlPattern: `^.*://www.reddit.com/r/[^/]+/comments/[^/]+/`,
	}

	return []webPage{CNN, Reddit}
}

func muteDevice(ctx context.Context, s *testing.State) error {
	// The custom variable for the developer to mute the device before the test,
	// so it doesn't make any noise when some of the visited pages play video.
	if _, ok := s.Var("mute"); !ok {
		return nil
	}

	kw, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to open the keyboard")
	}
	defer kw.Close(ctx)

	topRow, err := input.KeyboardTopRowLayout(ctx, kw)
	if err != nil {
		return errors.Wrap(err, "failed to obtain the top-row layout")
	}
	if err = kw.Accel(ctx, topRow.VolumeMute); err != nil {
		return errors.Wrap(err, "failed to press mute key")
	}

	return nil
}

// findAnchorURLs returns the unique URLs of the anchors, which matches the pattern.
// If it finds more than limit, returns the first limit elements.
func findAnchorURLs(ctx context.Context, c *chrome.Conn, pattern string, limit int) ([]string, error) {
	var urls []string
	if err := c.Call(ctx, &urls, `(pattern, limit) => {
		const anchors = [...document.getElementsByTagName('A')];
		const founds = new Set();
		const results = [];
		const regexp = new RegExp(pattern);
		for (let i = 0; i < anchors.length && results.length < limit; i++) {
		  const href = new URL(anchors[i].href).toString();
		  if (founds.has(href)) {
		    continue;
		  }
		  founds.add(href);
		  if (regexp.test(href)) {
		    results.push(href);
		  }
		}
		return results;
	}`, pattern, limit); err != nil {
		return nil, err
	}
	if len(urls) == 0 {
		return nil, errors.New("no urls found")
	}
	return urls, nil
}

func waitUntilAllTabsLoaded(ctx context.Context, tconn *chrome.TestConn, timeout time.Duration) error {
	query := map[string]interface{}{
		"status":        "loading",
		"currentWindow": true,
	}
	return testing.Poll(ctx, func(ctx context.Context) error {
		var tabs []map[string]interface{}
		if err := tconn.Call(ctx, &tabs, `tast.promisify(chrome.tabs.query)`, query); err != nil {
			return testing.PollBreak(err)
		}
		if len(tabs) != 0 {
			return errors.Errorf("still %d tabs are loading", len(tabs))
		}
		return nil
	}, &testing.PollOptions{Timeout: timeout})
}

func retrieveAllTabs(ctx context.Context, tconn *chrome.TestConn, timeout time.Duration) ([]map[string]interface{}, error) {
	emptyQuery := map[string]interface{}{}

	// Get all tabs
	var tabs []map[string]interface{}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	err := tconn.Call(ctx, &tabs, `tast.promisify(chrome.tabs.query)`, emptyQuery)
	return tabs, err
}

func focusTab(ctx context.Context, tconn *chrome.TestConn, tabs *[]map[string]interface{}, tabIndexWithinWindow int, timeout time.Duration) error {
	// Define parameters for API calls
	activateTabProperties := map[string]interface{}{
		"active": true,
	}

	// Find id of tab with positional index.
	tabID := int((*tabs)[tabIndexWithinWindow]["id"].(float64))

	// Switch to this tab as the active window
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return tconn.Call(ctx, nil, `tast.promisify(chrome.tabs.update)`, tabID, activateTabProperties)
}

func testBody(ctx context.Context, test *tabSwitchVariables) error {
	const (
		numPages         = 7
		tabSwitchTimeout = 10 * time.Second
	)

	info, err := display.GetPrimaryInfo(ctx, test.tconn)
	if err != nil {
		return errors.Wrap(err, "failed to get the primary display info")
	}

	kw, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to open the keyboard")
	}
	defer kw.Close(ctx)

	// Create a virtual mouse.
	mw, err := input.Mouse(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create a mouse")
	}
	defer mw.Close(ctx)

	ac := uiauto.New(test.tconn)

	for index, data := range test.webPages {
		conns := make([]*chrome.Conn, 0, numPages)
		test.recorder.Annotate(ctx, "Start_opening_"+data.name)
		// Create the homepage of the site.
		firstPage, err := test.br.NewConn(ctx, data.startURL)
		if err != nil {
			return errors.Wrapf(err, "failed to open %s", data.startURL)
		}
		conns = append(conns, firstPage)

		if test.param.BrowserType == browser.TypeLacros {
			if err := browser.CloseTabByTitle(ctx, test.bTconn, "New Tab"); err != nil {
				return errors.Wrap(err, `failed to close "New Tab" tab`)
			}
		}

		// Find extra urls to navigate to.
		urls, err := findAnchorURLs(ctx, firstPage, data.urlPattern, numPages-1)
		if err != nil {
			return errors.Wrapf(err, "failed to get URLs for %s", data.startURL)
		}

		// Open those found URLs as new tabs.
		for _, url := range urls {
			newConnection, err := test.br.NewConn(ctx, url)
			if err != nil {
				return errors.Wrapf(err, "failed to open the URL %s", url)
			}
			conns = append(conns, newConnection)
		}

		// Ensure that all tabs are properly loaded before starting test.
		if err := waitUntilAllTabsLoaded(ctx, test.bTconn, time.Minute); err != nil {
			testing.ContextLog(ctx, "Some tabs are still in loading state, but proceeding with the test: ", err)
		}

		// Repeat the test as many times as necessary to fulfill its time requirements.
		// e.g. If there are two windows that need to be tested sequentially, and the
		// total core test duration is 10 mins, each window will be tested for 5 mins.
		//
		// Note: Test runs for coreTestDuration minutes.
		if len(test.webPages) == 0 {
			return errors.New("test scenario does not specify any web pages")
		}
		// Switch through tabs in a skip-order fashion.
		// Note: when skipSize = N-1, then the skip-order is 1,1,1,1 ... N times
		// Therefore i + skipSize + 1 % N holds when 0 <= skipSize < N-1
		skipSize := 0
		i := 0
		currentTab := 0
		endTime := time.Now().Add(coreTestDuration/time.Duration(len(test.webPages)) + time.Second)
		test.recorder.Annotate(ctx, "Start_tab_switching_"+data.name)
		for time.Now().Before(endTime) {
			if index == 0 && i == 0 {
				if skipSize == 0 {
					// See go/trace-in-cuj-tests about rules for tracing.
					if err := test.recorder.StartTracing(ctx, test.outDir, test.perfettoConfigPath); err != nil {
						return errors.Wrap(err, "failed to start tracing")
					}
				} else if skipSize == 1 {
					if err := test.recorder.StopTracing(ctx); err != nil {
						return errors.Wrap(err, "failed to stop tracing")
					}
				}
			}
			tabToClick := nodewith.HasClass("TabIcon").Nth(currentTab)
			if err := action.Combine(
				"click on tab and move mouse back to the center of the display",
				ac.MouseMoveTo(tabToClick, 20*time.Millisecond),
				ac.LeftClick(tabToClick),
				mouse.Move(test.tconn, info.Bounds.CenterPoint(), 20*time.Millisecond),
			)(ctx); err != nil {
				return err
			}

			if err := webutil.WaitForQuiescence(ctx, conns[currentTab], tabSwitchTimeout); err != nil {
				return errors.Wrap(err, "failed to wait for the tab to quiesce")
			}

			for _, key := range []string{"Down", "Up"} {
				if err := sim.RepeatKeyPress(ctx, kw, key, 20*time.Millisecond, 3); err != nil {
					return errors.Wrapf(err, "failed to repeatedly press %s in between tab switches", key)
				}
			}

			if err := ac.WithTimeout(5*time.Second).WaitUntilNoEvent(nodewith.Root(), event.LocationChanged)(ctx); err != nil {
				testing.ContextLog(ctx, "Scroll animations haven't stabilized yet, continuing anyway: ", err)
			}

			currentTab = (currentTab + skipSize + 1) % len(conns)

			// Once we have seen every tab, adjust the skipSize to
			// vary the tab visitation order.
			if i == len(conns)-1 {
				i = 0
				currentTab = 0
				skipSize = (skipSize + 1) % len(conns)
			} else {
				i++
			}
		}

		// Take a screenshot to see the status of the CNN/Reddit
		// window before closing it.
		test.recorder.CustomScreenshot(ctx)

		switch test.param.BrowserType {
		case browser.TypeLacros:
			if err := browser.ReplaceAllTabsWithSingleNewTab(ctx, test.bTconn); err != nil {
				return errors.Wrap(err, "failed to close all tabs and leave a single new tab open")
			}
		case browser.TypeAsh:
			if err := browser.CloseAllTabs(ctx, test.bTconn); err != nil {
				return errors.Wrap(err, "failed to close all tabs")
			}
		default:
			return errors.Errorf("unsupported browser type %v", test.param.BrowserType)
		}
	}

	return nil
}

// Run runs the setup, core part of the TabSwitchPerf test, and cleanup.
func Run(ctx context.Context, s *testing.State) {
	// Reserve time for cleanup
	closeCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 2*time.Second)
	defer cancel()

	pv, err := localPerf.CaptureDeviceSnapshot(ctx, "Initial")
	if err != nil {
		s.Fatal("Failed to capture device snapshot: ", err)
	}

	// Perform initial test setup
	setupVars, err := runSetup(ctx, s)
	if err != nil {
		s.Fatal("Failed to run setup: ", err)
	}
	defer setupVars.closeBrowser(closeCtx)
	defer setupVars.recorder.Close(closeCtx)

	if err := muteDevice(ctx, s); err != nil {
		s.Log("(non-error) Failed to mute device: ", err)
	}

	// Execute Test
	if err := setupVars.recorder.Run(ctx, func(ctx context.Context) error {
		return testBody(ctx, setupVars)
	}); err != nil {
		s.Fatal("Failed to conduct the test scenario, or collect the histogram data: ", err)
	}

	// Write out values
	if err := setupVars.recorder.Record(ctx, pv); err != nil {
		s.Fatal("Failed to report: ", err)
	}
	if err := setupVars.recorder.SaveTraceFiles(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to save trace files: ", err)
	}
	if err := pv.Save(s.OutDir()); err != nil {
		s.Error("Failed to store values: ", err)
	}
}
