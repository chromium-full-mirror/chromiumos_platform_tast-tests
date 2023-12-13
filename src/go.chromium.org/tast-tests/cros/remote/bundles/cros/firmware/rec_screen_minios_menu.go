// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
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
		// TODO: When stable, change firmware_unstable to a different attr.
		Attr:         []string{"group:firmware", "firmware_unstable"},
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
	}(cleanupCtx)

	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		s.Fatal("Creating mode switcher: ", err)
	}

	if err := ms.EnableRecMode(ctx, servo.PowerStateRec, servo.USBMuxHost); err != nil {
		s.Fatal("Failed to enable recovery mode: ", err)
	}

	s.Logf("Sleeping %s (FirmwareScreen)", h.Config.FirmwareScreenRecMode)
	// GoBigSleepLint: Wait for firmware screen.
	if err := testing.Sleep(ctx, h.Config.FirmwareScreenRecMode); err != nil {
		s.Fatalf("Failed to sleep for %s (FirmwareScreen): %v", h.Config.FirmwareScreenRecMode, err)
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
	// When remotely running the test on moli, dojo, and dewatt, we saw that
	// it took them more than one minute to become reachable again. Document the
	// duration needed here. We may need to create a new config called minios_screen_to_ping,
	// and use this duration to set model specific times.
	// To-do: manual ping/ssh'ing to the lab machines turned out to be faster
	// than h.WaitConnect. Investigate what might have caused the connection
	// to freeze in h.WaitConnect.
	startTime := time.Now()
	waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, 2*time.Minute)
	defer cancelWaitConnect()
	if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
		s.Fatal("Failed to reconnect to DUT: ", err)
	}
	s.Log("Found DUT reachable again after: ", time.Since(startTime))

	s.Log("Checking if DUT boots to MiniOS")
	miniOSBoot, err := h.Reporter.CheckMiniOSBoot(ctx)
	if err != nil {
		s.Fatal("Failed to check MiniOS boot: ", err)
	}
	if !miniOSBoot {
		s.Fatal("MiniOS boot was unsuccessful")
	}
}
