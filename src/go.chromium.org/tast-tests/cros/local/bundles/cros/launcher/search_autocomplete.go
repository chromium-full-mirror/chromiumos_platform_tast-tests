// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package launcher

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
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
			"chromeos-launcher@google.com",
			"chromeos-sw-engprod@google.com",
		},
		BugComponent: "b:1288350",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-fd3faffe-194b-44e1-b829-eddc3ec07639",
		}},
		Params: []testing.Param{{
			Name:    "clamshell_mode_isaac_newto",
			Fixture: "chromeLoggedInExtendedAutocomplete",
			Val: searchAutocompleteTestCase{TabletMode: false,
				searchKeyword:          "Isaac Newto",
				result:                 "Isaac Newton, Google Search",
				expectedSearchBoxText:  "Isaac Newton",
				expectedGhostGhostText: "Search and Assistant",
			},
		}, {
			Name:    "tablet_mode_isaac_newto",
			Fixture: "chromeLoggedInExtendedAutocomplete",
			Val: searchAutocompleteTestCase{TabletMode: true,
				searchKeyword:          "Isaac Newto",
				result:                 "Isaac Newton, Google Search",
				expectedSearchBoxText:  "Isaac Newton",
				expectedGhostGhostText: "Search and Assistant"},
			ExtraHardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		}, {
			Name:    "clamshell_mode_saac_newton",
			Fixture: "chromeLoggedInExtendedAutocomplete",
			Val: searchAutocompleteTestCase{TabletMode: false,
				searchKeyword:          "saac Newton",
				result:                 "Isaac Newton, Google Search",
				expectedSearchBoxText:  "saac Newton",
				expectedGhostGhostText: "Isaac Newton - Search and Assistant",
			},
		}, {
			Name:    "tablet_mode_saac_newton",
			Fixture: "chromeLoggedInExtendedAutocomplete",
			Val: searchAutocompleteTestCase{TabletMode: true,
				searchKeyword:          "saac Newton",
				result:                 "Isaac Newton, Google Search",
				expectedSearchBoxText:  "saac Newton",
				expectedGhostGhostText: "Isaac Newton - Search and Assistant"},
			ExtraHardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		}},
	})
}

// SearchAutocomplete checks launcher search box behavior for autocompleting
// for highest ranked result.
func SearchAutocomplete(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

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
		s.Fatal(errors.Wrapf(err, "failed to search for: %s", testCase.searchKeyword))
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
