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
			"cienet-firmware@cienet.corp-partner.google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware", "firmware_bios", "firmware_level2", "firmware_usb"},
		Requirements: []string{"sys-fw-0021-v01", "sys-fw-0024-v01", "sys-fw-0025-v01"},
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
			Name: "timeout_usb",
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
			Name: "menu_usb",
			Val: &devDefaultBootParam{
				trigger:    triggerByMenu,
				bootTarget: devDefaultBootFromTheUSB,
			},
			ExtraHardwareDeps: hwdep.D(hwdep.FirmwareUIType(hwdep.MenuUI, hwdep.LegacyMenuUI)),
		},
		}})
}

func DevDefaultBoot(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
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
	cmd := fmt.Sprintf("dev_default_boot=%s", testOpt.bootTarget)
	s.Logf("Setting %s", cmd)
	if err := h.DUT.Conn().CommandContext(ctx, "crossystem", cmd).Run(); err != nil {
		s.Fatalf("Failed to set crossystem %s: %v", cmd, err)
	}
	s.Log("Inserting a valid USB to DUT")
	if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxDUT); err != nil {
		s.Fatal("Failed to set USBMux: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 4*time.Minute)
	defer cancel()

	// Reset crossystem parameter at the end of the test.
	defer func(ctx context.Context) {
		if err := h.EnsureDUTBooted(ctx); err != nil {
			s.Fatal("Failed to reconnect to dut: ", err)
		}
		if err := h.DisableDevBootUSB(ctx); err != nil {
			s.Fatal("Failed to disable usb boot: ", err)
		}
		if err := h.DUT.Conn().CommandContext(ctx, "crossystem", "dev_default_boot=disk").Run(ssh.DumpLogOnError); err != nil {
			s.Fatal("Failed to set crossystem dev_default_boot to disk: ", err)
		}
	}(cleanupCtx)

	s.Log("Rebooting DUT to developer screen")
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateWarmReset); err != nil {
		s.Fatal("Failed to warm reset dut: ", err)
	}

	switch testOpt.trigger {
	case triggerByTimeout:
		reconnectTimeout += firmware.DevScreenTimeout
	case triggerByMenu:
		s.Logf("Sleeping for %s (FirmwareScreen)", h.Config.FirmwareScreen)
		// GoBigSleepLint: Delay to wait for the firmware screen during boot-up.
		if err := testing.Sleep(ctx, h.Config.FirmwareScreen); err != nil {
			s.Fatalf("Failed to sleep for %s: %v", h.Config.FirmwareScreen, err)
		}
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
