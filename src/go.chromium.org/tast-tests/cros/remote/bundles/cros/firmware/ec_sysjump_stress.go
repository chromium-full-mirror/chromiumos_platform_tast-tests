// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const (
	// Reset flag bit indicating the EC entered the current state via sysjump.
	ecResetFlagSysjump = 0x00000400
	// Reset flag bit indicating an EC watchdog reset.
	ecResetFlagWatchdog = 0x00000010
	// Reset flag bit indicating an AP watchdog reset.
	ecResetFlagAPWatchdog = 0x00040000
)

type ecSysjumpStressParams struct {
	iterations  int
	consecutive bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func: ECSysjumpStress,
		Desc: "Stress EC sysjumps repeatedly and verify device state",
		Contacts: []string{
			"chromeos-faft@google.com",
			"cros-ec@google.com",
			"zeleena@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Vars:         []string{"firmware.ecSysjumpStressIterations"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		TestBedDeps:  tbdep.ServoPresentAndWorking,
		Fixture:      fixture.NormalMode,
		Timeout:      2 * time.Hour,
		Params: []testing.Param{
			{
				Name: "stress",
				Val: ecSysjumpStressParams{
					iterations:  10,
					consecutive: false,
				},
			},
			{
				Name: "consecutive",
				Val: ecSysjumpStressParams{
					iterations:  100,
					consecutive: true,
				},
			},
		},
	})
}

// ecSysjumpStress performs a sysjump e.g. RW->RO and verifies the post-jump
// state. Performs the second jump back e.g. RO->RW and verifies the post-jump
// state as part of one iteration.
func ecSysjumpStress(ctx context.Context, h *firmware.Helper, firstSysjumpDest string, iteration int) error {
	secondSysjumpDest := getSysjumpDest(firstSysjumpDest)

	// First jump.
	if err := sysjumpAndVerify(ctx, h, firstSysjumpDest, iteration); err != nil {
		return errors.Wrapf(err, "first jump to %s failed", firstSysjumpDest)
	}

	// Second jump back.
	if err := sysjumpAndVerify(ctx, h, secondSysjumpDest, iteration); err != nil {
		return errors.Wrapf(err, "second jump to %s failed", secondSysjumpDest)
	}

	return nil
}

// ecSysjumpStressConsecutive performs a sysjump and only the minimal active
// copy verification after each jump. This helper should be ran in a loop that
// performs a large sequence of sysjumps to maximize cycle throughput. This
// should be followed by the full post-jump state verification at the conclusion
// of all the jumps.
func ecSysjumpStressConsecutive(ctx context.Context, h *firmware.Helper, sysjumpDest string, iteration int) error {
	if err := executeSysjump(ctx, h, sysjumpDest, iteration); err != nil {
		return errors.Wrapf(err, "jump to %s failed", sysjumpDest)
	}
	// Ensure we actually performed a sysjump by verifying that the active copy
	// jumped to sysjumpDest.
	if err := h.Servo.CheckECActiveCopyMatch(ctx, sysjumpDest); err != nil {
		return errors.Wrapf(err, "consecutive sysjump failed on jump %d while jumping to %s", iteration, sysjumpDest)
	}
	return nil
}

// sysjumpAndVerify performs one sysjump and full state verification.
func sysjumpAndVerify(ctx context.Context, h *firmware.Helper, sysjumpDest string, iteration int) error {
	isChargerAttachedBefore, err := h.Servo.GetChargerAttached(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get if battery charger is attached before jump")
	}
	if err := executeSysjump(ctx, h, sysjumpDest, iteration); err != nil {
		return errors.Wrapf(err, "jump to %s failed", sysjumpDest)
	}
	if err := verifyPostJumpState(ctx, h, sysjumpDest, isChargerAttachedBefore, iteration); err != nil {
		return errors.Wrapf(err, "state verification failed after jump to %s", sysjumpDest)
	}
	return nil
}

