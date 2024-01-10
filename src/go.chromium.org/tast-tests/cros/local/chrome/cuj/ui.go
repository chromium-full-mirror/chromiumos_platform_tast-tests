// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package cuj contains fixtures, utils for cuj.
package cuj

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj/inputsimulations"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	"go.chromium.org/tast-tests/cros/local/chrome/metrics"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/pointer"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/prompts"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/touch"
	"go.chromium.org/tast-tests/cros/local/input"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const promptTimeout = 5 * time.Second

// CloseAllWindows closes all currently open windows by iterating over
// the shelf icons and calling apps.closeApp on each one.
func CloseAllWindows(ctx context.Context, tconn *chrome.TestConn) error {
	shelfItems, err := ash.ShelfItems(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to get shelf items")
	}
	for _, shelfItem := range shelfItems {
		if shelfItem.Status != ash.ShelfItemClosed {
			if err := apps.Close(ctx, tconn, shelfItem.AppID); err != nil {
				return errors.Wrapf(err, "failed to close the app %v", shelfItem.AppID)
			}
		}
	}
	return nil
}

// CloseAllTabs closes all tabs in Ash, leave a new tab in Lacros.
func CloseAllTabs(ctx context.Context, bTconn *chrome.TestConn, bt browser.Type) error {
	closeTabsFunc := browser.CloseAllTabs
	if bt == browser.TypeLacros {
		// For lacros-Chrome, we will close all existing tabs but leave a new tab to keep the Chrome
		// process alive.
		closeTabsFunc = browser.ReplaceAllTabsWithSingleNewTab
	}
	return closeTabsFunc(ctx, bTconn)
}

// GetBrowserStartTime opens chrome browser and returns the browser start time.
// If lfixtVal is given, it will open the lacros-Chrome, and return the lacros instance.
func GetBrowserStartTime(ctx context.Context, tconn *chrome.TestConn,
	closeTabs, tabletMode bool, bt browser.Type) (*lacros.Lacros, time.Duration, error) {
	const (
		retryTimes       = 3
		appLaunchTimeout = 10 * time.Second
	)
	var l *lacros.Lacros
	chromeApp, err := apps.PrimaryBrowser(ctx, tconn)
	if err != nil {
		return nil, -1, errors.Wrap(err, "could not find the Chrome app")
	}

	// Make sure the browser hasn't been opened.
	shown, err := ash.AppShown(ctx, tconn, chromeApp.ID)
	if err != nil {
		return nil, -1, errors.Wrap(err, "failed to check if the browser window is shown or not")
	}
	if shown {
		// Close the browser if it is aready opened.
		if err := apps.Close(ctx, tconn, chromeApp.ID); err != nil {
			return nil, -1, errors.Wrap(err, "failed to close the opened browser")
		}
	}

	msg := "shelf"
	launchFunc := LaunchAppFromShelf
	if tabletMode {
		msg = "hotseat"
		launchFunc = LaunchAppFromHotseat
	}
	testing.ContextLog(ctx, "Launch Google Chrome from "+msg)

	var startTime, totalStartTime time.Time
	retryCount := 0

	launchChromeApp := func(ctx context.Context) error {
		retryCount++
		startTime, err = launchFunc(ctx, tconn, "Chrome", "Chromium", "Lacros")
		if err != nil {
			return errors.Wrap(err, "failed to open Chrome")
		}
		// Make sure app is launched.
		if err := ash.WaitForApp(ctx, tconn, chromeApp.ID, appLaunchTimeout); err != nil {
			return errors.Wrap(err, "failed to wait for the app to be launched")
		}
		return nil
	}
	ui := uiauto.New(tconn)
	totalStartTime = time.Now()
	if err := ui.Retry(retryTimes, launchChromeApp)(ctx); err != nil {
		testing.ContextLogf(ctx, "Failed to launch the Chrome app after %v retries in %v", retryTimes, time.Since(totalStartTime))
		// Browser launch time is calculated from opening the extension launcher.
		// Expect to take longer than starting straight from the shelf.
		startTime = time.Now()
		if err := launcher.LaunchApp(tconn, chromeApp.ShortName())(ctx); err != nil {
			return nil, -1, errors.Wrap(err, "failed to launch the Chrome app from launcher")
		}
		// Make sure app is launched.
		if err := ash.WaitForApp(ctx, tconn, chromeApp.ID, appLaunchTimeout); err != nil {
			return nil, -1, errors.Wrap(err, "failed to wait for the app to be launched")
		}
	} else {
		testing.ContextLogf(ctx, "Attempts %v to successfully launch the Chrome app", retryCount)
	}
	browserStartTime := time.Since(startTime)
	totalLaunchTime := time.Since(totalStartTime)
	testing.ContextLogf(ctx, "It took a total of %v to launch the Chrome app, browser start time is %v", totalLaunchTime, browserStartTime)
	// If it's ash-Chrome, we will close all existing tabs so the test case will start with a
	// clean Chrome.
	closeTabsFunc := browser.CloseAllTabs
	bTconn := tconn
	if bt == browser.TypeLacros {
		// Connect to lacros-Chrome started from UI.
		l, err = lacros.Connect(ctx, tconn)
		if err != nil {
			return nil, -1, errors.Wrap(err, "failed to get lacros instance")
		}
		bTconn, err = l.TestAPIConn(ctx)
		if err != nil {
			return nil, -1, errors.Wrap(err, "failed to create test API conn")
		}
		// For lacros-Chrome, we will close all existing tabs but leave a new tab to keep the Chrome
		// process alive.
		closeTabsFunc = browser.ReplaceAllTabsWithSingleNewTab
	}

	// Maximize all windows to ensure a consistent state.
	if err := ash.ForEachWindow(ctx, tconn, func(w *ash.Window) error {
		return ash.SetWindowStateAndWait(ctx, tconn, w.ID, ash.WindowStateMaximized)
	}); err != nil {
		return nil, -1, errors.Wrap(err, "failed to maximize windows")
	}

	// Depending on the settings, Chrome might open all left-off pages automatically from last session.
	// Close all existing tabs and test can open new pages in the browser.
	if closeTabs {
		if err := closeTabsFunc(ctx, bTconn); err != nil {
			return nil, -1, errors.Wrap(err, "failed to close extra Chrome tabs")
		}
	}

	return l, browserStartTime, nil
}

