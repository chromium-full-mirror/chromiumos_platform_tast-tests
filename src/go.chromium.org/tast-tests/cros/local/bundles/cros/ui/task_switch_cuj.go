// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/ui/taskswitchcuj"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"

	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         TaskSwitchCUJ,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Measures the performance of the critical user journey for task switching",
		Contacts: []string{
			"cros-sw-perf@google.com",
			"ramsaroop@chromium.org",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Attr:         []string{"group:cuj"},
		SoftwareDeps: []string{"chrome", "arc"},
		Data:         []string{cujrecorder.SystemTraceConfigFile},
		Timeout:      25 * time.Minute,
		Vars: []string{
			"mute",
		},
		Params: []testing.Param{
			{
				ExtraHardwareDeps: hwdep.D(hwdep.InternalDisplay()),
				Fixture:           "loggedInToCUJUser",
				Val: taskswitchcuj.TaskSwitchTest{
					BrowserType: browser.TypeAsh,
				},
			}, {
				Name:              "lacros",
				ExtraHardwareDeps: hwdep.D(hwdep.InternalDisplay()),
				ExtraSoftwareDeps: []string{"lacros"},
				Fixture:           "loggedInToCUJUserLacros",
				Val: taskswitchcuj.TaskSwitchTest{
					BrowserType: browser.TypeLacros,
				},
			}, {
				Name:              "tablet",
				ExtraHardwareDeps: hwdep.D(hwdep.InternalDisplay()),
				Fixture:           "loggedInToCUJUser",
				Val: taskswitchcuj.TaskSwitchTest{
					BrowserType: browser.TypeAsh,
					Tablet:      true,
				},
			}, {
				Name:              "lacros_tablet",
				ExtraHardwareDeps: hwdep.D(hwdep.InternalDisplay()),
				ExtraSoftwareDeps: []string{"lacros"},
				Fixture:           "loggedInToCUJUserLacros",
				Val: taskswitchcuj.TaskSwitchTest{
					BrowserType: browser.TypeLacros,
					Tablet:      true,
				},
			},

			// Experimental variants.
			{
				Name:      "backup_ref_ptr",
				ExtraAttr: []string{"cuj_experimental"},
				Val: taskswitchcuj.TaskSwitchTest{
					BrowserType: browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUserWithBackupRefPtr",
			},
			{
				Name:      "field_trials",
				ExtraAttr: []string{"cuj_experimental"},
				Val: taskswitchcuj.TaskSwitchTest{
					BrowserType: browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUserWithFieldTrials",
			},
			{
				Name:      "app_rescue",
				ExtraAttr: []string{"cuj_experimental"},
				Val: taskswitchcuj.TaskSwitchTest{
					BrowserType: browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUserWithAppRescue",
			},
			{
				Name:              "battery_saver",
				ExtraAttr:         []string{"cuj_experimental"},
				ExtraHardwareDeps: hwdep.D(hwdep.InternalDisplay()),
				Fixture:           "loggedInToCUJUserWithBatterySaver",
				Val: taskswitchcuj.TaskSwitchTest{
					BrowserType: browser.TypeAsh,
				},
			},
		},
	})
}

func TaskSwitchCUJ(ctx context.Context, s *testing.State) {
	taskswitchcuj.Run(ctx, s)
}
