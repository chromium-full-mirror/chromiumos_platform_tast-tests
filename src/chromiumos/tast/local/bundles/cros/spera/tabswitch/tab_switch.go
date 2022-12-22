// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package tabswitch contains the test code for TabSwitch. The test is
// extracted into this package to be shared between TabSwitchRecorder and
// TabSwitch.
package tabswitch

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/mafredri/cdp/protocol/target"

	"chromiumos/tast/common/perf"
	"chromiumos/tast/common/testexec"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/apps"
	"chromiumos/tast/local/audio/crastestclient"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/cuj"
	"chromiumos/tast/local/chrome/metrics"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/local/chrome/webutil"
	"chromiumos/tast/local/ui/cujrecorder"
	"chromiumos/tast/testing"
)

const (
	shortUITimeout = 5 * time.Second

	tabSwitchTimeout = 2 * time.Minute
	clickLinkTimeout = 1 * time.Minute

	replayPageLoadingTimeout    = 2 * time.Minute
	recordingPageLoadingTimeout = 5 * time.Minute

	// LocalWebZIPFile is the name of the ZIP file used to construct the local web pages.
	LocalWebZIPFile = "TabSwitchLocalWeb.zip"
	localWebFolder  = "TabSwitchLocalWeb"
)

// pageLoadingTimeout returns the timeout value when waiting for a page being loaded.
func pageLoadingTimeout(isRecordMode bool) time.Duration {
	// In record mode, give more time to loading to ensure web content is fully recorded.
	if isRecordMode {
		return recordingPageLoadingTimeout
	}
	return replayPageLoadingTimeout
}

// chromeTab holds the information of a Chrome browser tab.
type chromeTab struct {
	conn           *chrome.Conn
	pageInfo       *webPageInfo // static information (e.g. the type of website being visited, which contents to search to click) of this tab
	url            string       // current url of the website being visited
	currentPattern int          // the index of current page's corresponding content pattern within pageInfo
}

var (
	errHTMLelementNotFound = errors.New("failed to find HTML element on page")
	errNotClickAndNavigate = errors.New("has not clicked HTML link and navigate")
)

func (tab *chromeTab) searchElementWithPatternAndClick(ctx context.Context, patterns []string) (int, error) {
	if err := tab.conn.Eval(ctx, "window.location.href", &tab.url); err != nil {
		return 0, errors.Wrap(err, "failed to get URL")
	}
	testing.ContextLogf(ctx, "Current URL: %q", tab.url)

	const (
		statusDone            = -1
		statusElementNotFound = -2
	)

	links := `'` + strings.Join(patterns, `', '`) + `'`
	script := fmt.Sprintf(`() => {
		if (window.location.href !== '%s') {
			return %d;
		}
		const elements = [%s];
		for (let i = 0; i < elements.length; i++) {
			const pattern = elements[i];
			const name = "a[href*='" + pattern + "']";
			const els = document.querySelectorAll(name);
			if (els.length === 0) {
				continue;
			} else {
				els[0].click();
				return i;
			}
		}
		return %d;
	}`, tab.url, statusDone, links, statusElementNotFound)

	pattern := ""
	foundPatternIndex := 0
	timeout := 90 * time.Second
	numberOfRetryForJsError := 5
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		status := 0
		if err := tab.conn.Call(ctx, &status, script); err != nil {
			// [cienet private] retry JS script evaluation for error 211
			if numberOfRetryForJsError <= 0 {
				return testing.PollBreak(errors.Wrap(err, "failed to execute JavaScript query to click HTML link to navigate"))
			}
			testing.ContextLog(ctx, "Retry JavaScript query on error")
			numberOfRetryForJsError--
		}
		if status == statusElementNotFound {
			return testing.PollBreak(errHTMLelementNotFound)
		}
		if status == statusDone {
			return errNotClickAndNavigate
		}
		// When "status" is greater than or equal to 0, returns an integer representing the index of the element that can be found.
		if status > 0 {
			testing.ContextLog(ctx, "Missing patterns: ", patterns[0:status])
		}
		foundPatternIndex = status
		pattern = patterns[foundPatternIndex]
		return nil
	}, &testing.PollOptions{Timeout: timeout, Interval: 100 * time.Millisecond}); err != nil {
		return 0, errors.Wrapf(err, "failed to click HTML element and navigate within %v", timeout)
	}
	if err := tab.conn.Eval(ctx, "window.location.href", &tab.url); err != nil {
		return 0, errors.Wrap(err, "failed to get URL")
	}
	testing.ContextLogf(ctx, "HTML element clicked [%s], page navigates to: %q", pattern, tab.url)
	tab.url = strings.TrimSuffix(tab.url, "/")

	return foundPatternIndex, nil
}

