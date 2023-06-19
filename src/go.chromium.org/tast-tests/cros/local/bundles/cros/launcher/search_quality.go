// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package launcher

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast-tests/cros/local/input"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

// The weatherPattern is for weather answer card, the result is dynamic so we use a regex here.
// Weather normally has following formats:
// 17, cloudy, Tue, Canberra ACT
// 17, rain, Canberra See more weather info
// 17, sunny, Canberra ACT
// 17, mostly cloudy, see more forcase for Canberra
const weatherPattern = `(?i)^\d+,\s*.*(?:sunny|clear|cloudy|showers|rain|thunderstorms|overcast|haze|fog|mist|drizzle|snow|sleet|windy).*\bCanberra\b`

// searchQualityTestCase struct encapsulates parameters for test.
type searchQualityTestCase struct {
	query          string
	useRegex       bool
	expectedResult string
	category       string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         SearchQuality,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test different search queries should show content in the right category",
		Contacts:     []string{"launcher-search-notify@google.com", "xiuwen@google.com"},
		BugComponent: "b:1257106",
		Attr:         []string{"group:launcher_search_quality_daily"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "chromeLoggedIn",

		Params: []testing.Param{
			// --- Answer card test cases. ---
			{
				Name: "answer_card_calculator",
				Val: searchQualityTestCase{
					query:          "45+45",
					useRegex:       false,
					expectedResult: "= 90",
					category:       "Answer Card",
				},
			},
			// See details in: https://bugs.chromium.org/p/chromium/issues/detail?id=1432692.
			{
				Name: "answer_card_calculator_large_number",
				Val: searchQualityTestCase{
					query:          "1234+5678",
					useRegex:       false,
					expectedResult: "= 6912",
					category:       "Answer Card",
				},
			},
			{
				Name: "answer_card_caps_lock",
				Val: searchQualityTestCase{
					query:          "caps lock",
					useRegex:       false,
					expectedResult: "Turn Caps Lock on and off, Shortcuts",
					category:       "Answer Card",
				},
			},
			{
				Name: "answer_card_screen_rotate",
				Val: searchQualityTestCase{
					query:          "screen rotate",
					useRegex:       false,
					expectedResult: "Rotate screen 90 degrees, Shortcuts",
					category:       "Answer Card",
				},
			},
			{
				Name: "answer_card_weather",
				Val: searchQualityTestCase{
					query:          "canberra weather",
					useRegex:       true,
					expectedResult: weatherPattern,
					category:       "Answer Card",
				},
			},

			// --- Best match test cases. ---
			{
				Name: "best_match_app_chrome",
				Val: searchQualityTestCase{
					query:          "chrome",
					useRegex:       false,
					expectedResult: "Chrome, Installed App",
					category:       "Best Match",
				},
			},
			{
				Name: "best_match_apps_files",
				Val: searchQualityTestCase{
					query:          "files",
					useRegex:       false,
					expectedResult: "Files, Installed App",
					category:       "Best Match",
				},
			},
			{
				Name: "best_match_apps_settings",
				Val: searchQualityTestCase{
					query:          "settings",
					useRegex:       false,
					expectedResult: "Settings, Installed App",
					category:       "Best Match",
				},
			},
			{
				Name: "best_match_apps_snapchat",
				Val: searchQualityTestCase{
					query:          "chrome",
					useRegex:       false,
					expectedResult: "Chrome, Installed App",
					category:       "Best Match",
				},
			},
			{
				Name: "best_match_files_downloads",
				Val: searchQualityTestCase{
					query:          "downloads",
					useRegex:       false,
					expectedResult: "Downloads, MyFiles",
					category:       "Best Match",
				},
			},
			// TODO(b/286171481): Unsupported, we need to enabled showoff launcher search feature flag.
			{
				Name: "best_match_help_manage_account",
				Val: searchQualityTestCase{
					query:          "manage account",
					useRegex:       false,
					expectedResult: "Manage Google Accounts on your Chromebook, Help",
					category:       "Best Match",
				},
			},
			{
				Name: "best_match_settings_bluetooth",
				Val: searchQualityTestCase{
					query:          "bluetooth",
					useRegex:       false,
					expectedResult: "Bluetooth, Bluetooth, Settings",
					category:       "Best Match",
				},
			},
			{
				Name: "best_match_settings_display_size",
				Val: searchQualityTestCase{
					query:          "display size",
					useRegex:       false,
					expectedResult: "Display size, Displays, Settings",
					category:       "Best Match",
				},
			},
			{
				Name: "best_match_settings_language",
				Val: searchQualityTestCase{
					query:          "language",
					useRegex:       false,
					expectedResult: "Languages, Languages and inputs, Settings",
					category:       "Best Match",
				},
			},
			{
				Name: "best_match_settings_reverse_scroll",
				Val: searchQualityTestCase{
					query:          "reverse scroll",
					useRegex:       false,
					expectedResult: "Touchpad reverse scrolling, Mouse and touchpad, Settings",
					category:       "Best Match",
				},
			},

			// --- Apps test cases. ---
			{
				Name: "apps_keyboard_shortcut",
				Val: searchQualityTestCase{
					query:          "keyboard shortcut",
					useRegex:       false,
					expectedResult: "Shortcuts",
					category:       "Apps",
				},
			},

			// --- Help app test cases. ---
			{
				Name: "help_new_tab",
				Val: searchQualityTestCase{
					query:          "new tab",
					useRegex:       false,
					expectedResult: "Open the link in a new tab, Shortcuts, Drag the link to a blank area on the tab strip",
					category:       "Help",
				},
			},

			// --- Play store test cases. ---
			// TODO(b/286171481): Unsupported, we need to change fixture to support arc++ app search.
			{
				Name: "play_store_snapchat",
				Val: searchQualityTestCase{
					query:          "snapchat",
					useRegex:       false,
					expectedResult: "Snapchat, Play Store",
					category:       "Play Store",
				},
			},
		},
	})
}

// SearchQuality checks inline answers for special queries.
func SearchQuality(ctx context.Context, s *testing.State) {
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

	testCase := s.Param().(searchQualityTestCase)

	cleanup, err := launcher.SetUpLauncherTest(ctx, tconn, false /*tabletMode*/, false /*stabilizeAppCount*/)
	if err != nil {
		s.Fatal("Failed to set up launcher test case: ", err)
	}
	defer cleanup(cleanupCtx)

	query := testCase.query

	if err := uiauto.Retry(2, uiauto.NamedCombine(query,
		launcher.ClearSearchField(tconn, kb),
		launcher.Search(tconn, kb, query),
		launcher.WaitForResultWithCategory(tconn, launcher.SearchCategoryInfo{
			Category:  testCase.category,
			NeedRegex: testCase.useRegex,
			Result:    testCase.expectedResult,
		}),
	))(ctx); err != nil {
		s.Fatalf("Failed to search %s: %v", query, err)
	}
}
