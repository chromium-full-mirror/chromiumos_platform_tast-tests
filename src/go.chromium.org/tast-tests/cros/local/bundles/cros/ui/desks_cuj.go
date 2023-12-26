// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"time"

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
		Attr:         []string{"group:cuj"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Data:         []string{cujrecorder.SystemTraceConfigFile},
		Timeout:      30 * time.Minute,
		Params: []testing.Param{
			{
				Val:     browser.TypeAsh,
				Fixture: "loggedInToCUJUser",
			}, {
				Name:              "lacros",
				Val:               browser.TypeLacros,
				ExtraSoftwareDeps: []string{"lacros"},
				Fixture:           "loggedInToCUJUserLacros",
			},
			{
				Name:              "lacros_virtio_balloon",
				Val:               browser.TypeLacros,
				ExtraSoftwareDeps: []string{"lacros"},
				Fixture:           "loggedInToCUJUserLacrosWithVirtioBalloon",
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
			// TODO(b/302748186): Remove rounded window tests once A/B testing
			// for rounded windows is done.
			{
				Name:      "rounded_windows",
				Val:       browser.TypeAsh,
				ExtraAttr: []string{"cuj_experimental"},
				Fixture:   "loggedInToCUJUserWithRoundedWindows",
			},
			{
				Name:              "rounded_windows_lacros",
				Val:               browser.TypeLacros,
				ExtraSoftwareDeps: []string{"lacros"},
				ExtraAttr:         []string{"cuj_experimental"},
				Fixture:           "loggedInToCUJUserLacrosWithRoundedWindows",
			},
			// TODO(b/292249282): Remove when Vulkan is launched on brya, volteer, and skyrim
			{
				Name:              "vulkan",
				Val:               browser.TypeAsh,
				Fixture:           "loggedInToCUJUserVulkan",
				ExtraHardwareDeps: hwdep.D(hwdep.Model("redrix", "drobit", "frostflow")),
			},
			{
				Name:      "partial_low_end_mode",
				ExtraAttr: []string{"cuj_experimental"},
				Val:       browser.TypeAsh,
				Fixture:   "loggedInToCUJUserWithPartialLowEndModeOnMidRangeDevices",
			},
			{
				Name:              "blt_1gb",
				Val:               browser.TypeAsh,
				ExtraAttr:         []string{"cuj_experimental"},
				ExtraHardwareDeps: hwdep.D(cuj.Experimental8GBModelConditions()...),
				Fixture:           "loggedInToCUJUserWithBackgroundLoad1GB",
			},
			{
				Name:              "blt_2gb",
				Val:               browser.TypeAsh,
				ExtraAttr:         []string{"cuj_experimental"},
				ExtraHardwareDeps: hwdep.D(cuj.Experimental8GBModelConditions()...),
				Fixture:           "loggedInToCUJUserWithBackgroundLoad2GB",
			},
			{
				Name:              "blt_3gb",
				Val:               browser.TypeAsh,
				ExtraAttr:         []string{"cuj_experimental"},
				ExtraHardwareDeps: hwdep.D(cuj.Experimental8GBModelConditions()...),
				Fixture:           "loggedInToCUJUserWithBackgroundLoad3GB",
			},
			{
				Name:              "blt_4gb",
				Val:               browser.TypeAsh,
				ExtraAttr:         []string{"cuj_experimental"},
				ExtraHardwareDeps: hwdep.D(cuj.Experimental8GBModelConditions()...),
				Fixture:           "loggedInToCUJUserWithBackgroundLoad4GB",
			},
		},
	})
}

func DesksCUJ(ctx context.Context, s *testing.State) {
	deskscuj.Run(ctx, s, cujrecorder.SystemTraceConfigFile)
}
