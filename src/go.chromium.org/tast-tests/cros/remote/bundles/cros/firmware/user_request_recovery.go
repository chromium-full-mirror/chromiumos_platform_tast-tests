// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"path/filepath"
	"strconv"
	"time"

	fwCommon "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: UserRequestRecovery,
		Desc: "Verify broken screen during user-request-recovery-boot, and manual recovery with usb",
		Contacts: []string{
			"chromeos-faft@google.com",
			"shchen@google.com",
			"cienet-firmware@cienet.corp-partner.google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware", "firmware_bios", "firmware_level2", "firmware_usb"},
		Requirements: []string{"sys-fw-0021-v01", "sys-fw-0024-v01", "sys-fw-0025-v01"},
		Vars:         []string{"firmware.skipFlashUSB"},
		Timeout:      120 * time.Minute,
		Params: []testing.Param{{
			Name:    "normal",
			Fixture: fixture.NormalMode,
		}, {
			Name:    "dev",
			Fixture: fixture.DevModeGBB,
		}},
	})
}

func UserRequestRecovery(ctx context.Context, s *testing.State) {
	pv := s.FixtValue().(*fixture.Value)
	h := pv.Helper
	var removeServoCharger bool

	type bootUSBTimeout struct {
		err error
	}
	bootUSBTimeoutErr := bootUSBTimeout{}
	expectedUSBBootCount := 0

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}
	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}

	hasBrokenScreen := pv.BootMode != fwCommon.BootModeDev || !h.Config.NoBrokenScreenInDev

	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		s.Fatal("Failed to create mode switcher: ", err)
	}

	skipFlashUSB := false
	if skipFlashUSBStr, ok := s.Var("firmware.skipFlashUSB"); ok {
		skipFlashUSB, err = strconv.ParseBool(skipFlashUSBStr)
		if err != nil {
			s.Fatalf("Invalid value for var firmware.skipFlashUSB: got %q, want true/false", skipFlashUSBStr)
		}
	}
	cs := s.CloudStorage()
	if skipFlashUSB {
		cs = nil
	}
	if err := h.SetupUSBKey(ctx, cs); err != nil {
		s.Fatal("USBKey not working: ", err)
	}

	if err := h.Reporter.ClearEventlog(ctx); err != nil {
		s.Fatal("Failed to clear event log: ", err)
	}

	s.Log("Powering off the USB")
	if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxOff); err != nil {
		s.Fatal("Failed to power off the USB: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Minute)
	defer cancel()

	defer func(ctx context.Context) {
		if bootUSBTimeoutErr.err != nil && hasBrokenScreen {
			if err := h.Servo.SetPowerState(ctx, servo.PowerStateReset); err != nil {
				s.Fatal("Failed to reboot the DUT: ", err)
			}
			waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
			defer cancelWaitConnect()

			if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
				s.Fatal("Failed to reconnect to the DUT: ", err)
			}
			var checkRecoveryReasons []reporters.RecoveryReason
			switch expectedUSBBootCount {
			case 1:
				checkRecoveryReasons = []reporters.RecoveryReason{reporters.RecoveryReasonUSTest, reporters.RecoveryReasonUSTest, reporters.RecoveryReasonLegacy, reporters.RecoveryReasonNotRequested}
			case 2:
				checkRecoveryReasons = []reporters.RecoveryReason{reporters.RecoveryReasonUSTest, reporters.RecoveryReasonUSTest, reporters.RecoveryReasonROManual, reporters.RecoveryReasonLegacy, reporters.RecoveryReasonNotRequested}
			default:
				s.Fatal("Unexpected count found: ", expectedUSBBootCount)
			}
			s.Log("Verifying the expected recovery reasons from event log")
			newEvents, err := h.Reporter.EventlogList(ctx)
			if err != nil {
				s.Fatal(err, "failed to find events")
			}
			if err := h.Reporter.CheckRecoveryEventsInEventLog(ctx, newEvents, checkRecoveryReasons); err != nil {
				s.Error("Failed to check event logs for recovery reasons: ", err)
			} else {
				s.Error("Found DUT booted to the broken screen")
			}
			saveEventLogPath := filepath.Join(s.OutDir(), "eventlog.txt")
			if err := h.SaveEventLog(ctx, saveEventLogPath); err != nil {
				s.Error("Failed to save event log: ", err)
			}
		}
		if err := h.EnsureDUTBooted(ctx); err != nil {
			s.Fatal("Failed to ensure DUT connected at the end of test: ", err)
		}
		s.Log("Restoring crossystem recovery_request to 0")
		if err := h.DUT.Conn().CommandContext(ctx, "crossystem", "recovery_request=0").Run(); err != nil {
			s.Error("Failed to restore crossystem recovery_request to 0: ", err)
		}
		if removeServoCharger {
			if err := h.SetDUTPower(ctx, true); err != nil {
				s.Fatal("Failed to connect charger: ", err)
			}
			waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, 90*time.Second)
			defer cancelWaitConnect()
			if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
				s.Fatal("Failed to reconnect to the DUT: ", err)
			}
		}
	}(cleanupCtx)

	s.Log("Setting crossystem recovery_request to 193")
	if err := h.DUT.Conn().CommandContext(ctx, "crossystem", "recovery_request=193").Run(); err != nil {
		s.Fatal("Failed to set crossystem recovery_request to 193: ", err)
	}

	batteryExists, err := h.CheckBatteryAvailable(ctx)
	if err != nil {
		s.Fatal("Failed to check if battery is available: ", err)
	}
	supportPDRole, err := h.Servo.IsServoTypeC(ctx)
	if err != nil {
		s.Fatal("Failed to check the connection type: ", err)
	}
	// We saw that setting servo_pd_role:snk helps some machines
	// to boot the USB in recovery mode.
	if batteryExists && supportPDRole {
		removeServoCharger = true
	}

	s.Log("Rebooting the DUT with a warm reset")
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateWarmReset); err != nil {
		s.Fatal("Failed to warm reset the DUT: ", err)
	}
	waitDisconnectCtx, cancelWaitDisconnect := context.WithTimeout(ctx, 1*time.Minute)
	defer cancelWaitDisconnect()
	if err := h.DUT.WaitUnreachable(waitDisconnectCtx); err != nil {
		s.Fatal("Failed to wait for DUT to become unreachable, warm reset failed: ", err)
	}

	if hasBrokenScreen {
		s.Log("Waiting for DUT to reach the firmware screen")
		if err := h.WaitFirmwareScreen(ctx, h.Config.FirmwareScreen); err != nil {
			s.Fatal("Failed to get to firmware screen: ", err)
		}
		if err := insertUSBInFirmwareScreen(ctx, h, removeServoCharger); err != nil {
			s.Fatal("Failed to insert USB in firmware screen: ", err)
		}
		s.Log("Checking if DUT reaches Broken Screen")
		if err := h.WaitDUTConnectDuringBootFromUSB(ctx, false); err != nil {
			s.Fatal("Failed to stay at the broken screen: ", err)
		}
		s.Log("Powering off the USB")
		if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxOff); err != nil {
			s.Fatal("Failed to power off the USB: ", err)
		}
		if removeServoCharger {
			// Sending power state command after removing charger might cause
			// cr50 uart unresponsive on board grunt. Therefore, connect the
			// charger before sending power state command.
			if err := h.SetDUTPower(ctx, true); err != nil {
				s.Fatal("Failed to remove charger: ", err)
			}
		}
		s.Log("Rebooting the DUT to recovery screen")
		if err := ms.EnableRecMode(ctx, servo.PowerStateRec, servo.USBMuxOff); err != nil {
			s.Fatal("Failed to reboot into recovery mode: ", err)
		}
	}

	s.Log("Waiting for DUT to reach the firmware screen")
	if err := h.WaitFirmwareScreen(ctx, h.Config.FirmwareScreenRecMode); err != nil {
		s.Fatal("Failed to get to firmware screen: ", err)
	}
	if err := insertUSBInFirmwareScreen(ctx, h, removeServoCharger); err != nil {
		s.Fatal("Failed to insert USB in firmware screen: ", err)
	}
	expectedUSBBootCount++
	s.Log("Checking if DUT boots from USB")
	if err := h.WaitDUTConnectDuringBootFromUSB(ctx, true); err != nil {
		if errors.As(err, &context.DeadlineExceeded) {
			bootUSBTimeoutErr.err = err
		}
		s.Fatal("Failed to boot from USB: ", err)
	}
	if err := checkRecoveryReason(ctx, h, reporters.RecoveryReasonUSTest); err != nil {
		s.Fatal("Failed to check recovery reason: ", err)
	}
	if err := checkEventlog(ctx, h, hasBrokenScreen); err != nil {
		s.Fatal("Failed to check event log: ", err)
	}
	if removeServoCharger {
		// As mentioned in b/274953387, running the remote command 'poweroff' on trogdor and
		// strongbad machines without AC connected will bring them into the hibernation
		// state, which can cause error 'EC: No data was sent from the pty'.
		if err := h.Servo.SetPowerState(ctx, servo.PowerStateOff); err != nil {
			s.Fatal("Failed to set power_state:off: ", err)
		}
		waitUnreachableCtx, cancelWaitUnreachable := context.WithTimeout(ctx, 30*time.Second)
		defer cancelWaitUnreachable()
		if err := h.DUT.WaitUnreachable(waitUnreachableCtx); err != nil {
			s.Fatal("Failed to wait for DUT to be unreachable after sending poweroff command: ", err)
		}
		if err := h.SetDUTPower(ctx, true); err != nil {
			s.Fatal("Failed to remove charger: ", err)
		}
	}
	s.Logf("Rebooting to %s mode", fwCommon.BootModeRecovery)
	if err := ms.EnableRecMode(ctx, servo.PowerStateRec, servo.USBMuxOff); err != nil {
		s.Fatal("Failed to reboot into recovery mode: ", err)
	}
	if err := insertUSBInFirmwareScreen(ctx, h, removeServoCharger); err != nil {
		s.Fatal("Failed to insert USB in firmware screen: ", err)
	}
	expectedUSBBootCount++
	s.Log("Checking if DUT boots from USB")
	if err := h.WaitDUTConnectDuringBootFromUSB(ctx, true); err != nil {
		if errors.As(err, &context.DeadlineExceeded) {
			bootUSBTimeoutErr.err = err
		}
		s.Fatal("Failed to boot from USB: ", err)
	}
	if err := checkRecoveryReason(ctx, h, reporters.RecoveryReasonROManual); err != nil {
		s.Fatal("Failed to check recovery reason: ", err)
	}

	var opts []firmware.ModeSwitchOption
	if pv.BootMode == fwCommon.BootModeDev {
		opts = append(opts, firmware.ExpectDevModeAfterReboot)
	}
	if err := ms.ModeAwareReboot(ctx, firmware.ColdReset, opts...); err != nil {
		s.Fatal("Failed to reboot the DUT: ", err)
	}
}