// getECResetFlags gets the EC reset flags from sysinfo and parses them.
func getECResetFlags(ctx context.Context, h *firmware.Helper) (int64, error) {
	sysinfoOutput, err := h.Servo.RunECCommandGetOutputNoConsoleLogs(ctx, "sysinfo", []string{`Reset flags:\s+0x([0-9a-f]{8})`})
	if err != nil || len(sysinfoOutput) == 0 || len(sysinfoOutput[0]) < 2 {
		return 0, errors.Wrap(err, "failed to read EC reset flags via sysinfo")
	}
	resetFlags, err := strconv.ParseInt(sysinfoOutput[0][1], 16, 64)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to parse reset flags %q", sysinfoOutput[0][1])
	}
	return resetFlags, err
}

// executeSysjump executes the sysjump command to sysjumpDest and waits for the EC to settle.
func executeSysjump(ctx context.Context, h *firmware.Helper, sysjumpDest string, iteration int) error {
	testing.ContextLogf(ctx, "Iteration %d: Executing sysjump to %s", iteration, sysjumpDest)

	if err := h.Servo.RunECCommand(ctx, "sysjump "+sysjumpDest); err != nil {
		return errors.Wrapf(err, "failed to run EC command sysjump %s", sysjumpDest)
	}

	// GoBigSleepLint: Wait for the EC to complete the sysjump and re-initialize its console.
	testing.Sleep(ctx, 3*time.Second)
	return nil
}

// verifyPostJumpState verifies the state of the device after a sysjump to expectedSysjumpDest.
func verifyPostJumpState(ctx context.Context, h *firmware.Helper, expectedSysjumpDest string, isChargerAttachedBefore bool, iteration int) error {
	testing.ContextLogf(ctx, "Iteration %d: Verifying post-sysjump state after sysjumping to %s", iteration, expectedSysjumpDest)

	if err := verifyECResponsive(ctx, h); err != nil {
		return err
	}
	if err := verifyNoECCrashes(ctx, h); err != nil {
		return err
	}
	if err := h.Servo.CheckECActiveCopyMatch(ctx, expectedSysjumpDest); err != nil {
		return err
	}
	if err := verifySysjumpResetFlags(ctx, h); err != nil {
		return err
	}
	if err := verifyBatteryChargerAttached(ctx, h, isChargerAttachedBefore); err != nil {
		return err
	}
	if err := verifyPDStateFunctioning(ctx, h); err != nil {
		return err
	}
	if err := verifyS0PowerState(ctx, h); err != nil {
		return err
	}
	return nil
}

// verifyECResponsive runs a simple EC command to verify that the EC is responsive.
func verifyECResponsive(ctx context.Context, h *firmware.Helper) error {
	version, err := h.Servo.RunECCommandGetOutput(ctx, "version", []string{`Build:\s+`})
	if err != nil || len(version) == 0 {
		return errors.Wrap(err, "EC console unresponsive after sysjump")
	}
	return nil
}

// verifyNoECCrashes actively checks the crash cache to see if there were any
// recent EC crashes. This can be called after a sysjump instead of waiting
// until the end to check for crashes.
func verifyNoECCrashes(ctx context.Context, h *firmware.Helper) error {
	waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
	defer cancelWaitConnect()
	if err := h.WaitConnect(waitConnectCtx); err != nil {
		return errors.Wrap(err, "failed to reconnect to DUT after sysjump")
	}

	crashes, err := h.GetNewECCrashes(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to query EC crash cache")
	}
	if len(crashes) > 0 {
		return errors.Errorf("EC crash detected after sysjump: %v", crashes)
	}
	return nil
}

// verifySysjumpResetFlags ensures that the reset flag indicates a sysjump and
// did not indicate an unexpected reset, such as a watchdog reset.
func verifySysjumpResetFlags(ctx context.Context, h *firmware.Helper) error {
	// Verify reset flags.
	resetFlags, err := getECResetFlags(ctx, h)
	if err != nil {
		return errors.Wrap(err, "failed to get EC reset flags after jump")
	}
	if resetFlags&ecResetFlagSysjump == 0 {
		return errors.Errorf("EC reset flags 0x%08x should indicate sysjump (0x400)", resetFlags)
	}
	if resetFlags&(ecResetFlagWatchdog|ecResetFlagAPWatchdog) != 0 {
		return errors.Errorf("EC reset flags 0x%08x should not indicate a watchdog reset (0x40000|0x10)", resetFlags)
	}
	return nil
}

