// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package taskmanager

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"time"

	"github.com/mafredri/cdp/protocol/target"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
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
		Func:           EndProcess,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		LacrosStatus:   testing.LacrosVariantExists,
		Desc:           "Verify the 'End process' button works on plugin, non-plugin and grouped tabs",
		Contacts: []string{
			// "chromeos-sw-engprod@google.com",
			// "afakhry@google.com",
			"alfredyu@cienet.com",
			"chromeos-connectivity-cienet-external@google.com",
		},
		BugComponent: "b:1238037", // ChromeOS > Software > Task Manager
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{
			{
				Fixture: "chromeLoggedIn",
				Val:     browser.TypeAsh,
			}, {
				Name:              "lacros",
				ExtraSoftwareDeps: []string{"lacros"},
				Fixture:           "lacros",
				Val:               browser.TypeLacros,
			},
		},
		Timeout: 10 * time.Minute,
	})
}

type endProcessTestResources struct {
	cr          *chrome.Chrome
	br          *browser.Browser
	ui          *uiauto.Context
	taskManager *taskmanager.TaskManager
}

// EndProcess verifies the "End process" button works on plugin, non-plugin and grouped tabs.
func EndProcess(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		errors.Wrap(err, "failed to get keyboard")
	}
	defer kb.Close(ctx)

	resources := &endProcessTestResources{
		cr:          cr,
		ui:          uiauto.New(tconn),
		taskManager: taskmanager.New(tconn, kb),
	}

	browserType := s.Param().(browser.Type)
	pluginTest, cleanupPluginTest, err := newPluginTest(browserType)
	if err != nil {
		s.Fatal("Failed to initialize plugin test: ", err)
	}
	defer cleanupPluginTest()

	for _, test := range []endProcessTest{
		newNonPluginTest(browserType),
		pluginTest,
		newGroupedTabsTest(browserType),
	} {
		f := func(ctx context.Context, s *testing.State) {
			cleanupCtx := ctx
			ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
			defer cancel()

			if browserType == browser.TypeLacros {
				// Ensuring Lacros is not running before launching it since the launched Lacros
				// from previous run could be hasn't terminated yet.
				if err := lacros.WaitForLacrosNotRunning(ctx, tconn, 3*time.Second); err != nil {
					s.Fatal("Failed to wait for Lacros is not running: ", err)
				}
			}

			// browserfixt.SetUp sets up the lacros by bringing up the window (with one empty tab),
			// but no window will be brought up for ash-Chrome.
			// browserfixt.SetUpWithURL is used to bring up the browser window uniformly
			// to make tab amount to be consistent between ash/lacros variants.
			conn, br, closeBrowser, err := browserfixt.SetUpWithURL(ctx, cr, browserType, chrome.BlankURL)
			if err != nil {
				s.Fatal("Failed to set up browser: ", err)
			}
			defer func(ctx context.Context) {
				conn.CloseTarget(ctx)
				conn.Close()
				closeBrowser(ctx)
			}(cleanupCtx)
			resources.br = br

			for _, process := range test.getProcesses() {
				if err := process.Open(ctx, br); err != nil {
					s.Fatal("Failed to open the process: ", err)
				}
				defer process.Close(cleanupCtx)
				defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, test.getDescription()+"_before_closing_tab")

				if tab, ok := process.(*pluginTab); ok {
					if err := tab.waitUntilPluginStable(ctx, resources); err != nil {
						s.Fatal("Failed to wait until plugin to be stable: ", err)
					}
				}
			}

			// Close the blank tab after all processes are opened.
			if err := br.CloseWithURL(ctx, chrome.BlankURL); err != nil {
				s.Fatal("Failed to close blank tab: ", err)
			}

			if err := resources.taskManager.Open(ctx); err != nil {
				s.Fatal("Failed to open the task manager: ", err)
			}
			defer resources.taskManager.Close(cleanupCtx, tconn)
			defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, test.getDescription()+"_before_closing_tm")

			if err := uiauto.Combine("order the processes and wait until the task manager to be stable",
				resources.taskManager.OrderBy(taskmanager.ProcessIDColumn, true /* ascending */),
				resources.taskManager.WaitUntilStable,
			)(ctx); err != nil {
				s.Fatal("Failed to order the process by id: ", err)
			}

			if err := test.terminateAndVerify(ctx, resources); err != nil {
				s.Fatal("Failed to terminate the process: ", err)
			}
		}

		if !s.Run(ctx, test.getDescription(), f) {
			s.Error("Failed to run ", test.getDescription())
		}
	}
}

