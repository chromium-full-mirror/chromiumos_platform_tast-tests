// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package launcher

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/shortcutcustomization"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ShortcutSearch,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests that searching for queries associated with a keyhboard shortcut returns a keyboard shortcut result",
		Contacts: []string{
			"chromeos-launcher@google.com",
			"chromeos-sw-engprod@google.com",
		},
		BugComponent: "b:1288350",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-c7b23296-4a15-4460-afaf-c0203dc2c224",
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

type shortcutSearchTestCase struct {
	// Search keyword.
	searchKeyword string
	// The result item name that is expected to be found within search results.
	result string
}

// ShortcutSearch tests launcher searches for keyboard shortcuts.
func ShortcutSearch(ctx context.Context, s *testing.State) {
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

	subtests := []shortcutSearchTestCase{
		{
			searchKeyword: "Lock Screen",
			result:        "Lock screen, Shortcuts, (Search|Launcher)\\+ l",
		},
		{
			searchKeyword: "Launcher",
			result:        "Open/close the launcher, Shortcuts, (Search|Launcher)",
		},
		{
			searchKeyword: "Overview",
			result:        "Overview mode, Shortcuts, Overview mode key",
		},
		{
			searchKeyword: "new window",
			result:        "Open new window, Shortcuts, Ctrl\\+ n",
		},
		{
			searchKeyword: "new window",
			result:        "Open a new window in Incognito mode, Shortcuts, Ctrl\\+ Shift\\+ n",
		},
		{
			searchKeyword: "incognito",
			result:        "Open a new window in Incognito mode, Shortcuts, Ctrl\\+ Shift\\+ n",
		},
	}

	for _, subtest := range subtests {
		s.Run(ctx, subtest.searchKeyword, func(ctx context.Context, s *testing.State) {
			ui := uiauto.New(tconn)
			clearSearchButton := nodewith.Role(role.Button).Name("Clear searchbox text")

			defer ui.LeftClick(clearSearchButton)(cleanupCtx)
			defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree_"+string(subtest.searchKeyword))

			resultFinder := launcher.SearchResultListItemFinder.NameRegex(regexp.MustCompile(subtest.result))
			if err := uiauto.Combine("search launcher",
				launcher.Open(tconn),
				launcher.Search(tconn, kb, subtest.searchKeyword),
				ui.WaitUntilExists(resultFinder),
			)(ctx); err != nil {
				s.Fatal("Failed to search: ", err)
			}

			if err := ui.LeftClick(resultFinder)(ctx); err != nil {
				s.Fatal("Failed to launch the search result: ", err)
			}

			// Verify shortcut customization app is launched.
			if err := shortcutcustomization.VerifyShortcutCustomizationIsLaunched(ctx, tconn, ui); err != nil {
				s.Fatal("Failed to verify that the Shortcut Customization app is launched: ", err)
			}

			if err := apps.Close(ctx, tconn, apps.ShortcutCustomization.ID); err != nil {
				s.Fatal("Failed to close Shortcut Customization app: ", err)
			}

			if err := ash.WaitForAppClosed(ctx, tconn, apps.ShortcutCustomization.ID); err != nil {
				s.Fatal("Shortcut Customization app did not close successfully: ", err)
			}
		})
	}
}
