// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package graphics

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/graphics"
	"go.chromium.org/tast/core/testing"
)

type pmTestParam struct {
	pmMode graphics.PmTestMode
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         Pmtest,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that suspend through kernel pm_test works and GPU is alive afterwards",
		Attr:         []string{"group:graphics", "graphics_weekly"},
		BugComponent: "b:995569", // ChromeOS > Platform > Graphics > GPU
		Contacts: []string{
			"chromeos-gfx@google.com",
			"ddmail@google.com",
		},
		Fixture: "gpuWatchDog",
		Timeout: 3 * time.Minute,
		Params: []testing.Param{
			// Run the mode from less invasive mode to most invasive mode.
			{
				Name: "00_none",
				Val: pmTestParam{
					pmMode: graphics.PmTestNone,
				},
			},
			{
				Name: "01_freezers",
				Val: pmTestParam{
					pmMode: graphics.PmTestFreezer,
				},
			}, {
				Name: "02_devices",
				Val: pmTestParam{
					pmMode: graphics.PmTestDevices,
				},
			}, {
				Name: "03_platform",
				Val: pmTestParam{
					pmMode: graphics.PmTestPlatform,
				},
			}, {
				Name: "04_processors",
				Val: pmTestParam{
					pmMode: graphics.PmTestProcessors,
				},
			}, {
				Name: "05_core",
				Val: pmTestParam{
					pmMode: graphics.PmTestCore,
				},
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

	suspendMode := graphics.GetSuspendState(ctx)
	s.Log("Target suspend state: ", suspendMode)
	inList := func(str graphics.PmTestMode, list []graphics.PmTestMode) bool {
		for _, l := range list {
			if str == l {
				return true
			}
		}
		return false
	}
	// pm_test processors and core doesn't support S2idle(S0ix).
	if inList(mode, []graphics.PmTestMode{graphics.PmTestProcessors, graphics.PmTestCore}) && suspendMode == graphics.SuspendS0ix {
		s.Logf("pm_test %q doesn't support %q. Skipping the test", mode, suspendMode)
		return
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
	out, err := testexec.CommandContext(ctx, "suspend_stress_test", "--count", "2", "--nopremature_wake", "--record_dmesg_dir", s.OutDir()).Output(testexec.DumpLogOnError)
	testing.ContextLog(ctx, "suspend_stress_test Output: ", string(out))
	if err != nil {
		s.Fatal("Failed to run suspend_stress_test: ", err)
	}
	if match := regexp.MustCompile(`(?m)^(Suspend failed.*)$`).FindSubmatch(out); len(match) > 0 {
		s.Fatal("Failed running suspend_stress_test: ", string(match[1]))
	}
}
