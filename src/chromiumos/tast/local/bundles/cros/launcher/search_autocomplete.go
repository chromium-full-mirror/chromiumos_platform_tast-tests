// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package launcher

import (
	"context"
	"time"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/launcher"
	"chromiumos/tast/local/input"
	"chromiumos/tast/testing"
	"chromiumos/tast/testing/hwdep"
)

// searchAutocompleteTestCase describes modes in which the launcher UI can be
// shown, and by which launcher test should generally be parameterized.
// It additionally provides a search query and the expected results.
// Use a struct because it makes the individual test cases more readable.
type searchAutocompleteTestCase struct {
	TabletMode             bool
	searchKeyword          string
	category               string
	result                 string
	expectedSearchBoxText  string
	expectedGhostGhostText string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         SearchAutocomplete,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks Autocomplete behavior in Launcher Search",
		Contacts: []string{
			"cros-system-ui-eng@google.com",
			"chromeos-sw-engprod@google.com",
			"yulunwu@chromium.org",
		},
		BugComponent: "b:1288350",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{{
			Name:    "clamshell_mode_joe_bide",
			Fixture: "chromeLoggedInExtendedAutocomplete",
			Val: searchAutocompleteTestCase{TabletMode: false,
				searchKeyword:          "Joe Bide",
				result:                 "Joe Biden, 46th U.S. President - Google Search, Google Search",
				expectedSearchBoxText:  "Joe Biden",
				expectedGhostGhostText: "Search and Assistant",
			},
		}, {
			Name:    "tablet_mode_joe_bide",
			Fixture: "chromeLoggedInExtendedAutocomplete",
			Val: searchAutocompleteTestCase{TabletMode: true,
				searchKeyword:          "Joe Bide",
				result:                 "Joe Biden, 46th U.S. President - Google Search, Google Search",
				expectedSearchBoxText:  "Joe Biden",
				expectedGhostGhostText: "Search and Assistant"},
			ExtraHardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		}, {
			Name:    "clamshell_mode_oe_biden",
			Fixture: "chromeLoggedInExtendedAutocomplete",
			Val: searchAutocompleteTestCase{TabletMode: false,
				searchKeyword:          "oe Biden",
				result:                 "Joe Biden, 46th U.S. President - Google Search, Google Search",
				expectedSearchBoxText:  "oe Biden",
				expectedGhostGhostText: "Joe Biden - Search and Assistant",
			},
		}, {
			Name:    "tablet_mode_oe_biden",
			Fixture: "chromeLoggedInExtendedAutocomplete",
			Val: searchAutocompleteTestCase{TabletMode: true,
				searchKeyword:          "oe Biden",
				result:                 "Joe Biden, 46th U.S. President - Google Search, Google Search",
				expectedSearchBoxText:  "oe Biden",
				expectedGhostGhostText: "Joe Biden - Search and Assistant"},
			ExtraHardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		}},
	})
}

// SearchAutocomplete checks launcher search box behavior for autocompleting
// for highest ranked result.
func SearchAutocomplete(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*chrome.Chrome)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	defer kb.Close(ctx)

	testCase := s.Param().(searchAutocompleteTestCase)

	cleanup, err := launcher.SetUpLauncherTest(ctx, tconn, testCase.TabletMode, false /*stabilizeAppCount*/)
	if err != nil {
		s.Fatal("Failed to set up launcher test case: ", err)
	}
	defer cleanup(cleanupCtx)

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree_query_"+string(testCase.searchKeyword))

	if err := uiauto.Combine("search launcher and verify ghost text",
		launcher.Search(tconn, kb, testCase.searchKeyword),
		launcher.WaitForResult(tconn, testCase.result))(ctx); err != nil {
		s.Fatal("Failed to search for: ", testCase.searchKeyword)
	}
	res, err :=
		launcher.GetSearchBoxGhostText(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get ghost text: ", err)
	}

	if res != testCase.expectedGhostGhostText {
		s.Fatalf("Failed to verify ghost text: got:%s, want:%s", res, testCase.expectedGhostGhostText)
	}
}
