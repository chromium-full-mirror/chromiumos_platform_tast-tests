// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"time"

	"go.chromium.org/chromiumos/config/go/api"
	fwCommon "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	fwUtils "go.chromium.org/tast-tests/cros/remote/bundles/cros/firmware/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/framework/protocol"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: WriteProtectCrossystem,
		Desc: "Verify that enabled and disabled hardware write protect is reflected in crossystem wpsw_cur",
		Contacts: []string{
			"cros-flashrom-team@google.com",
			"tij@google.com",
		},
		BugComponent: "b:750299",
		// TODO(b/427195218): Add servo-exists + servo_state:WORKING after bug resolved.
		TestBedDeps:  []string{tbdep.ServoPresent},
		Attr:         []string{"group:firmware", "firmware_bios", "firmware_stressed", "firmware_bios_ro", "firmware_bios_rw"},
		SoftwareDeps: []string{"crossystem", "flashrom"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Timeout:      25 * time.Minute,
		Params: []testing.Param{
			{
				Name:      "normal",
				ExtraAttr: []string{"firmware_enabled", "firmware_meets_kpi"},
				Fixture:   fixture.NormalMode,
			},
			{
				Name:    "dev",
				Fixture: fixture.DevModeGBB,
			},
		},
	})
}

func isDutHasAPIdle(dutFeatures *protocol.DUTFeatures) bool {
	switch dutFeatures.GetHardware().GetHardwareFeatures().GetFormFactor().GetFormFactor() {
	case api.HardwareFeatures_FormFactor_CHROMEBOX:
		return true
	}
	return false
}

func WriteProtectCrossystem(ctx context.Context, s *testing.State) {
	pv := s.FixtValue().(*fixture.Value)
	h := pv.Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}
	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to require configs: ", err)
	}

	hasAPIdle := isDutHasAPIdle(s.Features(""))

	cleanupContext := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Minute)
	defer cancel()
	defer func(ctx context.Context) {
		// Add another h.WaitConnect() to check if the DUT takes longer to reboot than expected.
		testing.ContextLog(ctx, "Waiting longer to see if the DUT can reconnect")
		waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, 5*time.Minute)
		defer cancelWaitConnect()
		if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
			s.Error("DUT failed to reconnect even after waiting externally: ", err)
		}
		s.Log("Disabling write protection")
		if err := h.Servo.SetFWWPState(ctx, servo.FWWPStateForceOff); err != nil {
			s.Fatal("Failed to disable hardware WP: ", err)
		}
		s.Log("Rebooting DUT to ensure hardware WP disabled")
		if err := h.Servo.SetPowerState(ctx, servo.PowerStateReset); err != nil {
			s.Fatal("Faild to reset DUT: ", err)
		}
		if err := h.WaitConnect(ctx, firmware.ResetEthernetDongle); err != nil {
			s.Fatal("Failed to reconnect to DUT: ", err)
		}
	}(cleanupContext)

	rebootFuncs := map[string]func(context.Context, *firmware.Helper, fwCommon.BootMode, bool) error{
		"mode aware reboot":        performModeAwareReboot,
		"reboot with shutdown cmd": performRebootWithShutdownCmd,
		"reboot with reboot cmd":   performRebootWithRebootCmd,
		"reboot with power key":    performRebootWithPowerBtn,
		"ec reboot":                performRebootWithECReboot,
	}

	for rebootType, rebootFunc := range rebootFuncs {
		if err := checkWPOverReboot(ctx, h, rebootFunc, pv.BootMode, hasAPIdle); err != nil {
			s.Fatalf("Failed to preserve WP over %q: %v", rebootType, err)
		}
	}
}

func checkWPOverReboot(ctx context.Context, h *firmware.Helper, rebootFunc func(context.Context, *firmware.Helper, fwCommon.BootMode, bool) error, fromMode fwCommon.BootMode, DutHasAPIdle bool) error {
	if err := h.Servo.SetFWWPState(ctx, servo.FWWPStateForceOff); err != nil {
		return errors.Wrap(err, "failed to disable hardware WP")
	}
	if err := rebootFunc(ctx, h, fromMode, DutHasAPIdle); err != nil {
		return errors.Wrap(err, "failed to reboot")
	}
	if _, err := waitForTi50Reset(ctx, h, h.Config.DelayRebootToPing, false); err != nil {
		return errors.Wrap(err, "failed to recover from ti50 reset")
	}
	if err := fwUtils.CheckCrossystemWPSW(ctx, h, 0); err != nil {
		return errors.Wrap(err, "failed to confirm WP is off")
	}
	if err := h.Servo.SetFWWPState(ctx, servo.FWWPStateForceOn); err != nil {
		return errors.Wrap(err, "failed to enable hardware WP")
	}
	if err := rebootFunc(ctx, h, fromMode, DutHasAPIdle); err != nil {
		return errors.Wrap(err, "failed to reboot")
	}
	if _, err := waitForTi50Reset(ctx, h, h.Config.DelayRebootToPing, false); err != nil {
		return errors.Wrap(err, "failed to recover from ti50 reset")
	}
	if err := fwUtils.CheckCrossystemWPSW(ctx, h, 1); err != nil {
		return errors.Wrap(err, "failed to confirm WP is on")
	}
	return nil
}