// CloseChrome closes Chrome browser application properly.
// If there exist unsave changes on web page, e.g. media content is playing or online document is editing,
// "leave site" prompt will prevent the tab from closing.
// This function confirms the "leave site" prompts so browser can be closed.
func CloseChrome(ctx context.Context, tconn *chrome.TestConn) error {
	chromeApp, err := apps.PrimaryBrowser(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "could not find the Chrome app")
	}

	if err := apps.Close(ctx, tconn, chromeApp.ID); err != nil {
		return errors.Wrap(err, "failed to close Chrome")
	}

	return prompts.ClearPotentialPrompts(tconn, time.Second, prompts.LeaveSitePrompt)(ctx)
}

// LaunchAppFromShelf opens an app by name which is currently pinned to the shelf.
// Which it is also support multiple names for a single app (e.g. "Chrome"||"Chromium" for Google Chrome, the browser).
// It returns the time when a mouse click event is injected to the app icon.
func LaunchAppFromShelf(ctx context.Context, tconn *chrome.TestConn, appName string, appOtherPossibleNames ...string) (time.Time, error) {
	params := appIconFinder(appName, appOtherPossibleNames...)

	ac := uiauto.New(tconn)
	if err := ac.WithTimeout(10 * time.Second).WaitForLocation(params)(ctx); err != nil {
		return time.Time{}, errors.Wrapf(err, "failed to find app %q", appName)
	}

	// Click mouse to launch app.
	startTime := time.Now()
	if err := ac.LeftClick(params)(ctx); err != nil {
		return startTime, errors.Wrapf(err, "failed to launch app %q", appName)
	}
	return startTime, nil
}

