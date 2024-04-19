// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: RecScreenMiniOSMenu,
		Desc: "Verify DUT can boot MiniOS through the recovery screen menu",
		Contacts: []string{
			"chromeos-faft@google.com",
			"cienet-firmware@cienet.corp-partner.google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		// TODO: Remove the test since it has been merged to rec_screen_minios.go.
		HardwareDeps: hwdep.D(hwdep.MiniOS(), hwdep.FirmwareUIType(hwdep.MenuUI)),
		Fixture:      fixture.NormalMode,
		Params: []testing.Param{{
			Val: false,
		}, {
			Name: "old",
			Val:  true,
		}},
		Timeout: 15 * time.Minute,
	})
}

func RecScreenMiniOSMenu(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}

	type miniOSConnectTimeout struct {
		err error
	}
	miniOSConnectTimeoutErr := miniOSConnectTimeout{}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 4*time.Minute)
	defer cancel()

	defer func(ctx context.Context) {
		s.Log("Leaving MiniOS by a warm reset")
		if err := h.Servo.SetPowerState(ctx, servo.PowerStateWarmReset); err != nil {
			s.Fatal("Failed to warm reset DUT: ", err)
		}
		waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
		defer cancelWaitConnect()
		if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
			s.Fatal("Failed to reconnect to dut: ", err)
		}
		if miniOSConnectTimeoutErr.err != nil && h.Config.HasMiniDiagCapability(firmware.CbmemPreservedByAPReset) {
			out, err := h.Reporter.GetCBMEMLogs(ctx, reporters.ConsoleLog)
			if err != nil {
				s.Fatal("Failed to get CBMEM logs: ", err)
			}
			miniosVersionRe := regexp.MustCompile(`cros_minios_version=\d+.\d+.\d+`)
			match := miniosVersionRe.FindStringSubmatch(out)
			if match == nil {
				s.Fatal("DUT might contains an invalid MiniOS or keys did not be pressed on firmware screen")
			} else {
				s.Fatalf("Reconnect timeout is not enough to connect to MiniOS, got %s", match[0])
			}
		}
	}(cleanupCtx)

	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		s.Fatal("Creating mode switcher: ", err)
	}

	if err := ms.EnableRecMode(ctx, servo.PowerStateRec, servo.USBMuxHost); err != nil {
		s.Fatal("Failed to enable recovery mode: ", err)
	}

	if err := h.WaitFirmwareScreen(ctx, h.Config.FirmwareScreenRecMode); err != nil {
		s.Fatal("Failed to get to firmware screen: ", err)
	}

	menuOperator, err := firmware.NewMenuOperator(ctx, h)
	if err != nil {
		s.Fatal("Failed to create a new menu operator: ", err)
	}

	miniOSOlderVersion := s.Param().(bool)
	if err := menuOperator.TriggerRecToMiniOS(ctx, miniOSOlderVersion); err != nil {
		s.Fatal("Failed to boot MiniOS: ", err)
	}

	s.Log("Waiting for DUT to reconnect")
	waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, 90*time.Second)
	defer cancelWaitConnect()
	if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
		miniOSConnectTimeoutErr.err = err
		s.Fatal("Failed to reconnect to DUT: ", err)
	}

	s.Log("Checking if DUT boots to MiniOS")
	miniOSBoot, err := h.Reporter.CheckMiniOSBoot(ctx)
	if err != nil {
		s.Fatal("Failed to check MiniOS boot: ", err)
	}
	if !miniOSBoot {
		s.Fatal("MiniOS boot was unsuccessful")
	}
}
