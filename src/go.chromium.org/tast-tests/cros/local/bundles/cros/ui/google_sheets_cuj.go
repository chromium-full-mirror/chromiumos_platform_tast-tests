// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast-tests/cros/local/ui/googlesheetscuj"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         GoogleSheetsCUJ,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Measures the total performance of critical user journey for Google Sheets",
		Contacts: []string{
			"cros-sw-perf@google.com",
			"yichenz@chromium.org",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Attr:         []string{"group:cuj"},
		SoftwareDeps: []string{"chrome"},
		Data:         []string{cujrecorder.SystemTraceConfigFile},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Timeout:      15 * time.Minute,
		Params: []testing.Param{
			{
				Val: googlesheetscuj.TestParam{
					BrowserType: browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUser",
			},
			{
				Name: "lacros",
				Val: googlesheetscuj.TestParam{
					BrowserType: browser.TypeLacros,
				},
				Fixture:           "loggedInToCUJUserLacros",
				ExtraSoftwareDeps: []string{"lacros"},
			},

			// Experimental variants.
			{
				Name:      "field_trials",
				ExtraAttr: []string{"cuj_experimental"},
				Val: googlesheetscuj.TestParam{
					BrowserType: browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUserWithFieldTrials",
			},
			{
				Name:      "battery_saver",
				ExtraAttr: []string{"cuj_experimental"},
				Val: googlesheetscuj.TestParam{
					BrowserType: browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUserWithBatterySaver",
			},
		},
	})
}

// GoogleSheetsCUJ measures the total performance of critical user journey for Google Sheets.
func GoogleSheetsCUJ(ctx context.Context, s *testing.State) {
	googlesheetscuj.Run(ctx, s)
}
