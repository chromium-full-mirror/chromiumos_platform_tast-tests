// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: SelfSignedBoot,
		Desc: "Verifies that the crossystem flag dev_boot_signed_only works both when enabled and disabled",
		Contacts: []string{
			"chromeos-faft@google.com",
			"tij@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware", "firmware_unstable"},
		Vars:         []string{"firmware.skipFlashUSB"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Fixture:      fixture.DevModeGBB,
		Timeout:      10 * time.Minute,
	})
}

func SelfSignedBoot(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}
	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to get config: ", err)
	}
	if err := h.Servo.RemoveCCDWatchdogs(ctx); err != nil {
		s.Fatal("Failed to remove ccd watchdog: ", err)
	}

	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		s.Fatal("Creating mode switcher: ", err)
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
		s.Fatal("USBKey not working: ", err)
	}
	if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxDUT); err != nil {
		s.Fatal("Failed to set 'usb3_mux_sel:dut_sees_usbkey': ", err)
	}

	devBootUSB, err := h.Reporter.CrossystemParam(ctx, reporters.CrossystemParamDevBootUsb)
	if err != nil {
		s.Fatal("Failed to get dev_boot_usb from crossytem: ", err)
	}
	s.Log("Initial dev_boot_usb value = ", devBootUSB)

	devBootSignedOnly, err := h.Reporter.CrossystemParam(ctx, reporters.CrossystemParamDevBootSignedOnly)
	if err != nil {
		s.Fatal("Failed to get dev_boot_signed_only from crossytem: ", err)
	}
	s.Log("Initial dev_boot_signed_only value = ", devBootSignedOnly)

	getUSBDev := func(ctx context.Context) string {
		if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxDUT); err != nil {
			s.Fatal("Failed to set 'usb3_mux_sel:dut_sees_usbkey': ", err)
		}

		outRaw, err := h.DUT.Conn().CommandContext(ctx, "sh", "-c", "lsblk -nd --output NAME | grep sd").Output(ssh.DumpLogOnError)
		if err != nil {
			s.Fatal("Failed find usb key: ", err)
		}

		out := strings.TrimSpace(string(outRaw))
		if out == "" {
			s.Fatal("No USB key detected: ", err)
		}
		return fmt.Sprintf("/dev/%s", out)
	}

	cleanupContext := ctx
	ctx, closeFunc := ctxutil.Shorten(ctx, 5*time.Minute)
	defer closeFunc()

	if err := h.Reporter.CrossystemSetParam(ctx, reporters.CrossystemParamDevBootSignedOnly, "1"); err != nil {
		s.Fatal("Failed to set dev_boot_signed_only=1 with crossytem: ", err)
	}
	defer func(ctx context.Context) {
		if err := h.Reporter.CrossystemSetParam(ctx, reporters.CrossystemParamDevBootSignedOnly, devBootSignedOnly); err != nil {
			s.Fatalf("Failed to set dev_boot_signed_only=%v with crossytem: %v", devBootSignedOnly, err)
		}
	}(cleanupContext)

	if err := h.Reporter.CrossystemSetParam(ctx, reporters.CrossystemParamDevBootUsb, "1"); err != nil {
		s.Fatal("Failed to set dev_boot_usb=1 with crossytem: ", err)
	}
	defer func(ctx context.Context) {
		if err := h.Reporter.CrossystemSetParam(ctx, reporters.CrossystemParamDevBootUsb, devBootUSB); err != nil {
			s.Fatalf("Failed to set dev_boot_usb=%v with crossytem: %v", devBootUSB, err)
		}
	}(cleanupContext)

	if err := ms.ModeAwareReboot(ctx, firmware.ColdReset); err != nil {
		s.Fatal("Failed to reboot: ", err)
	}

	s.Log("Checking that DUT didn't unexpectedly boot from usb")
	bootedFromRemovableDevice, err := h.Reporter.BootedFromRemovableDevice(ctx)
	if err != nil {
		s.Fatal("Could not determine boot device type: ", err)
	}
	if bootedFromRemovableDevice {
		s.Fatalf("DUT did not boot from the internal device: got %v, want false", bootedFromRemovableDevice)
	}

	s.Log("Resigning KERN_A on USB with ssd key")
	if _, err := h.DUT.Conn().CommandContext(ctx,
		"/usr/share/vboot/bin/make_dev_ssd.sh",
		"--partitions", "2", // Partition ID 2 corresponds to cgpt partition "KERN_A".
		"-i", getUSBDev(ctx),
	).Output(ssh.DumpLogOnError); err != nil {
		s.Fatal("Failed to resign usb with ssd keys: ", err)
	}
	defer func(ctx context.Context) {
		if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxDUT); err != nil {
			s.Fatal("Failed to mux usb to dut: ", err)
		}

		s.Log("Resigning KERN_A on USB with recovery key")
		if _, err := h.DUT.Conn().CommandContext(ctx,
			"/usr/share/vboot/bin/make_dev_ssd.sh",
			"--partitions", "2", // Partition ID 2 corresponds to cgpt partition "KERN_A".
			"-i", getUSBDev(ctx),
			"--recovery_key",
		).Output(ssh.DumpLogOnError); err != nil {
			s.Fatal("Failed restore recovery keys: ", err)
		}
	}(cleanupContext)

	// Not using ms.RebootToMode(ctx, fwCommon.BootModeUSBDev) here as it messes with the crossystem params.
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateReset); err != nil {
		s.Fatal("Failed to do power state reset: ", err)
	}
	params := firmware.RunBypasser{BypasserMethod: ms.BypassDevBootUSB, RepeatBypasser: true, WaitUntilDUTConnected: h.Config.DelayRebootToPing}
	if err := ms.RunBypasserUntilDUTConnected(ctx, params); err != nil {
		s.Fatal("Failed to transition from fw screen to usb boot: ", err)
	}
	defer func(ctx context.Context) {
		s.Log("Rebooting to disk")
		if err := h.Servo.SetPowerState(ctx, servo.PowerStateReset); err != nil {
			s.Fatal("Failed to do power state reset: ", err)
		}

		if err := h.EnsureDUTBooted(ctx); err != nil {
			s.Fatal("Failed to connect to DUT: ", err)
		}

		bootedFromRemovableDevice, err = h.Reporter.BootedFromRemovableDevice(ctx)
		if err != nil {
			s.Fatal("Could not determine boot device type: ", err)
		}
		if bootedFromRemovableDevice {
			s.Fatalf("DUT did not boot from the internal device: got %v, want false", bootedFromRemovableDevice)
		}
	}(cleanupContext)

	s.Log("Checking that DUT booted from usb")
	bootedFromRemovableDevice, err = h.Reporter.BootedFromRemovableDevice(ctx)
	if err != nil {
		s.Fatal("Could not determine boot device type: ", err)
	}
	if !bootedFromRemovableDevice {
		s.Fatalf("DUT did not boot from usb: got %v, want true", bootedFromRemovableDevice)
	}
}
