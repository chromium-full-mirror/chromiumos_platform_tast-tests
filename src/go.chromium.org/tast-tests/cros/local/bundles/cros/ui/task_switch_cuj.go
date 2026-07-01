// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/ui/taskswitchcuj"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"

	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: TaskSwitchCUJ,
		Desc: "Measures the performance of the critical user journey for task switching",
		Contacts: []string{
			"cros-sw-perf@google.com",
			"vincentchiang@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		SoftwareDeps: []string{"chrome", "arc"},
		Data:         []string{cujrecorder.SystemTraceConfigFile},
		Timeout:      taskswitchcuj.TestTimeout,
		Vars: []string{
			"mute",
		},
		Params: []testing.Param{
			{
				ExtraAttr: []string{"group:cuj"},
				Fixture:   "loggedInToCUJUserARCSupported",
				Val:       false, /*tablet*/
			}, {
				Name:              "tablet",
				ExtraAttr:         []string{"group:cuj"},
				ExtraHardwareDeps: hwdep.D(hwdep.TouchScreen()),
				Fixture:           "loggedInToCUJUserARCSupported",
				Val:               true, /*tablet*/
			},

			// Experimental variants.
			{
				Name:      "field_trials",
				ExtraAttr: []string{"group:cuj", "cuj_experimental"},
				Val:       false, /*tablet*/
				Fixture:   "loggedInToCUJUserARCSupportedWithFieldTrials",
			},
		},
	})
}

func TaskSwitchCUJ(ctx context.Context, s *testing.State) {
	cuj.WriteMetadataFile(ctx, s.TestName())

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	a := s.FixtValue().(cuj.FixtureData).ARC
	isTablet := s.Param().(bool)
	perfettoCfgPath := s.DataPath(cujrecorder.SystemTraceConfigFile)

	if err := taskswitchcuj.Run(ctx, cr, a, isTablet, s.OutDir(), perfettoCfgPath); err != nil {
		s.Fatal("Failed to run TaskSwitchCUJ: ", err)
	}
}
