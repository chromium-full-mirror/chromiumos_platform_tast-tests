// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package platform

import (
	"context"
	"strconv"
	"time"

	fwCommon "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	fwpb "go.chromium.org/tast-tests/cros/services/cros/firmware"
	"go.chromium.org/tast/core/testing"
)

var (
	usbBootTime = testing.RegisterVarString(
		"platform.BootupTimesUSB.bootTime",
		"60.0",
		"Expected bootup time in seconds")
)

const (
	bootRetry        = 2
	waitBootTimeout  = 600 // seconds
	waitBootInterval = 1   // seconds
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         BootupTimesUSB,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Measures boot performance from USB",
		Contacts:     []string{"peep-fleet-infra-sw@google.com"},
		BugComponent: "b:1032353", // Chrome Operations > Fleet > Software > OS Fleet Automation
		Attr:         []string{"group:labqual_informational"},
		SoftwareDeps: []string{"chrome"},
		ServiceDeps:  []string{"tast.cros.arc.PerfBootService", "tast.cros.platform.BootPerfService", "tast.cros.security.BootLockboxService"},
		Vars:         []string{"servo"},
		Params: []testing.Param{{
			Name:    "usb_dev",
			Fixture: fixture.DevMode,
			Val:     fwCommon.BootModeUSBDev,
			Timeout: 20 * time.Minute,
		}},
	})
}

func BootupTimesUSB(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}

	bootTime, err := strconv.ParseFloat(usbBootTime.Value(), 8)
	if err != nil {
		s.Fatal("Failed to convert boot time: ", err)
	}
	s.Log("Boot Time for validation: ", bootTime)

	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		s.Fatal("Failed to create new boot mode switcher: ", err)
	}

	pv := s.FixtValue().(*fixture.Value)
	// Double-check that DUT starts in the right mode.
	curr, err := h.Reporter.CurrentBootMode(ctx)
	if err != nil {
		s.Fatal("Checking boot mode at beginning of test: ", err)
	}
	if curr != pv.BootMode {
		s.Logf("DUT started in boot mode %s. Setting up %s", curr, pv.BootMode)
		if err = ms.RebootToMode(ctx, pv.BootMode); err != nil {
			s.Fatalf("Failed to set up %s mode: %s", pv.BootMode, err)
		}
	}

	// Make sure correct image is on USB key
	cs := s.CloudStorage()
	if err := h.SetupUSBKey(ctx, cs); err != nil {
		s.Fatal("USBKey not working: ", err)
	}

	// Set GBB flags for dev boot from USB
	flags := fwpb.GBBFlagsState{
		Set:   []fwpb.GBBFlag{fwpb.GBBFlag_FORCE_DEV_BOOT_USB, fwpb.GBBFlag_FORCE_DEV_SWITCH_ON},
		Clear: []fwpb.GBBFlag{fwpb.GBBFlag_DEV_SCREEN_SHORT_DELAY},
	}
	if _, err := fwCommon.ClearAndSetGBBFlags(ctx, h.DUT, &flags); err != nil {
		s.Fatal("ClearAndSetGBBFlags for USB dev boot failed: ", err)
	}

	// Make USB visible to DUT and reset DUT
	s.Log("Enabling USB connection to DUT")
	if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxDUT); err != nil {
		s.Fatal("Failed to set 'usb3_mux_sel:dut_sees_usbkey': ", err)
	}
	s.Logf("Sleeping %s to let USB become visible to DUT", firmware.UsbVisibleTime)
	// GoBigSleepLint: It takes some time for usb mux state to take effect.
	if err := testing.Sleep(ctx, firmware.UsbVisibleTime); err != nil {
		s.Fatal("Failed to sleep and wait for usb mux state: ", err)
	}
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateReset); err != nil {
		s.Fatal("Failed to reset dut by servo power reset: ", err)
	}

	// Cleanup.
	defer func(ctx context.Context) {
		s.Log("Performing clean up")

		if err := h.Servo.SetPowerState(ctx, servo.PowerStateReset); err != nil {
			s.Error("Failed to reset dut by servo power reset: ", err)
		}
		if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxOff); err != nil {
			s.Fatal("Failed to set 'usb3_mux_sel:dut_sees_usbkey': ", err)
		}
	}(ctx)

	s.Logf("Sleeping %s (FirmwareScreen)", h.Config.FirmwareScreen)
	// GoBigSleepLint: Wait for firmware screen.
	if err := testing.Sleep(ctx, h.Config.FirmwareScreen); err != nil {
		s.Fatalf("Failed to sleep for %s (FirmwareScreen): %v", h.Config.FirmwareScreen, err)
	}

	// Sometimes, the sleep in waiting for the firmware screen to appear
	// might be too short for a few specific duts. Increase the number of
	// presses on ctrl_u to ensure that at least one of them is effective.
	for i := 0; i < 3; i++ {
		// Document ap states for debugging purposes.
		apPower, screenState, err := h.Servo.GetAPState(ctx)
		if err != nil {
			s.Log("Failed to get ap status: ", err)
		} else {
			s.Logf("Found ap %s and screen status %s", apPower, screenState)
		}
		s.Logf("Testing shortcuts %q", servo.CtrlU)
		if err := h.Servo.KeypressWithDuration(ctx, servo.CtrlU, servo.DurTab); err != nil {
			s.Fatalf("Failed to press %s: %v", servo.CtrlU, err)
		}

		if h.Config.ModeSwitcherType == firmware.KeyboardDevSwitcher {
			// Pressing space leads DUT to the confirmation page
			// for booting to normal mode, which helps bypass the
			// fw screen timeout, and ensures an extended stay.
			s.Log(ctx, "Pressing space key to bypass fw screen timeout")
			if err := h.Servo.PressKey(ctx, " ", servo.DurTab); err != nil {
				s.Fatal("Failed to press space: ", err)
			}
		}
		// GoBigSleepLint: Simulate a specific speed of button presses.
		if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
			s.Fatalf("Failed to sleep for %v second: %v", h.Config.KeypressDelay, err)
		}

		s.Log(ctx, "Pressing esc to return to the developer screen")
		if err := h.Servo.PressKey(ctx, "<esc>", servo.DurTab); err != nil {
			s.Fatal("Failed to press esc key: ", err)
		}

		// GoBigSleepLint: Simulate a specific speed of button presses.
		if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
			s.Fatalf("Failed to sleep for %v second: %v", h.Config.KeypressDelay, err)
		}

	}

	s.Log("Reconnecting to DUT")
	if err := h.WaitConnect(ctx); err != nil {
		s.Fatal("Failed to reconnect to DUT: ", err)
	}

	s.Log("Checking that DUT has booted from removable device")
	bootedFromRemovableDevice, err := h.Reporter.BootedFromRemovableDevice(ctx)
	if err != nil {
		s.Fatal("Failed to determine boot device type: ", err)
	}
	if !bootedFromRemovableDevice {
		s.Fatalf("DUT did not boot from the bootable device: got %v, want true", bootedFromRemovableDevice)
	}
}