func (tab *chromeTab) clickAnchor(ctx context.Context, timeout time.Duration, tconn *chrome.TestConn) error {
	const (
		histogramName     = "PageLoad.PaintTiming.NavigationToLargestContentfulPaint2"
		histogramWaitTime = time.Second
	)
	p := tab.currentPattern
	pn := p + 1
	numPatterns := len(tab.pageInfo.contentPatterns)
	if pn == numPatterns {
		pn = 0
	}

	if err := webutil.WaitForQuiescence(ctx, tab.conn, timeout); err != nil {
		// It has been seen that content sites such as ESPN sometimes can take minutes to reach
		// quiescence on DUTs. When this occurred, it can be seen from screenshots that the UI has
		// actually loaded but background tasks prevented the site to reach quiescence. Therefore,
		// logic is added here to check whether the site has loaded. If the site has loaded, i.e.,
		// the site readyState is not "loading", no error will be returned here.
		if err := tab.conn.WaitForExprFailOnErrWithTimeout(ctx, `document.readyState === "interactive" || document.readyState === "complete"`, 3*time.Second); err != nil {
			return errors.Wrapf(err, "failed to wait for tab to load within %v before clicking anchor", timeout)
		}
		testing.ContextLogf(ctx, "%s could not reach quiescence within %v, but document state has passed loading", tab.url, timeout)
	}

	h1, err := metrics.GetHistogram(ctx, tconn, histogramName)
	if err != nil {
		return errors.Wrap(err, "failed to get histogram")
	}
	testing.ContextLog(ctx, "Got LCP2 histogram: ", h1)

	patternsToFind := append(tab.pageInfo.contentPatterns[pn:numPatterns], tab.pageInfo.contentPatterns[0:p]...)
	foundPatternIndex, err := tab.searchElementWithPatternAndClick(ctx, patternsToFind)
	if err != nil {
		// Check whether the failure to search and click pattern was due to issues on the content site.
		if contentSiteErr := contentSiteUnavailable(ctx, tconn); contentSiteErr != nil {
			return errors.Wrapf(contentSiteErr, "failed to show content on page %s", tab.url)
		}
		return errors.Wrapf(err, "failed to click anchor on page %s", tab.url)
	}

	testing.ContextLogf(ctx, "Waiting for %v histogram update", histogramName)
	h2, err := metrics.WaitForHistogramUpdate(ctx, tconn, histogramName, h1, histogramWaitTime)
	if err != nil {
		testing.ContextLog(ctx, "Failed to get histogram update: ", err)
	} else {
		testing.ContextLog(ctx, "Got LCP2 histogram update: ", h2)
	}

	tab.currentPattern = (pn + foundPatternIndex) % numPatterns
	return nil
}

func contentSiteUnavailable(ctx context.Context, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn)

	errorMessages := []string{
		"503 Service Temporarily Unavailable",
		"504 Gateway Time-out",
		"HTTP ERROR 404",
		"Our CDN was unable to reach our servers",
		"Apologies, but something went wrong on our end.",
		"This site can’t provide a secure connection",
		"This site can’t be reached",
	}

	for _, m := range errorMessages {
		node := nodewith.Name(m).Role(role.StaticText)
		if err := ui.Exists(node)(ctx); err == nil {
			return errors.Errorf("content site error - %s", m)
		}
	}
	return nil
}

func (tab *chromeTab) close(ctx context.Context) error {
	if tab.conn == nil {
		return nil
	}
	if err := tab.conn.CloseTarget(ctx); err != nil {
		return errors.Wrap(err, "failed to close tab")
	}
	if err := tab.conn.Close(); err != nil {
		return errors.Wrap(err, "failed to close tab connection")
	}
	tab.conn = nil
	return nil
}

