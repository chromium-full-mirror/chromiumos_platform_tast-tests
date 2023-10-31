// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package lacros

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast-tests/cros/local/input"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ChromeURLsInLauncher,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Opens various chrome:// and os:// URLs via the ChromeOS launcher search",
		Contacts: []string{
			"lacros-tast@google.com",
			"neis@chromium.org",
		},
		BugComponent: "crbug:OS>LaCrOS",
		Attr:         []string{"group:mainline", "informational", "group:criticalstaging"},
		SoftwareDeps: []string{"chrome", "lacros"},
		Timeout:      4 * time.Minute,
		Params: []testing.Param{{
			// This variant tests the regular case.
			Fixture: "lacrosKeepAlive",
			Val:     true,
		}, {
			// This variant tests the case where Lacros isn't running when the URLs are entered into the
			// launcher (which is very rare in production). In that case, only os:// URLs are supported.
			Name:    "lacros_not_running",
			Fixture: "lacros",
			Val:     false,
		}},
	})
}

func ChromeURLsInLauncher(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	testChromeURLs := s.Param().(bool)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	defer kb.Close(ctx)

	runSubTests(ctx, tconn, kb, cr, s, osURLs)
	if testChromeURLs {
		runSubTests(ctx, tconn, kb, cr, s, chromeURLs)
	}
}

type subtest struct {
	url           string
	windowMatcher func(w *ash.Window) bool
}

var chromeURLs = []subtest{{
	url:           "chrome://version",
	windowMatcher: matchLacrosWindow("About Version"),
}, {
	url:           "chrome://histograms",
	windowMatcher: matchLacrosWindow("Histograms"),
}, {
	url:           "chrome://flags",
	windowMatcher: matchLacrosWindow("Experiments"),
}, {
	url:           "chrome://crashes",
	windowMatcher: matchSWAWindow("ChromeOS-URLs - Crashes"), // OS_URL_HANDLER SWA
}}

var osURLs = []subtest{{
	url:           "os://version",
	windowMatcher: matchSWAWindow("ChromeOS-URLs - About Version"), // OS_URL_HANDLER SWA
}, {
	url:           "os://histograms",
	windowMatcher: matchSWAWindow("ChromeOS-URLs - Histograms"), // OS_URL_HANDLER SWA
}, {
	url:           "os://flags",
	windowMatcher: matchSWAWindow("Flags - Experiments"), // FLAGS SWA
}, {
	url:           "os://crashes",
	windowMatcher: matchSWAWindow("ChromeOS-URLs - Crashes"), // OS_URL_HANDLER SWA
}}

func runSubTests(ctx context.Context, tconn *chrome.TestConn, kb *input.KeyboardEventWriter, cr *chrome.Chrome, s *testing.State, subtests []subtest) {
	for i, t := range subtests {
		s.Run(ctx, t.url, func(ctx context.Context, s *testing.State) {
			cleanupCtx := ctx
			ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
			defer cancel()

			cleanup, err := launcher.SetUpLauncherTest(ctx, tconn, false /*tabletMode*/, false /*stabilizeAppCount*/)
			if err != nil {
				s.Fatal("Failed to set up launcher test case: ", err)
			}
			defer cleanup(cleanupCtx)
			defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, fmt.Sprintf("ui_%d", i))

			if err := searchAndOpen(ctx, tconn, kb, t); err != nil {
				s.Fatal("Failed to search and open: ", err)
			}

			// Make sure nothing else was opened.
			ws, err := ash.GetAllWindows(ctx, tconn)
			if err != nil {
				s.Fatal("Failed to get all windows: ", err)
			}
			if len(ws) != 1 {
				s.Fatalf("Unexpected number of windows: want 1, got %d", len(ws))
			}

			// This is only needed for the keepalive variant, since
			// ResetState below unfortunately does not touch Lacros
			// in that case.
			if err := ash.CloseAllWindows(ctx, tconn); err != nil {
				s.Fatal("Failed to close all windows: ", err)
			}
		})
		if err := cr.ResetState(ctx); err != nil {
			s.Fatal("Failed to reset Chrome: ", err)
		}
	}
}

func searchAndOpen(ctx context.Context, tconn *chrome.TestConn, kb *input.KeyboardEventWriter, t subtest) error {
	if err := launcher.Search(tconn, kb, t.url)(ctx); err != nil {
		return errors.Wrap(err, "failed to search")
	}

	searchResult := launcher.SearchResultListItemFinder.Name(t.url)
	if err := uiauto.New(tconn).DoDefault(searchResult)(ctx); err != nil {
		return errors.Wrap(err, "failed to find or activate search result")
	}

	if err := ash.WaitForCondition(ctx, tconn, t.windowMatcher, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
		return errors.Wrap(err, "failed to find window")
	}

	return nil
}

func matchLacrosWindow(title string) func(w *ash.Window) bool {
	return func(w *ash.Window) bool {
		return w.Title == title && w.WindowType == ash.WindowTypeLacros
	}
}

func matchSWAWindow(title string) func(w *ash.Window) bool {
	return func(w *ash.Window) bool {
		return w.Title == title && w.WindowType == ash.WindowTypeSystem
	}
}
