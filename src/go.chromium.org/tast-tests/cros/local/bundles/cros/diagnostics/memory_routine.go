// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package diagnostics

import (
	"context"
	"time"

	"github.com/shirou/gopsutil/v3/process"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/diagnostics/utils"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/diagnosticsapp"
	"go.chromium.org/tast-tests/cros/local/cpu"
	"go.chromium.org/tast-tests/cros/local/memory/kernelmeter"
	"go.chromium.org/tast-tests/cros/local/procutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         MemoryRoutine,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Diagnostics app memory routine runs and stops successfully",
		// ChromeOS > Platform > Enablement > Health
		BugComponent: "b:982097",
		Contacts: []string{
			"cros-tdm-tpe-eng@google.com",
			"weiluanwang@google.com",
		},
		Attr: []string{"group:mainline",
			// TODO(b/362930919): Remove the below attributes after the test is stable on all boards.
			"group:healthd", "healthd_perbuild"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      2 * time.Minute,
		Params: []testing.Param{{
			Fixture:   "diagnosticsPrep",
			ExtraAttr: []string{"informational", "group:criticalstaging"},
		}, {
			Name:      "no_mojo_check",
			Fixture:   "diagnosticsPrepWithoutMojoCheck",
			ExtraAttr: []string{"informational", "group:criticalstaging"},
		}},
	})
}

const (
	// Full path to stress test launched by diagnostics routine service.
	memtesterExecPath = "/usr/sbin/memtester"
)

// MemoryRoutine verifies the memory routine can be started, running, and
// cancelled successfully.
func MemoryRoutine(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 15*time.Second)
	defer cancel()

	tconn := s.FixtValue().(*utils.FixtureData).Tconn
	ui := uiauto.New(tconn).WithTimeout(5 * time.Second)

	// Find the first routine action button.
	memoryButton := diagnosticsapp.DxMemoryTestButton.Ancestor(diagnosticsapp.DxRootNode).First()
	if err := ui.WithTimeout(20 * time.Second).WaitUntilExists(memoryButton)(ctx); err != nil {
		s.Fatal("Failed to find the memory test routine button: ", err)
	}

	// If needed, scroll down to make the memory button visible.
	if err := ui.FocusAndWait(memoryButton)(ctx); err != nil {
		s.Fatal("Failed to locate memory button within the screen bounds: ", err)
	}

	// Clean up after the memory test.
	// Wait for CPU idle to make sure memory test doesn't leave a bad state.
	// See b/255701247.
	defer func() {
		if err := cpu.WaitUntilIdle(cleanupCtx); err != nil {
			// Do not block test even if we failed to wait cpu idle time.
			s.Log("Failed to wait cpu idle after running MemoryRoutine test")
		}
	}()

	memInfo, err := kernelmeter.ReadMemInfo()
	if err != nil {
		s.Log("Cannot obtain memory info: ", err)
	} else {
		s.Logf("Meminfo: total=%s, avaialble=%s", memInfo["MemTotal"], memInfo["MemAvailable"])
	}

	// Test memory routine.
	pollOpts := testing.PollOptions{Interval: time.Second, Timeout: 20 * time.Second}
	if err := ui.WithPollOpts(pollOpts).LeftClick(memoryButton)(ctx); err != nil {
		s.Fatal("Could not click the memory test button: ", err)
	}
	s.Log("Starting memory test routine")

	// Wait for UI to swap to "in progress" state to give time for diagnostics
	// service to start routine.
	if err := ui.WithPollOpts(pollOpts).WaitUntilExists(
		diagnosticsapp.DxRunningMemoryTestMsg.Ancestor(diagnosticsapp.DxRootNode).First())(
		ctx); err != nil {
		s.Fatal("Could not verify test routine has started: ", err)
	}

	// Detect memtester launched using process lookup.
	var memtesterProc *process.Process
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		proc, err := procutil.FindUnique(procutil.ByExe(memtesterExecPath))
		if err == nil {
			memtesterProc = proc
		}
		return err
	}, &testing.PollOptions{Interval: 500 * time.Millisecond, Timeout: 3 * time.Second}); err != nil {
		s.Fatal("Memtester did not start: ", err)
	}
	s.Log("Memtester running at ", memtesterProc)

	// GoBigSleepLint: Wait to verify the routine can be running for a period of
	// time.
	if err := testing.Sleep(ctx, 10*time.Second); err != nil {
		s.Error("Failed to sleep after the memtester starts running")
	}

	// Cancel the test.
	cancelBtn := diagnosticsapp.DxCancelTestButton.Ancestor(diagnosticsapp.DxRootNode)
	if err := uiauto.Combine("click Cancel",
		ui.WithTimeout(20*time.Second).WaitUntilExists(cancelBtn),
		ui.MakeVisible(cancelBtn),
		ui.EnsureFocused(cancelBtn),
		ui.WithPollOpts(pollOpts).LeftClick(cancelBtn),
	)(ctx); err != nil {
		s.Fatal("Failed to click cancel button: ", err)
	}

	if err := ui.WithTimeout(10 * time.Second).WaitUntilExists(
		diagnosticsapp.DxCancelledBadge.Ancestor(diagnosticsapp.DxRootNode).First())(
		ctx); err != nil {
		s.Fatal("Could not verify cancellation of routine: ", err)
	}

	// Detect memtester process terminated.
	if err := procutil.WaitForTerminated(ctx, memtesterProc, 10*time.Second); err != nil {
		s.Fatal("Memtester did not stop: ", err)
	}
	s.Log("Memtester process no longer running")
}