type endProcessTest interface {
	terminateAndVerify(ctx context.Context, res *endProcessTestResources) error
	getDescription() string
	getProcesses() []taskmanager.Process
}

type nonPluginTest struct {
	description string
	processes   []taskmanager.Process
}

func newNonPluginTest(browserType browser.Type) *nonPluginTest {
	processes := []taskmanager.Process{
		taskmanager.NewChromeTabProcess("https://translate.google.com/?hl=en", browserType),
		taskmanager.NewChromeTabProcess("https://news.ycombinator.com/news", browserType),
		taskmanager.NewChromeTabProcess("http://lite.cnn.com/en", browserType),
		taskmanager.NewChromeTabProcess("https://help.netflix.com/en", browserType),
		taskmanager.NewChromeTabProcess("https://www.cbc.ca/lite/trending-news", browserType),
	}

	return &nonPluginTest{"non_plugin_test", processes}
}

func (npt *nonPluginTest) terminateAndVerify(ctx context.Context, res *endProcessTestResources) error {
	return terminateAndVerify(ctx, npt, res)
}

func (npt *nonPluginTest) getDescription() string {
	return npt.description
}

func (npt *nonPluginTest) getProcesses() []taskmanager.Process {
	return npt.processes
}

type plugin struct {
	// name is the plugin name displayed in task manager.
	name string
	// nodeFinder is the plugin node finder on the website.
	nodeFinder *nodewith.Finder
	// source is the plugin source url which would be used to filter the expected Chrome target.
	source string
	// targets is the expected Chrome targets.
	// It is not initialized until using the filter to filter the expected Chrome targets.
	targets []*chrome.Target
}

type pluginTab struct {
	*taskmanager.ChromeTab
	plugin *plugin
}

func (pTab *pluginTab) NameInTaskManager(ctx context.Context) (string, error) {
	name := "Subframe: " + pTab.plugin.name
	if pTab.BrowserType() == browser.TypeLacros {
		name = fmt.Sprintf("Lacros: %s", name)
	}

	return name, nil
}

func (pTab *pluginTab) waitUntilPluginStable(ctx context.Context, res *endProcessTestResources) error {
	// Under some slow network connections or DUTs, the plugin node might not be loaded instantly.
	// Therefore, give some time to wait until the target node exists.
	if err := res.ui.WithTimeout(time.Minute).WaitUntilExists(pTab.plugin.nodeFinder)(ctx); err != nil {
		return errors.Wrap(err, "failed to find the plugin node")
	}

	ts, err := res.br.FindTargets(ctx, func(t *target.Info) bool {
		// If the network or the plugin source website is down,
		// the plugin target could still be found.
		return strings.Contains(t.URL, pTab.plugin.source)
	})
	if err != nil {
		return errors.Wrap(err, "failed to obtain the targets")
	}
	if len(ts) == 0 {
		return errors.New("failed to find the plugin targets")
	}

	// One plugin might be the combination of multiple chrome targets.
	// Store all of them, and check if they all are terminated in the end.
	pTab.plugin.targets = ts
	return nil
}

type pluginTest struct {
	description string
	processes   []taskmanager.Process
}

// newPluginTest starts a local http test server to serve the html file with iframe plugin.
// This function returns an instance of pluginTest, a cleanup function to close http test server
// and an error if exists.
func newPluginTest(browserType browser.Type) (*pluginTest, func(), error) {
	tabs := map[string]*pluginTab{
		"youtube": {
			plugin: &plugin{
				name:       "https://youtube.com/",
				nodeFinder: nodewith.NameStartingWith("COSTA RICA IN 4K 60fps HDR (ULTRA HD)").Role(role.RootWebArea),
				source:     "https://www.youtube.com/embed/LXb3EKWsInQ",
			},
		},
		"googlemap": {
			plugin: &plugin{
				name:       "https://google.com/",
				nodeFinder: nodewith.Name("Map").Role(role.Region).First(),
				source:     "https://www.google.com/maps/embed?origin=mfe&pb=!1m3!2m1!1sMountain+View,+CA,+USA!6i13",
			},
		},
	}

	mux := http.NewServeMux()
	handleFunc := func(source string) func(w http.ResponseWriter, r *http.Request) {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add("Content-Type", "text/html")
			fmt.Fprintf(w, `<iframe src="%s" width="640" height="360" frameborder="0" allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture" allowfullscreen></iframe>`, source)
		}
	}
	for path, tab := range tabs {
		mux.HandleFunc(fmt.Sprintf("/%s", path), handleFunc(tab.plugin.source))
	}

	httpServer := httptest.NewServer(mux)

	var processes []taskmanager.Process
	for path, tab := range tabs {
		url, err := url.JoinPath(httpServer.URL, path)
		if err != nil {
			return nil, nil, err
		}
		tab.ChromeTab = taskmanager.NewChromeTabProcess(url, browserType)
		processes = append(processes, tab)
	}

	return &pluginTest{"plugin_test", processes}, httpServer.Close, nil
}