// reconnect checks if the tab connection is still alive and reconnect if it isn't.
//
// It has been observed that tabs can be discarded when DUT runs low on memory, causing unexpected behaviors
// and ultimately test failures. For details, please refer to b/184571798.
func (tab *chromeTab) reconnect(ctx context.Context, br *browser.Browser) error {
	var url string
	// Verify if the tab connection is still usable.
	err := tab.conn.Eval(ctx, "window.location.href", &url)
	if err == nil || !tabDiscarded(err) {
		testing.ContextLog(ctx, "The connection is alive and therefore no need to reconnect")
		return nil
	}
	testing.ContextLog(ctx, "Tab has been discarded/killed, reconnecting")

	matcher := func(t *target.Info) bool {
		actualURL := strings.Split(strings.TrimSuffix(t.URL, "/"), "?")[0] // Ignore all possible parameters.
		expectedURL := strings.Split(strings.TrimSuffix(tab.url, "/"), "?")[0]
		return t.Type == "page" && actualURL == expectedURL
	}

	if tab.conn, err = br.NewConnForTarget(ctx, matcher); err != nil {
		return errors.Wrapf(err, "failed to reconnect to target %q", tab.url)
	}

	testing.ContextLogf(ctx, "Target (tab: [%s][%s]) was detached but successfully reconnected", tab.pageInfo.webName, tab.url)
	return nil
}

// tabDiscarded returns true if the input error is caused by tab discarding.
func tabDiscarded(err error) bool {
	// Discarded tab will return errors with the following pattern (b/184571798).
	p := regexp.MustCompile(`rpcc: the connection is closing: session: detach failed for session [0-9A-F]{32}: cdp.Target: DetachFromTarget: rpc error: No session with given id`)
	return p.MatchString(err.Error())
}

// chromeWindow is the struct for Chrome browser window. It holds multiple tabs.
type chromeWindow struct {
	tabs []*chromeTab
}

// generateTabSwitchTargets sets all web targets according to the input tier.
func generateTabSwitchTargets(tier cuj.Tier, webSource webSourceType) ([]*chromeWindow, error) {
	winNum, ok1 := windowNumberMap[tier]
	tabNum, ok2 := tabNumberMap[tier]
	if !ok1 || !ok2 {
		return nil, errors.Errorf("unacceptable tier: %v", tier)
	}

	var targets []tabTarget
	for _, tabTarget := range tabTargetsMap[webSource] {
		if tabTarget.info.tier <= tier {
			targets = append(targets, tabTarget)
		}
	}
	if len(targets) < winNum*tabNum {
		return nil, errors.New("no enough web page targets to construct tabs")
	}
	// Shuffle the URLs to random order.
	rand := rand.New(rand.NewSource(1))
	rand.Shuffle(len(targets), func(i, j int) { targets[i], targets[j] = targets[j], targets[i] })

	idx := 0
	windows := make([]*chromeWindow, winNum)
	for i := range windows {
		tabs := make([]*chromeTab, tabNum)
		for j := range tabs {
			tabs[j] = &chromeTab{conn: nil, pageInfo: targets[idx].info, url: targets[idx].url, currentPattern: 0}
			idx++
		}
		windows[i] = &chromeWindow{tabs: tabs}
	}

	return windows, nil
}

