// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package taskmanager

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/ui"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/cws"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/taskmanager"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DisplayAllTabs,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test that all tabs should be displayed in the task manager",
		Contacts: []string{
			"afakhry@google.com",
			"chromeos-sw-engprod@google.com",
		},
		BugComponent:   "b:1457613",
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Attr:           []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps:   []string{"chrome"},
		VarDeps:        []string{ui.GaiaPoolDefaultVarName},
		Params: []testing.Param{
			{
				// GAIA is required to install an app from Chrome Webstore.
				Fixture: "chromeLoggedInWithGaia",
				Val:     browser.TypeAsh,
			}, {
				Name:              "lacros",
				ExtraSoftwareDeps: []string{"lacros"},
				Fixture:           "lacrosGaiaLogin",
				Val:               browser.TypeLacros,
			},
		},
		// There are 10 tabs to be opened.
		Timeout: 10*taskmanager.ChromeTabQuiescenceTimeout + 2*time.Minute,
	})
}

// DisplayAllTabs tests that all tabs should be displayed in the task manager.
func DisplayAllTabs(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get the connection to the test API: ", err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer kb.Close(ctx)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	browserType := s.Param().(browser.Type)
	br, closeBrowser, err := browserfixt.SetUp(ctx, cr, browserType)
	if err != nil {
		s.Fatal("Failed to set up browser: ", err)
	}
	defer closeBrowser(cleanupCtx)

	ui := uiauto.New(tconn)

	// Expecting 3 windows, 2 tabs on the first window, 4 tabs on the second and the third window.
	browserTabs := [][]taskmanager.Process{
		{
			newChromeTabInNewWindow("https://www.facebook.com/", browserType),
			newChromeExtension(ui, chrome.BlankURL, "Speedtest", browserType),
		}, {
			newChromeTabInNewWindow("https://www.amazon.com/", browserType),
			taskmanager.NewChromeTabProcess("https://www.apple.com/", browserType),
			newYoutubeTab(tconn, "https://www.youtube.com/", browserType),
			taskmanager.NewChromeTabProcess("https://www.instagram.com/", browserType),
		}, {
			newChromeTabInNewWindow("https://en.wikipedia.org/wiki/Main_Page", browserType),
			taskmanager.NewChromeTabProcess("https://news.google.com/", browserType),
			taskmanager.NewChromeTabProcess("https://news.ycombinator.com/news", browserType),
			taskmanager.NewChromeTabProcess("https://www.cbc.ca/lite/trending-news", browserType),
		},
	}

	cwsApp := cws.App{Name: cwsAppName, URL: cwsAppURL}
	if err := cws.InstallApp(ctx, br, tconn, cwsApp); err != nil {
		s.Fatal("Failed to install CWS app: ", err)
	}
	defer cws.UninstallApp(cleanupCtx, br, tconn, cwsApp)

	for _, browserWindow := range browserTabs {
		for _, process := range browserWindow {
			if err := process.Open(ctx, br); err != nil {
				s.Fatal("Failed to open browser tab: ", err)
			}
			defer process.Close(cleanupCtx)
		}
	}

	tm := taskmanager.New(tconn, kb)
	if err := tm.Open(ctx); err != nil {
		s.Fatal("Failed to launch the task manager: ", err)
	}
	defer tm.Close(cleanupCtx, tconn)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_dump")

	for _, browserWindow := range browserTabs {
		for _, process := range browserWindow {
			name, err := process.NameInTaskManager(ctx)
			if err != nil {
				s.Fatal("Failed to obtain the process name in task manager: ", err)
			}
			// The processes might be grouped. Such as the extension "Speedtest" in this test.
			if err := ui.WithTimeout(30 * time.Second).WaitUntilExists(taskmanager.FindProcess().NameStartingWith(name).First())(ctx); err != nil {
				s.Fatalf("Failed to find process %q in task manager: %v", name, err)
			}
		}
	}
}

