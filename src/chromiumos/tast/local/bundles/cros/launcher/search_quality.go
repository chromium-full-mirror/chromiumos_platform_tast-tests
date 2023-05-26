// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package launcher

import (
	"context"
	"time"

	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/launcher"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/local/input"

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

// Query's content.
const (
	canberraWeather    = "canberra weather"
	calculator45Plus45 = "45+45"
	screenRotate       = "screen rotate"
	calculator4Digits  = "1234+5678" // https://bugs.chromium.org/p/chromium/issues/detail?id=1432692
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SearchQuality,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test different search query should shown content in the right category",
		Contacts:     []string{"launcher-search-notify@google.com", "xiuwen@google.com"},
		BugComponent: "b:1257106",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{
			{
				Fixture: "chromeLoggedIn",
				Val:     launcher.TestCase{TabletMode: false},
			},
		},
	})
}

type searchQualityCase struct {
	searchQuery string
	steps       uiauto.Action
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

	testCase := s.Param().(launcher.TestCase)

	cleanup, err := launcher.SetUpLauncherTest(ctx, tconn, testCase.TabletMode, false /*stabilizeAppCount*/)
	if err != nil {
		s.Fatal("Failed to set up launcher test case: ", err)
	}
	defer cleanup(cleanupCtx)

	subtests := []searchQualityCase{
		{
			searchQuery: canberraWeather,
			steps: uiauto.Retry(2, uiauto.NamedCombine("Answer Card: Weather",
				launcher.ClearSearchField(tconn, kb),
				launcher.Search(tconn, kb, canberraWeather),
				launcher.WaitForCategorizedResultFromRegex(tconn, weatherPattern),
			)),
		},
		{
			searchQuery: calculator45Plus45,
			steps: uiauto.Retry(2, uiauto.NamedCombine("Answer Card: Calculator",
				launcher.ClearSearchField(tconn, kb),
				launcher.Search(tconn, kb, calculator45Plus45),
				launcher.WaitForResult(tconn, "45+45, 90"),
			)),
		},
		{
			searchQuery: screenRotate,
			steps: uiauto.Retry(2, uiauto.NamedCombine("Answer Card: screen rotate",
				launcher.ClearSearchField(tconn, kb),
				launcher.Search(tconn, kb, screenRotate),
				launcher.WaitForResult(tconn, "Rotate screen 90 degrees, Shortcuts, Ctrl+ Shift+ BrowserRefresh"),
			)),
		},
		{
			searchQuery: calculator4Digits,
			steps: uiauto.Retry(2, uiauto.NamedCombine("Answer Card: 4 digits calculator",
				launcher.ClearSearchField(tconn, kb),
				launcher.Search(tconn, kb, calculator4Digits),
				launcher.WaitForResult(tconn, "1234+5678, 6912"),
			)),
		},
	}

	ui := uiauto.New(tconn)
	clearSearchButton := nodewith.Role(role.Button).Name("Clear searchbox text")

	for _, subtest := range subtests {
		s.Run(ctx, subtest.searchQuery, func(ctx context.Context, s *testing.State) {
			defer ui.DoDefault(clearSearchButton)(cleanupCtx)

			defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree_"+string(subtest.searchQuery))

			if err := subtest.steps(ctx); err != nil {
				s.Log(uiauto.RootDebugInfo(ctx, tconn))

				s.Fatalf("Failed to search %s: %v", subtest.searchQuery, err)
			}
		})
	}
}
