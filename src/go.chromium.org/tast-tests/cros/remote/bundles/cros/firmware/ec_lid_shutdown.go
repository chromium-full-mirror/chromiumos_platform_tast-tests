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
	pb "go.chromium.org/tast-tests/cros/services/cros/firmware"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ECLidShutdown,
		Desc: "Verify setting GBBFlag_DISABLE_LID_SHUTDOWN flag prevents shutdown with closed lid on fw screen",
		Contacts: []string{
			"chromeos-faft@google.com",
			"tij@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware", "firmware_ec"},
		Requirements: []string{"sys-fw-0022-v02"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.Lid()),
		Fixture:      fixture.NormalMode,
		Timeout:      15 * time.Minute,
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

func ECLidShutdown(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}

	if err := h.Servo.OpenLid(ctx); err != nil {
		s.Fatal("Failed to re open lid: ", err)
	}

	s.Log("Setting USB mux state to off to make sure it doesn't use USB for recovery")
	if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxOff); err != nil {
		s.Fatal("Failed to set usb mux state to off: ", err)
	}

	cleanupContext := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 2*time.Minute)
	defer cancel()
	defer func(ctx context.Context) {
		s.Log("Resetting DUT after test")
		if err := h.Servo.OpenLid(ctx); err != nil {
			s.Fatal("Failed to re open lid: ", err)
		}

		if err := h.Servo.SetPowerState(ctx, servo.PowerStateReset); err != nil {
			s.Fatal("Failed to reset DUT: ", err)
		}

		s.Log("Reconnecting to DUT")
		if err := h.EnsureDUTBooted(ctx); err != nil {
			s.Fatal("Failed to connect to DUT: ", err)
		}

		s.Log("Clear GBBFlag_DISABLE_LID_SHUTDOWN flag after test end")
		flags := pb.GBBFlagsState{Clear: []pb.GBBFlag{pb.GBBFlag_DISABLE_LID_SHUTDOWN}, Set: []pb.GBBFlag{}}
		if _, err := fwCommon.ClearAndSetGBBFlags(ctx, s.DUT(), &flags); err != nil {
			s.Fatal("Failed to clear GBBFlag_DISABLE_LID_SHUTDOWN flag after test end: ", err)
		}
	}(cleanupContext)

	s.Log("Set flag then go to recovery mode, expect S0 after lid close")
	if err := setFlagBeforeRecMode(ctx, h, true); err != nil {
		s.Fatal("Failed to power on and off correctly with GBBFlag_DISABLE_LID_SHUTDOWN set: ", err)
	}

	if err := h.Servo.OpenLid(ctx); err != nil {
		s.Fatal("Failed to open lid: ", err)
	}
	testing.ContextLog(ctx, "Resetting DUT")
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateReset); err != nil {
		s.Fatal("Failed to reset dut: ", err)
	}
	testing.ContextLog(ctx, "Reconnecting to DUT")
	if err := h.EnsureDUTBooted(ctx); err != nil {
		s.Fatal("Failed to reconnect to DUT: ", err)
	}
}

func setFlagBeforeRecMode(ctx context.Context, h *firmware.Helper, flag bool) (reterr error) {

	flags := pb.GBBFlagsState{Clear: []pb.GBBFlag{pb.GBBFlag_DISABLE_LID_SHUTDOWN}, Set: []pb.GBBFlag{}}
	flagState := "clearing"
	if flag {
		flags = pb.GBBFlagsState{Clear: []pb.GBBFlag{}, Set: []pb.GBBFlag{pb.GBBFlag_DISABLE_LID_SHUTDOWN}}
		flagState = "setting"
	}
	testing.ContextLogf(ctx, "%s GBBFlag_DISABLE_LID_SHUTDOWN flag", flagState)
	if _, err := fwCommon.ClearAndSetGBBFlags(ctx, h.DUT, &flags); err != nil {
		return errors.Wrapf(err, "failed %s GBBFlag_DISABLE_LID_SHUTDOWN flag", flagState)
	}

	testing.ContextLog(ctx, "Booting to recovery")
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateRec); err != nil {
		return errors.Wrap(err, "powering off DUT")
	}

	if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout, "S0"); err != nil {
		return errors.Wrap(err, "failed to wait for S0 powerstate")
	}

	if err := h.Servo.CloseLid(ctx); err != nil {
		return errors.Wrap(err, "failed to close lid")
	}

	if flag {
		testing.ContextLog(ctx, "Sleep so lid close has time to affect power_state")
		// GoBigSleepLint: Immediately checking for S0 powerstate might cause a false positive since it was in S0 before lidclose
		// so we want to wait a bit to make sure the DUT had time to settle into a powerstate.
		if err := testing.Sleep(ctx, 10*time.Second); err != nil {
			return errors.Wrap(err, "failed to sleep for 10s")
		}

		if currPowerState, err := h.Servo.GetECSystemPowerState(ctx); err != nil {
			return errors.Wrap(err, "failed to check powerstate")
		} else if currPowerState != "S0" {
			return errors.Errorf("expected DUT to remain in S0, but got powerstate %v instead", currPowerState)
		}
		return nil
	}

	testing.ContextLog(ctx, "Waiting for G3 powerstate")
	if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout, "G3"); err != nil {
		return errors.Wrap(err, "failed to get G3 powerstate")
	}

	return nil
}
