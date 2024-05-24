// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"strconv"
	"time"

	fwCommon "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: RecScreenUSBInserted,
		Desc: "Verify that DUT can boot from the USB inserted prior to reboot to the recovery screen",
		Contacts: []string{
			"chromeos-faft@google.com",
			"cienet-firmware@cienet.corp-partner.google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware", "firmware_bios", "firmware_level2", "firmware_usb", "firmware_ro"},
		Requirements: []string{"sys-fw-0021-v01", "sys-fw-0024-v01"},
		Vars:         []string{"firmware.skipFlashUSB"},
		Timeout:      2 * time.Hour,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Params: []testing.Param{{
			Name:    "dev",
			Fixture: fixture.DevMode,
		}, {
			Name:    "normal",
			Fixture: fixture.NormalMode,
		}},
	})
}

func RecScreenUSBInserted(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}
	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}
	skipFlashUSB := false
	if skipFlashUSBStr, ok := s.Var("firmware.skipFlashUSB"); ok {
		var err error
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
		s.Fatal("USBkey not working: ", err)
	}
	s.Log("Enabling USB connection to DUT")
	if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxDUT); err != nil {
		s.Fatal("Failed to enable the USB: ", err)
	}
	// GoBigSleepLint: It may take some time for usb mux state to
	// take effect.
	if err := testing.Sleep(ctx, firmware.UsbVisibleTime); err != nil {
		s.Fatalf("Failed to sleep for %v s: %v", firmware.UsbDisableTime, err)
	}
	s.Log("Rebooting the DUT to recovery screen")
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateRec); err != nil {
		s.Fatal("Failed to set power state to rec: ", err)
	}
	s.Log("Checking if DUT boots from the USB")
	if err := h.WaitDUTConnectDuringBootFromUSB(ctx, true); err != nil {
		s.Fatal("Failed to boot from the USB: ", err)
	}
	s.Log("Checking DUT has booted to recovery mode")
	if curr, err := h.Reporter.CurrentBootMode(ctx); err != nil {
		s.Fatal("Failed to get current boot mode: ", err)
	} else if curr != fwCommon.BootModeRecovery {
		s.Fatalf("Found unexpected boot mode: %q, wanted recovery mode", curr)
	}
}
