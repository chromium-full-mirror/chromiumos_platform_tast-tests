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
		},
		BugComponent: "b:750299",
		Attr:         []string{"group:mainline", "informational", "group:firmware", "firmware_unstable"},
		SoftwareDeps: []string{"crossystem", "flashrom"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		// Each of 5 reboots waits up to DelayRebootToPing (~90s) 2 times + 1 more reboot at end to reset.
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

	// Reset FWWP state to off before test end.
	testing.ContextLog(ctx, "Disable write protect at test end")
	if err := h.Servo.SetFWWPState(ctx, servo.FWWPStateOff); err != nil {
		s.Fatal("Failed to disable hardware WP: ", err)
	}
	if err := performModeAwareReboot(ctx, h); err != nil {
		s.Fatal("Failed to reboot: ", err)
	}
}

func checkWPOverReboot(ctx context.Context, h *firmware.Helper, rebootFunc func(context.Context, *firmware.Helper) error) error {
	if err := h.Servo.SetFWWPState(ctx, servo.FWWPStateOff); err != nil {
		return errors.Wrap(err, "failed to disable hardware WP")
	}

	testing.ContextLog(ctx, "Performing reboot")
	if err := rebootFunc(ctx, h); err != nil {
		return errors.Wrap(err, "failed to reboot")
	}

	if err := fwUtils.CheckCrossystemWPSW(ctx, h, 0); err != nil {
		return errors.Wrap(err, "failed to confirm WP is off")
	}

	if err := h.Servo.SetFWWPState(ctx, servo.FWWPStateOn); err != nil {
		return errors.Wrap(err, "failed to enable hardware WP")
	}

	testing.ContextLog(ctx, "Performing reboot")
	if err := rebootFunc(ctx, h); err != nil {
		return errors.Wrap(err, "failed to reboot")
	}

	if err := fwUtils.CheckCrossystemWPSW(ctx, h, 1); err != nil {
		return errors.Wrap(err, "failed to confirm WP is on")
	}

	return nil
}

func performRebootWithECReboot(ctx context.Context, h *firmware.Helper) error {
	testing.ContextLog(ctx, "Reboot with EC reboot command")
	if err := h.Servo.RunECCommand(ctx, "reboot"); err != nil {
		return errors.Wrap(err, "failed to ping EC console")
	}

	connectCtx, cancel := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
	defer cancel()
	if err := h.WaitConnect(connectCtx); err != nil {
		return errors.Wrap(err, "failed to connect to DUT")
	}
	return nil
}

func performRebootWithRebootCmd(ctx context.Context, h *firmware.Helper) error {
	cmd := h.DUT.Conn().CommandContext(ctx, "reboot")
	if err := cmd.Start(); err != nil {
		return errors.Wrap(err, "failed to reboot DUT")
	}

	connectCtx, cancel := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
	defer cancel()
	if err := h.WaitConnect(connectCtx); err != nil {
		return errors.Wrap(err, "failed to connect to DUT")
	}
	return nil
}

func performRebootWithShutdownCmd(ctx context.Context, h *firmware.Helper) error {
	cmd := h.DUT.Conn().CommandContext(ctx, "/sbin/shutdown", "-P", "now")
	if err := cmd.Start(); err != nil {
		return errors.Wrap(err, "failed to shut down DUT")
	}

	testing.ContextLog(ctx, "Check for G3 or S5 powerstate")
	if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout, "G3", "S5"); err != nil {
		return errors.Wrap(err, "failed to get G3 or S5 powerstate")
	}

	testing.ContextLog(ctx, "Power DUT back on with short press of the power button")
	if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.Dur(h.Config.HoldPwrButtonPowerOn)); err != nil {
		return errors.Wrap(err, "failed to power on DUT with short press of the power button")
	}

	connectCtx, cancel := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
	defer cancel()
	if err := h.WaitConnect(connectCtx); err != nil {
		return errors.Wrap(err, "failed to connect to DUT")
	}
	return nil
}

func performRebootWithPowerBtn(ctx context.Context, h *firmware.Helper) error {
	testing.ContextLog(ctx, "Power DUT off with long press of the power button")
	if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.Dur(h.Config.HoldPwrButtonPowerOff)); err != nil {
		return errors.Wrap(err, "failed to power on DUT with short press of the power button")
	}

	testing.ContextLog(ctx, "Check for G3 or S5 powerstate")
	if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout, "G3", "S5"); err != nil {
		return errors.Wrap(err, "failed to get G3 or S5 powerstate")
	}

	testing.ContextLog(ctx, "Power DUT back on with short press of the power button")
	if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.Dur(h.Config.HoldPwrButtonPowerOn)); err != nil {
		return errors.Wrap(err, "failed to power on DUT with short press of the power button")
	}

	connectCtx, cancel := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
	defer cancel()
	if err := h.WaitConnect(connectCtx); err != nil {
		return errors.Wrap(err, "failed to connect to DUT")
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
