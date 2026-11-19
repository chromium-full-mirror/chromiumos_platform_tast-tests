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
		Desc: "Stress EC RO to RW sysjumps repeatedly and verify device state",
		Contacts: []string{
			"chromeos-faft@google.com",
			"cros-ec@google.com",
			"zeleena@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware", "firmware_stress"},
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

// ecSysjumpStress resets the EC into RO, performs an RO->RW sysjump (or asserts
// EFS's automatic RO->RW sysjump on battery-less USB-C EC-TCPC boards), boots
// the AP back to S0, and verifies the post-jump state.
func ecSysjumpStress(ctx context.Context, h *firmware.Helper, losesPower bool, iteration int) error {
	isChargerAttachedBefore, err := getChargerAttached(ctx, h)
	if err != nil {
		return errors.Wrap(err, "failed to get if battery charger is attached before jump")
	}
	if losesPower {
		if err := triggerEFSSysjump(ctx, h, iteration); err != nil {
			return errors.Wrap(err, "EFS automatic sysjump to RW failed")
		}
		if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, 10*time.Second, "S0"); err != nil {
			if err := bootAPFromOff(ctx, h); err != nil {
				return errors.Wrap(err, "failed to power on AP after EFS sysjump")
			}
		}
	} else {
		if err := enterECROWithAPOff(ctx, h); err != nil {
			return errors.Wrap(err, "entering EC RO with AP off failed")
		}
		if err := executeSysjumpToRW(ctx, h, iteration); err != nil {
			return errors.Wrap(err, "jump to RW failed")
		}
		if err := bootAPFromOff(ctx, h); err != nil {
			return errors.Wrap(err, "failed to power on AP after sysjump to RW")
		}
	}
	if err := verifyPostJumpState(ctx, h, "RW", isChargerAttachedBefore, iteration); err != nil {
		return errors.Wrap(err, "state verification failed after jump to RW")
	}
	return nil
}

// ecSysjumpStressConsecutive resets the EC into RO and performs an RO->RW
// sysjump with active-copy and reset-flag verification after each jump while
// keeping the AP off to maximize cycle throughput. On battery-less USB-C
// EC-TCPC boards, resetting the EC drops Type-C VBUS and exercises EFS's
// automatic RO->RW sysjump on VBUS restore. Full post-jump state verification
// runs after all iterations complete.
func ecSysjumpStressConsecutive(ctx context.Context, h *firmware.Helper, losesPower bool, iteration int) error {
	if losesPower {
		if err := triggerEFSSysjump(ctx, h, iteration); err != nil {
			return errors.Wrapf(err, "consecutive EFS sysjump failed on jump %d", iteration)
		}
		return nil
	}
	if err := enterECROWithAPOff(ctx, h); err != nil {
		return errors.Wrapf(err, "entering EC RO failed on jump %d", iteration)
	}
	if err := executeSysjumpToRW(ctx, h, iteration); err != nil {
		return errors.Wrapf(err, "jump to RW failed on jump %d", iteration)
	}
	if err := h.Servo.CheckECActiveCopyMatch(ctx, "RW"); err != nil {
		return errors.Wrapf(err, "consecutive sysjump failed on jump %d while jumping to RW", iteration)
	}
	if err := verifySysjumpResetFlags(ctx, h); err != nil {
		return errors.Wrapf(err, "reset flag check failed on jump %d", iteration)
	}
	return nil
}

func isAPOffState(state string) bool {
	switch state {
	case "G3", "S5", "AP_POWER_STATE_G3", "AP_POWER_STATE_S5":
		return true
	default:
		return false
	}
}

// enterECROWithAPOff shuts down the AP cleanly if it is running, then cold-resets
// the EC into RO while keeping the AP off.
func enterECROWithAPOff(ctx context.Context, h *firmware.Helper) error {
	powerState, err := h.Servo.GetECSystemPowerState(ctx)
	if err != nil || !isAPOffState(powerState) {
		if err := h.CloseRPCConnection(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to close RPC connection before shutdown: ", err)
		}
		if h.DUT.Connected(ctx) {
			_ = h.DUT.Conn().CommandContext(ctx, "sync").Run()
			_ = h.DUT.Conn().CommandContext(ctx, "poweroff").Start()
		}
		if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, 30*time.Second, "G3", "S5"); err != nil {
			if offErr := h.Servo.SetPowerState(ctx, servo.PowerStateOff); offErr != nil {
				testing.ContextLog(ctx, "Failed to set power_state:off: ", offErr)
			}
			if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, 15*time.Second, "G3", "S5"); err != nil {
				return errors.Wrap(err, "failed to reach G3/S5 before EC reset to RO")
			}
		}
	}

	if err := h.Servo.RunECCommand(ctx, "reboot ap-off-in-ro"); err != nil {
		return errors.Wrap(err, "failed to run EC command reboot ap-off-in-ro")
	}
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return h.Servo.CheckECActiveCopyMatch(ctx, "RO")
	}, &testing.PollOptions{Timeout: 5 * time.Second, Interval: 500 * time.Millisecond}); err != nil {
		// Pre-EFS2 EC firmware built before 2020-03 lacks 'ap-off-in-ro', and
		// stays in RO on 'reboot ap-off' until AP RW sends EC_REBOOT_JUMP_RW.
		testing.ContextLog(ctx, "EC did not enter RO via reboot ap-off-in-ro, falling back to reboot ap-off: ", err)
		if rbErr := h.Servo.RunECCommand(ctx, "reboot ap-off"); rbErr != nil {
			return errors.Wrap(rbErr, "failed to run EC command reboot ap-off")
		}
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			return h.Servo.CheckECActiveCopyMatch(ctx, "RO")
		}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: 500 * time.Millisecond}); err != nil {
			return errors.Wrap(err, "EC failed to enter RO with AP off")
		}
	}
	return nil
}