// Run runs the TabSwitch test. It is invoked by TabSwitchRecorder to
// record web contents via WPR and invoked by TabSwitch to execute the tests
// from the recorded contents. Additional actions will be executed in each tab.
func Run(ctx context.Context, s *testing.State, cr *chrome.Chrome, tier cuj.Tier, bt browser.Type, isTablet, isRecordMode bool) {
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API, error: ", err)
	}

	// Shorten the context to cleanup crastestclient and resume battery charging.
	cleanUpCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	if _, ok := s.Var("spera.cuj_mute"); ok {
		if err := crastestclient.Mute(ctx); err != nil {
			s.Fatal("Failed to mute: ", err)
		}
		defer crastestclient.Unmute(cleanUpCtx)
	}

	// Give 10 seconds to set initial settings. It is critical to ensure
	// cleanupSetting can be executed with a valid context so it has its
	// own cleanup context from other cleanup functions. This is to avoid
	// other cleanup functions executed earlier to use up the context time.
	cleanupSettingsCtx := ctx
	ctx, cancel = ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cleanupSetting, err := cuj.InitializeSetting(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to set initial settings: ", err)
	}
	defer cleanupSetting(cleanupSettingsCtx)

	// The default web source is the external websites.
	webSource := externalWebSource
	if ws, ok := s.Var("spera.web_source"); ok {
		ws := webSourceType(strings.ToLower(ws))
		if _, ok := tabTargetsMap[ws]; ok {
			webSource = ws
		} else {
			s.Fatal("Unknown web source: ", ws)
		}
	}
	if webSource == localWebSource {
		tabSwitchDirPath := path.Join(os.TempDir(), "spera.tabswitch")
		if err := os.MkdirAll(tabSwitchDirPath, 0755); err != nil {
			s.Fatal("Failed to create tab switch directory: ", err)
		}
		defer os.RemoveAll(tabSwitchDirPath)

		if err := testexec.CommandContext(ctx, "unzip", "-o", s.DataPath(LocalWebZIPFile), "-d", tabSwitchDirPath).Run(testexec.DumpLogOnError); err != nil {
			s.Fatal("Failed to unzip local web source file to tab switch directory: ", err)
		}

		localWebDirPath := path.Join(tabSwitchDirPath, localWebFolder)
		localWebDir := http.Dir(localWebDirPath)
		localServer := httptest.NewTLSServer(http.FileServer(localWebDir))
		defer localServer.Close()

		if err := generateLocalWebsitesTargets(ctx, localWebDirPath, localServer.URL, tier); err != nil {
			s.Fatal("Failed to generate local web sites: ", err)
		}
	}
	windows, err := generateTabSwitchTargets(tier, webSource)
	if err != nil {
		s.Fatal("Failed to generate tab targets: ", err)
	}

	var tsAction cuj.UIActionHandler
	if isTablet {
		if tsAction, err = cuj.NewTabletActionHandler(ctx, tconn); err != nil {
			s.Fatal("Failed to create tablet action handler: ", err)
		}
	} else {
		if tsAction, err = cuj.NewClamshellActionHandler(ctx, tconn); err != nil {
			s.Fatal("Failed to create clamshell action handler: ", err)
		}
	}
	defer tsAction.Close()

	timeTabsOpenStart := time.Now()
	// Launch browser and track the elapsed time.
	l, browserStartTime, err := cuj.GetBrowserStartTime(ctx, tconn, true, isTablet, bt)
	if err != nil {
		s.Fatal("Failed to launch Chrome: ", err)
	}
	if l != nil {
		defer l.Close(ctx)
	}
	s.Log("Browser start ms: ", browserStartTime)
	br := cr.Browser()
	if l != nil {
		br = l.Browser()
	}
	bTconn, err := br.TestAPIConn(ctx)
	if err != nil {
		s.Fatalf("Failed to create Test API connection for %v browser: %v", bt, err)
	}

	// Shorten the context to cleanup recorder.
	cleanUpRecorderCtx := ctx
	ctx, cancel = ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	options := cujrecorder.NewPerformanceCUJOptions()
	recorder, err := cujrecorder.NewRecorder(ctx, cr, bTconn, nil, options)
	if err != nil {
		s.Fatal("Failed to create a recorder, error: ", err)
	}
	defer recorder.Close(cleanUpRecorderCtx)
	if err := cuj.AddPerformanceCUJMetrics(bt, tconn, bTconn, recorder); err != nil {
		s.Fatal("Failed to add metrics to recorder: ", err)
	}
	if collect, ok := s.Var("spera.collectTrace"); ok && collect == "enable" {
		recorder.EnableTracing(s.OutDir(), s.DataPath(cujrecorder.SystemTraceConfigFile))
	}

	var timeElapsed time.Duration
	if err = recorder.Run(ctx, func(ctx context.Context) (retErr error) {
		// Open all windows and tabs.
		if err := openAllWindowsAndTabs(ctx, br, &windows, tsAction, isRecordMode); err != nil {
			return errors.Wrap(err, "failed to open targets for tab switch")
		}

		// Total time used from beginning to load all pages.
		timeElapsed = time.Since(timeTabsOpenStart)
		testing.ContextLog(ctx, "All tabs opened Elapsed: ", timeElapsed)

		// Maximize all windows to ensure a consistent state.
		if err := ash.ForEachWindow(ctx, tconn, func(w *ash.Window) error {
			return ash.SetWindowStateAndWait(ctx, tconn, w.ID, ash.WindowStateMaximized)
		}); err != nil {
			return errors.Wrap(err, "failed to maximize windows")
		}

		// Given time to close all tabs.
		cleanupCtx := ctx
		ctx, cancel = ctxutil.Shorten(ctx, 15*time.Second)
		defer cancel()

		defer func(ctx context.Context) {
			faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), func() bool { return retErr != nil }, cr, "ui_tree")
			if err := cuj.CloseAllTabs(ctx, bTconn, bt); err != nil {
				testing.ContextLog(ctx, "Failed to close all tabs: ", err)
			}
		}(cleanupCtx)

		if err := tabSwitchAction(ctx, br, tconn, &windows, tsAction, isRecordMode); err != nil {
			return errors.Wrap(err, "failed to execute tab switch action")
		}
		if err := cuj.GenerateADF(ctx, tconn, isTablet); err != nil {
			return errors.Wrap(err, "failed to generate ADF")
		}
		return nil
	}); err != nil {
		s.Fatal("Failed to execute tab switch action: ", err)
	}

	pv := perf.NewValues()

	pv.Set(perf.Metric{
		Name:      "Browser.StartTime",
		Unit:      "ms",
		Direction: perf.SmallerIsBetter,
	}, float64(browserStartTime.Milliseconds()))

	pv.Set(perf.Metric{
		Name:      "TabSwitchCUJ.ElapsedTime",
		Unit:      "ms",
		Direction: perf.SmallerIsBetter,
	}, float64(timeElapsed.Milliseconds()))

	// Use a short timeout value so it can return fast in case of failure.
	recordCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	if err := recorder.Record(recordCtx, pv); err != nil {
		s.Fatal("Failed to report, error: ", err)
	}
	if err = pv.Save(s.OutDir()); err != nil {
		s.Error("Failed to store values, error: ", err)
	}
	if err := recorder.SaveHistograms(s.OutDir()); err != nil {
		s.Error("Failed to save histogram raw data: ", err)
	}
}

