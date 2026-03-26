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
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	sim "go.chromium.org/tast-tests/cros/local/chrome/cuj/inputsimulations"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/event"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/prompts"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
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

	cnnWebSiteName    = "CNN"
	redditWebSiteName = "Reddit"
)

// tabSwitchRunner holds all the necessary variables used by the test.
type tabSwitchRunner struct {
	webPages           []webPageData // List of sites to visit
	cr                 *chrome.Chrome
	tconn              *chrome.TestConn
	recorder           *cujrecorder.Recorder
	outDir             string
	perfettoConfigPath string
	kb                 *input.KeyboardEventWriter
	ui                 *uiauto.Context
}

// webPageData holds the info used to visit new sites in the test.
type webPageData struct {
	name       string // Display Name of the Website
	startURL   string // Base URL to the Website
	urlPattern string // RegExp Pattern to Open Relevant Links on the Website
}

// coreTestDuration is a minimum duration for the core part of the test.
// The actual test duration could be longer because of various setup.
const coreTestDuration = 3 * time.Minute

func newTabSwitchRunner(ctx context.Context, cr *chrome.Chrome, outDir, perfettoConfigPath string) (r *tabSwitchRunner, retErr error) {
	r = &tabSwitchRunner{
		webPages:           getTestWebpages(),
		cr:                 cr,
		outDir:             outDir,
		perfettoConfigPath: perfettoConfigPath,
	}

	var err error
	r.tconn, err = r.cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get ash-chrome test connection")
	}

	r.ui = uiauto.New(r.tconn)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	r.kb, err = input.Keyboard(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open the keyboard")
	}
	defer func(ctx context.Context) {
		if retErr != nil {
			r.kb.Close(ctx)
		}
	}(cleanupCtx)

	r.recorder, err = cujrecorder.NewRecorder(ctx, r.tconn, r.cr, nil, cujrecorder.RecorderOptions{
		Mode:              cujrecorder.Benchmark,
		CooldownBeforeRun: true,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create a recorder")
	}
	defer func(ctx context.Context) {
		if retErr != nil {
			r.recorder.Close(ctx)
		}
	}(cleanupCtx)

	if err := r.recorder.AddCommonMetrics(); err != nil {
		return nil, errors.Wrap(err, "failed to add common metrics to the recorder")
	}

	// Add an empty screenshot recorder.
	if err := r.recorder.AddScreenshotRecorder(ctx, 0, 0); err != nil {
		testing.ContextLog(ctx, "Failed to add screenshot recorder: ", err)
	}

	return r, nil
}

func (r *tabSwitchRunner) cleanup(ctx context.Context) error {
	var errs []error

	if err := r.kb.Close(ctx); err != nil {
		errs = append(errs, errors.Wrap(err, "failed to close the keyboard"))
	}

	if err := r.recorder.Close(ctx); err != nil {
		errs = append(errs, errors.Wrap(err, "failed to close the recorder"))
	}

	return errors.Join(errs...)
}

func (r *tabSwitchRunner) muteDevice(ctx context.Context, mute bool) error {
	// The custom variable for the developer to mute the device before the test,
	// so it doesn't make any noise when some of the visited pages play video.
	if !mute {
		return nil
	}
	topRow, err := input.KeyboardTopRowLayout(ctx, r.kb)
	if err != nil {
		return errors.Wrap(err, "failed to obtain the top-row layout")
	}
	if err = r.kb.Accel(ctx, topRow.VolumeMute); err != nil {
		return errors.Wrap(err, "failed to press mute key")
	}

	return nil
}

func (r *tabSwitchRunner) run(ctx context.Context) error {
	if len(r.webPages) == 0 {
		return errors.New("test scenario does not specify any web pages")
	}

	for webNum, webPage := range r.webPages {
		if err := r.startWebPageAndPerformTest(ctx, webPage, webNum == 0 /* isFirstPage */); err != nil {
			return errors.Wrap(err, "failed to start web page and perform test")
		}
	}
	return nil
}

