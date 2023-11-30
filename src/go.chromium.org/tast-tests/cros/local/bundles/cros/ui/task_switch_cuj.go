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
			"ramsaroop@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Attr:         []string{"group:cuj"},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		SoftwareDeps: []string{"chrome", "arc"},
		Data:         []string{cujrecorder.SystemTraceConfigFile},
		Timeout:      25 * time.Minute,
		Vars: []string{
			"mute",
		},
		Params: []testing.Param{
			{
				Fixture: "loggedInToCUJUserARCSupported",
				Val: taskswitchcuj.TaskSwitchTest{
					BrowserType: browser.TypeAsh,
				},
			}, {
				Name:              "lacros",
				ExtraSoftwareDeps: []string{"lacros"},
				Fixture:           "loggedInToCUJUserARCSupportedLacros",
				Val: taskswitchcuj.TaskSwitchTest{
					BrowserType: browser.TypeLacros,
				},
			}, {
				Name:              "tablet",
				ExtraHardwareDeps: hwdep.D(hwdep.TouchScreen()),
				Fixture:           "loggedInToCUJUserARCSupported",
				Val: taskswitchcuj.TaskSwitchTest{
					BrowserType: browser.TypeAsh,
					Tablet:      true,
				},
			}, {
				Name:              "lacros_tablet",
				ExtraHardwareDeps: hwdep.D(hwdep.TouchScreen()),
				ExtraSoftwareDeps: []string{"lacros"},
				Fixture:           "loggedInToCUJUserARCSupportedLacros",
				Val: taskswitchcuj.TaskSwitchTest{
					BrowserType: browser.TypeLacros,
					Tablet:      true,
				},
			},

			// Experimental variants.
			{
				Name:      "field_trials",
				ExtraAttr: []string{"cuj_experimental"},
				Val: taskswitchcuj.TaskSwitchTest{
					BrowserType: browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUserARCSupportedWithFieldTrials",
			},
			{
				Name:      "virtio_balloon",
				ExtraAttr: []string{"cuj_experimental"},
				Fixture:   "loggedInToCUJUserARCSupportedWithVirtioBalloon",
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
