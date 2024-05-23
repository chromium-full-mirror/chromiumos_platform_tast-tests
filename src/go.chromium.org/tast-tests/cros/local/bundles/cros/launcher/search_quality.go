// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package launcher

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/launcher/fixture"
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
const rotateScreenPattern = `^Rotate screen.*Shortcuts.*`
const capsLockPattern = "^Turn on/off Caps Lock.*Shortcuts.*"

// searchQualityTestCase struct encapsulates parameters for test.
type searchQualityTestCase struct {
	query          string
	useRegex       bool
	expectedResult string
	category       string
	provider       string
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
		Timeout:      10 * time.Minute,

		Params: []testing.Param{
			// --- Answer card test cases. ---
			{
				Name:    "answer_card_calculator",
				Fixture: fixture.NormalLauncherSearch,
				Val: searchQualityTestCase{
					query:          "45+45",
					useRegex:       false,
					expectedResult: "= 90",
					category:       "Answer Card",
					provider:       "omnibox",
				},
				ExtraAttr: []string{"group:hw_agnostic"},
			},
			// See details in: https://bugs.chromium.org/p/chromium/issues/detail?id=1432692.
			{
				Name:    "answer_card_calculator_large_number",
				Fixture: fixture.NormalLauncherSearch,
				Val: searchQualityTestCase{
					query:          "1234+5678",
					useRegex:       false,
					expectedResult: "= 6912",
					category:       "Answer Card",
					provider:       "omnibox",
				},
				ExtraAttr: []string{"group:hw_agnostic"},
			},
			{
				Name:    "answer_card_caps_lock",
				Fixture: fixture.NormalLauncherSearch,
				Val: searchQualityTestCase{
					query:          "caps lock",
					useRegex:       true,
					expectedResult: capsLockPattern,
					category:       "Answer Card",
				},
				ExtraAttr: []string{"group:hw_agnostic"},
			},
			{
				Name:    "answer_card_screen_rotate",
				Fixture: fixture.NormalLauncherSearch,
				Val: searchQualityTestCase{
					query:          "screen rotate",
					useRegex:       true,
					expectedResult: rotateScreenPattern,
					category:       "Answer Card",
				},
				ExtraAttr: []string{"group:hw_agnostic"},
			},
			{
				Name:    "answer_card_weather",
				Fixture: fixture.NormalLauncherSearch,
				Val: searchQualityTestCase{
					query:          "canberra weather",
					useRegex:       true,
					expectedResult: weatherPattern,
					category:       "Answer Card",
					provider:       "omnibox",
				},
				ExtraAttr: []string{"group:hw_agnostic"},
			},

			// --- Best match test cases. ---
			{
				Name:    "best_match_app_chrome",
				Fixture: fixture.NormalLauncherSearch,
				Val: searchQualityTestCase{
					query:          "chrome",
					useRegex:       false,
					expectedResult: "Chrome, Installed App",
					category:       "Best Match",
				},
				ExtraAttr: []string{"group:hw_agnostic"},
			},
			{
				Name:    "best_match_apps_files",
				Fixture: fixture.NormalLauncherSearch,
				Val: searchQualityTestCase{
					query:          "files",
					useRegex:       false,
					expectedResult: "Files, Installed App",
					category:       "Best Match",
				},
				ExtraAttr: []string{"group:hw_agnostic"},
			},
			{
				Name:    "best_match_apps_settings",
				Fixture: fixture.NormalLauncherSearch,
				Val: searchQualityTestCase{
					query:          "settings",
					useRegex:       false,
					expectedResult: "Settings, Installed App",
					category:       "Best Match",
				},
				ExtraAttr: []string{"group:hw_agnostic"},
			},
			{
				Name:    "best_match_apps_chrome",
				Fixture: fixture.NormalLauncherSearch,
				Val: searchQualityTestCase{
					query:          "chrome",
					useRegex:       false,
					expectedResult: "Chrome, Installed App",
					category:       "Best Match",
				},
			},
			{
				Name:    "best_match_files_downloads",
				Fixture: fixture.NormalLauncherSearch,
				Val: searchQualityTestCase{
					query:          "downloads",
					useRegex:       false,
					expectedResult: "Downloads, MyFiles",
					category:       "Best Match",
				},
				ExtraAttr: []string{"group:hw_agnostic"},
			},
			{
				Name:    "best_match_help_manage_account",
				Fixture: fixture.NormalLauncherSearchWithOOBE,
				Val: searchQualityTestCase{
					query:          "manage account",
					useRegex:       false,
					expectedResult: "Manage Google Accounts on your Chromebook, Help",
					category:       "Help",
				},
				ExtraAttr: []string{"group:hw_agnostic"},
			},
			{
				Name:    "best_match_personalization_screen_saver",
				Fixture: fixture.NormalLauncherSearch,
				Val: searchQualityTestCase{
					query:          "screen saver",
					useRegex:       false,
					expectedResult: "Screen saver",
					category:       "Best Match",
				},
				ExtraAttr: []string{"group:hw_agnostic"},
			},
			{
				Name:    "best_match_settings_bluetooth",
				Fixture: fixture.NormalLauncherSearch,
				Val: searchQualityTestCase{
					query:          "bluetooth",
					useRegex:       false,
					expectedResult: "Bluetooth, Bluetooth, Settings",
					category:       "Best Match",
				},
				ExtraAttr: []string{"group:hw_agnostic"},
			},
			{
				Name:    "best_match_settings_display_size",
				Fixture: fixture.NormalLauncherSearch,
				Val: searchQualityTestCase{
					query:          "display size",
					useRegex:       false,
					expectedResult: "Display size, Displays, Settings",
					category:       "Best Match",
				},
				ExtraAttr: []string{"group:hw_agnostic"},
			},
			{
				Name:    "best_match_settings_language",
				Fixture: fixture.NormalLauncherSearch,
				Val: searchQualityTestCase{
					query:          "language",
					useRegex:       false,
					expectedResult: "Languages, System preferences",
					category:       "Best Match",
				},
				ExtraAttr: []string{"group:hw_agnostic"},
			},
			{
				Name:    "best_match_settings_reverse_scroll",
				Fixture: fixture.NormalLauncherSearch,
				Val: searchQualityTestCase{
					query:          "reverse scroll",
					useRegex:       false,
					expectedResult: "Touchpad reverse scrolling, Mouse and touchpad, Settings",
					category:       "Best Match",
				},
				ExtraAttr: []string{"group:hw_agnostic"},
			},

			// --- Apps test cases. ---
			{
				Name:    "apps_keyboard_shortcut",
				Fixture: fixture.NormalLauncherSearch,
				Val: searchQualityTestCase{
					query:          "keyboard shortcut",
					useRegex:       false,
					expectedResult: "Shortcuts",
					category:       "Apps",
				},
				ExtraAttr: []string{"group:hw_agnostic"},
			},

			// --- Help app test cases. ---
			{
				Name:    "help_change_channel",
				Fixture: fixture.NormalLauncherSearchWithOOBE,
				Val: searchQualityTestCase{
					query:          "change channel",
					useRegex:       false,
					expectedResult: "Switch between stable, beta and dev software",
					category:       "Help",
				},
				ExtraAttr: []string{"group:hw_agnostic"},
			},

			// --- Play store test cases. ---
			{
				Name:    "play_store_snapchat",
				Fixture: fixture.NormalLauncherSearchWithArc,
				Val: searchQualityTestCase{
					query:          "snapchat",
					useRegex:       false,
					expectedResult: "Snapchat, Play Store",
					category:       "Play Store",
				},
				ExtraAttr: []string{"group:hw_agnostic"},
			},
		},
	})
}

// SearchQuality checks inline answers for special queries.
func SearchQuality(ctx context.Context, s *testing.State) {
	tconn := s.FixtValue().(fixture.LauncherSearchFixtData).TestAPIConn
	kb := s.FixtValue().(fixture.LauncherSearchFixtData).Keyboard
	cr := s.FixtValue().(fixture.LauncherSearchFixtData).Chrome

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

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
		// currently, we are aware of failed test case from omnibox.
		// we will add more check steps if needed.
		if testCase.provider == "omnibox" {
			checkOmnibox(ctx, s, cr, kb, query)
		}

		s.Fatalf("Failed to search %s: %v", query, err)
	}
}

// checkOmnibox check omnibox when the query search failed.
func checkOmnibox(ctx context.Context, s *testing.State, cr *chrome.Chrome, kb *input.KeyboardEventWriter, query string) {
	if _, err := cr.NewConn(ctx, "chrome://omnibox"); err != nil {
		s.Log("Failed to open omnibox: ", err)
	}

	if err := kb.Type(ctx, query); err != nil {
		s.Log("Failed to search in omnibox: ", err)
	}
}
