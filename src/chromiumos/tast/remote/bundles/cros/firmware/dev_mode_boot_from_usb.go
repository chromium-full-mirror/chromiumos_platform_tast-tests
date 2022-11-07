// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"bytes"
	"context"
	"path/filepath"
	"strconv"
	"time"

	"chromiumos/tast/common/servo"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/remote/firmware"
	"chromiumos/tast/remote/firmware/fixture"
	"chromiumos/tast/ssh"
	"chromiumos/tast/ssh/linuxssh"
	"chromiumos/tast/testing"
)

// ctrluParams contain parameters relevant to this test.
type ctrluParams struct {
	validUSB         bool
	reconnectTimeout time.Duration
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         DevModeBootFromUSB,
		Desc:         "Verify the functionality of Ctrl+U while on the dev screen",
		Contacts:     []string{"cienet-firmware@cienet.corp-partner.google.com", "chromeos-firmware@google.com"},
		Attr:         []string{"group:firmware", "firmware_unstable"},
		SoftwareDeps: []string{"crossystem"},
		Vars:         []string{"firmware.skipFlashUSB"},
		Fixture:      fixture.DevMode,
		Params: []testing.Param{{
			Val: &ctrluParams{
				validUSB:         true, // Test b:200305066.
				reconnectTimeout: 10 * time.Minute,
			},
			ExtraAttr: []string{"firmware_usb"},
			Timeout:   60 * time.Minute,
		}, {
			Name: "no_usb",
			Val: &ctrluParams{
				validUSB:         false, // Test b:200305314.
				reconnectTimeout: 2 * time.Minute,
			},
			Timeout: 20 * time.Minute,
		}},
	})
}

func DevModeBootFromUSB(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}

	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		s.Fatal("Failed to create mode switcher: ", err)
	}

	s.Log("Setting dev boot usb value to 1")
	if err := h.DUT.Conn().CommandContext(ctx, "crossystem", "dev_boot_usb=1").Run(ssh.DumpLogOnError); err != nil {
		s.Fatal("Failed to set crossystem dev_boot_usb to 1: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 20*time.Second)
	defer cancel()

	// Set dev_boot_usb back to 0 at the end of the test.
	defer func(ctx context.Context) {
		if err := h.DUT.Conn().CommandContext(ctx, "crossystem", "dev_boot_usb=0").Run(ssh.DumpLogOnError); err != nil {
			s.Fatal("Failed to set crossystem dev_boot_usb to 0: ", err)
		}
	}(cleanupCtx)

	s.Log("Removing USB from the DUT")
	if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxHost); err != nil {
		s.Fatal("Failed to set USBMux: ", err)
	}

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
	}

	// Rebooting DUT would ensure previous records wiped in
	// the firmware log, and start a new one.
	if err := ms.PowerOff(ctx); err != nil {
		s.Fatal("Failed to power off dut: ", err)
	}

	if err := h.Servo.SetPowerState(ctx, servo.PowerStateOn); err != nil {
		s.Log("Failed to set powerstate to ON, retrying with power button: ", err)
		if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.DurTab); err != nil {
			s.Fatal("Failed to press power button: ", err)
		}
	}

	// Store a copy of the firmware log on the local machine,
	// which would get uploaded to Stainless for debugging purposes.
	defer func(ctx context.Context) {
		s.Log("Sending file to local machine")
		destPath := filepath.Join(s.OutDir(), "/firmwareLog")
		if err := linuxssh.GetFile(ctx, s.DUT().Conn(), "/sys/firmware/log", destPath, linuxssh.DereferenceSymlinks); err != nil {
			s.Fatal("Failed to send firmware log to local machine: ", err)
		}
	}(cleanupCtx)

	s.Logf("Sleeping %s (FirmwareScreen)", h.Config.FirmwareScreen)
	if err := testing.Sleep(ctx, h.Config.FirmwareScreen); err != nil {
		s.Fatalf("Failed to sleep for %s (FirmwareScreen): %v", h.Config.FirmwareScreen, err)
	}

	// Pressing ctrl_u should trigger a beep sound on
	// the DUT at this point.
	s.Logf("Testing shortcuts %q", servo.CtrlU)
	if err := h.Servo.KeypressWithDuration(ctx, servo.CtrlU, servo.DurTab); err != nil {
		s.Fatalf("Failed to press %s: %v", servo.CtrlU, err)
	}

	if testOpt.validUSB {
		s.Log("Connecting USB to the DUT")
		if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxDUT); err != nil {
			s.Fatal("Failed to set USBMux: ", err)
		}

		// In order to press ctrl_u sucessfully, sleep is required.
		if err := testing.Sleep(ctx, 5*time.Second); err != nil {
			s.Fatal("Failed to sleep for 5 seconds: ", err)
		}

		// Pressing ctrl_u here should boot DUT from the USB.
		s.Logf("Testing shortcuts %q", servo.CtrlU)
		if err := h.Servo.KeypressWithDuration(ctx, servo.CtrlU, servo.DurTab); err != nil {
			s.Fatalf("Failed to press %s: %v", servo.CtrlU, err)
		}
	}

	// When there's no valid usb, pressing ctrl_d would help duts
	// leave the firmware screen and continue booting to ChromeOS.
	if !testOpt.validUSB {
		s.Log("Pressing ctrl_d to leave firmware screen")
		if err := h.Servo.KeypressWithDuration(ctx, servo.CtrlD, servo.DurTab); err != nil {
			s.Fatalf("Failed to press %s: %v", servo.CtrlD, err)
		}
	}

	s.Log("Waiting for DUT to reconnect")
	waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, testOpt.reconnectTimeout)
	defer cancelWaitConnect()

	if err := h.WaitConnect(waitConnectCtx); err != nil {
		s.Fatal("Failed to reconnect to DUT: ", err)
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
	}

	firmwareLog, err := h.DUT.Conn().CommandContext(ctx, "cat", "/sys/firmware/log").Output(ssh.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed to read firmware log: ", err)
	}

	s.Log("Matching for the invalid usb message from firmware log")
	var foundMatch bool
	for _, val := range ctrlUFailMsgs {
		count := bytes.Count(firmwareLog, []byte(val))
		s.Logf("Found %d matches for: %q", count, val)
		if count == 1 {
			foundMatch = true
			break
		}
		// Based on the test's procedure, we would expect the
		// no valid usb screen to only appear once.
		if count > 1 {
			s.Fatalf("Found more than one match for %s", val)
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
