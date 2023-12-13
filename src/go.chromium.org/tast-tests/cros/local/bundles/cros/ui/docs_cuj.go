// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/ui/docscuj"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"

	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DocsCUJ,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Measures the total performance of the critical user journey for Google Docs",
		Contacts: []string{
			"cros-sw-perf@google.com",
			"ramsaroop@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Attr:         []string{"group:cuj"},
		SoftwareDeps: []string{"chrome"},
		Data:         []string{cujrecorder.SystemTraceConfigFile},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Timeout:      20 * time.Minute,
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
				Name:      "chromevox",
				ExtraAttr: []string{"cuj_experimental"},
				Val:       browser.TypeAsh,
				Fixture:   "loggedInToCUJUserWithChromeVox",
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

func DocsCUJ(ctx context.Context, s *testing.State) {
	docscuj.Run(ctx, s)
}
