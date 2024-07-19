// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package taskmanager

import (
	"context"
	"math/rand"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/taskmanager"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CloseTab,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test the entry should be removed in task manager automatically after closing tab",
		Contacts: []string{
			"chromeos-wm@google.com",
			"chromeos-consumer-engprod@google.com",
			"afakhry@google.com",
		},
		BugComponent: "b:1457613",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "chromeLoggedIn",
		Timeout:      5 * time.Minute,
	})
}

type closeTabTestResources struct {
	cr          *chrome.Chrome
	outDir      string
	tconn       *chrome.TestConn
	ui          *uiauto.Context
	taskManager *taskmanager.TaskManager
	processes   []taskmanager.Process
}

// CloseTab tests the entry should be removed in task manager automatically after closing tab.
func CloseTab(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to create keyboard input: ", err)
	}
	defer kb.Close(ctx)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	resources := &closeTabTestResources{
		cr:          cr,
		outDir:      s.OutDir(),
		tconn:       tconn,
		ui:          uiauto.New(tconn),
		taskManager: taskmanager.New(tconn, kb),
		processes: []taskmanager.Process{
			newBrowserTabInCloseTabTest("https://www.facebook.com"),
			newBrowserTabInCloseTabTest("https://www.amazon.com"),
			newBrowserTabInCloseTabTest("https://www.apple.com"),
			newBrowserTabInCloseTabTest("https://en.wikipedia.org/wiki/Main_Page"),
			newBrowserTabInCloseTabTest("https://news.google.com"),
			newBrowserTabInCloseTabTest("https://www.youtube.com"),
			newBrowserTabInCloseTabTest("https://help.netflix.com/en"),
			newBrowserTabInCloseTabTest("https://news.ycombinator.com/news"),
			newBrowserTabInCloseTabTest("https://www.cbc.ca/lite/trending-news"),
			newBrowserTabInCloseTabTest("https://translate.google.com/?hl=en"),
		},
	}
	numberOfTabs := len(resources.processes)

	// Select half of tabs to close.
	needToClose := make([]bool, numberOfTabs)
	for idx := range needToClose {
		needToClose[idx] = idx < numberOfTabs/2
	}
	// Shuffle to make the selection randomly to avoid accessing some same websites every time.
	rand.Shuffle(numberOfTabs, func(i, j int) { needToClose[i], needToClose[j] = needToClose[j], needToClose[i] })
	for idx, process := range resources.processes {
		if tab, ok := process.(*browserTabInCloseTabTest); ok {
			tab.needToClose = needToClose[idx]
		}
	}

	// Close button on the tab will be hidden if DUT is in tablet mode.
	// Therefore, force the test to run under clamshell mode.
	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, false)
	if err != nil {
		s.Fatal("Failed to ensure in tablet mode: ", err)
	}
	defer cleanup(cleanupCtx)

	for _, process := range resources.processes {
		if err := process.Open(ctx, cr); err != nil {
			s.Fatal("Failed to open tab: ", err)
		}
		defer process.Close(cleanupCtx)
	}
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "chrome_tab_ui_dump")

	if err := checkTabsInTaskManager(ctx, resources); err != nil {
		s.Fatal("Failed to check all tabs exist in task manager: ", err)
	}

	for _, process := range resources.processes {
		if tab, ok := process.(*browserTabInCloseTabTest); ok && tab.needToClose {
			name, err := process.NameInTaskManager(ctx)
			if err != nil {
				s.Fatal("Failed to obtain the process name in task manager: ", err)
			}
			tab.name = name
			targetTab := nodewith.NameStartingWith(tab.Title).HasClass("Tab").Ancestor(nodewith.HasClass("BrowserView"))
			if err := uiauto.Combine("active the target tab and close it",
				resources.ui.LeftClick(targetTab),
				resources.ui.WaitUntilExists(nodewith.Name("Close").HasClass("TabCloseButton").Ancestor(targetTab)),
				resources.ui.LeftClick(nodewith.Name("Close").HasClass("TabCloseButton").Ancestor(targetTab)),
				resources.ui.WaitUntilGone(targetTab),
			)(ctx); err != nil {
				s.Fatal("Failed to complete the actions: ", err)
			}
			tab.closed = true
		}
	}

	s.Log("Check the tabs in the task manager again to verify the result is expected")
	if err := checkTabsInTaskManager(ctx, resources); err != nil {
		s.Fatal("Failed to check the state of tabs: ", err)
	}
}

func checkTabsInTaskManager(ctx context.Context, resources *closeTabTestResources) (retErr error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	if err := resources.taskManager.Open(ctx); err != nil {
		return errors.Wrap(err, "failed to launch the task manager")
	}
	defer resources.taskManager.Close(cleanupCtx, resources.tconn)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, resources.outDir, func() bool { return retErr != nil }, resources.cr, "taskManager_ui_dump")

	verify := map[bool]func(*nodewith.Finder) action.Action{
		true:  resources.ui.WaitUntilGone,
		false: resources.ui.WaitUntilExists,
	}

	for _, process := range resources.processes {
		if tab, ok := process.(*browserTabInCloseTabTest); ok {
			name := tab.name
			if !tab.closed {
				var err error
				name, err = process.NameInTaskManager(ctx)
				if err != nil {
					return err
				}
			}
			if err := verify[tab.closed](nodewith.Name(name))(ctx); err != nil {
				return errors.Wrapf(err, "failed to check the state of %q in task manager", name)
			}

			if tab.closed {
				testing.ContextLogf(ctx, "%q is closed", name)
			} else {
				testing.ContextLogf(ctx, "%q is opened", name)
			}
		}
	}

	return nil
}

type browserTabInCloseTabTest struct {
	*taskmanager.ChromeTab
	closed      bool
	needToClose bool
	// Tab name in task manager is dynamically fetched and stored in *taskmanager.ChromeTab.
	// If the tab is truly closed, it will fail on fetching the tab name.
	// Therefore, declare a field to store name for the tab which will be closed.
	name string
}

func newBrowserTabInCloseTabTest(url string) *browserTabInCloseTabTest {
	return &browserTabInCloseTabTest{
		ChromeTab:   taskmanager.NewChromeTabProcess(url),
		closed:      false,
		needToClose: false,
	}
}
