// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"path/filepath"
	"regexp"
	"time"

	fwCommon "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         MiniDiag,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify if DUT can boot to recovery screen and launch MiniDiag",
		Contacts: []string{
			"chromeos-faft@google.com",
			"cienet-firmware@cienet.corp-partner.google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		// TODO: When stable, change firmware_unstable to a different attr.
		Attr:         []string{"group:firmware", "firmware_unstable"},
		HardwareDeps: hwdep.D(hwdep.MiniDiag()),
		Fixture:      fixture.NormalMode,
		Timeout:      30 * time.Minute,
	})
}

func MiniDiag(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}
	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}
	if err := h.Reporter.ClearEventlog(ctx); err != nil {
		s.Fatal("Failed to clear event log: ", err)
	}
	launchCount := 0
	if h.Config.HasMiniDiagCapability(firmware.CbmemPreservedByAPReset) {
		s.Log("Launching MiniDiag and leave with warm reset")
		if err := launchMiniDiag(ctx, h); err != nil {
			s.Fatal("Failed to launch MiniDiag: ", err)
		}
		launchCount++
		if err := warmResetDUT(ctx, h); err != nil {
			s.Fatal("Failed to warm reset the DUT: ", err)
		}
		saveLogPath := filepath.Join(s.OutDir(), "firmware.log")
		s.Log("Verifying CBMEM logs for MiniDiag")
		if err := miniDiagVerifyCBMEM(ctx, h, saveLogPath); err != nil {
			s.Fatal("Failed to verify cbmem: ", err)
		}
	} else {
		s.Log("Skipping verifying with warm reset")
	}

	if h.Config.HasMiniDiagCapability(firmware.EventLogTestReport) ||
		!h.Config.HasMiniDiagCapability(firmware.CbmemPreservedByAPReset) {
		s.Log("Launching MiniDiag and leave with power off")
		if err := launchMiniDiag(ctx, h); err != nil {
			s.Fatal("Failed to launch MiniDiag: ", err)
		}
		launchCount++
		if err := menuPowerOffDUT(ctx, h); err != nil {
			s.Fatal("Failed to power off the DUT: ", err)
		}
		if err := powerButtonPowerOnDUT(ctx, h); err != nil {
			s.Fatal("Failed to power on the DUT: ", err)
		}
		if h.Config.HasMiniDiagCapability(firmware.EventLogTestReport) {
			s.Log("Verifying event log test report for MiniDiag")
			if err := verifyElogTestReport(ctx, h); err != nil {
				s.Fatal("Failed to verify event log test report: ", err)
			}
		} else {
			s.Log("Skipping verifying event log test report")
		}
	} else {
		s.Log("Skipping verifying with power off")
	}

	if h.Config.HasMiniDiagCapability(firmware.EventLogLaunchCount) {
		s.Log("Checking event log to verify event log launch count for MiniDiag")
		if err := verifyElogLaunchCount(ctx, h, launchCount); err != nil {
			s.Fatal("Failed to verify event log launch count: ", err)
		}
	} else {
		s.Log("Skipping verifying event log launch count")
	}
}

func launchMiniDiag(ctx context.Context, h *firmware.Helper) error {
	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		return errors.Wrap(err, "failed to create mode switcher")
	}
	testing.ContextLog(ctx, "Booting the DUT to the recovery screen")
	if err := ms.EnableRecMode(ctx, servo.PowerStateRec, servo.USBMuxOff); err != nil {
		return errors.Wrap(err, "failed to boot to recovery screen")
	}
	testing.ContextLog(ctx, "Waiting for DUT to reach the firmware screen")
	if err := h.WaitFirmwareScreen(ctx, h.Config.FirmwareScreenRecMode); err != nil {
		return errors.Wrap(err, "failed to get to firmware screen")
	}
	menuOperator, err := firmware.NewMenuOperator(ctx, h)
	if err != nil {
		return errors.Wrap(err, "failed to create menu operator")
	}
	testing.ContextLog(ctx, "Launching MiniDiag")
	if err := menuOperator.TriggerRecToMinidiag(ctx); err != nil {
		return errors.Wrap(err, "failed to trigger to minidiag")
	}
	testing.ContextLog(ctx, "Navigating to \"quick memory test screen\"")
	if err := menuOperator.NavigateMinidiagQuickMemoryCheck(ctx); err != nil {
		return errors.Wrap(err, "failed to navigate to quick memory test screen")
	}
	testing.ContextLog(ctx, "Navigating to \"storage screen\"")
	if err := menuOperator.NavigateMinidiagStorage(ctx); err != nil {
		return errors.Wrap(err, "failed to navigate to storage screen")
	}
	return nil
}

