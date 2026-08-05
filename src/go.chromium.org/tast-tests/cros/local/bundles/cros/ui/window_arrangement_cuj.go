// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast-tests/cros/local/ui/windowarrangementcuj"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		// TODO(b/310387643) re-structure WindowArrangementCUJ to be simpler,
		// in an attempt to fail on less niche bugs. Once this test has been
		// improved, re-add it to the CUJ suite. Currently, the test is not
		// run in the lab due to hard to resolve failures.
		Func: WindowArrangementCUJ,
		Desc: "Measures the performance of critical user journey for window arrangements",
		Contacts: []string{
			"cros-sw-perf@google.com",
			"vincentchiang@chromium.org",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		SoftwareDeps: []string{"chrome", "chrome_internal"},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Timeout:      15 * time.Minute,
		Data:         []string{"shaka_720.webm", "pip.html", cujrecorder.SystemTraceConfigFile},
		Params: []testing.Param{
			{
				Name:    "clamshell_mode",
				Val:     windowarrangementcuj.TestParam{},
				Fixture: "loggedInToCUJUser",
			},
			{
				Name:              "tablet_mode",
				ExtraHardwareDeps: hwdep.D(hwdep.TouchScreen()),
				Val: windowarrangementcuj.TestParam{
					Tablet: true,
				},
				Fixture: "loggedInToCUJUser",
			},
			// TODO(b/292249282): Remove when Vulkan is launched on brya, volteer, and skyrim
			{
				Name:              "vulkan",
				Val:               windowarrangementcuj.TestParam{},
				Fixture:           "loggedInToCUJUserVulkan",
				ExtraHardwareDeps: hwdep.D(hwdep.Model("redrix", "drobit", "frostflow")),
			},
		},
	})
}

func WindowArrangementCUJ(ctx context.Context, s *testing.State) {
	windowarrangementcuj.Run(ctx, s)
}
