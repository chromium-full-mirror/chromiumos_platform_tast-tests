// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/ui/deskscuj"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"

	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DesksCUJ,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Measures the performance of critical user journey for virtual desks",
		Contacts:     []string{"cros-sw-perf@google.com", "ramsaroop@chromium.org"},
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
			// TODO(b/292249282): Remove when Vulkan is launched on brya and volteer.
			{
				Name:              "vulkan",
				Val:               browser.TypeAsh,
				Fixture:           "loggedInToCUJUserVulkan",
				ExtraHardwareDeps: hwdep.D(hwdep.Model("redrix", "drobit")),
			},
		},
	})
}

func DesksCUJ(ctx context.Context, s *testing.State) {
	deskscuj.Run(ctx, s)
}
