// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/ui/videocuj"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"

	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         VideoCUJ,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Measures the performance of a critical user journey of watching a video",
		Contacts: []string{
			"cros-sw-perf@google.com",
			"ramsaroop@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Attr:         []string{"group:cuj"},
		SoftwareDeps: []string{"chrome"},
		Data:         []string{cujrecorder.SystemTraceConfigFile},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Timeout:      15 * time.Minute,
		Params: []testing.Param{
			{
				Val:     browser.TypeAsh,
				Fixture: "loggedInToCUJUser",
			}, {
				Name:              "lacros",
				Val:               browser.TypeLacros,
				Fixture:           "loggedInToCUJUserLacros",
				ExtraSoftwareDeps: []string{"lacros"},
			},
			// Experimental variants.
			{
				Name:      "field_trials",
				ExtraAttr: []string{"cuj_experimental"},
				Val:       browser.TypeAsh,
				Fixture:   "loggedInToCUJUserWithFieldTrials",
			},
			{
				Name:      "battery_saver",
				ExtraAttr: []string{"cuj_experimental"},
				Val:       browser.TypeAsh,
				Fixture:   "loggedInToCUJUserWithBatterySaver",
			},
			// TODO(b/298151007): Remove when sufficient data is collected.
			{
				Name:              "hrtimer_off",
				ExtraAttr:         []string{"cuj_experimental"},
				Val:               browser.TypeAsh,
				Fixture:           "loggedInToCUJUserWithHighResTimerOff",
				ExtraHardwareDeps: hwdep.D(hwdep.HasDynamicHighResTimerControl()),
			},
			// TODO(b/292249282): Remove when Vulkan is launched on brya, volteer, and skyrim
			{
				Name:              "vulkan",
				Val:               browser.TypeAsh,
				Fixture:           "loggedInToCUJUserVulkan",
				ExtraHardwareDeps: hwdep.D(hwdep.Model("redrix", "drobit", "frostflow")),
			},
		},
	})
}

func VideoCUJ(ctx context.Context, s *testing.State) {
	videocuj.Run(ctx, s)
}
