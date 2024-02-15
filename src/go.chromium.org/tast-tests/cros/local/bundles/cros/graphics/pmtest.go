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
	pmMode      graphics.PmTestMode
	suspendMode graphics.SuspendMode
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
				Name: "00_none_s3",
				Val: pmTestParam{
					pmMode:      graphics.PmTestNone,
					suspendMode: graphics.SuspendS3,
				},
			},
			{
				Name: "01_freezers_s0",
				Val: pmTestParam{
					pmMode:      graphics.PmTestFreezer,
					suspendMode: graphics.SuspendS0ix,
				},
			}, {
				Name: "02_freezers_s3",
				Val: pmTestParam{
					pmMode:      graphics.PmTestFreezer,
					suspendMode: graphics.SuspendS3,
				},
			}, {
				Name: "03_devices_s0",
				Val: pmTestParam{
					pmMode:      graphics.PmTestDevices,
					suspendMode: graphics.SuspendS0ix,
				},
			}, {
				Name: "04_devices_s3",
				Val: pmTestParam{
					pmMode:      graphics.PmTestDevices,
					suspendMode: graphics.SuspendS3,
				},
			}, {
				Name: "05_platform_s0",
				Val: pmTestParam{
					pmMode:      graphics.PmTestPlatform,
					suspendMode: graphics.SuspendS0ix,
				},
			}, {
				Name: "06_platform_s3",
				Val: pmTestParam{
					pmMode:      graphics.PmTestPlatform,
					suspendMode: graphics.SuspendS3,
				},
			}, {
				Name: "07_processors_s0",
				Val: pmTestParam{
					pmMode:      graphics.PmTestProcessors,
					suspendMode: graphics.SuspendS0ix,
				},
			}, {
				Name: "08_processors_s3",
				Val: pmTestParam{
					pmMode:      graphics.PmTestProcessors,
					suspendMode: graphics.SuspendS3,
				},
			}, {
				Name: "09_core_s0",
				Val: pmTestParam{
					pmMode:      graphics.PmTestCore,
					suspendMode: graphics.SuspendS0ix,
				},
			}, {
				Name: "10_core_s3",
				Val: pmTestParam{
					pmMode:      graphics.PmTestCore,
					suspendMode: graphics.SuspendS3,
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

	oriSuspendMode := graphics.GetSuspendState(ctx)
	s.Log("Original suspend state: ", oriSuspendMode)
	defer graphics.SetSuspendState(ctx, oriSuspendMode)

	params := s.Param().(pmTestParam)
	mode := params.pmMode
	// Check if mode is supported.
	inList := func(str graphics.PmTestMode, list []graphics.PmTestMode) bool {
		for _, l := range list {
			if str == l {
				return true
			}
		}
		return false
	}
	if !inList(mode, origPmMode.Available) {
		s.Logf("pm_test doesn't support requested mode: %v. Skipping the test", mode)
		return
	}
	if err := graphics.SetPMTest(ctx, mode); err != nil {
		s.Fatal("Failed to set pm_test: ", err)
	}

	// We request two consecutive suspend_resumes to ensure each cycle can be repeated.
	// In other words if the resume was unclean we give the test a chance to fail itself (and not some following test).
	out, err := testexec.CommandContext(ctx, "suspend_stress_test", "--count", "2", "--record_dmesg_dir", s.OutDir()).Output(testexec.DumpLogOnError)
	testing.ContextLog(ctx, "suspend_stress_test Output: ", string(out))
	if err != nil {
		s.Fatal("Failed to run suspend_stress_test: ", err)
	}
	if match := regexp.MustCompile(`(?m)^(Suspend failed.*)$`).FindSubmatch(out); len(match) > 0 {
		s.Fatal("Failed running suspend_stress_test: ", string(match[1]))
	}
}