// verifyBatteryChargerAttached verifies whether the battery charger is attached
// before the sysjump and is in the same state after the sysjump.
func verifyBatteryChargerAttached(ctx context.Context, h *firmware.Helper, isChargerAttachedBefore bool) error {
	isChargerAttachedAfter, err := h.Servo.GetChargerAttached(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get if battery charger is attached after sysjump")
	}
	if isChargerAttachedAfter != isChargerAttachedBefore {
		return errors.Errorf("whether the battery charger was attached unexpectedly changed across sysjump: was %v, now %v", isChargerAttachedBefore, isChargerAttachedAfter)
	}
	return nil
}

// verifyPDStateFunctioning verifies that the USB Power Delivery (PD) state
// machine is still functioning. Gracefully handles boards that do not have
// USB-C PD ports.
func verifyPDStateFunctioning(ctx context.Context, h *firmware.Helper) error {
	var pdState *servo.PDState
	err := testing.Poll(ctx, func(ctx context.Context) error {
		var err error
		pdState, err = h.Servo.GetDUTPDState(ctx)
		return err
	}, &testing.PollOptions{Timeout: 45 * time.Second, Interval: 2 * time.Second})
	if err != nil {
		if strings.Contains(err.Error(), "no active PD ports found") {
			// Handle boards without USB-C PD ports.
			testing.ContextLogf(ctx, "Skipping PD state check (no active PD port on this board): %v", err)
			return nil
		}
		return errors.Wrap(err, "failed to get PD state after sysjump")
	}
	testing.ContextLogf(ctx, "Verified USB PD Port %d after sysjump: state=%v, role=%s-%s, polarity=%s", pdState.Port, pdState.GetStateName(), pdState.PowerRole, pdState.DataRole, pdState.Polarity)
	return nil
}

// verifyS0PowerState verifies that the EC maintained stable S0 power after the
// sysjump.
func verifyS0PowerState(ctx context.Context, h *firmware.Helper) error {
	if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, 15*time.Second, "S0"); err != nil {
		return errors.Wrap(err, "failed to maintain or return to S0 power state after sysjump")
	}
	return nil
}

// getSysjumpDest takes the current active copy, either "RO" or "RW", and
// outputs the opposite one, "RW" or "RO", representing the location to sysjump
// to next.
func getSysjumpDest(activeCopy string) string {
	if strings.HasPrefix(activeCopy, "RW") {
		return "RO"
	}
	return "RW"
}

// recoverDUT resets the DUT power state via Servo and waits for the DUT to
// reconnect to prepare the DUT for subsequent iterations.
func recoverDUT(ctx context.Context, h *firmware.Helper) error {
	// If the EC experienced a crash or hang, reboot the EC to clear transient crash flags.
	if err := h.Servo.RunECCommand(ctx, "reboot"); err != nil {
		testing.ContextLog(ctx, "Notice: EC console reboot failed, falling back to Servo power reset: ", err)
	}

	// Reset DUT power state via Servo.
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateReset); err != nil {
		testing.ContextLog(ctx, "Warning: Failed to set power state reset on servo: ", err)
	}
	// Wait for the DUT to come back online so the next iteration can run.
	waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
	defer cancelWaitConnect()
	if err := h.WaitConnect(waitConnectCtx); err != nil {
		return errors.Wrap(err, "failed to reconnect to DUT after power state reset")
	}
	return nil
}

