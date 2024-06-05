// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
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
		// TODO: When stable, change firmware_unstable to a different attr.
		Attr:         []string{"group:firmware", "firmware_unstable"},
		Vars:         []string{"firmware.skipFlashUSB"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Fixture:      fixture.DevMode,
		Timeout:      120 * time.Minute,
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

	cleanupContext := ctx
	ctx, closeFunc := ctxutil.Shorten(ctx, 5*time.Minute)
	defer closeFunc()

	if err := h.Reporter.CrossystemSetParam(ctx, reporters.CrossystemParamDevBootSignedOnly, "1"); err != nil {
		s.Fatal("Failed to set dev_boot_signed_only=1 with crossytem: ", err)
	}
	defer func(ctx context.Context) {
		if err := h.EnsureDUTBooted(ctx); err != nil {
			s.Fatal("Failed to reconnect to DUT: ", err)
		}
		if err := h.Reporter.CrossystemSetParam(ctx, reporters.CrossystemParamDevBootSignedOnly, devBootSignedOnly); err != nil {
			s.Fatalf("Failed to set dev_boot_signed_only=%v with crossytem: %v", devBootSignedOnly, err)
		}
	}(cleanupContext)

	if err := h.Reporter.CrossystemSetParam(ctx, reporters.CrossystemParamDevBootUsb, "1"); err != nil {
		s.Fatal("Failed to set dev_boot_usb=1 with crossytem: ", err)
	}
	defer func(ctx context.Context) {
		if err := h.EnsureDUTBooted(ctx); err != nil {
			s.Fatal("Failed to reconnect to DUT: ", err)
		}
		if err := h.Reporter.CrossystemSetParam(ctx, reporters.CrossystemParamDevBootUsb, devBootUSB); err != nil {
			s.Fatalf("Failed to set dev_boot_usb=%v with crossytem: %v", devBootUSB, err)
		}
	}(cleanupContext)

	if err := developerUSBBoot(ctx, h, false); err != nil {
		s.Fatal("Failed to boot from internal disk: ", err)
	}
	usbDev, err := grepServoUSBPathOnDUT(ctx, h)
	if err != nil {
		s.Fatal("Failed to get USB path on DUT: ", err)
	}
	s.Log("Resigning KERN_A on USB with ssd key")
	if _, err := h.DUT.Conn().CommandContext(ctx,
		"/usr/share/vboot/bin/make_dev_ssd.sh",
		"--partitions", "2", // Partition ID 2 corresponds to cgpt partition "KERN_A".
		"-i", usbDev,
	).Output(ssh.DumpLogOnError); err != nil {
		s.Fatal("Failed to resign usb with ssd keys: ", err)
	}
	defer func(ctx context.Context) {
		if err := h.Servo.SetPowerState(ctx, servo.PowerStateReset); err != nil {
			s.Fatal("Failed to cold reset the DUT: ", err)
		}
		waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, h.Config.DelayRebootToPing+h.Config.FirmwareScreen)
		defer cancelWaitConnect()
		if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
			s.Fatal("Failed to reconnect to the DUT: ", err)
		}
		s.Log("Inserting the USB to DUT")
		if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxDUT); err != nil {
			s.Fatal("Failed to insert the USB to DUT: ", err)
		}
		s.Logf("Sleeping %s to let USB become visible to DUT", firmware.UsbVisibleTime)
		// GoBigSleepLint: It may take some time for usb mux state to
		// take effect.
		if err := testing.Sleep(ctx, firmware.UsbVisibleTime); err != nil {
			s.Fatalf("Failed to sleep for %v s: %v", firmware.UsbDisableTime, err)
		}
		usbDev, err := grepServoUSBPathOnDUT(ctx, h)
		if err != nil {
			s.Fatal("Failed to get USB path on DUT: ", err)
		}
		s.Log("Resigning KERN_A on USB with recovery key")
		if _, err := h.DUT.Conn().CommandContext(ctx,
			"/usr/share/vboot/bin/make_dev_ssd.sh",
			"--partitions", "2", // Partition ID 2 corresponds to cgpt partition "KERN_A".
			"-i", usbDev,
			"--recovery_key",
		).Output(ssh.DumpLogOnError); err != nil {
			s.Fatal("Failed restore recovery keys: ", err)
		}
	}(cleanupContext)

	if err := developerUSBBoot(ctx, h, true); err != nil {
		s.Fatal("Failed to boot from USB: ", err)
	}
}