func performRebootWithECReboot(ctx context.Context, h *firmware.Helper, fromMode fwCommon.BootMode, DutHasAPIdle bool) error {
	testing.ContextLog(ctx, "Rebooting the DUT with EC reboot command")
	if err := h.Servo.RunECCommand(ctx, "reboot"); err != nil {
		return errors.Wrap(err, "failed to ping EC console")
	}
	waitDisconnectCtx, cancelWaitDisconnect := context.WithTimeout(ctx, 2*time.Minute)
	defer cancelWaitDisconnect()
	if err := h.DUT.WaitUnreachable(waitDisconnectCtx); err != nil {
		return errors.Wrap(err, "failed to wait for DUT to become unreachable")
	}

	reconnectTimeout := h.Config.DelayRebootToPing
	if fromMode == fwCommon.BootModeDev {
		reconnectTimeout += firmware.DevScreenShortDelay + h.Config.FirmwareScreen
	}

	waitConnectCtx, cancel := context.WithTimeout(ctx, reconnectTimeout)
	defer cancel()
	if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
		currPowerState, stateErr := h.Servo.GetECSystemPowerState(ctx)
		if stateErr != nil {
			return errors.Wrap(stateErr, "failed to reconnect to DUT, failed to check powerstate")
		}
		return errors.Wrapf(err, "failed to reconnect to DUT, got power state: %v", currPowerState)
	}
	return nil
}

func performRebootWithRebootCmd(ctx context.Context, h *firmware.Helper, fromMode fwCommon.BootMode, DutHasAPIdle bool) error {
	testing.ContextLog(ctx, "Rebooting the DUT with a reboot command in VT2")
	if err := h.DUT.Conn().CommandContext(ctx, "reboot").Run(); err != nil && !errors.As(err, &context.DeadlineExceeded) {
		return errors.Wrap(err, "failed to run reboot command")
	}
	waitDisconnectCtx, cancelWaitDisconnect := context.WithTimeout(ctx, 2*time.Minute)
	defer cancelWaitDisconnect()
	if err := h.DUT.WaitUnreachable(waitDisconnectCtx); err != nil {
		return errors.Wrap(err, "failed to wait for DUT to become unreachable")
	}

	reconnectTimeout := h.Config.DelayRebootToPing
	if fromMode == fwCommon.BootModeDev {
		reconnectTimeout += firmware.DevScreenShortDelay + h.Config.FirmwareScreen
	}

	waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, reconnectTimeout)
	defer cancelWaitConnect()
	if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
		currPowerState, stateErr := h.Servo.GetECSystemPowerState(ctx)
		if stateErr != nil {
			return errors.Wrap(stateErr, "failed to reconnect to DUT, failed to check powerstate")
		}
		return errors.Wrapf(err, "failed to reconnect to DUT, got power state: %v", currPowerState)
	}
	return nil
}

func performRebootWithShutdownCmd(ctx context.Context, h *firmware.Helper, fromMode fwCommon.BootMode, DutHasAPIdle bool) error {
	testing.ContextLog(ctx, "Powering off the DUT with a shutdown command in VT2")
	if err := h.DUT.Conn().CommandContext(ctx, "shutdown", "-P", "now").Run(); err != nil && !errors.As(err, &context.DeadlineExceeded) {
		return errors.Wrap(err, "failed to run shutdown command")
	}

	if gscReset, err := waitForTi50Reset(ctx, h, h.Config.DelayRebootToPing, DutHasAPIdle); err != nil {
		return errors.Wrap(err, "failed to recover from ti50 reset")
	} else if gscReset {
		// If Ti50 resets to do AP RO verification, run the shutdown command again.
		testing.ContextLog(ctx, "Run shutdown again to power off the DUT after Ti50 reset")
		if err := h.DUT.Conn().CommandContext(ctx, "shutdown", "-P", "now").Run(); err != nil && !errors.As(err, &context.DeadlineExceeded) {
			return errors.Wrap(err, "failed to run shutdown command")
		}
	}

	testing.ContextLog(ctx, "Waiting for the G3 power state")
	if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout, "G3"); err != nil {
		return errors.Wrap(err, "failed to get G3 power state")
	}

	testing.ContextLog(ctx, "Powering on the DUT by pressing power button for ", h.Config.HoldPwrButtonPowerOn)
	if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.Dur(h.Config.HoldPwrButtonPowerOn)); err != nil {
		return errors.Wrapf(err, "failed to power on the DUT by pressing the power button for %v", h.Config.HoldPwrButtonPowerOn)
	}

	reconnectTimeout := h.Config.DelayRebootToPing
	if fromMode == fwCommon.BootModeDev {
		reconnectTimeout += firmware.DevScreenShortDelay + h.Config.FirmwareScreen
	}

	waitConnectCtx, cancel := context.WithTimeout(ctx, reconnectTimeout)
	defer cancel()
	if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
		currPowerState, stateErr := h.Servo.GetECSystemPowerState(ctx)
		if stateErr != nil {
			return errors.Wrap(stateErr, "failed to reconnect to DUT, failed to check powerstate")
		}
		return errors.Wrapf(err, "failed to reconnect to DUT, got power state: %v", currPowerState)
	}
	return nil
}