// triggerEFSSysjump triggers an EC reset on battery-less USB-C EC-TCPC boards,
// where resetting the EC drops Type-C VBUS and causes EFS2 to automatically
// sysjump RO->RW when VBUS is restored.
func triggerEFSSysjump(ctx context.Context, h *firmware.Helper, iteration int) error {
	testing.ContextLogf(ctx, "Iteration %d: Triggering EC reboot to exercise EFS RO->RW sysjump", iteration)
	if err := h.CloseRPCConnection(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to close RPC connection before EC reboot: ", err)
	}
	if h.DUT.Connected(ctx) {
		_ = h.DUT.Conn().CommandContext(ctx, "sync").Run()
	}
	if err := h.Servo.RunECCommand(ctx, "reboot"); err != nil {
		return errors.Wrap(err, "failed to run EC command reboot")
	}
	// GoBigSleepLint: Allow Type-C VBUS to drop, re-source, and complete EFS RO->RW sysjump.
	if err := testing.Sleep(ctx, 3*time.Second); err != nil {
		return err
	}
	if err := h.RestoreServoPower(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to restore servo power after EC reboot: ", err)
	}
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return h.Servo.CheckECActiveCopyMatch(ctx, "RW")
	}, &testing.PollOptions{Timeout: 20 * time.Second, Interval: time.Second}); err != nil {
		return errors.Wrap(err, "EC did not complete EFS sysjump to RW after reboot")
	}
	return verifySysjumpResetFlags(ctx, h)
}

// bootAPFromOff powers on the AP from G3/S5 without resetting the EC so the
// EC's post-sysjump state and reset flags are preserved.
func bootAPFromOff(ctx context.Context, h *firmware.Helper) error {
	// GoBigSleepLint: Allow battery and charger state to settle after sysjump before pressing power key.
	if err := testing.Sleep(ctx, 2*time.Second); err != nil {
		return err
	}
	if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.DurShortPress); err != nil {
		return errors.Wrap(err, "failed to press power key to boot AP")
	}
	if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, 10*time.Second, "S0"); err != nil {
		h.PowerOnDUT(ctx)
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

// executeSysjumpToRW executes sysjump RW on the EC console and waits for the EC to settle.
func executeSysjumpToRW(ctx context.Context, h *firmware.Helper, iteration int) error {
	testing.ContextLogf(ctx, "Iteration %d: Executing sysjump to RW", iteration)

	if err := h.Servo.RunECCommand(ctx, "sysjump RW"); err != nil {
		return errors.Wrap(err, "failed to run EC command sysjump RW")
	}

	// GoBigSleepLint: Wait for the EC to complete the sysjump and re-initialize its console.
	if err := testing.Sleep(ctx, 3*time.Second); err != nil {
		return err
	}
	return nil
}

// verifyPostJumpState verifies the state of the device after a sysjump to expectedSysjumpDest.
func verifyPostJumpState(ctx context.Context, h *firmware.Helper, expectedSysjumpDest string, isChargerAttachedBefore *bool, iteration int) error {
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

// verifyECResponsive runs a simple EC command via Servo UART and via ectool on
// the DUT to verify that both the EC console and AP to EC host communication
// are responsive.
func verifyECResponsive(ctx context.Context, h *firmware.Helper) error {
	// Verify that the EC UART debug console is responsive with a simple command.
	if _, err := h.Servo.RunECCommandGetOutput(ctx, "version", []string{`Build:\s+`}); err != nil {
		return errors.Wrap(err, "EC console unresponsive after sysjump")
	}

	// Wait for the DUT SSH connection before running host commands (like ectool
	// below or crash checks in the caller).
	connectTimeout := h.Config.DelayRebootToPing
	if connectTimeout <= 0 {
		connectTimeout = 2 * time.Minute
	}
	waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, connectTimeout)
	defer cancelWaitConnect()
	if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
		return errors.Wrap(err, "failed to reconnect to DUT after sysjump")
	}

	// Verify AP to EC communication over the host bus via ectool version. The
	// above check tests the EC UART debug console, which can remain responsive
	// even if a sysjump disrupts the AP to EC host interface, so this check tries
	// to catch this issue (e.g. b/549100556).
	ec := firmware.NewECTool(h.DUT, firmware.ECToolNameMain)
	if _, err := ec.Version(ctx); err != nil {
		return errors.Wrap(err, "AP to EC communication via ectool version unresponsive after sysjump")
	}
	return nil
}