// Speedtest by Ookla is an extension which can be used to test internet performance.
const (
	cwsAppID   = "pgjjikdiikihdfpoppgaidccahalehjh"
	cwsAppURL  = "https://chromewebstore.google.com/detail/speedtest-by-ookla/" + cwsAppID
	cwsAppName = "Speedtest by Ookla"
)

func newChromeTabInNewWindow(url string, browserType browser.Type) *taskmanager.ChromeTab {
	tab := taskmanager.NewChromeTabProcess(url, browserType)
	tab.SetOpenInNewWindow()
	return tab
}

// chromeExtension represents the installed chrome extension.
// It has a few different behaviors than a ChromeTab, how to open it and its name in the task manager for instance.
type chromeExtension struct {
	*taskmanager.ChromeTab
	ui   *uiauto.Context
	name string
}

func newChromeExtension(ui *uiauto.Context, url, name string, browserType browser.Type) *chromeExtension {
	return &chromeExtension{
		ChromeTab: taskmanager.NewChromeTabProcess(url, browserType),
		ui:        ui,
		name:      name,
	}
}

// Open opens the installed chrome extension.
func (extension *chromeExtension) Open(ctx context.Context, br *browser.Browser) error {
	if err := extension.ChromeTab.Open(ctx, br); err != nil {
		return err
	}

	browserFrame := nodewith.HasClass("BrowserFrame").Role(role.Window)
	if extension.BrowserType() == browser.TypeLacros {
		classNameRegexp := regexp.MustCompile(`^ExoShellSurface(-\d+)?$`)
		browserFrame = nodewith.Role(role.Window).NameStartingWith(extension.ChromeTab.Title).ClassNameRegex(classNameRegexp)
	}

	extensionMenu := nodewith.HasClass("ExtensionsMenuView").Role(role.Window)

	return uiauto.Combine("open the extension",
		extension.ui.LeftClick(nodewith.Name("Extensions").Role(role.PopUpButton).Ancestor(browserFrame)),
		extension.ui.LeftClick(nodewith.NameStartingWith(extension.name).HasClass("ExtensionsMenuButton").Ancestor(extensionMenu)),
		extension.ui.WaitUntilExists(nodewith.Name(extension.name).Role(role.RootWebArea)),
	)(ctx)
}

func (extension *chromeExtension) NameInTaskManager(ctx context.Context) (string, error) {
	name := "Extension: " + extension.name
	if extension.ChromeTab.BrowserType() == browser.TypeLacros {
		name = fmt.Sprintf("Lacros: %s", name)
	}

	return name, nil
}

type youtubeTab struct {
	*taskmanager.ChromeTab
	tconn           *chrome.TestConn
	cwsAppInstalled bool
}

func newYoutubeTab(tconn *chrome.TestConn, url string, browserType browser.Type) *youtubeTab {
	return &youtubeTab{
		ChromeTab: taskmanager.NewChromeTabProcess(url, browserType),
		tconn:     tconn,
	}
}

func (tab *youtubeTab) Open(ctx context.Context, br *browser.Browser) error {
	if err := tab.ChromeTab.Open(ctx, br); err != nil {
		return err
	}

	// If the YouTube app from Chrome Web Store is installed, it will be used to open the YouTube link by default.
	// The displayed name in the task manager will be different.
	cwsAppInstalled, err := ash.ChromeAppInstalled(ctx, tab.tconn, apps.YouTubeCWS.ID)
	if err != nil {
		return errors.Wrap(err, "failed to get Chrome Apps list")
	}

	tab.cwsAppInstalled = cwsAppInstalled
	return nil
}

func (tab *youtubeTab) NameInTaskManager(ctx context.Context) (string, error) {
	// Tab name might dynamically change.
	// Update the tab information to ensure the latest title returned.
	if err := tab.UpdateInfo(ctx); err != nil {
		return "", errors.Wrap(err, "failed to update tab information")
	}

	if tab.cwsAppInstalled {
		return "App: " + tab.Title, nil
	}

	return tab.ChromeTab.NameInTaskManager(ctx)
}