func developerUSBBoot(ctx context.Context, h *firmware.Helper, expectedBootFromUSB bool) error {
	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		return errors.Wrap(err, "failed to create mode switcher")
	}
	testing.ContextLog(ctx, "Removing the USB to DUT")
	if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxOff); err != nil {
		return errors.Wrap(err, "failed to remove USB")
	}
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateReset); err != nil {
		return errors.Wrap(err, "failed to cold reset the DUT")
	}
	testing.ContextLog(ctx, "Setting DFP mode")
	if err := h.Servo.SetDUTPDDataRole(ctx, servo.DFP); err != nil {
		testing.ContextLogf(ctx, "Failed to set pd data role to DFP: %.400s", err)
	}
	testing.ContextLog(ctx, "Inserting the USB to DUT")
	if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxDUT); err != nil {
		return errors.Wrap(err, "failed to insert USB to DUT")
	}
	if expectedBootFromUSB {
		params := firmware.RunBypasser{BypasserMethod: ms.BypassDevBootUSB, RepeatBypasser: true, WaitUntilDUTConnected: h.Config.USBImageBootTimeout}
		if err := ms.RunBypasserUntilDUTConnected(ctx, params); err != nil {
			return errors.Wrap(err, "failed to transition from fw screen to usb boot")
		}
	} else {
		waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, h.Config.DelayRebootToPing+h.Config.FirmwareScreen)
		defer cancelWaitConnect()
		if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
			return errors.Wrap(err, "failed to reconnect to the DUT")
		}
	}
	testing.ContextLog(ctx, "Expecting that DUT booted from usb: ", expectedBootFromUSB)
	bootedFromRemovableDevice, err := h.Reporter.BootedFromRemovableDevice(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to determine boot device type")
	}
	if bootedFromRemovableDevice != expectedBootFromUSB {
		return errors.Errorf("expected boot from usb: %v, got %v", expectedBootFromUSB, bootedFromRemovableDevice)
	}
	return nil
}

func lsblkGrepUSBPaths(ctx context.Context, h *firmware.Helper) ([]string, error) {
	outRaw, err := h.DUT.Conn().CommandContext(ctx, "sh", "-c", "lsblk -nd --output NAME").Output(ssh.DumpLogOnError)
	if err != nil {
		return []string{}, errors.Wrap(err, "failed to run lsblk command")
	}
	var usbPathSlice []string
	usbRegex := regexp.MustCompile(`sd\w`)
	disableMatches := usbRegex.FindAllSubmatch(outRaw, -1)
	if disableMatches != nil {
		for _, match := range disableMatches {
			usbPathSlice = append(usbPathSlice, string(match[0]))
		}
	}
	return usbPathSlice, nil
}

func grepServoUSBPathOnDUT(ctx context.Context, h *firmware.Helper) (string, error) {
	testing.ContextLog(ctx, "Removing the USB")
	if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxOff); err != nil {
		return "", errors.Wrap(err, "failed to remove USB")
	}
	testing.ContextLogf(ctx, "Sleeping %s to let USB become invisible to DUT", firmware.UsbDisableTime)
	// GoBigSleepLint: It may take some time for usb mux state to take effect.
	if err := testing.Sleep(ctx, firmware.UsbDisableTime); err != nil {
		return "", errors.Wrap(err, "failed to sleep for usb disable time")
	}

	disableOutSlice, err := lsblkGrepUSBPaths(ctx, h)
	if err != nil {
		return "", errors.Wrap(err, "failed to get lsblk output for diabling USB")
	}
	disableOutString := strings.Join(disableOutSlice, " ")

	testing.ContextLog(ctx, "Enabling the USB to DUT")
	if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxDUT); err != nil {
		return "", errors.Wrap(err, "failed to enable USB to DUT")
	}
	testing.ContextLogf(ctx, "Sleeping %s to let USB become visible to DUT", firmware.UsbVisibleTime)
	// GoBigSleepLint: It may take some time for usb mux state to take effect.
	if err := testing.Sleep(ctx, firmware.UsbVisibleTime); err != nil {
		return "", errors.Wrap(err, "failed to sleep for usb visible time")
	}

	enableOutSlice, err := lsblkGrepUSBPaths(ctx, h)
	if err != nil {
		return "", errors.Wrap(err, "failed to get lsblk output for enabling USB to DUT")
	}
	enableOutString := strings.Join(enableOutSlice, " ")

	outCmp := strings.Trim(enableOutString, disableOutString)
	return fmt.Sprintf("/dev/%s", strings.TrimSpace(outCmp)), nil
}