func (r *tabSwitchRunner) startWebPageAndPerformTest(ctx context.Context, webPage webPageData, isFirstPage bool) (retErr error) {
	r.recorder.Annotate(ctx, "Start_opening_"+webPage.name)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 15*time.Second)
	defer cancel()

	// Create the homepage of the site.
	conn, err := r.cr.NewConn(ctx, webPage.startURL)
	if err != nil {
		return errors.Wrapf(err, "failed to open %s", webPage.startURL)
	}
	defer conn.Close()
	defer conn.CloseTarget(cleanupCtx)

	if webPage.name == redditWebSiteName {
		if err := prompts.ClearPotentialPrompts(r.tconn, 15*time.Second, prompts.ShowNotificationsPrompt)(ctx); err != nil {
			return errors.Wrap(err, "failed to clear notifications prompt dialog")
		}
	}

	const totalPageForWeb = 7

	conns := make([]*chrome.Conn, 0, totalPageForWeb)
	conns = append(conns, conn)

	// Find extra URLs to navigate to.
	anchorURLs, err := findAnchorURLs(ctx, conn, webPage.urlPattern, totalPageForWeb-1)
	if err != nil {
		return errors.Wrapf(err, "failed to get URLs for %s", webPage.startURL)
	}

	if len(anchorURLs) != totalPageForWeb-1 {
		return errors.Errorf("failed to find the expected number of anchor URLs, got: %d, want: %d", len(anchorURLs), totalPageForWeb-1)
	}

	// Open those found URLs as new tabs.
	for _, anchorURL := range anchorURLs {
		conn, err := r.cr.NewConn(ctx, anchorURL)
		if err != nil {
			return errors.Wrapf(err, "failed to open URL: %s", anchorURL)
		}
		defer conn.Close()
		defer conn.CloseTarget(cleanupCtx)

		conns = append(conns, conn)
	}

	// Ensure that all tabs are properly loaded before starting test.
	if err := r.waitUntilAllTabsLoaded(ctx, time.Minute); err != nil {
		testing.ContextLog(ctx, "Some tabs are still in loading state, but proceeding with the test: ", err)
	}
	defer faillog.DumpUITreeWithScreenshotWithTestAPIOnError(cleanupCtx, r.outDir, func() bool { return retErr != nil }, r.tconn, fmt.Sprintf("tab_switch_ui_dump_%s", webPage.name))

	if err := r.scrollEachTab(ctx, conns, webPage.name, isFirstPage); err != nil {
		return errors.Wrap(err, "failed to scroll each tab")
	}

	// Take a screenshot to see the status of the CNN/Reddit
	// window before closing it.
	r.recorder.CustomScreenshot(ctx)

	return browser.CloseAllTabs(ctx, r.tconn)
}

func (r *tabSwitchRunner) scrollEachTab(ctx context.Context, conns []*chrome.Conn, webPageName string, isFirstPage bool) (retErr error) {
	// Switch through tabs in a skip-order fashion.
	// Note: when skipSize = N-1, then the skip-order is 1,1,1,1 ... N times
	// And when skipSize >= N, it is effectively equal to skipSize % N.
	// When skipSize = N, the skip-order will be 1,2,3,4 ... N.
	var skipSize, i, currentTab int

	// Repeat the test as many times as necessary to fulfill its time requirements.
	// e.g. If there are two windows that need to be tested sequentially, and the
	// total core test duration is 10 mins, each window will be tested for 5 mins.
	//
	// Note: Test runs for coreTestDuration minutes.
	testDuration := coreTestDuration/time.Duration(len(r.webPages)) + time.Second
	endTime := time.Now().Add(testDuration)
	r.recorder.Annotate(ctx, "Start_tab_switching_"+webPageName)
	for time.Now().Before(endTime) {
		// Only collect the tracing data at the very beginning (first web pages, first skip-order).
		// Start and stop tracing at the beginning (i == 0) of the skip-order.
		if isFirstPage && i == 0 {
			// Start tracing at the beginning of the skipSize 0.
			if skipSize == 0 {
				// See go/trace-in-cuj-tests about rules for tracing.
				if err := r.recorder.StartTracing(ctx, r.outDir, r.perfettoConfigPath); err != nil {
					return errors.Wrap(err, "failed to start tracing")
				}
				// Stop tracing at the beginning of the skipSize 1.
			} else if skipSize == 1 {
				if err := r.recorder.StopTracing(ctx); err != nil {
					return errors.Wrap(err, "failed to stop tracing")
				}
			}
		}

		tabIcon := nodewith.HasClass("TabIcon").Nth(currentTab)
		contentsWebView := nodewith.Role(role.WebView).HasClass("ContentsWebView")
		if err := uiauto.Combine(
			"switch to the tab to be browsed.",
			r.ui.MouseMoveTo(tabIcon, 20*time.Millisecond),
			r.ui.LeftClick(tabIcon),
			r.ui.MouseMoveTo(contentsWebView, 20*time.Millisecond),
			r.ui.EnsureFocused(contentsWebView),
		)(ctx); err != nil {
			return err
		}

		if err := webutil.WaitForQuiescence(ctx, conns[currentTab], 30*time.Second); err != nil {
			return errors.Wrap(err, "failed to wait for the tab to quiesce")
		}

		for _, key := range []string{"Down", "Up"} {
			if err := sim.RepeatKeyPress(ctx, r.kb, key, 20*time.Millisecond, 3); err != nil {
				return errors.Wrapf(err, "failed to repeatedly press %s in between tab switches", key)
			}
		}

		if err := r.ui.WithTimeout(5*time.Second).WaitUntilNoEvent(nodewith.Root(), event.LocationChanged)(ctx); err != nil {
			testing.ContextLog(ctx, "Scroll animations haven't stabilized yet, continuing anyway: ", err)
		}

		currentTab = (currentTab + skipSize + 1) % len(conns)

		// Once we have seen every tab, adjust the skipSize to
		// vary the tab visitation order.
		if i == len(conns)-1 {
			i = 0
			currentTab = 0
			skipSize++
		} else {
			i++
		}
	}

	return nil
}

