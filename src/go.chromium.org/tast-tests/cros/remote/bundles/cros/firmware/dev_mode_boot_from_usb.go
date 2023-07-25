// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"bytes"
	"context"
	"io/ioutil"
	"path/filepath"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
)

// ctrluParams contain parameters relevant to this test.
type ctrluParams struct {
	validUSB bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         DevModeBootFromUSB,
		Desc:         "Verify the functionality of Ctrl+U while on the dev screen",
		Contacts:     []string{"chromeos-faft@google.com", "cienet-firmware@cienet.corp-partner.google.com"},
		BugComponent: "b:792402",
		// TODO: When stable, change firmware_unstable to a different attr and add linto@chromium.org to gerrit review.
		Attr:         []string{"group:firmware", "firmware_unstable"},
		SoftwareDeps: []string{"crossystem"},
		Vars:         []string{"firmware.skipFlashUSB"},
		Fixture:      fixture.DevMode,
		Params: []testing.Param{{
			Val: &ctrluParams{
				validUSB: true, // Test b:200305066.
			},
			ExtraAttr: []string{"firmware_usb"},
			Timeout:   2 * time.Hour,
		}, {
			Name: "no_usb",
			Val: &ctrluParams{
				validUSB: false, // Test b:200305314.
			},
			Timeout: 30 * time.Minute,
		}},
	})
}

