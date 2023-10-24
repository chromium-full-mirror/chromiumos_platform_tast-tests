// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: FwScreenPowerOff,
		Desc: "Test power off from developer and recovery screen using power button and UI menu if exists",
		Contacts: []string{
			"chromeos-faft@google.com",
			"tj@semihalf.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware"},
		Fixture:      fixture.DevMode,
		Timeout:      8 * time.Minute,
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

func FwScreenPowerOff(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servod")
	}
	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}

	// disable USB to avoid booting in recovery mode
	s.Logf("Setting USBMux to %s", servo.USBMuxOff)
	if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxOff); err != nil {
		s.Fatal("Failed to set USBMux: ", err)
	}

	for _, steps := range []struct {
		useUIMenu string
		powerOff  func(context.Context, *firmware.Helper, string) error
	}{
		{
			useUIMenu: "no",
			powerOff: func(ctx context.Context, h *firmware.Helper, mode string) error {
				s.Log("Pressing power key")
				if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.DurTab); err != nil {
					return errors.Wrap(err, "failed to press power key on DUT")
				}
				s.Log("Waiting for power state to become G3 or S5")
				if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout, "G3", "S5"); err != nil {
					return errors.Wrap(err, "failed to get G3 or S5 powerstate")
				}
				return nil
			},
		},
		{
			useUIMenu: "yes",
			powerOff: func(ctx context.Context, h *firmware.Helper, mode string) error {
				if err := powerOffFromUIMenu(ctx, h, mode); err != nil {
					return errors.Wrapf(err, "failed to power off from %s screen", mode)
				}
				return nil
			},
		},
	} {
		if switcher := h.Config.ModeSwitcherType; switcher != "menu_switcher" && switcher != "tablet_detachable_switcher" && steps.useUIMenu == "yes" {
			s.Logf("Power off from menu skipped for %s", switcher)
		} else {
			if err := h.Servo.SetPowerState(ctx, servo.PowerStateRec); err != nil {
				s.Fatal("Failed to enter recovery mode: ", err)
			}
			s.Logf("Sleeping %s (FirmwareScreen)", h.Config.FirmwareScreen)
			if err := testing.Sleep(ctx, h.Config.FirmwareScreen); err != nil {
				s.Fatal("Failed to sleep: ", err)
			}
			steps.powerOff(ctx, h, "rec")
			if err := h.Servo.SetPowerState(ctx, servo.PowerStateOn); err != nil {
				s.Fatal("Failed to restart: ", err)
			}
			s.Logf("Sleeping %s (FirmwareScreen)", h.Config.FirmwareScreen)
			if err := testing.Sleep(ctx, h.Config.FirmwareScreen); err != nil {
				s.Fatal("Failed to sleep: ", err)
			}
			s.Log("Waiting for S0 powerstate")
			if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout, "S0"); err != nil {
				s.Fatal("Failed to get S0 powerstate: ", err)
			}
			steps.powerOff(ctx, h, "dev")
		}
	}
}

func powerOffFromUIMenu(ctx context.Context, h *firmware.Helper, screen string) error {
	testing.ContextLog(ctx, "Choosing power off option from the UI menu")
	switch h.Config.ModeSwitcherType {
	case "menu_switcher":
		// Power off is the last option in the menu for both developer and recovery screen
		if err := nTimeKeyPress(ctx, h, "<down>", 7); err != nil {
			return errors.Wrap(err, "failed to type the key")
		}
	case "tablet_detachable_switcher":
		switch screen {
		case "rec":
			// Enter the menu by pressing <down> and choose power off option which is directly below the default option
			if err := nTimeKeyPress(ctx, h, "<down>", 2); err != nil {
				return errors.Wrap(err, "failed to type the key")
			}
		case "dev":
			// Power off is the default option
		}
	}
	if err := nTimeKeyPress(ctx, h, "<enter>", 1); err != nil {
		return errors.Wrap(err, "failed to type the key")
	}
	testing.ContextLog(ctx, "Waiting for power state to become G3 or S5")
	if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout, "G3", "S5"); err != nil {
		return errors.Wrap(err, "failed to get G3 or S5 powerstate")
	}
	return nil
}

func nTimeKeyPress(ctx context.Context, h *firmware.Helper, key string, n int) error {
	for i := 0; i < n; i++ {
		if err := h.Servo.ECPressKey(ctx, key, h.Model); err != nil {
			return err
		}
		if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
			return errors.Wrapf(err, "sleeping for %s (KeypressDelay)", h.Config.KeypressDelay)
		}
	}
	return nil
}
