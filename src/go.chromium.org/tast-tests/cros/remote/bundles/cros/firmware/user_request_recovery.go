// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"time"

	fwCommon "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"
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
		// TODO: When stable, change firmware_unstable to a different attr.
		Attr:    []string{"group:firmware", "firmware_unstable", "firmware_usb"},
		Timeout: 120 * time.Minute,
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
	hasBrokenScreen := pv.BootMode != fwCommon.BootModeDev || !h.Config.NoBrokenScreenInDev

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}
	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}
	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		s.Fatal("Failed to create mode switcher: ", err)
	}

	cs := s.CloudStorage()
	if err := h.SetupUSBKey(ctx, cs); err != nil {
		s.Fatal("USBKey not working: ", err)
	}

	if err := h.ClearEventlog(ctx); err != nil {
		s.Fatal("Failed to clear event log: ", err)
	}

	s.Log("Powering off the USB")
	if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxOff); err != nil {
		s.Fatal("Failed to power off the USB: ", err)
	}

	s.Log("Setting crossystem recovery_request to 193")
	if err := h.DUT.Conn().CommandContext(ctx, "crossystem", "recovery_request=193").Run(); err != nil {
		s.Fatal("Failed to set crossystem recovery_request to 193: ", err)
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
		if err := insertUSBInFirmwareScreen(ctx, h); err != nil {
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
		s.Log("Rebooting the DUT to recovery screen")
		if err := h.Servo.SetPowerState(ctx, servo.PowerStateRec); err != nil {
			s.Fatal("Failed to boot to recovery screen: ", err)
		}
	}

	if err := insertUSBInFirmwareScreen(ctx, h); err != nil {
		s.Fatal("Failed to insert USB in firmware screen: ", err)
	}
	s.Log("Checking if DUT boots from USB")
	if err := h.WaitDUTConnectDuringBootFromUSB(ctx, true); err != nil {
		s.Fatal("Failed to boot from USB: ", err)
	}
	if err := checkRecoveryReason(ctx, h, reporters.RecoveryReasonUSTest); err != nil {
		s.Fatal("Failed to check recovery reason: ", err)
	}
	if err := checkEventlog(ctx, h, hasBrokenScreen); err != nil {
		s.Fatal("Failed to check event log: ", err)
	}

	s.Logf("Rebooting to %s mode", fwCommon.BootModeRecovery)
	if err := ms.EnableRecMode(ctx, servo.PowerStateRec, servo.USBMuxDUT); err != nil {
		s.Fatal("Failed to reboot into recovery mode: ", err)
	}
	s.Log("Checking if DUT boots from USB")
	if err := h.WaitDUTConnectDuringBootFromUSB(ctx, true); err != nil {
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

func insertUSBInFirmwareScreen(ctx context.Context, h *firmware.Helper) error {
	testing.ContextLogf(ctx, "Sleeping for %s (FirmwareScreen)", h.Config.FirmwareScreen)
	// GoBigSleepLint: Sleep for model specific time.
	if err := testing.Sleep(ctx, h.Config.FirmwareScreen); err != nil {
		return errors.Wrap(err, "failed to sleep")
	}
	testing.ContextLog(ctx, "Set DFP mode")
	if err := h.Servo.SetDUTPDDataRole(ctx, servo.DFP); err != nil {
		testing.ContextLogf(ctx, "Failed to set pd data role to DFP: %s", err)
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
	testing.ContextLogf(ctx, "Checking if crossystem recovery_reason is %s", expectedRecoveryReason)
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