func (pt *pluginTest) terminateAndVerify(ctx context.Context, res *endProcessTestResources) error {
	rand.Seed(time.Now().UnixNano())
	p := pt.processes[rand.Intn(len(pt.processes))]

	name, err := p.NameInTaskManager(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to obtain the process name in task manager")
	}
	testing.ContextLogf(ctx, "Terminate plugin process %q", name)
	if err := res.taskManager.TerminateProcess(name)(ctx); err != nil {
		return errors.Wrap(err, "failed to verify 'End process' button works")
	}

	// Contents in the task manager will keep changing and might not be updated onto UI tree immediately,
	// reopen the task manager to enforce the UI tree to update once.
	if err := res.taskManager.Reopen(ctx); err != nil {
		return errors.Wrap(err, "failed to re-open the task manager")
	}

	// Verify that 'End process' button works by checking process's representing node disappears from
	// the task manager, since a process in task manager can always be found on UI tree even if it's offscreen.
	nameNode := nodewith.Name(name).ClassName("AXVirtualView")
	return uiauto.Combine(fmt.Sprintf("wait until the %q process gone within the task manager", name),
		res.taskManager.WaitUntilStable,
		res.ui.WaitUntilGone(nameNode),
		res.ui.EnsureGoneFor(nameNode, 5*time.Second),
	)(ctx)
}

func (pt *pluginTest) getDescription() string {
	return pt.description
}

func (pt *pluginTest) getProcesses() []taskmanager.Process {
	return pt.processes
}

type groupedTabsTest struct {
	description string
	processes   []taskmanager.Process
}

func newGroupedTabsTest(browserType browser.Type) *groupedTabsTest {
	var processes []taskmanager.Process
	const groupedTabsAmount = 5

	for i := 0; i < groupedTabsAmount; i++ {
		tab := taskmanager.NewChromeTabProcess(chrome.NewTabURL, browserType)
		processes = append(processes, tab)
	}

	return &groupedTabsTest{"grouped_tabs_test", processes}
}

func (gtt *groupedTabsTest) terminateAndVerify(ctx context.Context, res *endProcessTestResources) error {
	return terminateAndVerify(ctx, gtt, res)
}

func (gtt *groupedTabsTest) getDescription() string {
	return gtt.description
}

func (gtt *groupedTabsTest) getProcesses() []taskmanager.Process {
	return gtt.processes
}

func terminateAndVerify(ctx context.Context, test endProcessTest, res *endProcessTestResources) error {
	rand.Seed(time.Now().UnixNano())
	n := rand.Intn(len(test.getProcesses()))
	p := test.getProcesses()[n]

	var processesToBeVerified []taskmanager.Process
	switch test.(type) {
	case *nonPluginTest:
		processesToBeVerified = append(processesToBeVerified, p)
	case *groupedTabsTest:
		for _, process := range test.getProcesses() {
			processesToBeVerified = append(processesToBeVerified, process)
		}
	default:
		return errors.New("unexpected test type")
	}

	for _, process := range processesToBeVerified {
		if status, err := process.Status(ctx); err != nil {
			return err
		} else if status != taskmanager.ProcessAlive {
			return errors.Errorf("expecting the tab process to be alive, but got %q", status)
		}
	}

	name, err := p.NameInTaskManager(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to obtain the process name in task manager")
	}
	testing.ContextLogf(ctx, "Terminate process %q", name)
	if err := res.taskManager.TerminateProcess(name)(ctx); err != nil {
		return errors.Wrap(err, "failed to verify 'End process' button works")
	}

	for _, process := range processesToBeVerified {
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			if status, err := process.Status(ctx); err != nil {
				return err
			} else if status != taskmanager.ProcessDead {
				return errors.Errorf("expecting the tab process to be dead, but got %q", status)
			}
			return nil
		}, &testing.PollOptions{Timeout: 5 * time.Second}); err != nil {
			return errors.Wrapf(err, "failed to verify the process %q is terminated", name)
		}
	}

	return nil
}