// LaunchAppFromHotseat opens an app by name which is currently pinned to the hotseat.
// Which it is also support multiple names for a single app (e.g. "Chrome"||"Chromium" for Google Chrome, the browser).
// It returns the time when a touch event is injected to the app icon.
func LaunchAppFromHotseat(ctx context.Context, tconn *chrome.TestConn, appName string, appOtherPossibleNames ...string) (time.Time, error) {
	var startTime time.Time
	// Get touch controller for tablet.
	tc, err := touch.New(ctx, tconn)
	if err != nil {
		return startTime, errors.Wrap(err, "failed to create the touch controller")
	}
	defer tc.Close(ctx)
	tsew, tcc, err := touch.NewTouchscreenAndConverter(ctx, tconn)
	if err != nil {
		return startTime, errors.Wrap(err, "failed to access to the touch screen")
	}
	defer tsew.Close(ctx)
	stw, err := tsew.NewSingleTouchWriter()
	if err != nil {
		return startTime, errors.Wrap(err, "failed to create a new single touch writer")
	}
	defer stw.Close()

	// Make sure hotseat is shown.
	if err := ash.SwipeUpHotseatAndWaitForCompletion(ctx, tconn, stw, tcc); err != nil {
		return startTime, errors.Wrap(err, "failed to show hotseat")
	}

	params := appIconFinder(appName, appOtherPossibleNames...)

	ac := uiauto.New(tconn)
	if err := ac.WithTimeout(10 * time.Second).WaitForLocation(params)(ctx); err != nil {
		return startTime, errors.Wrapf(err, "failed to find app %q", appName)
	}

	// Press button to launch app.
	startTime = time.Now()
	if err := tc.Tap(params)(ctx); err != nil {
		return startTime, errors.Wrapf(err, "failed to tap %q", appName)
	}
	return startTime, nil
}

// appIconFinder returns the finder of app icon with input app name(s).
// It will only look for the icon locate on internal display.
func appIconFinder(appName string, appOtherPossibleNames ...string) *nodewith.Finder {
	internalDisplay := nodewith.ClassName("RootWindow-0").Role(role.Window)
	finder := nodewith.ClassName("ash/ShelfAppButton").Ancestor(internalDisplay)

	if len(appOtherPossibleNames) == 0 {
		return finder.Name(appName)
	}

	pattern := "(" + appName
	for _, name := range appOtherPossibleNames {
		pattern += "|"
		pattern += regexp.QuoteMeta(name)
	}
	pattern += ")"

	return finder.NameRegex(regexp.MustCompile(pattern))
}

// extendedDisplayWindowClassName obtains the class name of the root window on the extended display.
// If multiple display windows are present, the first one will be returned.
func extendedDisplayWindowClassName(ctx context.Context, tconn *chrome.TestConn) (string, error) {
	ui := uiauto.New(tconn)

	// Root window on extended display has the class name in RootWindow-<id> format.
	// We found extended display window could be RootWindow-1, or RootWindow-2.
	// Here we try 1 to 10.
	for i := 1; i <= 10; i++ {
		className := fmt.Sprintf("RootWindow-%d", i)
		win := nodewith.ClassName(className).Role(role.Window)
		if err := ui.Exists(win)(ctx); err == nil {
			return className, nil
		}
	}
	return "", errors.New("failed to find any window with class name RootWindow-1 to RootWindow-10")
}

// SwitchWindowToDisplay switches current window to expected display.
func SwitchWindowToDisplay(ctx context.Context, tconn *chrome.TestConn, kb *input.KeyboardEventWriter, externalDisplay bool) action.Action {
	return func(ctx context.Context) error {
		var expectedRootWindow *nodewith.Finder
		var display string
		ui := uiauto.New(tconn)
		w, err := ash.FindWindow(ctx, tconn, func(w *ash.Window) bool {
			return w.IsActive && w.IsFrameVisible
		})
		if err != nil {
			return errors.Wrap(err, "failed to get current active window")
		}
		if externalDisplay {
			display = "external display"
			extendedWinClassName, err := extendedDisplayWindowClassName(ctx, tconn)
			if err != nil {
				return errors.Wrap(err, "failed to find root window on external display")
			}
			expectedRootWindow = nodewith.ClassName(extendedWinClassName).Role(role.Window)
		} else {
			display = "internal display"
			// Root window on built-in display.
			expectedRootWindow = nodewith.ClassName("RootWindow-0").Role(role.Window)
		}
		currentWindow := nodewith.NameContaining(w.Title).Role(role.Window)
		expectedWindow := currentWindow.Ancestor(expectedRootWindow).First()
		if err := ui.Exists(expectedWindow)(ctx); err != nil {
			testing.ContextLog(ctx, "Expected window not found: ", err)
			testing.ContextLogf(ctx, "Switch window %q to %s", w.Title, display)
			return uiauto.Combine("switch window to "+display,
				kb.AccelAction("Search+Alt+M"),
				ui.WithTimeout(3*time.Second).WaitUntilExists(expectedWindow),
			)(ctx)
		}
		return nil
	}
}

