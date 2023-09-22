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
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome", "lacros"},
		Fixture:      "lacros",
		Timeout:      4 * time.Minute,
	})
}

type subtest struct {
	url           string
	windowMatcher func(w *ash.Window) bool
}

func ChromeURLsInLauncher(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	defer kb.Close(ctx)

	for i, t := range []subtest{{
		url:           "chrome://version",
		windowMatcher: matchLacrosWindow("About Version"),
	}, {
		url:           "os://version",
		windowMatcher: matchSWAWindow("ChromeOS-URLs - About Version"), // OS_URL_HANDLER SWA
	}, {
		url:           "chrome://histograms",
		windowMatcher: matchLacrosWindow("Histograms"),
	}, {
		url:           "os://histograms",
		windowMatcher: matchSWAWindow("ChromeOS-URLs - Histograms"), // OS_URL_HANDLER SWA
	}, {
		url:           "chrome://flags",
		windowMatcher: matchLacrosWindow("Experiments"),
	}, {
		url:           "os://flags",
		windowMatcher: matchSWAWindow("Flags - Experiments"), // FLAGS SWA
	}, {
		url:           "chrome://crashes",
		windowMatcher: matchSWAWindow("ChromeOS-URLs - Crashes"), // OS_URL_HANDLER SWA
	}, {
		url:           "os://crashes",
		windowMatcher: matchSWAWindow("ChromeOS-URLs - Crashes"), // OS_URL_HANDLER SWA
	}} {
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