func menuPowerOffDUT(ctx context.Context, h *firmware.Helper) error {
	menuOperator, err := firmware.NewMenuOperator(ctx, h)
	if err != nil {
		return errors.Wrap(err, "failed to create menu operator")
	}
	if err := menuOperator.PowerOff(ctx); err != nil {
		return errors.Wrap(err, "failed to power off the DUT")
	}
	if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, 30*time.Second, "G3"); err != nil {
		return errors.Wrap(err, "failed to wait for G3 power state")
	}
	return nil
}

func powerButtonPowerOnDUT(ctx context.Context, h *firmware.Helper) error {
	if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.Dur(h.Config.HoldPwrButtonPowerOn)); err != nil {
		return errors.Wrap(err, "failed to press power key via servo")
	}
	waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
	defer cancelWaitConnect()

	if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
		return errors.Wrap(err, "failed to reconnect to the DUT")
	}
	return nil
}

func warmResetDUT(ctx context.Context, h *firmware.Helper) error {
	testing.ContextLog(ctx, "Rebooting the DUT with a warm reset")
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateWarmReset); err != nil {
		return errors.Wrap(err, "failed to warm reset the DUT")
	}
	waitDisconnectCtx, cancelWaitDisconnect := context.WithTimeout(ctx, 1*time.Minute)
	defer cancelWaitDisconnect()
	if err := h.DUT.WaitUnreachable(waitDisconnectCtx); err != nil {
		return errors.Wrap(err, "failed to wait for DUT to become unreachable, warm reset failed")
	}
	waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
	defer cancelWaitConnect()

	if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
		return errors.Wrap(err, "failed to reconnect to the DUT")
	}
	return nil
}

func verifyElogLaunchCount(ctx context.Context, h *firmware.Helper, expectedLaunchCount int) error {
	newEvents, err := h.Reporter.EventlogList(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to find events")
	}
	foundBootModes, err := h.Reporter.GetBootModes(ctx, newEvents)
	if err != nil {
		return errors.Wrap(err, "failed to get boot modes")
	}
	diagCnt := 0
	for _, bootMode := range foundBootModes {
		if bootMode == reporters.Diagnostic {
			diagCnt++
		}
	}
	if diagCnt != expectedLaunchCount {
		return errors.Errorf("expected elog launch count is %d, but got %d", expectedLaunchCount, diagCnt)
	}
	return nil
}

func miniDiagVerifyCBMEM(ctx context.Context, h *firmware.Helper, saveLogPath string) (retErr error) {
	defer func() {
		if retErr != nil {
			if err := h.SaveCBMEMLogs(ctx, saveLogPath, reporters.ConsoleLog); err != nil {
				retErr = errors.Join(retErr, errors.Wrap(err, "failed to save firmware log"))
			}
		}
	}()
	expectedScreens := []fwCommon.FwScreenID{
		fwCommon.Diagnostics,
		fwCommon.DiagnosticsMemoryQuick,
		fwCommon.Diagnostics,
		fwCommon.DiagnosticsStorageHealth,
		fwCommon.Diagnostics,
	}
	matchFwScreens, err := h.Reporter.CheckDisplayedScreens(ctx, expectedScreens, reporters.SecondToLastBootLog)
	if err != nil {
		return errors.Wrap(err, "failed to verify firmware screen data")
	}
	if !matchFwScreens {
		return errors.New("failed to find matching firmware screen data")
	}
	out, err := h.Reporter.GetCBMEMLogs(ctx, reporters.SecondToLastBootLog)
	if err != nil {
		return errors.Wrap(err, "failed to run cbmem command")
	}
	var memoryTestSpeedRegexp *regexp.Regexp = regexp.MustCompile(`([0-9]+) ms \([0-9]+ bytes\/us\) ... \([0-9]+%\)`)
	memoryTestSpeedMatches := memoryTestSpeedRegexp.FindAllStringSubmatch(out, -1)
	if memoryTestSpeedMatches == nil {
		return errors.New("failed to find matching memory test result")
	}
	for _, match := range memoryTestSpeedMatches {
		if match[1] == "0" {
			return errors.New("memory test stuck, speed is 0 ms")
		}
	}
	return nil
}

func verifyElogTestReport(ctx context.Context, h *firmware.Helper) error {
	newEvents, err := h.Reporter.EventlogList(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to find events")
	}
	if err := h.Reporter.CheckDiagnosticsLogs(ctx, newEvents,
		[]reporters.DiagLog{
			{TypeMsg: reporters.StorageHealth, Result: reporters.MiniDiagPassed},
			{TypeMsg: reporters.MemoryQuick, Result: reporters.MiniDiagAborted},
		}); err != nil {
		return errors.Wrap(err, "failed to check for the expected diagnostics logs")
	}
	return nil
}