func (r *tabSwitchRunner) waitUntilAllTabsLoaded(ctx context.Context, timeout time.Duration) error {
	query := map[string]interface{}{
		"status":        "loading",
		"currentWindow": true,
	}
	return testing.Poll(ctx, func(ctx context.Context) error {
		var tabs []map[string]interface{}
		if err := r.tconn.Call(ctx, &tabs, `tast.promisify(chrome.tabs.query)`, query); err != nil {
			return testing.PollBreak(err)
		}
		if len(tabs) != 0 {
			return errors.Errorf("still %d tabs are loading", len(tabs))
		}
		return nil
	}, &testing.PollOptions{Timeout: timeout})
}

// Run runs the setup, core part of the TabSwitchPerf test, and cleanup.
func Run(ctx context.Context, cr *chrome.Chrome, mute bool, outDir, dataPath string) error {
	// Reserve time for cleanup
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	pv, err := localPerf.CaptureDeviceSnapshot(ctx, "Initial")
	if err != nil {
		return errors.Wrap(err, "failed to capture device snapshot")
	}

	// Perform initial test setup
	r, err := newTabSwitchRunner(ctx, cr, outDir, dataPath)
	if err != nil {
		return errors.Wrap(err, "failed to run setup")
	}
	defer r.cleanup(cleanupCtx)

	if err := r.muteDevice(ctx, mute); err != nil {
		testing.ContextLog(ctx, "(non-error) Failed to mute device: ", err)
	}

	// Execute Test
	if err := r.recorder.Run(ctx, r.run); err != nil {
		return errors.Wrap(err, "failed to conduct the test scenario, or collect the histogram data")
	}

	// Write out values
	if err := r.recorder.Record(ctx, pv); err != nil {
		return errors.Wrap(err, "failed to report")
	}
	if err := r.recorder.SaveTraceFiles(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to save trace files: ", err)
	}
	if err := pv.Save(outDir); err != nil {
		return errors.Wrap(err, "failed to store values")
	}
	return nil
}

func getTestWebpages() []webPageData {
	CNN := webPageData{
		name:       cnnWebSiteName,
		startURL:   "https://cnn.com",
		urlPattern: `^.*://www.cnn.com/\d{4}/\d{2}/\d{2}/`,
	}

	Reddit := webPageData{
		name:       redditWebSiteName,
		startURL:   "https://reddit.com",
		urlPattern: `^.*://www.reddit.com/r/[^/]+/comments/[^/]+/`,
	}

	return []webPageData{CNN, Reddit}
}

// findAnchorURLs returns the unique URLs of the anchors, which matches the pattern.
// If it finds more than limit, returns the first limit elements.
func findAnchorURLs(ctx context.Context, conn *chrome.Conn, pattern string, limit int) ([]string, error) {
	var urls []string
	if err := conn.Call(ctx, &urls, `(pattern, limit) => {
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
