// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package launcher

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/browser/browserui"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SearchWeb,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "App Launcher Search: Web",
		Contacts: []string{
			"chromeos-launcher@google.com",
			"chromeos-sw-engprod@google.com",
		},
		BugComponent: "b:1288350",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome", "drivefs", "chrome_internal"},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-fd3faffe-194b-44e1-b829-eddc3ec07639",
		}},
		Fixture: "chromeLoggedIn",
		Params: []testing.Param{{
			Name: "clamshell",
			Val:  launcher.TestCase{TabletMode: false},
		}, {
			Name: "tablet",
			Val:  launcher.TestCase{TabletMode: true},
		}},
	})
}

// SearchWeb tests that App Launcher Search: Web.
func SearchWeb(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to take keyboard: ", err)
	}
	defer kb.Close(ctx)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	tabletMode := s.Param().(launcher.TestCase).TabletMode

	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, tabletMode)
	if err != nil {
		s.Fatal("Failed to ensure clamshell/tablet mode: ", err)
	}
	defer cleanup(cleanupCtx)

	if !tabletMode {
		if err := ash.WaitForLauncherState(ctx, tconn, ash.Closed); err != nil {
			s.Fatal("Launcher not closed: ", err)
		}
	}

	query := "web browser"

	// The expected result will not be an app, so launcher.SearchAndLaunchWithQuery and other similar functions do not work.
	if err := uiauto.Combine(fmt.Sprintf("search %q in launcher", query),
		launcher.Open(tconn),
		launcher.Search(tconn, kb, query),
	)(ctx); err != nil {
		s.Fatalf("Failed to search %s in launcher: %v", query, err)
	}

	resultFinder := launcher.SearchResultListItemFinder.Name(query + ", Google Search")
	ui := uiauto.New(tconn)

	if err := ui.LeftClick(resultFinder)(ctx); err != nil {
		s.Fatalf("Failed to left click %s in launcher: %v", query, err)
	}
	defer ash.CloseAllWindows(cleanupCtx, tconn)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "launched_result_ui_dump")

	addressBarInfo, err := ui.Info(ctx, browserui.AddressBarFinder)
	if err != nil {
		s.Fatal("Failed to get address bar view info: ", err)
	}

	queryFromURL, err := extractSearchQuery(addressBarInfo.Value)
	if err != nil {
		s.Fatal("Unable to extract search query from browser's address bar: ", err)
	}
	if queryFromURL != query {
		s.Fatalf("Unexpected search query in browser's address bar; got %q, want %q", queryFromURL, query)
	}
}

// extractSearchQuery extracts query parameter `q` from a raw URL (supports captcha redirects).
func extractSearchQuery(rawURL string) (string, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return "", errors.Wrapf(err, "unable to parse URL %q", rawURL)
	}

	queryParams, err := url.ParseQuery(parsedURL.RawQuery)
	if err != nil {
		return "", errors.Wrapf(err, "unable to parse URL query params %q", parsedURL.RawQuery)
	}

	if queryParams.Has("continue") {
		return extractSearchQuery(queryParams.Get("continue"))
	}

	return queryParams.Get("q"), nil
}
