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

// Query's content.
const (
	canberraWeather    = "canberra weather"
	calculator45Plus45 = "45+45"
	screenRotate       = "screen rotate"
	calculator4Digits  = "1234+5678" // https://bugs.chromium.org/p/chromium/issues/detail?id=1432692
)

// searchQualityTestCase struct encapsulates parameters for test.
type searchQualityTestCase struct {
	query          string
	useRegex       bool
	expectedResult string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         SearchQuality,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test different search queries should show content in the right category",
		Contacts:     []string{"launcher-search-notify@google.com", "xiuwen@google.com"},
		BugComponent: "b:1257106",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "chromeLoggedIn",

		Params: []testing.Param{
			{
				Name: "weather",
				Val: searchQualityTestCase{
					query:          canberraWeather,
					useRegex:       true,
					expectedResult: weatherPattern,
				},
			},
			{
				Name: "calculator",
				Val: searchQualityTestCase{
					query:          calculator45Plus45,
					useRegex:       false,
					expectedResult: "45+45, 90",
				},
			},
			{
				Name: "screenrotate",
				Val: searchQualityTestCase{
					query:          screenRotate,
					useRegex:       false,
					expectedResult: "Rotate screen 90 degrees, Shortcuts, Ctrl+ Shift+ BrowserRefresh",
				},
			},
			{
				Name: "calculatorlargenumber",
				Val: searchQualityTestCase{
					query:          calculator4Digits,
					useRegex:       false,
					expectedResult: "1234+5678, 6912",
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
	expectedResult := testCase.expectedResult

	if err := uiauto.Retry(2, uiauto.NamedCombine(query,
		launcher.ClearSearchField(tconn, kb),
		launcher.Search(tconn, kb, query)))(ctx); err != nil {
		s.Fatalf("Failed to search %s: %v", query, err)
	}

	if testCase.useRegex {
		if err := launcher.WaitForCategorizedResultFromRegex(tconn, expectedResult)(ctx); err != nil {
			s.Fatalf("Failed to verify the result of search %s: %v", query, err)
		}
	} else {
		if err := launcher.WaitForResult(tconn, expectedResult)(ctx); err != nil {
			s.Fatalf("Failed to verify the result of search %s: %v", query, err)
		}
	}
}