// DismissMobilePrompt dismisses the prompt of "This app is designed for mobile".
func DismissMobilePrompt(ctx context.Context, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn)

	prompt := nodewith.Name("This app is designed for mobile").Role(role.Window)
	if err := ui.WithTimeout(promptTimeout).WaitUntilExists(prompt)(ctx); err == nil {
		testing.ContextLog(ctx, "Dismiss the app prompt")
		gotIt := nodewith.Name("Got it").Role(role.Button).Ancestor(prompt)
		if err := ui.LeftClickUntil(gotIt, ui.WithTimeout(time.Second).WaitUntilGone(gotIt))(ctx); err != nil {
			return errors.Wrap(err, "failed to click 'Got it' button")
		}
	}
	return nil
}

// DismissBatterySaverPrompt returns an action to dismiss the prompt of "Energy Saver turned on".
func DismissBatterySaverPrompt(tconn *chrome.TestConn) uiauto.Action {
	energySaverAlert := nodewith.NameContaining("Energy Saver").Role(role.Alert).HasClass("Widget")
	energySaverPrompt := prompts.Prompt{
		Name:              "Energy Saver",
		PromptFinder:      energySaverAlert,
		ClearButtonFinder: prompts.GotItButtonFinder.Ancestor(energySaverAlert),
	}

	return prompts.ClearPotentialPrompts(tconn, promptTimeout, energySaverPrompt)
}

// ExpandMenu returns a function that clicks the button and waits for the menu to expand to the given height.
// This function is useful when the target menu will expand to its full size with animation. On Low end DUTs
// the expansion animation might stuck for some time. The node might have returned a stable location if
// checking with a fixed interval before the animiation completes. This function ensures animation completes
// by checking the menu height.
func ExpandMenu(tconn *chrome.TestConn, button, menu *nodewith.Finder, height int) action.Action {
	ui := uiauto.New(tconn)
	startTime := time.Now()
	return func(ctx context.Context) error {
		if err := ui.DoDefault(button)(ctx); err != nil {
			return errors.Wrap(err, "failed to click button")
		}
		return testing.Poll(ctx, func(ctx context.Context) error {
			menuInfo, err := ui.Info(ctx, menu)
			if err != nil {
				return errors.Wrap(err, "failed to get menu info")
			}
			if menuInfo.Location.Height < height {
				return errors.Errorf("got menu height %d, want %d", menuInfo.Location.Height, height)
			}
			// Examine this log regularly to see how fast the menu is expanded and determine if
			// we still need to keep this ExpandMenu() function.
			testing.ContextLog(ctx, "Menu expanded to full height in ", time.Now().Sub(startTime))
			return nil
		}, &testing.PollOptions{Timeout: time.Minute, Interval: time.Second})
	}
}

// MaximizeBrowserWindow maximizes a specific browser window to show all the browser UI elements for precise clicking.
func MaximizeBrowserWindow(ctx context.Context, tconn *chrome.TestConn, tabletMode bool, title string) error {
	if !tabletMode {
		// Find the specific browser window.
		window, err := ash.FindWindow(ctx, tconn, func(w *ash.Window) bool {
			return (w.WindowType == ash.WindowTypeBrowser || w.WindowType == ash.WindowTypeLacros) && strings.Contains(w.Title, title)
		})
		if err != nil {
			return errors.Wrapf(err, "failed to find the %q window", title)
		}
		if err := ash.SetWindowStateAndWait(ctx, tconn, window.ID, ash.WindowStateMaximized); err != nil {
			// Just log the error and try to continue.
			testing.ContextLogf(ctx, "Try to continue the test even though maximizing the %q window failed: %v", title, err)
		}
	}
	return nil
}

