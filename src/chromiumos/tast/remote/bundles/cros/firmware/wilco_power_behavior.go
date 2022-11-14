// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"strings"
	"time"

	"chromiumos/tast/common/servo"
	"chromiumos/tast/errors"
	"chromiumos/tast/remote/firmware/fixture"
	"chromiumos/tast/testing"
)

// wilcoPowerBehaviorTestParams defines the params of interest.
// checkCharger denotes whether the test requires plugging/unplugging charger.
// checkLidState denotes whether the test requires opening/closing the dut's lid.
type wilcoPowerBehaviorTestParams struct {
	checkCharger  bool
	checkLidState bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         WilcoPowerBehavior,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify Wilco power behavior based on AC and lid states",
		Contacts: []string{
			"chromeos-faft@google.com",
			"cienet-firmware@cienet.corp-partner.google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware", "firmware_unstable"},
		SoftwareDeps: []string{"wilco"},
		Fixture:      fixture.NormalMode,
		Timeout:      10 * time.Minute,
		Params: []testing.Param{{
			// Verify that Wilco doesn't turn on from S5 (off) by opening the lid.
			Name: "lid_close_open",
			Val: wilcoPowerBehaviorTestParams{
				checkLidState: true,
			},
		}, {
			// Verify that Wilco wakes from pressing power, but not from AC.
			Val: wilcoPowerBehaviorTestParams{
				checkCharger: true,
			},
		}},
	})
}

func WilcoPowerBehavior(ctx context.Context, s *testing.State) {
	tc := s.Param().(wilcoPowerBehaviorTestParams)
	h := s.FixtValue().(*fixture.Value).Helper
	d := s.DUT()

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}

	// For debugging purposes, log servo type.
	servoType, err := h.Servo.GetServoType(ctx)
	if err != nil {
		s.Fatal("Failed to find servo type: ", err)
	}
	s.Logf("Servo type: %s", servoType)

	if tc.checkCharger {
		s.Log("Removing charger")
		if err := h.SetDUTPower(ctx, false); err != nil {
			s.Fatal("Unable to remove charger: ", err)
		}
		if err := h.Servo.WatchdogRemove(ctx, servo.WatchdogMain); err != nil {
			s.Fatal("Failed to remove watchdog main: ", err)
		}
	}
	if tc.checkLidState {
		// Lid emulations are only possible via servo micro.
		if hasMicroOrC2D2, err := h.Servo.PreferDebugHeader(ctx); err != nil {
			s.Fatal("PreferDebugHeader: ", err)
		} else if !hasMicroOrC2D2 {
			s.Fatal("No servo micro found for lid emulations")
		}
	}

	s.Logf("Pressing power button for %s to put DUT in deep sleep", h.Config.HoldPwrButtonPowerOff)
	if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.Dur(h.Config.HoldPwrButtonPowerOff)); err != nil {
		s.Fatal("Failed to hold power button: ", err)
	}

	s.Log("Waiting for DUT to power OFF")
	waitUnreachableCtx, cancelUnreachable := context.WithTimeout(ctx, 2*time.Minute)
	defer cancelUnreachable()

	if err := d.WaitUnreachable(waitUnreachableCtx); err != nil {
		s.Fatal("DUT did not power down: ", err)
	}

	if tc.checkCharger {
		// Increase timeout in getting response from cr50 uart.
		if err := h.Servo.SetString(ctx, "cr50_uart_timeout", "10"); err != nil {
			s.Fatal("Failed to set cr50 uart timeout: ", err)
		}
		defer func() {
			s.Log("Restoring cr50 uart timeout to the default value of 3 seconds")
			if err := h.Servo.SetString(ctx, "cr50_uart_timeout", "3"); err != nil {
				s.Fatal("Failed to restore default cr50 uart timeout: ", err)
			}
		}()

		s.Log("Verifying DUT's AP is off")
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			apState, err := h.Servo.RunCR50CommandGetOutput(ctx, "ccdstate", []string{`AP:(\s+\w+)`})
			if err != nil {
				return errors.Wrap(err, "failed to run cr50 command")
			}

			if strings.TrimSpace(apState[0][1]) != "off" {
				return errors.Wrapf(err, "unexpected AP state: %s", strings.TrimSpace(apState[0][1]))
			}
			return nil
		}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: time.Second}); err != nil {
			s.Fatal("Failed to verify DUT's AP is off: ", err)
		}

		s.Log("Connecting charger")
		if err := h.SetDUTPower(ctx, true); err != nil {
			s.Fatal("Unable to connect charger: ", err)
		}
	}
	if tc.checkLidState {
		// Close and then open DUT's lid.
		for _, expState := range []string{"no", "yes"} {
			s.Logf("Setting lid open to %s and checking for lid state", expState)
			if err := testing.Poll(ctx, func(ctx context.Context) error {
				if err := h.Servo.SetStringAndCheck(ctx, servo.LidOpen, expState); err != nil {
					s.Fatalf("Failed to set lid open to %s: %v", expState, err)
				}
				return nil
			}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
				s.Fatal("While setting and checking for the lid state: ", err)
			}
			if err := testing.Sleep(ctx, time.Second); err != nil {
				s.Fatal("Failed to sleep: ", err)
			}
		}
	}

	// Check that when Wilco devices are in deep sleep, or at the off state,
	// waking it would not be possible either by AC, or by opening lid.
	// Expect a timeout in waiting for DUT to reconnect.
	waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, 1*time.Minute)
	defer cancelWaitConnect()
	err = d.WaitConnect(waitConnectCtx)
	switch err.(type) {
	case nil:
		s.Fatal("DUT woke up unexpectedly")
	default:
		if !strings.Contains(err.Error(), context.DeadlineExceeded.Error()) {
			s.Fatal("Unexpected error occurred: ", err)
		}
	}
	s.Log("DUT remained offline")

	s.Logf("Pressing power button for %s seconds to wake DUT", servo.Dur(h.Config.HoldPwrButtonPowerOn))
	if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.Dur(h.Config.HoldPwrButtonPowerOn)); err != nil {
		s.Fatal("Failed to press power key via servo: ", err)
	}

	waitConnectFromPressPowerCtx, cancelWaitConnectFromPressPower := context.WithTimeout(ctx, 2*time.Minute)
	defer cancelWaitConnectFromPressPower()
	s.Log("Checking that DUT wakes up from a press on power button")
	if err := d.WaitConnect(waitConnectFromPressPowerCtx); err != nil {
		s.Fatal("Failed to reconnect to DUT: ", err)
	}
}
