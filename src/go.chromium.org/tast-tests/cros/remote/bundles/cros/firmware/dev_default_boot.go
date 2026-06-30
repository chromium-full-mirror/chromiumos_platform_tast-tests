// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"fmt"
	"strconv"
	"time"

	fwCommon "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type defaultBootTarget string

const (
	devDefaultBootFromMainDisk defaultBootTarget = "disk"
	devDefaultBootFromTheUSB   defaultBootTarget = "usb"
)

type defaultBootTrigger int

const (
	triggerByMenu defaultBootTrigger = iota
	triggerByTimeout
)

type devDefaultBootParam struct {
	trigger    defaultBootTrigger
	bootTarget defaultBootTarget
}

func init() {
	testing.AddTest(&testing.Test{
		Func: DevDefaultBoot,
		Desc: "Verify crossystem dev_default_boot functionality",
		Contacts: []string{
			"chromeos-faft@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT

		TestBedDeps:  append([]string{tbdep.ServoUSBState("NORMAL")}, tbdep.ServoPresentAndWorking...),
		Attr:         []string{"group:firmware", "firmware_bios", "firmware_stressed", "firmware_bios_ro", "firmware_bios_rw"},
		SoftwareDeps: []string{"crossystem"},
		Vars:         []string{"firmware.skipFlashUSB"},
		Fixture:      fixture.DevMode,
		Timeout:      2 * time.Hour,
		Params: []testing.Param{{
			Name: "timeout_internal",
			Val: &devDefaultBootParam{
				trigger:    triggerByTimeout,
				bootTarget: devDefaultBootFromMainDisk,
			},
		}, {
			Name:      "timeout_usb",
			ExtraAttr: []string{"firmware_ec", "firmware_ec_ro", "firmware_ec_rw"},
			Val: &devDefaultBootParam{
				trigger:    triggerByTimeout,
				bootTarget: devDefaultBootFromTheUSB,
			},
		}, {
			Name: "menu_internal",
			Val: &devDefaultBootParam{
				trigger:    triggerByMenu,
				bootTarget: devDefaultBootFromMainDisk,
			},
			ExtraHardwareDeps: hwdep.D(hwdep.FirmwareUIType(hwdep.MenuUI, hwdep.LegacyMenuUI)),
		}, {
			Name:      "menu_usb",
			ExtraAttr: []string{"firmware_ec", "firmware_ec_ro", "firmware_ec_rw"},
			Val: &devDefaultBootParam{
				trigger:    triggerByMenu,
				bootTarget: devDefaultBootFromTheUSB,
			},
			ExtraHardwareDeps: hwdep.D(hwdep.FirmwareUIType(hwdep.MenuUI, hwdep.LegacyMenuUI)),
		},
		}})
}

func DevDefaultBoot(ctx context.Context, s *testing.State) {
	pv := s.FixtValue().(*fixture.Value)
	h := pv.Helper

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}
	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}

	testOpt := s.Param().(*devDefaultBootParam)
	expEndBootMode := fwCommon.BootModeDev
	reconnectTimeout := h.Config.DelayRebootToPing
	if testOpt.bootTarget == devDefaultBootFromTheUSB {
		expEndBootMode = fwCommon.BootModeUSBDev
		reconnectTimeout = h.Config.USBImageBootTimeout
	}
	// Set up USB when there is one present, and
	// for cases that depend on it.
	s.Log("Setup USB key")
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
	if err := h.EnableDevBootUSB(ctx); err != nil {
		s.Fatal("Failed to enable usb boot: ", err)
	}

	s.Log("Removing the USB")
	if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxOff); err != nil {
		s.Fatal("Failed to remove the USB: ", err)
	}

	// GoBigSleepLint: It may take some time for usb mux state to
	// take effect.
	if err := testing.Sleep(ctx, firmware.UsbVisibleTime); err != nil {
		s.Fatalf("Failed to sleep for %v s: %v", firmware.UsbDisableTime, err)
	}

	cmd := fmt.Sprintf("dev_default_boot=%s", testOpt.bootTarget)
	s.Logf("Setting %s", cmd)
	if err := h.DUT.Conn().CommandContext(ctx, "crossystem", cmd).Run(); err != nil {
		s.Fatalf("Failed to set crossystem %s: %v", cmd, err)
	}

	var state firmware.CheckAndSetServoCharger = h.CheckServoChargerBeforeBootingFromUSB(ctx)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 25*time.Minute)
	defer cancel()

	// Reset crossystem parameter at the end of the test.
	defer func(ctx context.Context) {
		if err := h.EnsureDUTBooted(ctx); err != nil {
			s.Error("Failed to reconnect to dut: ", err)
		}
		if err := h.DisableDevBootUSB(ctx); err != nil {
			s.Error("Failed to disable usb boot: ", err)
		}
		if err := h.DUT.Conn().CommandContext(ctx, "crossystem", "dev_default_boot=disk").Run(ssh.DumpLogOnError); err != nil {
			s.Error("Failed to set crossystem dev_default_boot to disk: ", err)
		}

		if err := h.RebootWithSSHCommand(ctx, pv.BootMode); err != nil {
			s.Error("Failed to reboot with VT2 command: ", err)
		}

		if state.RemoveServoChargerRequired && !state.IsServoChargerConnected {
			if err := h.SetDUTPower(ctx, true); err != nil {
				s.Error("Failed to connect charger: ", err)
			}
			state.IsServoChargerConnected = true

			// It could take a longer time to reconnect to the DUT.
			waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, 15*time.Minute)
			defer cancelWaitConnect()
			if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
				s.Error("Failed to reconnect to the DUT: ", err)
			}
		}
	}(cleanupCtx)

	if h.HasAPFwState {
		closeUART, err := h.Servo.EnableUARTCapture(ctx, servo.ECUARTCapture)
		if err != nil {
			s.Fatal("Failed to enable capture EC UART: ", err)
		}
		defer func(ctx context.Context) {
			if err := closeUART(ctx); err != nil {
				s.Error("Failed to cancel capture EC UART: ", err)
			}
		}(cleanupCtx)
	}

	// For dm-default-key layouts, the dev image preservation requires an extra preservation step.
	// The binary will return success on all other layouts.
	if err := h.DUT.Conn().CommandContext(ctx, "/usr/local/bin/preserve_dev_image").Run(); err != nil {
		s.Fatal("Failed preserving dev image: ", err)
	}

	s.Log("Rebooting DUT to developer screen")
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateWarmReset); err != nil {
		s.Fatal("Failed to warm reset dut: ", err)
	}
	waitDisconnectCtx, cancelWaitDisconnect := context.WithTimeout(ctx, 2*time.Minute)
	defer cancelWaitDisconnect()
	if err := h.DUT.WaitUnreachable(waitDisconnectCtx); err != nil {
		s.Fatal("Failed to wait for DUT to become unreachable after sending a warm reset: ", err)
	}

	if h.HasAPFwState {
		if err := h.DetectFirmwareScreen(ctx, h.Config.FirmwareScreen, fwCommon.DeveloperMode); err != nil {
			s.Fatal("Failed to detect firmware screen: ", err)
		}
	} else {
		s.Log("Waiting for DUT to reach the firmware screen")
		if err := h.WaitFirmwareScreen(ctx, h.Config.FirmwareScreen); err != nil {
			s.Fatal("Failed to get to firmware screen: ", err)
		}
	}

	if state.RemoveServoChargerRequired && state.IsServoChargerConnected {
		s.Log("Removing servo charger")
		if err := h.SetDUTPower(ctx, false); err != nil {
			s.Fatal("Failed to remove charger: ", err)
		}
		state.IsServoChargerConnected = false
		// GoBigSleepLint: Wait for a while between removing the charger and
		// booting the DUT from USB to prevent USB disconnected issues.
		if err := testing.Sleep(ctx, 5*time.Second); err != nil {
			s.Fatal("Failed to sleep: ", err)
		}
	}

	s.Log("Setting DFP mode")
	if err := h.Servo.SetDUTPDDataRole(ctx, servo.DFP); err != nil {
		s.Logf("Failed to set pd data role to DFP: %.400s", err)
	}

	s.Log("Inserting a valid USB to DUT")
	if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxDUT); err != nil {
		s.Fatal("Failed to insert USB to DUT: ", err)
	}

	// GoBigSleepLint: It may take some time for usb mux state to
	// take effect.
	if err := testing.Sleep(ctx, firmware.UsbVisibleTime); err != nil {
		s.Fatalf("Failed to sleep for %v s: %v", firmware.UsbDisableTime, err)
	}

	switch testOpt.trigger {
	case triggerByTimeout:
		reconnectTimeout += firmware.DevScreenTimeout
	case triggerByMenu:
		menuBypasser, err := firmware.NewMenuBypasser(ctx, h)
		if err != nil {
			s.Fatal("Failed to create menu bypasser: ", err)
		}
		if err := menuBypasser.BypassDevDefaultBoot(ctx); err != nil {
			s.Fatal("Failed to trigger dev default boot: ", err)
		}
	}

	s.Log("Waiting for DUT to reconnect")
	waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, reconnectTimeout)
	defer cancelWaitConnect()
	if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
		s.Fatal("Failed to reconnect to DUT: ", err)
	}

	s.Log("Checking if DUT has reached the correct boot mode")
	isCorrectMode, err := h.Reporter.CheckBootMode(ctx, expEndBootMode)
	if err != nil {
		s.Fatal("Failed to get dut boot mode: ", err)
	}
	if !isCorrectMode {
		s.Fatal("Found DUT booted to the wrong mode")
	}
}