// ECSysjumpStress repeatedly jumps between EC firmware images (RW -> RO -> RW)
// and verifies EC console responsiveness, reset cause flags, and system power
// stability across repeated sysjumps.
func ECSysjumpStress(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to RequireServo: ", err)
	}
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()
	s.Log("Disabling write protect to allow arbitrary sysjumps between RO and RW")
	if err := h.Servo.SetFWWPState(ctx, servo.FWWPStateForceOff); err != nil {
		s.Fatal("Failed to disable firmware write protect: ", err)
	}
	defer func(ctx context.Context) {
		if err := h.Servo.SetFWWPState(ctx, servo.FWWPStateForceOn); err != nil {
			s.Error("Failed to restore firmware write protect: ", err)
		}
	}(cleanupCtx)
	params := s.Param().(ecSysjumpStressParams)
	numIters := params.iterations

	// Read test args for override of number of iterations.
	if numItersStr, ok := s.Var("firmware.ecSysjumpStressIterations"); ok {
		numItersInt, err := strconv.Atoi(numItersStr)
		if err != nil {
			s.Fatalf("Invalid value for var firmware.ecSysjumpStressIterations: got %q, expected int", numItersStr)
		}
		numIters = numItersInt
	}
	if numIters <= 0 {
		s.Fatalf("Requires positive amount of iterations, got %d", numIters)
	}

	// Get device state during test setup.
	isChargerAttachedBefore, err := h.Servo.GetChargerAttached(ctx)
	if err != nil {
		s.Errorf("Failed to get if battery charger is attached before running test: %v", err)
	}
	activeCopy, err := h.Servo.GetString(ctx, servo.ECActiveCopy)
	if err != nil {
		s.Fatalf("Failed to get initial EC active copy: %v", err)
	}
	sysjumpDest := getSysjumpDest(activeCopy)

	failures := make([][]error, numIters)
	numFails := 0
	startTime := time.Now()
	for i := 0; i < numIters; i++ {
		if i > 0 {
			estimatedTimeRemaining := time.Since(startTime) / time.Duration(i) * time.Duration(numIters-i)
			s.Logf("------ Running iteration %d out of %d (%d failures) Estimated time remaining: %s ------", i+1, numIters, numFails, estimatedTimeRemaining.Round(time.Second))
		} else {
			s.Logf("------ Running iteration %d out of %d ------", i+1, numIters)
		}
		h.Servo.Echo(ctx, fmt.Sprintf("%s iteration %d out of %d", s.TestName(), i+1, numIters))

		// Run selected test.
		var iterErr error
		if params.consecutive {
			// Make rapid consecutive sysjumps.
			iterErr = ecSysjumpStressConsecutive(ctx, h, sysjumpDest, i+1)
			if i+1 < numIters {
				// Don't increase sysjumpDest on the last iteration, keep it the same for final state verification.
				sysjumpDest = getSysjumpDest(sysjumpDest)
			}
		} else {
			// Perform a sysjump, verify state, then perform a sysjump back and verify state again.
			iterErr = ecSysjumpStress(ctx, h, sysjumpDest, i+1)
		}

		// Handle iteration failure and DUT recovery.
		if iterErr != nil {
			numFails += 1
			failures[i] = append(failures[i], iterErr)
			s.Logf("Iteration %d failed: %v. Attempting DUT recovery", i+1, iterErr)

			if err := recoverDUT(ctx, h); err != nil {
				// If the DUT cannot be recovered, abort the rest of the test.
				s.Fatalf("DUT unrecoverable after iteration %d: %v", i+1, err)
			}
			// After recovery and reboot, ensure accurate sysjumpDest since active copy could have changed.
			activeCopy, err := h.Servo.GetString(ctx, servo.ECActiveCopy)
			if err != nil {
				s.Fatalf("Failed to get EC active copy after DUT recovery: %v", err)
			}
			sysjumpDest = getSysjumpDest(activeCopy)
		}
	}

	if params.consecutive {
		// Perform final full state verification for the consecutive sysjump test.
		s.Logf("Completed %d consecutive sysjumps. Verifying full post-jump state.", numIters)
		if err := verifyPostJumpState(ctx, h, sysjumpDest, isChargerAttachedBefore, numIters); err != nil {
			s.Errorf("Final state verification failed after jump to %s: %v", sysjumpDest, err)
		}
	}

	if numFails > 0 {
		s.Logf("Encountered %d failures out of %d iterations during execution of stress test %s", numFails, numIters, s.TestName())

		for iter, errs := range failures {
			if len(errs) > 0 {
				s.Logf("Iteration %d had the following failures:", iter+1)
				for _, errMsg := range errs {
					s.Logf("\t%v", errMsg)
				}
			}
		}
		s.Fatalf("%s test had %d errors, see logs for details", s.TestName(), numFails)
	} else if !s.HasError() {
		s.Log("No failures encountered")
	}
}