func insertUSBInFirmwareScreen(ctx context.Context, h *firmware.Helper, removeServoCharger bool) error {
	if removeServoCharger {
		if err := h.SetDUTPower(ctx, false); err != nil {
			return errors.Wrap(err, "failed to remove charger")
		}
	}
	testing.ContextLog(ctx, "Set DFP mode")
	if err := h.Servo.SetDUTPDDataRole(ctx, servo.DFP); err != nil {
		testing.ContextLogf(ctx, "Failed to set pd data role to DFP: %.400s", err)
	}
	testing.ContextLog(ctx, "Inserting a valid USB to DUT")
	if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxDUT); err != nil {
		return errors.Wrap(err, "failed to insert the USB to DUT")
	}
	return nil
}

func checkRecoveryReason(ctx context.Context, h *firmware.Helper, expectedRecoveryReason reporters.RecoveryReason) error {
	testing.ContextLog(ctx, "Checking if DUT is in recovery mode")
	if isRecovery, err := h.Reporter.CheckBootMode(ctx, fwCommon.BootModeRecovery); err != nil {
		return errors.Wrap(err, "failed to check current boot mode")
	} else if !isRecovery {
		return errors.New("did not find DUT in recovery mode as expected")
	}
	testing.ContextLog(ctx, "Checking if crossystem recovery_reason is ", expectedRecoveryReason)
	if isExpected, err := h.Reporter.ContainsRecoveryReason(ctx, []reporters.RecoveryReason{expectedRecoveryReason}); err != nil {
		return errors.Wrap(err, "failed to check the recovery reason")
	} else if !isExpected {
		return errors.New("did not get the expected recovery reason")
	}
	return nil
}

func checkEventlog(ctx context.Context, h *firmware.Helper, hasBrokenScreen bool) error {
	testing.ContextLog(ctx, "Verifying the expected boot modes from event log")
	newEvents, err := h.Reporter.EventlogList(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to find events")
	}
	var expectedBootModes []reporters.EventlogBootMode
	if hasBrokenScreen {
		expectedBootModes = []reporters.EventlogBootMode{reporters.BrokenScreen, reporters.ManualRecovery}
	} else {
		expectedBootModes = []reporters.EventlogBootMode{reporters.ManualRecovery}
	}
	if err := h.Reporter.CheckBootModes(ctx, newEvents, expectedBootModes); err != nil {
		return errors.Wrap(err, "failed to check for the expected boot modes")
	}
	return nil
}
