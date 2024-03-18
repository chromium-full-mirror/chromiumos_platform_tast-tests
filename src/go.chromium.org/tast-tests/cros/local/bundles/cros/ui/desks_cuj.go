// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast-tests/cros/local/ui/deskscuj"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DesksCUJ,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Measures the performance of critical user journey for virtual desks",
		Contacts:     []string{"cros-sw-perf@google.com", "ramsaroop@google.com"},
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
		Timeout: 30 * time.Minute,
		Params: []testing.Param{
			{
				Val: deskscuj.TestParam{
					BrowserType: browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUser",
			}, {
				Name:         "pvsched",
				BugComponent: "b:167279",
				Val: deskscuj.TestParam{
					BrowserType: browser.TypeAsh,
				},
				ExtraAttr:         []string{"group:cuj", "cuj_experimental"},
				ExtraHardwareDeps: hwdep.D(hwdep.HasParavirtSchedControl()),
				Fixture:           "loggedInToCUJUserWithPvSchedEnabled",
			}, {
				Name: "lacros",
				Val: deskscuj.TestParam{
					BrowserType: browser.TypeLacros,
				},
				ExtraAttr:         []string{"group:cuj"},
				ExtraSoftwareDeps: []string{"lacros"},
				Fixture:           "loggedInToCUJUserLacros",
			},

			// Experimental variants.
			{
				Name:      "field_trials",
				ExtraAttr: []string{"group:cuj", "cuj_experimental"},
				Val: deskscuj.TestParam{
					BrowserType: browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUserWithFieldTrials",
			},
			{
				Name:      "battery_saver",
				ExtraAttr: []string{"group:cuj"},
				Val: deskscuj.TestParam{
					BrowserType: browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUserWithBatterySaver",
			},
			// TODO(b/302748186): Remove rounded window tests once A/B testing
			// for rounded windows is done.
			{
				Name: "rounded_windows",
				Val: deskscuj.TestParam{
					BrowserType: browser.TypeAsh,
				},
				ExtraAttr: []string{"group:cuj", "cuj_experimental"},
				Fixture:   "loggedInToCUJUserWithRoundedWindows",
			},
			{
				Name: "rounded_windows_lacros",
				Val: deskscuj.TestParam{
					BrowserType: browser.TypeLacros,
				},
				ExtraSoftwareDeps: []string{"lacros"},
				ExtraAttr:         []string{"group:cuj", "cuj_experimental"},
				Fixture:           "loggedInToCUJUserLacrosWithRoundedWindows",
			},
			// TODO(b/292249282): Remove when Vulkan is launched on brya, volteer, and skyrim
			{
				Name: "vulkan",
				Val: deskscuj.TestParam{
					BrowserType: browser.TypeAsh,
				},
				Fixture:           "loggedInToCUJUserVulkan",
				ExtraAttr:         []string{"group:cuj"},
				ExtraHardwareDeps: hwdep.D(hwdep.Model("redrix", "drobit", "frostflow")),
			},
			{
				Name: "blt_50mb",
				Val: deskscuj.TestParam{
					BrowserType: browser.TypeAsh,
				},
				ExtraHardwareDeps: hwdep.D(cuj.Experimental8GBModelConditions()...),
				Fixture:           "loggedInToCUJUserWithBackgroundLoad50MB",
			},
			{
				Name: "blt_1gb",
				Val: deskscuj.TestParam{
					BrowserType: browser.TypeAsh,
				},
				ExtraHardwareDeps: hwdep.D(cuj.Experimental8GBModelConditions()...),
				Fixture:           "loggedInToCUJUserWithBackgroundLoad1GB",
			},
			{
				Name: "blt_2gb",
				Val: deskscuj.TestParam{
					BrowserType: browser.TypeAsh,
				},
				ExtraHardwareDeps: hwdep.D(cuj.Experimental8GBModelConditions()...),
				Fixture:           "loggedInToCUJUserWithBackgroundLoad2GB",
			},
			{
				Name: "blt_3gb",
				Val: deskscuj.TestParam{
					BrowserType: browser.TypeAsh,
				},
				ExtraHardwareDeps: hwdep.D(cuj.Experimental8GBModelConditions()...),
				Fixture:           "loggedInToCUJUserWithBackgroundLoad3GB",
			},
			{
				Name: "blt_4gb",
				Val: deskscuj.TestParam{
					BrowserType: browser.TypeAsh,
				},
				ExtraHardwareDeps: hwdep.D(cuj.Experimental8GBModelConditions()...),
				Fixture:           "loggedInToCUJUserWithBackgroundLoad4GB",
			},
		},
	})
}

func DesksCUJ(ctx context.Context, s *testing.State) {
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
