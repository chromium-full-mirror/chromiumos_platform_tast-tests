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
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CategoricalSearch,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks for best match search results in the launcher",
		Contacts: []string{
			"chromeos-launcher@google.com",
			"chromeos-sw-engprod@google.com",
		},
		BugComponent: "b:1288350",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-90e4fecc-d2ea-40dc-b9db-eb9d61089e22",
		}},
		Params: []testing.Param{{
			Name:    "clamshell_mode",
			Fixture: "chromeLoggedIn",
			Val:     launcher.TestCase{TabletMode: false},
		}, {
			Name:              "tablet_mode",
			Fixture:           "chromeLoggedIn",
			Val:               launcher.TestCase{TabletMode: true},
			ExtraHardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		}},
	})
}

type categoricalSearchTestCase struct {
	searchKeyword string
	category      string
	categoryLabel string
	result        string
}

// CategoricalSearch checks inline answers for special queries.
func CategoricalSearch(ctx context.Context, s *testing.State) {
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

	testCase := s.Param().(launcher.TestCase)

	cleanup, err := launcher.SetUpLauncherTest(ctx, tconn, testCase.TabletMode, false /*stabilizeAppCount*/)
	if err != nil {
		s.Fatal("Failed to set up launcher test case: ", err)
	}
	defer cleanup(cleanupCtx)

	subtests := []categoricalSearchTestCase{
		{
			searchKeyword: "Chrome",
			category:      "Best Match , search result category",
			categoryLabel: "Best Match",
			result:        "Chrome, Installed App",
		},
		{
			searchKeyword: "Settings",
			category:      "Best Match , search result category",
			categoryLabel: "Best Match",
			result:        "Settings, Installed App",
		},
		{
			searchKeyword: "Files",
			category:      "Best Match , search result category",
			categoryLabel: "Best Match",
			result:        "Files, Installed App",
		},
		{
			searchKeyword: "Keyboard shortcuts",
			category:      "Best Match , search result category",
			categoryLabel: "Best Match",
			result:        "Keyboard shortcuts, Keyboard, Settings",
		},
	}

	for _, subtest := range subtests {
		s.Run(ctx, subtest.searchKeyword, func(ctx context.Context, s *testing.State) {
			ui := uiauto.New(tconn)
			clearSearchButton := nodewith.Role(role.Button).Name("Clear searchbox text")
			defer ui.DoDefault(clearSearchButton)(cleanupCtx)

			defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree_"+string(subtest.searchKeyword))

			if err := uiauto.Combine("search launcher",
				launcher.Search(tconn, kb, subtest.searchKeyword),
				launcher.WaitForCategoryLabel(tconn, subtest.category, subtest.categoryLabel),
				launcher.WaitForCategorizedResult(tconn, subtest.category, subtest.result),
			)(ctx); err != nil {
				s.Fatal("Failed to search: ", err)
			}
		})
	}
}
