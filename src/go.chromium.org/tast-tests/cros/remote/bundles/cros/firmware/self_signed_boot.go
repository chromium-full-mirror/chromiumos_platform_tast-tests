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
	"go.chromium.org/tast-tests/cros/common/tbdep"
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
		TestBedDeps:  append([]string{tbdep.ServoUSBState("NORMAL")}, tbdep.ServoPresentAndWorking...),
		Attr:         []string{"group:firmware", "firmware_bios", "firmware_enabled", "firmware_meets_kpi", "firmware_stressed", "firmware_bios_ro", "firmware_bios_rw"},
		Vars:         []string{"firmware.skipFlashUSB"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Fixture:      fixture.DevMode,
		Timeout:      120 * time.Minute,
	})
}

func SelfSignedBoot(ctx context.Context, s *testing.State) {
	pv := s.FixtValue().(*fixture.Value)
	h := pv.Helper
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

	cleanupContext := ctx
	ctx, closeFunc := ctxutil.Shorten(ctx, 5*time.Minute)
	defer closeFunc()

	defer func(ctx context.Context) {
		if err := h.EnsureDUTBooted(ctx); err != nil {
			s.Fatal("Failed to reconnect to DUT: ", err)
		}
		s.Log("Disabling crossystem self signed")
		if err := setCrossystemSelfSigned(ctx, h, false); err != nil {
			s.Fatal("Failed to disable self signed boot: ", err)
		}
	}(cleanupContext)

	s.Log("Enabling crossystem self signed")
	if err := setCrossystemSelfSigned(ctx, h, true); err != nil {
		s.Fatal("Failed to enable self signed boot: ", err)
	}

	if err := h.RebootWithSSHCommand(ctx, pv.BootMode); err != nil {
		s.Fatal("Failed to reboot with VT2 command: ", err)
	}
	s.Log("Checking that DUT has booted from internal disk")
	bootedFromRemovableDevice, err := h.Reporter.BootedFromRemovableDevice(ctx)
	if err != nil {
		s.Fatal("Could not determine boot device type: ", err)
	}
	if bootedFromRemovableDevice {
		s.Fatalf("DUT did not boot from the internal device: got %v, want false", bootedFromRemovableDevice)
	}

	var state firmware.CheckAndSetServoCharger = h.CheckServoChargerBeforeBootingFromUSB(ctx)

	if err := h.BootToRecoveryMode(ctx, &state, false); err != nil {
		s.Fatal("Failed to boot to recovery mode: ", err)
	}

	if isExpected, err := h.Reporter.ContainsRecoveryReason(ctx, []reporters.RecoveryReason{reporters.RecoveryReasonROManual}); err != nil {
		s.Fatal("Failed to get the recovery reason")
	} else if !isExpected {
		s.Fatal("Failed to get expected recovery reason")
	}

	if err := h.RebootWithSSHCommand(ctx, pv.BootMode); err != nil {
		s.Fatal("Failed to reboot with VT2 command: ", err)
	}
	s.Log("Checking that DUT has booted from internal disk")
	bootedFromRemovableDevice, err = h.Reporter.BootedFromRemovableDevice(ctx)
	if err != nil {
		s.Fatal("Could not determine boot device type: ", err)
	}
	if bootedFromRemovableDevice {
		s.Fatalf("DUT did not boot from the internal device: got %v, want false", bootedFromRemovableDevice)
	}

	if state.RemoveServoChargerRequired && !state.IsServoChargerConnected {
		if err := h.SetDUTPower(ctx, true); err != nil {
			s.Fatal("Failed to connect charger: ", err)
		}
		state.IsServoChargerConnected = true
		waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, 2*time.Minute)
		defer cancelWaitConnect()
		if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
			s.Fatal("Failed to reconnect to the DUT: ", err)
		}
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
		if err := h.RebootWithSSHCommand(ctx, pv.BootMode); err != nil {
			s.Fatal("Failed to reboot with VT2 command: ", err)
		}
		s.Log("Checking that DUT has booted from internal disk")
		bootedFromRemovableDevice, err := h.Reporter.BootedFromRemovableDevice(ctx)
		if err != nil {
			s.Fatal("Could not determine boot device type: ", err)
		}
		if bootedFromRemovableDevice {
			s.Fatalf("DUT did not boot from the internal device: got %v, want false", bootedFromRemovableDevice)
		}
		if state.RemoveServoChargerRequired && !state.IsServoChargerConnected {
			if err := h.SetDUTPower(ctx, true); err != nil {
				s.Fatal("Failed to connect charger: ", err)
			}
			state.IsServoChargerConnected = true
			waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, 2*time.Minute)
			defer cancelWaitConnect()
			if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
				s.Fatal("Failed to reconnect to the DUT: ", err)
			}
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

	if err := h.DeveloperUSBBoot(ctx, &state); err != nil {
		s.Fatal("Failed to boot from USB: ", err)
	}
}

func lsblkGrepUSBPaths(ctx context.Context, h *firmware.Helper) ([]string, error) {
	outRaw, err := h.DUT.Conn().CommandContext(ctx, "sh", "-c", "lsblk -nd --output NAME").Output(ssh.DumpLogOnError)
	if err != nil {
		return []string{}, errors.Wrap(err, "failed to run lsblk command")
	}
	var usbPathSlice []string
	usbRegex := regexp.MustCompile(`sd\w`)
	disableMatches := usbRegex.FindAllSubmatch(outRaw, -1)
	for _, match := range disableMatches {
		usbPathSlice = append(usbPathSlice, string(match[0]))
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

func setCrossystemSelfSigned(ctx context.Context, h *firmware.Helper, enable bool) error {
	setVal := "0"
	if enable {
		setVal = "1"
	}
	if err := h.Reporter.CrossystemSetParam(ctx, reporters.CrossystemParamDevBootSignedOnly, setVal); err != nil {
		return errors.Wrapf(err, "failed to set dev_boot_signed_only=%s", setVal)
	}
	if err := h.Reporter.CrossystemSetParam(ctx, reporters.CrossystemParamDevBootUsb, setVal); err != nil {
		return errors.Wrapf(err, "failed to set dev_boot_usb= %s", setVal)
	}
	return nil
}