func DevModeBootFromUSB(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}

	s.Log("Setting dev boot usb value to 1")
	if err := h.DUT.Conn().CommandContext(ctx, "crossystem", "dev_boot_usb=1").Run(ssh.DumpLogOnError); err != nil {
		s.Fatal("Failed to set crossystem dev_boot_usb to 1: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 4*time.Minute)
	defer cancel()

	// Set dev_boot_usb back to 0 at the end of the test.
	defer func(ctx context.Context) {
		if err := h.EnsureDUTBooted(ctx); err != nil {
			s.Fatal("Failed to reconnect to dut: ", err)
		}
		if err := h.DUT.Conn().CommandContext(ctx, "crossystem", "dev_boot_usb=0").Run(ssh.DumpLogOnError); err != nil {
			s.Fatal("Failed to set crossystem dev_boot_usb to 0: ", err)
		}
	}(cleanupCtx)

	s.Log("Removing USB from the DUT")
	if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxHost); err != nil {
		s.Fatal("Failed to set USBMux: ", err)
	}

	var reconnectTimeout time.Duration
	testOpt := s.Param().(*ctrluParams)
	if testOpt.validUSB {
		s.Log("Setting up the USB key")
		skipFlashUSB := false
		if skipFlashUSBStr, ok := s.Var("firmware.skipFlashUSB"); ok {
			var err error
			skipFlashUSB, err = strconv.ParseBool(skipFlashUSBStr)
			if err != nil {
				s.Fatalf("Invalid value for var firmware.skipFlashUSB: got %q, want true/false", skipFlashUSBStr)
			}
		}
		var cs *testing.CloudStorage
		if !skipFlashUSB {
			cs = s.CloudStorage()
		}
		if err := h.SetupUSBKey(ctx, cs); err != nil {
			s.Fatal("USBKey not working: ", err)
		}
		reconnectTimeout = h.Config.USBImageBootTimeout
	} else {
		reconnectTimeout = h.Config.DelayRebootToPing
	}

	h.CloseRPCConnection(ctx)
	s.Log("Rebooting the DUT with cold reset")
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateReset); err != nil {
		s.Fatal("Failed to reboot the DUT with cold reset: ", err)
	}

	// Store a copy of the firmware log on the local machine,
	// which would get uploaded to Stainless for debugging purposes.
	defer func(ctx context.Context) {
		if h.DUT.Connected(ctx) && s.HasError() {
			output, err := h.Reporter.CatFile(ctx, "/sys/firmware/log")
			if err != nil {
				s.Fatal("Failed to read firmware log: ", err)
			}
			destPath := filepath.Join(s.OutDir(), "firmware.log")
			if err := ioutil.WriteFile(destPath, []byte(output), 0666); err != nil {
				s.Fatal("Failed to write firmware log: ", err)
			}
		}
	}(cleanupCtx)

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

	/*
		 Documented below are behaviors seen on DUTs with various boot mode
		 methods, when ctrl_u was pressed, with an invalid USB and on the dev
		 firmware screen.
		 MenuSwitcher:
			DUT would go to a separate page that prevents boot and
			doesn't timeout.
		 TabletDetachableSwitcher:
			Counter for 30 secs timeout is reset. Connecting usb and
			pressing ctrlU again within 30 secs would allow the dut to
			boot from external device.
		 KeyboardDevSwitcher:
			 Counter for the 30 secs timeout was not reset, and once reached,
			the dut would continue to boot from main if no valid usb found.
	*/
	if testOpt.validUSB {
		if h.Config.ModeSwitcherType == firmware.KeyboardDevSwitcher {
			// Pressing space would lead DUT to the confirmation page
			// for booting to normal mode, which would buy us some time
			// to connect a valid usb, and bypass the fw screen timeout.
			s.Log(ctx, "Pressing space key to bypass fw screen timeout")
			if err := h.Servo.PressKey(ctx, " ", servo.DurTab); err != nil {
				s.Fatal("Failed to press space: ", err)
			}
		}

		s.Log("Connecting USB to the DUT")
		if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxDUT); err != nil {
			s.Fatal("Failed to set USBMux: ", err)
		}

		// GoBigSleepLint: Wait for a short delay here because the dut might not
		// immediately see the USB device when it's connected.
		if err := testing.Sleep(ctx, 5*time.Second); err != nil {
			s.Fatal("Failed to sleep for 5 seconds: ", err)
		}

		if h.Config.ModeSwitcherType == firmware.KeyboardDevSwitcher {
			s.Log(ctx, "Pressing esc to return to the developer screen")
			if err := h.Servo.PressKey(ctx, "<esc>", servo.DurTab); err != nil {
				s.Fatal("Failed to press esc key: ", err)
			}
		}

		apPower, screenState, err := h.Servo.GetAPState(ctx)
		if err != nil {
			s.Log("Failed to get ap status: ", err)
		} else {
			s.Logf("Found ap %s and screen status %s", apPower, screenState)
		}

		// Pressing ctrl_u here should boot DUT from the USB.
		s.Logf("Testing shortcuts %q", servo.CtrlU)
		if err := h.Servo.KeypressWithDuration(ctx, servo.CtrlU, servo.DurTab); err != nil {
			s.Fatalf("Failed to press %s: %v", servo.CtrlU, err)
		}
	}

	waitConnectOpt := []firmware.WaitConnectOption{firmware.ResetEthernetDongle}
	// When there's no valid usb, pressing ctrl_d would help duts
	// leave the firmware screen and continue booting to ChromeOS.
	if !testOpt.validUSB {
		s.Log("Pressing ctrl_d to leave firmware screen")
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			if err := h.Servo.KeypressWithDuration(ctx, servo.CtrlD, servo.DurTab); err != nil {
				return errors.Wrapf(err, "failed to press %s", servo.CtrlD)
			}
			ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			if err := h.WaitConnect(ctx, waitConnectOpt...); err != nil {
				waitConnectOpt = nil
				return errors.Wrap(err, "failed to connect to dut")
			}
			return nil
		}, &testing.PollOptions{Timeout: reconnectTimeout}); err != nil {
			s.Fatal("Failed to reconnect to dut after pressing ctrl d: ", err)
		}
	} else {
		s.Log("Waiting for DUT to reconnect")
		waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, reconnectTimeout)
		defer cancelWaitConnect()

		if err := h.WaitConnect(waitConnectCtx, waitConnectOpt...); err != nil {
			s.Fatal("Failed to reconnect to DUT: ", err)
		}
	}
	// ctrlUFailMsgs contain possible strings found in the firmware log
	// when pressing ctrl_u to boot from usb fails. When tested manually,
	// this would be signaled by a beep sound.
	ctrlUFailMsgs := []string{
		"External boot is disabled",
		"no kernel found on USB",
		"No bootable kernel found on USB/SD",
		"No external disk found",
		"USB booting is disabled",
		"Invalid external disk in dev mode",
	}

	firmwareLog, err := h.DUT.Conn().CommandContext(ctx, "cbmem", "-1").Output(ssh.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed to read firmware log: ", err)
	}

	s.Log("Matching for the invalid usb message from firmware log")
	var foundMatch bool
	for _, val := range ctrlUFailMsgs {
		count := bytes.Count(firmwareLog, []byte(val))
		s.Logf("Found %d matches for: %q", count, val)
		if count > 0 {
			foundMatch = true
			break
		}
	}
	if !foundMatch {
		s.Fatal("Did not find matches in firmware log for having an invalid usb")
	}

	s.Log("Checking that DUT has booted from expected source")
	bootedFromRemovableDevice, err := h.Reporter.BootedFromRemovableDevice(ctx)
	if err != nil {
		s.Fatal("Could not determine boot device type: ", err)
	}

	if testOpt.validUSB != bootedFromRemovableDevice {
		s.Fatalf("Expected dut to boot from USB: %v, but got: %v", testOpt.validUSB, bootedFromRemovableDevice)
	}
}
