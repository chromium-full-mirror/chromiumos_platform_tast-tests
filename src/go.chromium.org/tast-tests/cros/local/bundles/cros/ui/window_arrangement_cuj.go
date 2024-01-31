// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast-tests/cros/local/ui/windowarrangementcuj"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         WindowArrangementCUJ,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Measures the performance of critical user journey for window arrangements",
		Contacts: []string{
			"cros-sw-perf@google.com",
			"yichenz@chromium.org",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Attr:         []string{"group:cuj"},
		SoftwareDeps: []string{"chrome", "chrome_internal"},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Timeout:      15 * time.Minute,
		Data:         []string{"shaka_720.webm", "pip.html", cujrecorder.SystemTraceConfigFile},
		Params: []testing.Param{
			{
				Name: "clamshell_mode",
				Val: windowarrangementcuj.TestParam{
					BrowserType: browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUser",
			},
			{
				Name:              "tablet_mode",
				ExtraHardwareDeps: hwdep.D(hwdep.TouchScreen()),
				Val: windowarrangementcuj.TestParam{
					BrowserType: browser.TypeAsh,
					Tablet:      true,
				},
				Fixture: "loggedInToCUJUser",
			},
			{
				Name: "lacros",
				Val: windowarrangementcuj.TestParam{
					BrowserType: browser.TypeLacros,
				},
				Fixture:           "loggedInToCUJUserLacros",
				ExtraSoftwareDeps: []string{"lacros"},
			},
			{
				Name:              "tablet_mode_lacros",
				ExtraHardwareDeps: hwdep.D(hwdep.TouchScreen()),
				Val: windowarrangementcuj.TestParam{
					BrowserType: browser.TypeLacros,
					Tablet:      true,
				},
				Fixture:           "loggedInToCUJUserLacros",
				ExtraSoftwareDeps: []string{"lacros"},
			},
			// TODO(b/292249282): Remove when Vulkan is launched on brya, volteer, and skyrim
			{
				Name: "vulkan",
				Val: windowarrangementcuj.TestParam{
					BrowserType: browser.TypeAsh,
				},
				Fixture:           "loggedInToCUJUserVulkan",
				ExtraHardwareDeps: hwdep.D(hwdep.Model("redrix", "drobit", "frostflow")),
			},
			// Experimental variants.
			// TODO(b/302748186): Remove rounded window tests once A/B testing
			// for rounded windows is done.
			{
				Name: "rounded_windows",
				Val: windowarrangementcuj.TestParam{
					BrowserType: browser.TypeAsh,
				},
				ExtraAttr: []string{"cuj_experimental"},
				Fixture:   "loggedInToCUJUserWithRoundedWindows",
			},
			{
				Name: "rounded_windows_lacros",
				Val: windowarrangementcuj.TestParam{
					BrowserType: browser.TypeLacros,
				},
				ExtraSoftwareDeps: []string{"lacros"},
				ExtraAttr:         []string{"cuj_experimental"},
				Fixture:           "loggedInToCUJUserLacrosWithRoundedWindows",
			},
		},
	})
}

func WindowArrangementCUJ(ctx context.Context, s *testing.State) {
	windowarrangementcuj.Run(ctx, s)
}