// GenerateADF creates touch/mouse controller and generates ADF metrics.
func GenerateADF(ctx context.Context, tconn *chrome.TestConn, isTablet bool) error {
	var err error
	var pc pointer.Context
	if isTablet {
		if pc, err = pointer.NewTouch(ctx, tconn); err != nil {
			return errors.Wrap(err, "failed to create a touch controller")
		}
	} else {
		pc = pointer.NewMouse(tconn)
	}
	defer pc.Close(ctx)
	if err = inputsimulations.DoAshWorkflows(ctx, tconn, pc); err != nil {
		return errors.Wrap(err, "failed to do Ash workflows")
	}
	return nil
}

// GeneratePDF scrolls web page by keyboard to generate "Graphics.Smoothness.PercentDroppedFrames3.AllSequences" histogram.
func GeneratePDF(ctx context.Context, bTconn *chrome.TestConn, kb *input.KeyboardEventWriter) error {
	const (
		histogramName = "Graphics.Smoothness.PercentDroppedFrames3.AllSequences"
		repeatCount   = 10
	)
	initHistogram, err := metrics.GetHistogram(ctx, bTconn, histogramName)
	if err != nil {
		return errors.Wrap(err, "failed to get initial histogram")
	}
	testing.ContextLog(ctx, "Got initial histogram: ", initHistogram)

	// Scrolling a web page may generates a PDF histogram.
	testing.ContextLog(ctx, "Scroll page by keyboard")
	if err := uiauto.Repeat(repeatCount, uiauto.Combine("scroll page by keyboard",
		kb.AccelAction("Down"),
		uiauto.Sleep(200*time.Millisecond),
		kb.AccelAction("Up"),
		uiauto.Sleep(200*time.Millisecond),
	))(ctx); err != nil {
		return errors.Wrap(err, "failed to scroll Chrome page")
	}
	histogramDiff, err := metrics.WaitForHistogramUpdate(ctx, bTconn, histogramName, initHistogram, time.Second)
	if err != nil {
		return errors.Wrap(err, "failed to get histogram update")
	}
	testing.ContextLog(ctx, "Got histogram update: ", histogramDiff)
	return nil
}

// EnableTabletMode enables tablet mode if the test case variable "tabletVariable" is set as "tablet", and disables tablet mode if other values are set.
// If the variable is not set, it will use ash.TabletModeEnabled to get the tablet mode enabled status.
func EnableTabletMode(ctx context.Context, tconn *chrome.TestConn, testCaseVar func(string) (string, bool), tabletVariable string) (tabletMode bool, cleanup action.Action, err error) {
	cleanup = func(ctx context.Context) error { return nil }

	if mode, ok := testCaseVar(tabletVariable); ok {
		tabletMode = mode == "tablet"
		cleanup, err = ash.EnsureTabletModeEnabled(ctx, tconn, tabletMode)
		if err != nil {
			return tabletMode, cleanup, errors.Wrapf(err, "failed to enable tablet mode to %v", tabletMode)
		}
	} else {
		testing.ContextLog(ctx, "Screen mode is not given, running with the default mode of the DUT")
		tabletMode, err = ash.TabletModeEnabled(ctx, tconn)
		if err != nil {
			return tabletMode, cleanup, errors.Wrap(err, "failed to get tablet mode")
		}
	}
	testing.ContextLog(ctx, "Running test with tablet mode: ", tabletMode)
	return tabletMode, cleanup, nil
}

// DismissCriticalSecurityAlert closes the critical security alert, if that
// alert is found on screen. If clicking action fails, it will remove the
// alert dialog HTML element with javascript directly.
func DismissCriticalSecurityAlert(ctx context.Context, tconn *chrome.TestConn, conn *chrome.Conn) error {
	ui := uiauto.New(tconn)
	alertContainer := nodewith.NameStartingWith("Critical security alert").Role(role.Dialog)
	close := nodewith.NameStartingWith("Close").Ancestor(alertContainer)
	if err := uiauto.IfSuccessThen(ui.Exists(alertContainer),
		uiauto.NamedAction("close security alert", ui.DoDefaultUntil(close, ui.WithTimeout(promptTimeout).WaitUntilGone(close))),
	)(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to close critical security alert by clicking, use javascript to delete the HTML element instead")
		const alertDivElement = `document.querySelector("iframe[role='presentation'][name='callout']").parentElement`
		if err := conn.Eval(ctx, fmt.Sprintf("%s.remove()", alertDivElement), nil); err != nil {
			return errors.Wrap(err, "failed to remove critical security alert HTML element")
		}
		if err := ui.WithTimeout(promptTimeout).WaitUntilGone(alertContainer)(ctx); err != nil {
			return errors.Wrap(err, "failed to wait until critical security alert gone")
		}
	}
	return nil
}