func openAllWindowsAndTabs(ctx context.Context, br *browser.Browser, targets *[]*chromeWindow, tsAction cuj.UIActionHandler, isRecordMode bool) (err error) {
	windows := (*targets)
	plTimeout := pageLoadingTimeout(isRecordMode)
	for idxWindow, window := range windows {
		for idxTab, tab := range window.tabs {
			testing.ContextLogf(ctx, "Opening window %d, tab %d", idxWindow+1, idxTab+1)

			if tab.conn, err = tsAction.NewChromeTab(ctx, br, tab.url, idxTab == 0); err != nil {
				return errors.Wrap(err, "failed to create new Chrome tab")
			}

			if err := webutil.WaitForRender(ctx, tab.conn, plTimeout); err != nil {
				return errors.Wrap(err, "failed to wait for render to finish")
			}

			// In replay mode, user won't be able to know whether the page is quiescence or not,
			// and it is not necessary to wait for quiescence in replay mode.
			// In record mode, needs to wait for quiescence to properly record web content.
			if isRecordMode {
				if err := webutil.WaitForQuiescence(ctx, tab.conn, plTimeout); err != nil {
					return errors.Wrapf(err, "failed to wait for tab to achieve quiescence within %v", plTimeout)
				}
			}
		}
	}

	return nil
}

