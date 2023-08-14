// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/ui/tabswitchcuj"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/cpu"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         TabSwitchCUJ,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Measures the performance of tab-switching CUJ",
		Contacts: []string{
			"cros-sw-perf@google.com",
			"yichenz@chromium.org",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Attr:         []string{"group:cuj"},
		SoftwareDeps: []string{"chrome"},
		Data:         []string{cujrecorder.SystemTraceConfigFile},
		Timeout:      22*time.Minute + cuj.CPUStablizationTimeout,
		Vars: []string{
			"mute",
		},
		Params: []testing.Param{
			{
				ExtraData: []string{tabswitchcuj.WPRArchiveName},
				Val: tabswitchcuj.TabSwitchParam{
					BrowserType: browser.TypeAsh,
				},
				Fixture: "tabSwitchCUJWPRAsh",
			}, {
				Name: "lacros",
				Val: tabswitchcuj.TabSwitchParam{
					BrowserType: browser.TypeLacros,
				},
				Fixture:           "tabSwitchCUJWPRLacros",
				ExtraSoftwareDeps: []string{"lacros"},
			},

			// Experimental variants.
			{
				Name:      "field_trials",
				ExtraAttr: []string{"cuj_experimental"},
				Val: tabswitchcuj.TabSwitchParam{
					BrowserType: browser.TypeAsh,
				},
				Fixture: "tabSwitchCUJWPRAshWithFieldTrials",
			},
		},
	})
}

func TabSwitchCUJ(ctx context.Context, s *testing.State) {
	// Ensure display on to record ui performance correctly.
	if err := power.TurnOnDisplay(ctx); err != nil {
		s.Fatal("Failed to turn on display: ", err)
	}

	// Wait for cpu to stabilize before test.
	if _, err := cpu.WaitUntilStabilized(ctx, cujrecorder.CPUCoolDownConfig()); err != nil {
		// Log the cpu stabilizing wait failure instead of make it fatal.
		// TODO(b/213238698): Include the error as part of test data.
		s.Log("Failed to wait for CPU to become idle: ", err)
	}

	tabswitchcuj.Run(ctx, s)
}
