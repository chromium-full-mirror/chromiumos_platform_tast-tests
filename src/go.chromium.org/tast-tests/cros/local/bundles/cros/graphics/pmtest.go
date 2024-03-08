// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package graphics

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/graphics"
	"go.chromium.org/tast/core/testing"
)

type pmTestParam struct {
	pmMode graphics.PmTestMode
	count  int
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         Pmtest,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that suspend through kernel pm_test works and GPU is alive afterwards",
		BugComponent: "b:995569", // ChromeOS > Platform > Graphics > GPU
		Contacts: []string{
			"chromeos-gfx@google.com",
			"ddmail@google.com",
		},
		Fixture: "gpuWatchDog",
		Timeout: 3 * time.Minute,
		Attr:    []string{"group:graphics", "graphics_stress"},
		Params: []testing.Param{
			// Run the mode from less invasive mode to most invasive mode.
			{
				Name: "00_none",
				Val: pmTestParam{
					pmMode: graphics.PmTestNone,
					count:  2,
				},
				ExtraAttr: []string{"graphics_perbuild"},
			},
			{
				Name: "01_freezers",
				Val: pmTestParam{
					pmMode: graphics.PmTestFreezer,
					count:  2,
				},
				ExtraAttr: []string{"graphics_perbuild"},
			}, {
				Name: "02_devices",
				Val: pmTestParam{
					pmMode: graphics.PmTestDevices,
					count:  2,
				},
				ExtraAttr: []string{"graphics_perbuild"},
			}, {
				Name: "03_platform",
				Val: pmTestParam{
					pmMode: graphics.PmTestPlatform,
					count:  2,
				},
				ExtraAttr: []string{"graphics_perbuild"},
			}, {
				Name: "04_processors",
				Val: pmTestParam{
					pmMode: graphics.PmTestProcessors,
					count:  2,
				},
				ExtraAttr: []string{"graphics_perbuild"},
			}, {
				Name: "05_core",
				Val: pmTestParam{
					pmMode: graphics.PmTestCore,
					count:  2,
				},
				ExtraAttr: []string{"graphics_perbuild"},
			},
			// These tests are bringup version of the tests that are meant to run manually.
			// Run the mode from less invasive mode to most invasive mode.
			{
				Name: "00_none_bringup",
				Val: pmTestParam{
					pmMode: graphics.PmTestNone,
					count:  100,
				},
				ExtraAttr: []string{"graphics_manual", "graphics_bringup"},
			},
			{
				Name: "01_freezers_bringup",
				Val: pmTestParam{
					pmMode: graphics.PmTestFreezer,
					count:  100,
				},
				ExtraAttr: []string{"graphics_manual", "graphics_bringup"},
			}, {
				Name: "02_devices_bringup",
				Val: pmTestParam{
					pmMode: graphics.PmTestDevices,
					count:  100,
				},
				ExtraAttr: []string{"graphics_manual", "graphics_bringup"},
			}, {
				Name: "03_platform_bringup",
				Val: pmTestParam{
					pmMode: graphics.PmTestPlatform,
					count:  100,
				},
				ExtraAttr: []string{"graphics_manual", "graphics_bringup"},
			}, {
				Name: "04_processors_bringup",
				Val: pmTestParam{
					pmMode: graphics.PmTestProcessors,
					count:  100,
				},
				ExtraAttr: []string{"graphics_manual", "graphics_bringup"},
			}, {
				Name: "05_core_bringup",
				Val: pmTestParam{
					pmMode: graphics.PmTestCore,
					count:  100,
				},
				ExtraAttr: []string{"graphics_manual", "graphics_bringup"},
			},
		},
	})
}

func Pmtest(ctx context.Context, s *testing.State) {
	origPmMode, err := graphics.GetPMTestState(ctx)
	if err != nil {
		s.Fatal("Failed to get original pm_test state: ", err)
	}
	s.Logf("Original pm_test state: %q", origPmMode)
	// Always try set the mode back to original.
	defer graphics.SetPMTest(ctx, origPmMode.Current)

	params := s.Param().(pmTestParam)
	mode := params.pmMode

	origSuspendMode := graphics.GetSuspendState(ctx)
	s.Log("Original suspend state: ", origSuspendMode)
	defer graphics.SetSuspendState(ctx, origSuspendMode)

	if err := graphics.SetSuspendState(ctx, graphics.SuspendS3); err != nil {
		s.Fatal("Failed to set to suspend to S3")
	}
	s.Log("Target suspend state: ", graphics.SuspendS3)

	inList := func(str graphics.PmTestMode, list []graphics.PmTestMode) bool {
		for _, l := range list {
			if str == l {
				return true
			}
		}
		return false
	}
	// Check if mode is supported.
	if !inList(mode, origPmMode.Available) {
		s.Logf("pm_test doesn't support requested mode: %v. Skipping the test", mode)
		return
	}
	if origPmMode.Current != mode {
		if err := graphics.SetPMTest(ctx, mode); err != nil {
			s.Fatal("Failed to set pm_test: ", err)
		}
	}
	// We request two consecutive suspend_resumes to ensure each cycle can be repeated.
	// In other words if the resume was unclean we give the test a chance to fail itself (and not some following test).
	out, err := testexec.CommandContext(ctx, "suspend_stress_test", "--count", fmt.Sprintf("%v", params.count), "--nopremature_wake", "--record_dmesg_dir", s.OutDir()).Output(testexec.DumpLogOnError)
	testing.ContextLog(ctx, "suspend_stress_test Output: ", string(out))
	if err != nil {
		s.Fatal("Failed to run suspend_stress_test: ", err)
	}
	if match := regexp.MustCompile(`(?m)^(Suspend failed.*)$`).FindSubmatch(out); len(match) > 0 {
		s.Fatal("Failed running suspend_stress_test: ", string(match[1]))
	}
}
