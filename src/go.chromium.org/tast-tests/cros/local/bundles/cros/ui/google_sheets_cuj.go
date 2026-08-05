// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj/updateengine"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast-tests/cros/local/ui/googlesheetscuj"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: GoogleSheetsCUJ,
		Desc: "Measures the total performance of critical user journey for Google Sheets",
		Contacts: []string{
			"cros-sw-perf@google.com",
			"vincentchiang@chromium.org",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		SoftwareDeps: []string{"chrome"},
		Data:         []string{cujrecorder.SystemTraceConfigFile},
		Vars: []string{
			// Parsable test duration, like 10m or 60s, to run the test.
			"ui.GoogleSheetsCUJ.duration",
		},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Timeout:      15*time.Minute + cujrecorder.CooldownTimeout,
		Params: []testing.Param{
			{
				Val:       googlesheetscuj.TestParam{},
				ExtraAttr: []string{"group:cuj"},
				Fixture:   "loggedInToCUJUserWithoutCooldown",
			},
			// Experimental variants.
			{
				Name: "au_background",
				Val: googlesheetscuj.TestParam{
					AUBackgroundEnable: true,
				},
				ExtraData: []string{
					updateengine.PayloadFilename,
					updateengine.MetadataFilename},
				ExtraAttr:         []string{"group:cuj"},
				ExtraHardwareDeps: hwdep.D(hwdep.Platform("volteer")),
				Fixture:           "loggedInToCUJUserWithoutCooldown",
			},
			{
				Name:      "field_trials",
				Val:       googlesheetscuj.TestParam{},
				ExtraAttr: []string{"group:cuj", "cuj_experimental"},
				Fixture:   "loggedInToCUJUserWithFieldTrialsWithoutCooldown",
			},
			{
				Name:    "battery_saver",
				Val:     googlesheetscuj.TestParam{},
				Fixture: "loggedInToCUJUserWithBatterySaverWithoutCooldown",
			},
			{
				Name: "focusmode",
				Val: googlesheetscuj.TestParam{
					FocusModeEnabled: true,
				},
				Fixture: "loggedInToCUJUserWithFocusMode",
			},
		},
	})
}

// GoogleSheetsCUJ measures the total performance of critical user journey for Google Sheets.
func GoogleSheetsCUJ(ctx context.Context, s *testing.State) {
	cuj.WriteMetadataFile(ctx, s.TestName())

	// Ensured the GoogleSheets test params are properly formed.
	testParam := s.Param().(googlesheetscuj.TestParam)
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	if _, err := googlesheetscuj.Run(ctx, cr, testParam, s.OutDir(), s.DataPath, s.Var); err != nil {
		s.Fatal("Failed to run GoogleSheetsCUJ: ", err)
	}
}
