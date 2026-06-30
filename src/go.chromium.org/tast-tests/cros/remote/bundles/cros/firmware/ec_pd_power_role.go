// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ECPDPowerRole,
		Desc: "Verify DUT swaps its power role according to power policy",
		Contacts: []string{
			"chromeos-faft@google.com",
			"bszpila@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Fixture:      fixture.NormalMode,
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Timeout:      20 * time.Minute,
		TestBedDeps:  tbdep.ServoPresentAndWorking,
		Attr:         []string{"group:firmware", "firmware_pd_unstable"},
		Params: []testing.Param{{
			Name: "normal",
			Val: firmware.PDTestParams{
				DTS: firmware.DTSModeOff,
			},
		}, {
			Name: "flipcc",
			Val: firmware.PDTestParams{
				CC:  firmware.CCPolarityFlipped,
				DTS: firmware.DTSModeOff,
			},
		}},
	})
}

type testData struct {
	Commands  []string
	PowerRole string
}

const (
	lowChargeV        int           = 5
	highChargeV       int           = 15
	pwrRoleChgTimeout time.Duration = 3 * time.Second
	iters             int           = 10
)

// assertServoPDRole asserts that if servo reconnects, it will end up with
// expected power role in pwrRoleChgTimeout period
func assertServoPDRole(ctx context.Context, h *firmware.Helper, expectedPwrRole string) error {
	// Detach servo
	if err := h.Servo.RunServoCommand(ctx, "cc off"); err != nil {
		return errors.Wrap(err, "failed to turn off cc lines")
	}

	// GoBigSleepLint: Sleep a little for a better power role randomness
	if err := testing.Sleep(ctx, time.Duration(300+rand.Intn(300))*time.Millisecond); err != nil {
		return errors.Wrap(err, "failed to sleep")
	}

	// Attach servo in drp mode
	if err := h.Servo.RunServoCommand(ctx, "cc drp"); err != nil {
		return errors.Wrap(err, "failed to reattach servo as snk")
	}

	// Check the power role the device was connected in
	pdState, err := h.Servo.GetServoPDState(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get PD State")
	}

	testing.ContextLog(ctx, "Servo PD role after connection: ", string(pdState.PowerRole))

	if expectedPwrRole == string(pdState.PowerRole) {
		// GoBigSleepLint: If the power role is the same as expected power role
		// let PD state settle before checking if DUT stayed in it's power role.
		if err := testing.Sleep(ctx, pwrRoleChgTimeout); err != nil {
			return errors.Wrap(err, "failed to sleep")
		}

		if nextPdState, err := h.Servo.GetServoPDState(ctx); err != nil {
			return errors.Wrap(err, "failed to get PD State")
		} else if nextPdState.PowerRole != pdState.PowerRole {
			return errors.New("servo has unexpected power role")
		}
	} else if err := testing.Poll(ctx, func(ctx context.Context) error {
		if nextPdState, err := h.Servo.GetServoPDState(ctx); err != nil {
			return errors.Wrap(err, "failed to get PD State")
		} else if string(nextPdState.PowerRole) != expectedPwrRole {
			return errors.New("servo has unexpected power role")
		}

		return nil
	}, &testing.PollOptions{Interval: time.Second, Timeout: pwrRoleChgTimeout}); err != nil {
		return errors.Wrap(err, "failed poll for pd state")
	}

	return nil
}

// ECPDPowerRole checks if DUTs power role policy
//   - DUT should start sinking power when it's provided 27W or unconstrained power
//   - DUT should be source if partner offers low power capabilities, DUT has sufficient
//     battery charge and DUT is in power on state
func ECPDPowerRole(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}

	testParams := s.Param().(firmware.PDTestParams)

	if err := firmware.SetupPDTester(ctx, h, testParams, s.OutDir()); err != nil {
		s.Fatal("Failed to configure Servo for PD testing: ", err)
	}

	if err := h.Servo.EnableServoConsoleChannel(ctx, "usbpd"); err != nil {
		s.Fatal("Failed to enable usbpd console channel: ", err)
	}

	if err := h.Servo.SetDualroleState(ctx, servo.DROn); err != nil {
		s.Fatal("Could not enable DRP on EC: ", err)
	}

	if err := h.Servo.ServoSetDualRole(ctx, servo.USBPdDualRoleOn); err != nil {
		s.Fatal("Could not enable DRP on Servo: ", err)
	}

	defer h.Servo.SetPDTrySrc(ctx, servo.PDPortUnderTest, 1)
	if _, err := h.Servo.SetPDTrySrc(ctx, servo.PDPortUnderTest, 0); err != nil {
		s.Fatal("Could not disable TrySrc on DUT: ", err)
	}

	// Cleanup after test
	defer h.Servo.RunServoCommand(ctx, "usbc_action unconstrained_pwr 1")
	defer h.Servo.RunServoCommand(ctx, "usbc_action chg dev")

	// Tests setup
	states := []testData{
		{Commands: []string{"usbc upr 0", fmt.Sprintf("usbc chg %d", lowChargeV)},
			PowerRole: string(servo.PowerRoleSNK)},
		{Commands: []string{"usbc upr 0", fmt.Sprintf("usbc chg %d", highChargeV)},
			PowerRole: string(servo.PowerRoleSRC)},
		{Commands: []string{"usbc upr 1", fmt.Sprintf("usbc chg %d", lowChargeV)},
			PowerRole: string(servo.PowerRoleSRC)},
	}

	// Tests execution
	for index, state := range states {
		for _, command := range state.Commands {
			if err := h.Servo.RunServoCommand(ctx, command); err != nil {
				s.Fatalf("Test %d, failed to run servo cmd %s: %v", index, command, err)
			}
		}

		for i := 0; i < iters; i++ {
			if err := assertServoPDRole(ctx, h, string(state.PowerRole)); err != nil {
				s.Fatalf("Test %d servo should be %s: %v", index, state.PowerRole, err)
			}
		}
	}
}
