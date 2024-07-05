// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	fwUtils "go.chromium.org/tast-tests/cros/remote/bundles/cros/firmware/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/errors"
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
			"cienet-firmware@cienet.corp-partner.google.com",
		},
		BugComponent: "b:750299",
		Attr:         []string{"group:mainline", "informational", "group:firmware", "firmware_unstable"},
		SoftwareDeps: []string{"crossystem", "flashrom"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Timeout:      20 * time.Minute,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Params: []testing.Param{
			{
				Name:    "normal",
				Fixture: fixture.NormalMode,
			},
			{
				Name:    "dev",
				Fixture: fixture.DevModeGBB,
			},
		},
	})
}

func WriteProtectCrossystem(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}
	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to require configs: ", err)
	}

	defer func() {
		s.Log("Disabling write protection")
		if err := h.Servo.SetFWWPState(ctx, servo.FWWPStateOff); err != nil {
			s.Fatal("Failed to disable hardware WP: ", err)
		}
		if err := performModeAwareReboot(ctx, h); err != nil {
			s.Fatal("Failed to reboot: ", err)
		}
	}()

	rebootFuncs := map[string]func(context.Context, *firmware.Helper) error{
		"mode aware reboot":        performModeAwareReboot,
		"reboot with shutdown cmd": performRebootWithShutdownCmd,
		"reboot with reboot cmd":   performRebootWithRebootCmd,
		"reboot with power key":    performRebootWithPowerBtn,
		"ec reboot":                performRebootWithECReboot,
	}

	for rebootType, rebootFunc := range rebootFuncs {
		if err := checkWPOverReboot(ctx, h, rebootFunc); err != nil {
			s.Fatalf("Failed to preserve WP over %q: %v", rebootType, err)
		}
	}
}

func checkWPOverReboot(ctx context.Context, h *firmware.Helper, rebootFunc func(context.Context, *firmware.Helper) error) error {
	if err := h.Servo.SetFWWPState(ctx, servo.FWWPStateOff); err != nil {
		return errors.Wrap(err, "failed to disable hardware WP")
	}
	if err := rebootFunc(ctx, h); err != nil {
		return errors.Wrap(err, "failed to reboot")
	}
	if err := fwUtils.CheckCrossystemWPSW(ctx, h, 0); err != nil {
		return errors.Wrap(err, "failed to confirm WP is off")
	}
	if err := h.Servo.SetFWWPState(ctx, servo.FWWPStateOn); err != nil {
		return errors.Wrap(err, "failed to enable hardware WP")
	}
	if err := rebootFunc(ctx, h); err != nil {
		return errors.Wrap(err, "failed to reboot")
	}
	if err := fwUtils.CheckCrossystemWPSW(ctx, h, 1); err != nil {
		return errors.Wrap(err, "failed to confirm WP is on")
	}
	return nil
}

func performRebootWithECReboot(ctx context.Context, h *firmware.Helper) error {
	testing.ContextLog(ctx, "Rebooting the DUT with EC reboot command")
	if err := h.Servo.RunECCommand(ctx, "reboot"); err != nil {
		return errors.Wrap(err, "failed to ping EC console")
	}
	waitDisconnectCtx, cancelWaitDisconnect := context.WithTimeout(ctx, 2*time.Minute)
	defer cancelWaitDisconnect()
	if err := h.DUT.WaitUnreachable(waitDisconnectCtx); err != nil {
		return errors.Wrap(err, "failed to wait for DUT to become unreachable")
	}

	waitConnectCtx, cancel := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
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

func performRebootWithRebootCmd(ctx context.Context, h *firmware.Helper) error {
	testing.ContextLog(ctx, "Rebooting the DUT with a reboot command in VT2")
	if err := h.DUT.Conn().CommandContext(ctx, "reboot").Run(); err != nil && !errors.As(err, &context.DeadlineExceeded) {
		return errors.Wrap(err, "failed to run reboot command")
	}
	waitDisconnectCtx, cancelWaitDisconnect := context.WithTimeout(ctx, 2*time.Minute)
	defer cancelWaitDisconnect()
	if err := h.DUT.WaitUnreachable(waitDisconnectCtx); err != nil {
		return errors.Wrap(err, "failed to wait for DUT to become unreachable")
	}

	waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
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

func performRebootWithShutdownCmd(ctx context.Context, h *firmware.Helper) error {
	testing.ContextLog(ctx, "Powering off the DUT with a shutdown command in VT2")
	if err := h.DUT.Conn().CommandContext(ctx, "shutdown", "-P", "now").Run(); err != nil && !errors.As(err, &context.DeadlineExceeded) {
		return errors.Wrap(err, "failed to run shutdown command")
	}

	testing.ContextLog(ctx, "Waiting for the G3 power state")
	if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout, "G3"); err != nil {
		return errors.Wrap(err, "failed to get G3 power state")
	}

	testing.ContextLog(ctx, "Powering on the DUT by pressing power button for ", h.Config.HoldPwrButtonPowerOn)
	if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.Dur(h.Config.HoldPwrButtonPowerOn)); err != nil {
		return errors.Wrapf(err, "failed to power on the DUT by pressing the power button for %v", h.Config.HoldPwrButtonPowerOn)
	}

	waitConnectCtx, cancel := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
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

func performRebootWithPowerBtn(ctx context.Context, h *firmware.Helper) error {
	testing.ContextLog(ctx, "Pressing the power button to power off the DUT")
	if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.Dur(h.Config.HoldPwrButtonPowerOff)); err != nil {
		return errors.Wrapf(err, "failed to power off the DUT by pressing the power button for %v", h.Config.HoldPwrButtonPowerOff)
	}

	testing.ContextLog(ctx, "Waiting for G3 power state")
	if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout, "G3"); err != nil {
		return errors.Wrap(err, "failed to get G3 power state")
	}

	testing.ContextLog(ctx, "Powering on the DUT by pressing power button for ", h.Config.HoldPwrButtonPowerOn)
	if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.Dur(h.Config.HoldPwrButtonPowerOn)); err != nil {
		return errors.Wrapf(err, "failed to power on the DUT by pressing the power button for %v", h.Config.HoldPwrButtonPowerOn)
	}

	waitConnectCtx, cancel := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
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

func performModeAwareReboot(ctx context.Context, h *firmware.Helper) error {
	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		return errors.Wrap(err, "failed to create mode switcher")
	}
	testing.ContextLog(ctx, "Performing mode aware reboot")
	if err := ms.ModeAwareReboot(ctx, firmware.ColdReset, firmware.AllowGBBForce); err != nil {
		return errors.Wrap(err, "failed to perform mode aware reboot")
	}
	return nil
}
