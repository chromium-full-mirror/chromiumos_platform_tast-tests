// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast-tests/cros/local/ui/deskscuj"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DesksCUJ,
		Desc:         "Measures the performance of critical user journey for virtual desks",
		Contacts:     []string{"cros-sw-perf@google.com", "vincentchiang@chromium.org"},
		BugComponent: "b:1045832",
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Data:         []string{cujrecorder.SystemTraceConfigFile},
		Vars: []string{
			// Parsable test duration, like 10m or 60s, to run the test. This
			// duration is split into 3 sections, where each of the 3 sections
			// of the test gets a third of the total run time. The overall test
			// timeout is still 30 minutes, so command-line test durations must
			// still run in less than that total time. Test time defaults to
			// 10 minutes.
			"ui.DesksCUJ.duration",
		},
		// TODO(b/341597363): Extend the timeout for waiting for the page to load.
		// If the page loading time becomes shorter in the future, the case timeout
		// time can be shortened.
		Timeout: 50 * time.Minute,
		Params: []testing.Param{
			{
				Val:       deskscuj.TestParam{},
				ExtraAttr: []string{"group:cuj", "group:crosbolt", "crosbolt_release_gates"},
				Fixture:   "loggedInToCUJUser",
			}, {
				Name:      "arc_disabled",
				Val:       deskscuj.TestParam{},
				ExtraAttr: []string{"group:cuj"},
				Fixture:   "loggedInToCUJUserDisableARC",
			},

			// Experimental variants.
			{
				Name:      "field_trials",
				ExtraAttr: []string{"group:cuj", "cuj_experimental"},
				Val:       deskscuj.TestParam{},
				Fixture:   "loggedInToCUJUserWithFieldTrials",
			},
			{
				Name:      "battery_saver",
				ExtraAttr: []string{"group:cuj"},
				Val:       deskscuj.TestParam{},
				Fixture:   "loggedInToCUJUserWithBatterySaver",
			},
			// TODO(b/292249282): Remove when Vulkan is launched on brya, volteer, and skyrim
			{
				Name:              "vulkan",
				Val:               deskscuj.TestParam{},
				Fixture:           "loggedInToCUJUserVulkan",
				ExtraAttr:         []string{"group:cuj"},
				ExtraHardwareDeps: hwdep.D(hwdep.Model("redrix", "drobit", "frostflow")),
			},
		},
	})
}

func DesksCUJ(ctx context.Context, s *testing.State) {
	cuj.WriteMetadataFile(ctx, s.TestName())

	// Ensured the DesksCUJ test params are properly formed.
	testParam := s.Param().(deskscuj.TestParam)
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	pv, err := deskscuj.Run(ctx, cr, testParam, s.Var, s.OutDir(), s.DataPath(cujrecorder.SystemTraceConfigFile))
	if err != nil {
		s.Fatal("Failed to run DesksCUJ: ", err)
	}
	if err := pv.Save(s.OutDir()); err != nil {
		s.Error("Failed to save the perf data: ", err)
	}
}
