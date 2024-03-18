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
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type devInsertUSBScreenBoot int

const (
	insertUSBBoot devInsertUSBScreenBoot = iota
	mainDiskBootFromDevScreen
)

type devInsertUSBScreenParam struct {
	scenario       devInsertUSBScreenBoot
	expEndBootMode fwCommon.BootMode
	expFwScreens   []fwCommon.FwScreenID
}

func init() {
	testing.AddTest(&testing.Test{
		Func: DevInsertUSBScreen,
		Desc: "Verify USB boot in developer mode through the UI menu",
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
		HardwareDeps: hwdep.D(hwdep.FirmwareUIType(hwdep.MenuUI)),
		Params: []testing.Param{{
			Name: "insert_usb",
			Val: &devInsertUSBScreenParam{
				scenario:       insertUSBBoot,
				expEndBootMode: fwCommon.BootModeUSBDev,
				expFwScreens: []fwCommon.FwScreenID{
					fwCommon.DeveloperMode,
					fwCommon.DeveloperBootExternal,
				},
			},
		}, {
			Name: "back",
			Val: &devInsertUSBScreenParam{
				scenario:       mainDiskBootFromDevScreen,
				expEndBootMode: fwCommon.BootModeDev,
				expFwScreens: []fwCommon.FwScreenID{
					fwCommon.DeveloperMode,
					fwCommon.DeveloperBootExternal,
					fwCommon.DeveloperMode,
				},
			},
		}},
	})
}

func DevInsertUSBScreen(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}
	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}
	testOpt := s.Param().(*devInsertUSBScreenParam)
	menuNavigator, err := firmware.NewMenuNavigator(ctx, h)
	if err != nil {
		s.Fatal("Failed to create a new menu navigator: ", err)
	}
	menuBypasser, err := firmware.NewMenuBypasser(ctx, h)
	if err != nil {
		s.Fatal("Failed to create a new menu bypasser: ", err)
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
		s.Fatal("Failed to enable dev boot from USB: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 4*time.Minute)
	defer cancel()
	// Set dev_boot_usb back to 0 at the end of the test.
	defer func(ctx context.Context) {
		if err := h.EnsureDUTBooted(ctx); err != nil {
			s.Fatal("Failed to reconnect to dut: ", err)
		}
		if err := h.DisableDevBootUSB(ctx); err != nil {
			s.Fatal("Failed to disable dev boot from USB: ", err)
		}
	}(cleanupCtx)

	s.Log("Removing USB from DUT")
	if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxHost); err != nil {
		s.Fatal("Failed to insert USB to DUT: ", err)
	}

	s.Log("Rebooting DUT to developer screen")
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateReset); err != nil {
		s.Fatal("Failed to cold reset dut: ", err)
	}
	s.Logf("Sleeping for %s (FirmwareScreen)", h.Config.FirmwareScreen)
	// GoBigSleepLint: Delay to wait for the firmware screen during boot-up.
	if err := testing.Sleep(ctx, h.Config.FirmwareScreen); err != nil {
		s.Fatalf("Failed to sleep for %s: %v", h.Config.FirmwareScreen, err)
	}

	s.Log("Selecting \"Boot from external disk\"")
	if err := menuBypasser.BypassDevBootUSB(ctx); err != nil {
		s.Fatal("Failed to bypass firmware screen to usbdev mode: ", err)
	}

	var reconnectTimeout time.Duration
	switch testOpt.scenario {
	case insertUSBBoot:
		s.Log("Checking if DUT boots from the USB")
		if err := h.WaitDUTConnectDuringBootFromUSB(ctx, false); err != nil {
			s.Fatal("While verifying that DUT doesn't boot up from the USB: ", err)
		}
		s.Log("Setting DFP mode")
		if err := h.Servo.SetDUTPDDataRole(ctx, servo.DFP); err != nil {
			s.Logf("Failed to set pd data role to DFP: %.400s", err)
		}
		s.Log("Inserting a valid USB to DUT")
		if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxDUT); err != nil {
			s.Fatal("Failed to set USBMux: ", err)
		}
		// GoBigSleepLint: It may take some time for usb mux state to take effect.
		if err := testing.Sleep(ctx, firmware.UsbVisibleTime); err != nil {
			s.Fatalf("Failed to sleep for %s (UsbVisibleTime): %v", firmware.UsbDisableTime, err)
		}
		reconnectTimeout = h.Config.USBImageBootTimeout
	case mainDiskBootFromDevScreen:
		// GoBigSleepLint: Sleep for model specific time.
		if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
			s.Fatalf("Failed to sleep for %s (KeypressDelay): %v", h.Config.KeypressDelay, err)
		}
		s.Log("Selecting \"Back\"")
		if err := menuNavigator.SelectOption(ctx); err != nil {
			s.Fatal("Failed to press \"Back\": ", err)
		}
		// GoBigSleepLint: Sleep for model specific time.
		if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
			s.Fatalf("Failed to sleep for %s (KeypressDelay): %v", h.Config.KeypressDelay, err)
		}
		s.Log("Selecting \"Boot from internal disk\"")
		if err := menuBypasser.BypassDevMode(ctx); err != nil {
			s.Fatal("Failed to bypass firmware screen to dev mode: ", err)
		}
		reconnectTimeout = h.Config.DelayRebootToPing
	}
	s.Log("Waiting for DUT to reconnect")
	waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, reconnectTimeout)
	defer cancelWaitConnect()
	if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
		s.Fatal("Failed to reconnect to DUT: ", err)
	}

	s.Log("Checking if DUT has reached the correct boot mode")
	isCorrectMode, err := h.Reporter.CheckBootMode(ctx, testOpt.expEndBootMode)
	if err != nil {
		s.Fatal("Failed to get dut boot mode: ", err)
	}
	if !isCorrectMode {
		s.Fatal("Found DUT booted to the wrong mode")
	}

	match, err := h.Reporter.CheckDisplayedScreens(ctx, testOpt.expFwScreens)
	if err != nil {
		s.Fatal("Failed to check displayed screens: ", err)
	}
	if !match {
		s.Fatalf("Unable to find matches for the expected screens %x", testOpt.expFwScreens)
	}
}
