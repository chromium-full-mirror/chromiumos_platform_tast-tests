// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package launcher

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/launcher/fixture"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const cpuPattern = `^Showing CPU information. CPU usage snapshot, (\d+)\%. Temperature (\d+)\s+degrees celsius, current speed: ([\d.]+)GHz.`
const memoryPattern = `^Showing memory information\. Memory\s+([\d.]+)\s+GB available out of\s+([\d.]+)\s+GB total`
const batteryPattern = `^Showing battery information\. Current battery level\s+(\d+)%.\s+`
const storagePattern = `^Showing storage space information\. Storage\s+([\d.]+)\s+GB in use out of\s+([\d.]+)\s+GB total\.(?:.*)$`
const versionPattern = `^Showing OS version information\. Current version\s+(\d+\.\d+\.\d+\.\d+)\s+.*`

// systemAnswerCardTestCase struct encapsulates parameters for test.
type systemAnswerCardTestCase struct {
	query          string
	useRegex       bool
	expectedResult string
	category       string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         SearchSystemInfo,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test system answer card",
		Contacts:     []string{"launcher-search-notify@google.com", "xiuwen@google.com"},
		BugComponent: "b:1257106",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      4 * time.Minute,
		Fixture:      fixture.NormalLauncherSearch,
		Params: []testing.Param{
			{
				Name: "cpu_answer_card",
				Val: systemAnswerCardTestCase{
					query:          "cpu",
					useRegex:       true,
					expectedResult: cpuPattern,
					category:       "Answer Card",
				},
			},
			{
				Name: "memory_answer_card",
				Val: systemAnswerCardTestCase{
					query:          "memory",
					useRegex:       true,
					expectedResult: memoryPattern,
					category:       "Answer Card",
				},
			},
			{
				Name: "battery_answer_card",
				Val: systemAnswerCardTestCase{
					query:          "battery",
					useRegex:       true,
					expectedResult: batteryPattern,
					category:       "Answer Card",
				},
				ExtraHardwareDeps: hwdep.D(hwdep.Battery()),
			},
			{
				Name: "storage_answer_card",
				Val: systemAnswerCardTestCase{
					query:          "storage",
					useRegex:       true,
					expectedResult: storagePattern,
					category:       "Answer Card",
				},
			},
			{
				Name: "version_answer_card",
				Val: systemAnswerCardTestCase{
					query:          "version",
					useRegex:       true,
					expectedResult: versionPattern,
					category:       "Answer Card",
				},
			},
		},
	})
}

// SearchSystemInfo checks the query about system info works properly, related flag: launcher-system-info-answer-cards
func SearchSystemInfo(ctx context.Context, s *testing.State) {
	tconn := s.FixtValue().(fixture.LauncherSearchFixtData).TestAPIConn
	kb := s.FixtValue().(fixture.LauncherSearchFixtData).Keyboard

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	testCase := s.Param().(systemAnswerCardTestCase)

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
