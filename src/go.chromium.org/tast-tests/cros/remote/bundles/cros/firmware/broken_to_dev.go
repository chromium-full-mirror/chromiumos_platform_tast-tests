// Copyright 2023 The ChromiumOS Authors
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
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"
	pb "go.chromium.org/tast-tests/cros/services/cros/firmware"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         BrokenToDev,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify triggering to_dev from broken screen does not boot DUT to dev mode",
		Contacts: []string{
			"chromeos-faft@google.com",
			"cienet-firmware@cienet.corp-partner.google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		// TODO: When stable, change firmware_unstable to a different attr.
		Attr:    []string{"group:firmware", "firmware_unstable"},
		Fixture: fixture.NormalMode,
		Timeout: 20 * time.Minute,
	})
}

func BrokenToDev(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}
	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}

	defer func() {
		s.Log("Clearing the GBBFlag_DEV_SCREEN_SHORT_DELAY flag after test ends")
		if _, err := fwCommon.ClearAndSetGBBFlags(ctx, s.DUT(), &pb.GBBFlagsState{Clear: []pb.GBBFlag{pb.GBBFlag_DEV_SCREEN_SHORT_DELAY}}); err != nil {
			s.Fatal("Failed to clear the GBBFlag_DEV_SCREEN_SHORT_DELAY flag: ", err)
		}
	}()
	s.Log("Setting GBB flags to enable dev screen short delay")
	if _, err := fwCommon.ClearAndSetGBBFlags(ctx, s.DUT(), &pb.GBBFlagsState{Set: []pb.GBBFlag{pb.GBBFlag_DEV_SCREEN_SHORT_DELAY}}); err != nil {
		s.Fatal("Failed to set the GBBFlag_DEV_SCREEN_SHORT_DELAY flag: ", err)
	}

	if err := h.ClearEventlog(ctx); err != nil {
		s.Fatal("Failed to clear event log: ", err)
	}
	s.Log("Setting crossystem recovery_request to 193")
	if err := h.DUT.Conn().CommandContext(ctx, "crossystem", "recovery_request=193").Run(); err != nil {
		s.Fatal("Failed to set crossystem recovery_request to 193: ", err)
	}
	s.Log("Rebooting the DUT with a warm reset")
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateWarmReset); err != nil {
		s.Fatal("Failed to warm reset the DUT: ", err)
	}
	waitDisconnectCtx, cancelWaitDisconnect := context.WithTimeout(ctx, 1*time.Minute)
	defer cancelWaitDisconnect()
	if err := h.DUT.WaitUnreachable(waitDisconnectCtx); err != nil {
		s.Fatal("Failed to wait for DUT to become unreachable, warm reset failed: ", err)
	}
	s.Log("Waiting for DUT to reach the firmware screen")
	if err := h.WaitFirmwareScreen(ctx, h.Config.FirmwareScreen); err != nil {
		s.Fatal("Failed to get to firmware screen: ", err)
	}
	newbp, err := firmware.NewBypasser(ctx, h)
	if err != nil {
		s.Fatal("Failed to create a new bypasser: ", err)
	}
	s.Log("Triggering to-dev steps from the Broken Screen")
	if err := newbp.TriggerRecToDev(ctx); err != nil {
		s.Fatal("Failed to trigger to_dev: ", err)
	}
	s.Log("Checking if DUT stays at the Broken Screen")
	brokenToDevWaitConnectCtx, cancelWaitConnectBrokenToDev := context.WithTimeout(ctx, firmware.DevScreenShortDelay+h.Config.DelayRebootToPing)
	defer cancelWaitConnectBrokenToDev()

	err = h.WaitConnect(brokenToDevWaitConnectCtx, firmware.ResetEthernetDongle)
	switch err.(type) {
	case nil:
		s.Fatal("DUT woke up unexpectedly")
	default:
		if !errors.As(err, &context.DeadlineExceeded) {
			s.Fatal("Unexpected error occurred: ", err)
		}
	}

	s.Log("Restoring connection to the DUT with cold reset")
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateReset); err != nil {
		s.Fatal("Failed to cold reset the DUT: ", err)
	}
	s.Log("Reconnecting to the DUT")
	waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
	defer cancelWaitConnect()
	if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
		s.Fatal("Failed to reconnect to DUT: ", err)
	}

	if isNormal, err := h.Reporter.CheckBootMode(ctx, fwCommon.BootModeNormal); err != nil {
		s.Fatal("Failed to check current boot mode: ", err)
	} else if !isNormal {
		s.Fatal("Did not find DUT in normal mode as expected")
	}

	s.Log("Checking event log to verify DUT has reached the Broken Screen")
	newEvents, err := h.Reporter.EventlogList(ctx)
	if err != nil {
		s.Fatal("Failed to find events: ", err)
	}
	expectedBootModes := []reporters.EventlogBootMode{reporters.BrokenScreen, reporters.NormalMode}
	if err := h.Reporter.CheckBootModes(ctx, newEvents, expectedBootModes); err != nil {
		s.Fatal("Failed to check for the expected boot modes: ", err)
	}
}
