// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package diagnostics

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/diagnostics/utils"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/diagnosticsapp"
	"go.chromium.org/tast-tests/cros/local/cpu"
	"go.chromium.org/tast-tests/cros/local/procutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         RoutineSection,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Diagnostics app routines run successfully",
		// ChromeOS > Software > System Services > Serviceability > Diagnostics
		BugComponent: "b:1131925",
		Contacts: []string{
			"cros-peripherals@google.com",
			"ashleydp@google.com",
			"zentaro@google.com",
			"menghuan@google.com",
		},
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "diagnosticsPrep",
		Timeout:      10 * time.Minute,
	})
}

const (
	// Full path to stress test launched by diagnostics routine service.
	// See: src/platform2/diagnostics/cros_healthd/routines/cpu_stress/cpu_stress.cc
	cpuStressTestExecPath = "/usr/bin/stressapptest"
)

// RoutineSection verifies routine section functionality.
func RoutineSection(ctx context.Context, s *testing.State) {
	tconn := s.FixtValue().(*utils.FixtureData).Tconn
	ui := uiauto.New(tconn).WithTimeout(5 * time.Second)

	// Wait for CPU idle to reduce likelihood of stressapptest becoming a zombie.
	if err := cpu.WaitUntilIdle(ctx); err != nil {
		// Do not block test even if we failed to wait cpu idle time.
		s.Log("Failed to wait cpu idle before running RoutineSection test. Keep running RoutineSection test")
	}

	// Find the first routine action button.
	cpuButton := diagnosticsapp.DxCPUTestButton.Ancestor(diagnosticsapp.DxRootNode).First()
	if err := ui.WithTimeout(20 * time.Second).WaitUntilExists(cpuButton)(ctx); err != nil {
		s.Fatal("Failed to find the cpu test routine button: ", err)
	}

	// If needed, scroll down to make the cpu button visible.
	if err := ui.FocusAndWait(cpuButton)(ctx); err != nil {
		s.Fatal("Failed to locate cpu button within the screen bounds: ", err)
	}

	// Clean up after the CPU test.
	// Wait for CPU idle to make sure stress test doesn't leave a bad state.
	defer func() {
		if err := cpu.WaitUntilIdle(ctx); err != nil {
			// Do not block test even if we failed to wait cpu idle time.
			s.Log("Failed to wait cpu idle after running RoutineSection test")
		}
	}()

	// Test CPU routine.
	pollOpts := testing.PollOptions{Interval: time.Second, Timeout: 20 * time.Second}
	if err := ui.WithPollOpts(pollOpts).LeftClick(cpuButton)(ctx); err != nil {
		s.Fatal("Could not click the CPU test button: ", err)
	}
	s.Log("Starting CPU test routine")

	// Wait for UI to swap to "in progress" state to give time for diagnostics service to start routine.
	if err := ui.WithPollOpts(pollOpts).WaitUntilExists(
		diagnosticsapp.DxProgressBadge.Ancestor(diagnosticsapp.DxRootNode).First())(
		ctx); err != nil {
		s.Fatal("Could not verify test routine has started: ", err)
	}

	// Detect CPU stress test launched using process lookup.
	proc, err := procutil.FindUnique(procutil.ByExe(cpuStressTestExecPath))
	if err != nil {
		s.Fatal("Stress test did not start: ", err)
	}
	s.Log("Stress test running at ", proc)

	// Detect CPU stress test process terminated.
	if err := procutil.WaitForTerminated(ctx, proc, 2*time.Minute); err != nil {
		s.Fatal("Stress test did not stop: ", err)
	}
	s.Log("Stress test process no longer running")

	if err := uiauto.IfFailThen(ui.WaitUntilExists(
		diagnosticsapp.DxPassedBadge.Ancestor(diagnosticsapp.DxRootNode).First()), ui.WaitUntilExists(
		diagnosticsapp.DxFailedBadge.Ancestor(diagnosticsapp.DxRootNode).First()))(ctx); err != nil {
		s.Fatal("Could not verify successful run of at least one CPU routine: ", err)
	}

	// Cancel the test after first routine succeeds.
	cancelBtn := diagnosticsapp.DxCancelTestButton.Ancestor(diagnosticsapp.DxRootNode)
	if err := uiauto.Combine("click Cancel",
		ui.WithTimeout(20*time.Second).WaitUntilExists(cancelBtn),
		ui.MakeVisible(cancelBtn),
		ui.EnsureFocused(cancelBtn),
		ui.WithPollOpts(pollOpts).LeftClick(cancelBtn),
	)(ctx); err != nil {
		s.Fatal("Failed to click cancel button: ", err)
	}

	if err := ui.WithTimeout(20 * time.Second).WaitUntilExists(
		diagnosticsapp.DxCancelledBadge.Ancestor(diagnosticsapp.DxRootNode).First())(
		ctx); err != nil {
		s.Fatal("Could not verify cancellation of routine: ", err)
	}
}
