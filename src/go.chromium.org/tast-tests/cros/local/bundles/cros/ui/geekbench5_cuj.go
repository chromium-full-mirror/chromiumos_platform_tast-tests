// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/ui/geekbenchcuj"
	"go.chromium.org/tast-tests/cros/local/crostini"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Geekbench5CUJ,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Run Geekbench 5 CPU benchmark to test device performance",
		Contacts: []string{
			"cros-sw-perf@google.com",
			"vincentchiang@chromium.org",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Data: []string{
			"geekbench5.plar",
			"geekbench5_x86_64",
		},
		SoftwareDeps: []string{"chrome"},
		Vars: []string{
			"keepState",
			"ui.gaiaPoolDefault",
			"geekbench.email",
			"geekbench.key",
		},
		Params: []testing.Param{
			{
				Timeout: 15*time.Minute + cujrecorder.CooldownTimeout,
				Fixture: "loggedInToCUJUserWithoutCooldown",
				Val:     geekbenchcuj.GetGBInfo("native", 5, true /*needLicense*/),
			},
			{
				Name:              "crostini",
				Timeout:           20*time.Minute + cujrecorder.CooldownTimeout,
				ExtraSoftwareDeps: []string{"vm_host", "dlc"},
				ExtraData: []string{
					crostini.GetContainerMetadataArtifact("bullseye", false),
					crostini.GetContainerRootfsArtifact("bullseye", false),
				},
				ExtraHardwareDeps: crostini.CrostiniStable,
				Pre:               crostini.StartedByDlcBullseye(),
				Val:               geekbenchcuj.GetGBInfo("crostini", 5, true /*needLicense*/),
			},
		},
	})
}

func Geekbench5CUJ(ctx context.Context, s *testing.State) {
	geekbenchcuj.Run(ctx, s)
}
