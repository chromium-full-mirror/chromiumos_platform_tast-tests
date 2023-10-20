// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"time"

	fwCommon "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type fwScreenPowerOffParam struct {
	menuPowerOffScreen fwCommon.FwScreenType
}

func init() {
	testing.AddTest(&testing.Test{
		Func: FwScreenPowerOff,
		Desc: "Test power off from developer, recovery and broken screens using UI menu",
		Contacts: []string{
			"chromeos-faft@google.com",
			"cienet-firmware@cienet.corp-partner.google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware", "firmware_unstable"},
		Timeout:      8 * time.Minute,
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.FirmwareUIType(hwdep.MenuUI, hwdep.LegacyMenuUI)),
		LacrosStatus: testing.LacrosVariantUnneeded,
		Params: []testing.Param{{
			Name:    "dev",
			Fixture: fixture.DevMode,
			Val: &fwScreenPowerOffParam{
				menuPowerOffScreen: fwCommon.FwDeveloperScreen,
			},
		}, {
			Name:    "rec",
			Fixture: fixture.NormalMode,
			Val: &fwScreenPowerOffParam{
				menuPowerOffScreen: fwCommon.FwRecoveryScreen,
			},
		}, {
			Name:    "broken",
			Fixture: fixture.NormalMode,
			Val: &fwScreenPowerOffParam{
				menuPowerOffScreen: fwCommon.FwBrokenScreen,
			},
		}},
	})
}

func FwScreenPowerOff(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		s.Fatal("Failed to create mode switcher: ", err)
	}
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servod: ", err)
	}
	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}

	// Disable USB to avoid booting in recovery mode
	s.Logf("Setting USBMux to %s", servo.USBMuxOff)
	if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxOff); err != nil {
		s.Fatal("Failed to set USBMux: ", err)
	}

	testOpt := s.Param().(*fwScreenPowerOffParam)
	if err := ms.RebootToFirmwareScreen(ctx, testOpt.menuPowerOffScreen); err != nil {
		s.Fatal("Failed to boot to the firmware screen: ", err)
	}

	menuBypasser, err := firmware.NewMenuBypasser(ctx, h)
	if err != nil {
		s.Fatal("Failed to create a new menu bypasser: ", err)
	}
	// GoBigSleepLint: Delay to wait for the firmware screen during boot-up.
	if err := testing.Sleep(ctx, h.Config.FirmwareScreen); err != nil {
		s.Fatal("Failed to sleep for firmware screen: ", err)
	}
	s.Log("Selecting \"Power off\" from UI menu")
	if err := menuBypasser.PowerOff(ctx); err != nil {
		s.Fatal("Failed to power off the DUT: ", err)
	}
	s.Log("Waiting for DUT to power off")
	if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout, "G3"); err != nil {
		s.Fatal("Failed to get power state G3: ", err)
	}

	if err := ms.PowerOn(ctx); err != nil {
		s.Fatal("Failed to power on the DUT: ", err)
	}
}