// verifyNoECCrashes actively checks the crash cache to see if there were any
// recent EC crashes. This can be called after a sysjump instead of waiting
// until the end to check for crashes.
func verifyNoECCrashes(ctx context.Context, h *firmware.Helper) error {
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
// before the sysjump and is in the same state after the sysjump. This check is
// skipped if the DUT doesn't have EC battery capability.
func verifyBatteryChargerAttached(ctx context.Context, h *firmware.Helper, isChargerAttachedBefore *bool) error {
	isChargerAttachedAfter, err := getChargerAttached(ctx, h)
	if err != nil {
		return errors.Wrap(err, "failed to get if battery charger is attached after sysjump")
	}
	if isChargerAttachedBefore == nil || isChargerAttachedAfter == nil {
		testing.ContextLog(ctx, "DUT does not have EC battery capability, skipping battery charging check")
		return nil
	}
	if *isChargerAttachedAfter != *isChargerAttachedBefore {
		return errors.Errorf("whether the battery charger was attached unexpectedly changed across sysjump: was %v, now %v", *isChargerAttachedBefore, *isChargerAttachedAfter)
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
			testing.ContextLog(ctx, "Skipping PD state check (no active PD port on this board): ", err)
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

// getChargerAttached returns &true when the battery charger is attached,
// &false when it is not, and nil when it does not have EC battery charging
// capability (e.g. Chromeboxes).
func getChargerAttached(ctx context.Context, h *firmware.Helper) (*bool, error) {
	if !h.Config.HasECCapability(firmware.ECBattery) {
		return nil, nil
	}
	isChargerAttached, err := h.Servo.GetChargerAttached(ctx)
	if err != nil {
		return nil, err
	}
	return &isChargerAttached, nil
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
	if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
		return errors.Wrap(err, "failed to reconnect to DUT after power state reset")
	}
	return nil
}

// ECSysjumpStress repeatedly resets the EC into RO and jumps from RO to RW,
// verifying EC console responsiveness, reset cause flags, and system power
// stability across repeated sysjumps.
func ECSysjumpStress(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to RequireServo: ", err)
	}
	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to RequireConfig: ", err)
	}
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()
	s.Log("Disabling write protect to allow manual sysjump to RW")
	if err := h.Servo.SetFWWPState(ctx, servo.FWWPStateForceOff); err != nil {
		s.Fatal("Failed to disable firmware write protect: ", err)
	}
	defer func(ctx context.Context) {
		if err := h.EnsureDUTBooted(ctx); err != nil {
			s.Error("Failed to ensure DUT is booted during cleanup: ", err)
		}
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
	isChargerAttachedBefore, err := getChargerAttached(ctx, h)
	if err != nil {
		s.Error("Failed to get if battery charger is attached before running test: ", err)
	}
	losesPower, err := h.LosesPowerOnECReset(ctx)
	if err != nil {
		s.Fatal("Failed to check whether DUT loses power on EC reset: ", err)
	}
	if losesPower {
		s.Log("DUT loses power on EC reset; testing EFS automatic RO->RW sysjumps across EC reboots")
	}

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
			// Make rapid consecutive RO->RW sysjumps.
			iterErr = ecSysjumpStressConsecutive(ctx, h, losesPower, i+1)
		} else {
			// Reset EC to RO, sysjump to RW, boot AP, and verify state.
			iterErr = ecSysjumpStress(ctx, h, losesPower, i+1)
		}

		// Handle iteration failure and DUT recovery.
		if iterErr != nil {
			numFails++
			failures[i] = append(failures[i], iterErr)
			s.Logf("Iteration %d failed: %v. Attempting DUT recovery", i+1, iterErr)

			if err := recoverDUT(ctx, h); err != nil {
				// If the DUT cannot be recovered, abort the rest of the test.
				s.Fatalf("DUT unrecoverable after iteration %d: %v", i+1, err)
			}
		}
	}

	if params.consecutive {
		// Perform final full state verification for the consecutive sysjump test.
		s.Logf("Completed %d consecutive sysjumps; powering on AP and verifying full post-jump state", numIters)
		if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, 5*time.Second, "S0"); err != nil {
			if err := bootAPFromOff(ctx, h); err != nil {
				s.Error("Failed to power on AP after consecutive sysjumps: ", err)
			}
		}
		if err := verifyPostJumpState(ctx, h, "RW", isChargerAttachedBefore, numIters); err != nil {
			s.Error("Final state verification failed after jump to RW: ", err)
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
