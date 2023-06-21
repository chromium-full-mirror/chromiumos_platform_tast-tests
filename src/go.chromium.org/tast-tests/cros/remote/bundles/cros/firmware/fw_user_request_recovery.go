// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: FWUserRequestRecovery,
		Desc: "Servo-based user request recovery boot test that verifies that recovery boot is performed and the recovery reasons are recorded",
		Contacts: []string{
			"chromeos-faft@google.com",
			"shchen@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		// TODO: When stable, change firmware_unstable to a different attr and add linto@chromium.org to gerrit review.
		Attr:    []string{"group:firmware", "firmware_unstable"},
		Timeout: 20 * time.Minute,
		Vars:    []string{"firmware.skipFlashUSB"},
		Params: []testing.Param{
			{
				Name:    "normal",
				Fixture: fixture.NormalMode,
			},
			{
				Name:    "dev",
				Fixture: fixture.DevModeGBB,
			},
		},
	})
}

// FWUserRequestRecovery Servo based user request recovery boot test.
//
// This test requires a USB disk plugged-in, which contains a ChromeOS test
// image
// 1.  This test first requests a recovery mode on next boot by
// setting the crossystem recovery_request flag and seeing if the DUT
// enters the broken screen and checking the recovery reason and
// making sure that it matches.
// 2.  Then do a recovery request from the recovery boot to make sure
// that the recovery reason is the manual recovery.
func FWUserRequestRecovery(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}

	s.Log("Setup USB Key")
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

	fromMode, err := h.Reporter.CurrentBootMode(ctx)
	if err != nil {
		s.Fatal("Failed to get init boot mode: ", err)
	}

	s.Log("run crossystem recovery_request=193")
	cmd := h.DUT.Conn().CommandContext(ctx, "crossystem", "recovery_request=193")
	if err := cmd.Start(); err != nil {
		s.Fatal("Failed to run crossystem recovery_request=193: ", err)
	}
	s.Log("Do warm reboot.  Expect to be in the broken FW screen")
	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		s.Fatal("Failed to create new boot mode switcher: ", err)
	}
	if err := ms.ModeAwareReboot(ctx, firmware.WarmReset, firmware.SkipWaitConnect); err != nil {
		s.Fatal("Error resetting DUT: ", err)
	}

	// Give FW time to log the recovery reason. If we fall through
	// too quickly, next recovery boot won't transfer the recovery
	// reason to VPD
	s.Log("Wait 5 seconds to make sure that DUT is unreachable")
	connectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := h.WaitConnect(connectCtx); err == nil {
		s.Fatal("Unexpectedly connected to DUT: ", err)
	}

	s.Log("Request recovery boot")
	if err := ms.EnableRecMode(ctx, servo.PowerStateRec, servo.USBMuxDUT); err != nil {
		s.Fatal("Failed to reboot into recovery mode: ", err)
	}

	s.Log("Wait for system to be pingable")
	waitCtx, cancel := context.WithTimeout(ctx, h.Config.USBImageBootTimeout)
	defer cancel()
	if err := h.WaitConnect(waitCtx); err != nil {
		s.Fatal("ERROR: Unable to reconnect to the DUT")
	}

	s.Log("Check that mainfw_type=recovery")
	if mainfwType, err := h.Reporter.CrossystemParam(ctx, reporters.CrossystemParamMainfwType); err != nil {
		s.Fatal("Failed to get crossystem mainfw_type")
	} else if mainfwType != "recovery" {
		s.Fatalf("Expected mainfw_type to be 'recovery', got %q", mainfwType)
	}
	s.Log("Check crossystem recovery_reason and make sure that recovery_reason=193")
	if recoveryReason, err := h.Reporter.CrossystemParam(ctx, reporters.CrossystemParamRecoveryReason); err != nil {
		s.Fatal("Failed to get crossystem recovery_reason: ", err)
	} else if recoveryReason != "193" {
		s.Fatalf("Expected recovery_reason to be '193', got %q", recoveryReason)
	}

	s.Log("Do recovery boot again and make sure that it's in recovery")
	if err := ms.EnableRecMode(ctx, servo.PowerStateRec, servo.USBMuxDUT); err != nil {
		s.Fatal("Failed to reboot into recovery mode: ", err)
	}

	s.Log("Wait for system to be pingable")
	waitCtx2, cancel2 := context.WithTimeout(ctx, h.Config.USBImageBootTimeout)
	defer cancel2()
	if err := h.WaitConnect(waitCtx2); err != nil {
		s.Fatal("ERROR: Unable to reconnect to the DUT")
	}

	s.Log("Check that mainfw_type=recovery")
	if mainfwType, err := h.Reporter.CrossystemParam(ctx, reporters.CrossystemParamMainfwType); err != nil {
		s.Fatal("Failed to get crossystem mainfw_type")
	} else if mainfwType != "recovery" {
		s.Fatalf("Expected mainfw_type to be 'recovery', got %q", mainfwType)
	}

	s.Log("Check crossystem recovery_reason and make sure that recovery_reason=2")
	if recoveryReason, err := h.Reporter.CrossystemParam(ctx, reporters.CrossystemParamRecoveryReason); err != nil {
		s.Fatal("Failed to get crossystem recovery_reason: ", err)
	} else if recoveryReason != "2" {
		s.Fatalf("Expected recovery_reason to be '2', got %q", recoveryReason)
	}

	s.Log("Do a warm reset to make sure DUT is out of recovery mode")
	if err := ms.RebootToMode(ctx, fromMode); err != nil {
		s.Fatal("Error resetting DUT: ", err)
	}

	h.DisconnectDUT(ctx)

	s.Log("Reconnecting to DUT")
	if err := h.WaitConnect(ctx); err != nil {
		s.Fatal("Failed to reconnect to DUT: ", err)
	}
}