func tabSwitchAction(ctx context.Context, br *browser.Browser, tconn *chrome.TestConn, targets *[]*chromeWindow, tsAction cuj.UIActionHandler, isRecordMode bool) error {
	windows := (*targets)
	scrollActions := tsAction.ScrollChromePage(ctx)
	plTimeout := pageLoadingTimeout(isRecordMode)

	chromeApp, err := apps.PrimaryBrowser(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to find the Chrome app")
	}

	ui := uiauto.New(tconn)
	for idx, window := range windows {
		testing.ContextLogf(ctx, "Switching to window #%d", idx+1)
		if err := tsAction.SwitchToAppWindowByIndex(chromeApp.Name, idx)(ctx); err != nil {
			return errors.Wrap(err, "failed to switch window")
		}

		tabTotalNum := len(window.tabs)
		for tabIdx := 0; tabIdx < tabTotalNum; tabIdx++ {
			testing.ContextLogf(ctx, "Switching tab to window %d, tab %d", idx+1, tabIdx+1)

			if err := tsAction.SwitchToChromeTabByIndex(tabIdx)(ctx); err != nil {
				return errors.Wrap(err, "failed to switch tab")
			}

			tab := window.tabs[tabIdx]
			// Test the tab connection and reconnect if necessary. This is necessary for
			// discarded tabs due to OOM issue.
			// After tab switching, the current focused tab should be active again so
			// reconnect should succeed.
			if err := tab.reconnect(ctx, br); err != nil {
				return errors.Wrap(err, "cdp connection is invalid and failed to reconnect")
			}

			timeStart := time.Now()
			if err := webutil.WaitForRender(ctx, tab.conn, tabSwitchTimeout); err != nil {
				testing.ContextLog(ctx, "WaitForRender failed. Reconnect and retry")
				if err := tab.reconnect(ctx, br); err != nil {
					return errors.Wrap(err, "failed to reconnect cdp connection")
				}
				if err := webutil.WaitForRender(ctx, tab.conn, tabSwitchTimeout); err != nil {
					return errors.Wrap(err, "failed to wait for render on the second try after reconnect")
				}
			}
			renderTime := time.Since(timeStart)
			// Debugging purpose message, to observe which tab takes unusual long time to render.
			testing.ContextLog(ctx, "Tab rendering time after switching: ", renderTime)
			if isRecordMode {
				if err := webutil.WaitForQuiescence(ctx, tab.conn, plTimeout); err != nil {
					return errors.Wrapf(err, "failed to wait for tab to achieve quiescence within %v", plTimeout)
				}
				quiescenceTime := time.Now().Sub(timeStart)
				// Debugging purpose message, to observe which tab takes unusual long time to quiescence
				testing.ContextLog(ctx, "Tab quiescence time after switching: ", quiescenceTime)
			}

			// In case of lose connection to the tab, need to update the URL to reconnect to it
			if err := tab.conn.Eval(ctx, "window.location.href", &tab.url); err != nil {
				return errors.Wrap(err, "failed update the URL of tab")
			}

			if tabIdx%3 == 0 || isRecordMode {
				for _, act := range scrollActions {
					if err := act(ctx); err != nil {
						return errors.Wrap(err, "failed to execute action")
					}
					// Make sure the whole web content is recorded only under Recording.
					if isRecordMode {
						if err := webutil.WaitForRender(ctx, tab.conn, tabSwitchTimeout); err != nil {
							return errors.Wrap(err, "failed to wait for render to finish after scroll")
						}
						if err := webutil.WaitForQuiescence(ctx, tab.conn, plTimeout); err != nil {
							return errors.Wrapf(err, "failed to wait for tab to achieve quiescence after scroll within %v", plTimeout)
						}
					}
				}
			}

			// Click on 1 link per 2 tabs, or click on 1 link for every tab under Record mode to ensure all links are
			// accessible under any other tiers.
			if tabIdx%2 == 0 || isRecordMode {
				if err := tab.clickAnchor(ctx, plTimeout, tconn); err != nil {
					return errors.Wrap(err, "failed to click anchor")
				}
				if isRecordMode {
					// Ensure contents are rendered in recording mode.
					if err := webutil.WaitForRender(ctx, tab.conn, plTimeout); err != nil {
						return errors.Wrap(err, "failed to wait for render to finish")
					}
					if err := webutil.WaitForQuiescence(ctx, tab.conn, plTimeout); err != nil {
						return errors.Wrap(err, "failed to wait for tab to achieve quiescence")
					}
				} else {
					// It is normal that tabs might remain loading, hence no handle error here.
					webutil.WaitForQuiescence(ctx, tab.conn, clickLinkTimeout)
				}
				// Given some time after clicking any anchor before doing next operation.
				if err := testing.Sleep(ctx, time.Second); err != nil {
					return errors.Wrapf(err, "failed to sleep for %v", time.Second)
				}
			}

			// The tab.pageInfo.webName is the website name of the tab after switching.
			// After switching to Youtube or Reddit tabs, there might be a dialog popping up,
			// causing the test fails to switch to the next tab.
			// If the dialog exists, keep clicking the "Allow" button to ensure it is closed.
			if tab.pageInfo.webName == youtube || tab.pageInfo.webName == reddit {
				// The alert dialog pops up after the tab achieves quiescence.
				if err := webutil.WaitForQuiescence(ctx, tab.conn, plTimeout); err != nil {
					return errors.Wrap(err, "failed to wait for tab to achieve quiescence")
				}
				notificationsDialog := nodewith.NameContaining("Show notifications").Role(role.AlertDialog).HasClass("RootView")
				allowButton := nodewith.Name("Allow").Role(role.Button).Ancestor(notificationsDialog)
				if err := uiauto.IfSuccessThen(
					ui.WithTimeout(shortUITimeout).WaitUntilExists(notificationsDialog),
					tsAction.ClickUntil(
						allowButton,
						ui.WithTimeout(shortUITimeout).WaitUntilGone(notificationsDialog)),
				)(ctx); err != nil {
					return errors.Wrap(err, "failed to close alert dialog")
				}
			}
		}
	}
	return nil
}