// LogWindowMismatch prints out mismatched window details.
// It returns a string wrapping the details of the mismatched window
// so that it can be appended to the error message.
func LogWindowMismatch(ctx context.Context, initialWindows, finalWindows []*ash.Window) string {
	var missingWindowTitles, extraWindowTitles []string
	var logs []string

	for _, iw := range initialWindows {
		found := false
		for _, fw := range finalWindows {
			if iw.ID == fw.ID {
				found = true
				break
			}
		}
		if !found {
			missingWindowTitles = append(missingWindowTitles, iw.Title)
		}
	}
	missingNumWindows := len(missingWindowTitles)
	if missingNumWindows > 0 {
		logs = append(logs, "missing window(s): "+strings.Join(missingWindowTitles, ", "))
	}

	for _, fw := range finalWindows {
		found := false
		for _, iw := range initialWindows {
			if fw.ID == iw.ID {
				found = true
				break
			}
		}
		if !found {
			extraWindowTitles = append(extraWindowTitles, fw.Title)
		}
	}
	extraNumWindows := len(extraWindowTitles)
	if extraNumWindows > 0 {
		logs = append(logs, "extra window(s): "+strings.Join(extraWindowTitles, ", "))
	}

	if len(logs) == 0 {
		testing.ContextLog(ctx, "No window mismatch found")
		return ""
	}
	completeLog := strings.Join(logs, "; ")
	testing.ContextLog(ctx, "Found window mismatch: ", completeLog)
	return completeLog
}

// WaitForValidAccountInCookieJar opens the "signin-internals/" page
// and verifies if there is a valid account in the cookie jar.
func WaitForValidAccountInCookieJar(ctx context.Context, br *browser.Browser, tconn *chrome.TestConn) (retErr error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	conn, err := br.NewConn(ctx, chrome.SigninInternalsURL)
	if err != nil {
		return errors.Wrapf(err, "failed to open %q page", chrome.SigninInternalsURL)
	}
	defer func(ctx context.Context) {
		hasErr := func() bool { return retErr != nil }
		faillog.DumpUITreeWithScreenshotWithTestAPIOnErrorToContextOutDir(
			ctx, hasErr, tconn, "account_dump")
		if err := conn.CloseTarget(ctx); err != nil {
			testing.ContextLogf(ctx, "Failed to close %q page: %v", chrome.SigninInternalsURL, err)
			// If the account page is not closed, it's likely to cause the test to fail.
			// Return error if failed to close the page.
			if !hasErr() {
				retErr = errors.Wrapf(err, "failed to close %q page", chrome.SigninInternalsURL)
			}
		}
		conn.Close()
	}(cleanupCtx)

	// Account profile could be in the browser, but not in the cookie Jar yet.
	// Before the account being sync-ed to the cookie Jar, we will see the
	// account signin issue when opening Google applications.
	// See crbug/1375314 for details.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		// In the chrome://signin-internals page, look for the "Accounts in
		// Cookie Jar" table and check if there is a valid account.
		const script = `() => {
			let rows = document.querySelectorAll("#cookie-info tr");
			// The first row is the header. Start from the second row.
			for (i = 1; i< rows.length; i++){
				let row = rows[i];
				let validColumn = row.querySelector('[jscontent="valid"]');
				if (validColumn !== null && validColumn.textContent === "Valid") {
					return;
				}
			}
			throw new Error("no valid account in cookie jar");
		}`
		return conn.Call(ctx, nil, script)
	}, &testing.PollOptions{Timeout: 30 * time.Second, Interval: time.Second}); err != nil {
		return errors.Wrap(err, "failed to wait for accounts to be synced to Cookie Jar")
	}
	return nil
}