func performRebootWithPowerBtn(ctx context.Context, h *firmware.Helper, fromMode fwCommon.BootMode, DutHasAPIdle bool) error {
	testing.ContextLog(ctx, "Pressing the power button to power off the DUT")
	if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.Dur(h.Config.HoldPwrButtonPowerOff)); err != nil {
		return errors.Wrapf(err, "failed to power off the DUT by pressing the power button for %v", h.Config.HoldPwrButtonPowerOff)
	}

	if gscReset, err := waitForTi50Reset(ctx, h, h.Config.DelayRebootToPing, DutHasAPIdle); err != nil {
		return errors.Wrap(err, "failed to recover from ti50 reset")
	} else if gscReset {
		// If Ti50 resets to do AP RO verification, run the power button press to turn off the DUT again.
		testing.ContextLog(ctx, "Pressing the power button to power off the DUT after Ti50 reset")
		if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.Dur(h.Config.HoldPwrButtonPowerOff)); err != nil {
			return errors.Wrapf(err, "failed to power off the DUT by pressing the power button for %v", h.Config.HoldPwrButtonPowerOff)
		}
	}

	testing.ContextLog(ctx, "Waiting for G3 power state")
	if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout, "G3"); err != nil {
		return errors.Wrap(err, "failed to get G3 power state")
	}

	testing.ContextLog(ctx, "Powering on the DUT by pressing power button for ", h.Config.HoldPwrButtonPowerOn)
	if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.Dur(h.Config.HoldPwrButtonPowerOn)); err != nil {
		return errors.Wrapf(err, "failed to power on the DUT by pressing the power button for %v", h.Config.HoldPwrButtonPowerOn)
	}

	reconnectTimeout := h.Config.DelayRebootToPing
	if fromMode == fwCommon.BootModeDev {
		reconnectTimeout += firmware.DevScreenShortDelay + h.Config.FirmwareScreen
	}

	waitConnectCtx, cancel := context.WithTimeout(ctx, reconnectTimeout)
	defer cancel()
	if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
		currPowerState, stateErr := h.Servo.GetECSystemPowerState(ctx)
		if stateErr != nil {
			return errors.Wrap(stateErr, "failed to reconnect to DUT, failed to check powerstate")
		}
		return errors.Wrapf(err, "failed to reconnect to DUT, got power state: %v", currPowerState)
	}
	return nil
}

func performModeAwareReboot(ctx context.Context, h *firmware.Helper, fromMode fwCommon.BootMode, DutHasAPIdle bool) error {
	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		return errors.Wrap(err, "failed to create mode switcher")
	}
	var opts []firmware.ModeSwitchOption
	if fromMode == fwCommon.BootModeDev {
		opts = append(opts, firmware.AllowGBBForce)
	}
	testing.ContextLog(ctx, "Performing mode aware reboot")
	if err := ms.ModeAwareReboot(ctx, firmware.ColdReset, opts...); err != nil {
		return errors.Wrap(err, "failed to perform mode aware reboot")
	}
	return nil
}

// waitForTi50Reset waits for Ti50 to reset. Return true if GSC reset. False if it didn't
func waitForTi50Reset(ctx context.Context, h *firmware.Helper, reconnectTimeout time.Duration, pressPowerBTN bool) (bool, error) {
	// Don't do anything if the board isn't running a Ti50 image that resets
	// after WP is enabled.
	if !h.Servo.ExpectTi50WPEventReboot(ctx) {
		return false, nil
	}

	h.Servo.WaitForGSCReset(ctx, 10*time.Second)

	if pressPowerBTN {
		testing.ContextLog(ctx, "The DUT is off for AP_IDLE")
		// wait 5s for EC_RST released and EC finished init
		if err := testing.Sleep(ctx, 5*time.Second); err != nil {
			return false, errors.Wrap(err, "failed to sleep")
		}
		testing.ContextLog(ctx, "Powering on the DUT by pressing power button for ", h.Config.HoldPwrButtonPowerOn)
		h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.Dur(h.Config.HoldPwrButtonPowerOn))
	}

	waitConnectCtx, cancel := context.WithTimeout(ctx, reconnectTimeout)
	defer cancel()
	if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
		currPowerState, stateErr := h.Servo.GetECSystemPowerState(ctx)
		if stateErr != nil {
			return false, errors.Wrap(stateErr, "failed to reconnect to DUT, failed to check powerstate")
		}
		return false, errors.Wrapf(err, "failed to reconnect to DUT, got power state: %v", currPowerState)
	}
	return true, nil
}
