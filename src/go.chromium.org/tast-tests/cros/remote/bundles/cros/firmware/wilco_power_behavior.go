// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
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
		Attr:         []string{"group:firmware"},
		SoftwareDeps: []string{"wilco"},
		Fixture:      fixture.NormalMode,
		Timeout:      15 * time.Minute,
		Params: []testing.Param{{
			// Verify that Wilco doesn't turn on from S5 (off) by opening the lid.
			// TODO: When stable, change firmware_unstable to a different attr.
			Name: "lid_close_open",
			Val: wilcoPowerBehaviorTestParams{
				checkLidState: true,
			},
			ExtraAttr: []string{"firmware_unstable"},
		}, {
			// Verify that Wilco wakes from pressing power, but not from AC.
			Val: wilcoPowerBehaviorTestParams{
				checkCharger: true,
			},
			ExtraRequirements: []string{"sys-fw-0021-v01", "sys-fw-0024-v01", "sys-fw-0025-v01"},
			ExtraAttr:         []string{"firmware_bios"},
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
		if err := h.Servo.RemoveCCDWatchdogs(ctx); err != nil {
			s.Fatal("Failed to remove watchdog main: ", err)
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

	// Based on 'ap_state.c', we saw that TPM_RST_L should
	// change with the state of the ap. If cr50 console becomes
	// unresponsive during deep sleep, check for TPM_RST_L
	// after DUT awakened, and verify that its value has changed
	// from 0 to 1.
	checkTPMRSTLState := false
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
			if !strings.Contains(err.Error(), "Client.Timeout exceeded while awaiting headers") {
				s.Fatal("Failed to verify DUT's AP is off: ", err)
			}
			s.Log("Cr50 not responsive, check for TPM_RST_L state after rebooting the DUT")
			checkTPMRSTLState = true
		}
		s.Log("Connecting charger")
		if err := h.SetDUTPower(ctx, true); err != nil {
			s.Fatal("Unable to connect charger: ", err)
		}
	}
	if tc.checkLidState {
		defer func() {
			if err := h.Servo.OpenLid(ctx); err != nil {
				s.Fatal("Failed to ensure lid open at the end of test: ", err)
			}
		}()
		// Close and then open DUT's lid.
		for _, expState := range []string{"no", "yes"} {
			s.Logf("Setting lid open to %s and checking for lid state", expState)
			if err := testing.Poll(ctx, func(ctx context.Context) error {
				if err := h.Servo.SetStringAndCheck(ctx, servo.LidOpen, expState); err != nil {
					return errors.Wrapf(err, "failed to set lid open to %s", expState)
				}
				return nil
			}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
				// On a number of lab sarien devices from dut_pool_quota, we saw that
				// the lid_open control did not return the correct state that it was
				// set to. Specifically, lid_open was set to 'no', but the checked value
				// was 'yes'. A similar situation was mentioned in ticket b:134830532.
				// To avoid mistakenly failing the test, attempt a press on the power
				// button. If in reality the lid was closed, the dut would not wake up.
				if expState == "no" {
					currentState, err := h.Servo.GetString(ctx, servo.LidOpen)
					if err != nil {
						s.Fatal("Failed to get lid open state: ", err)
					}
					if currentState == "yes" {
						s.Logf("Pressing power button for %s seconds to wake DUT", servo.Dur(h.Config.HoldPwrButtonPowerOn))
						if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.Dur(h.Config.HoldPwrButtonPowerOn)); err != nil {
							s.Fatal("Failed to press power button: ", err)
						}
						waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, 3*time.Minute)
						defer cancelWaitConnect()

						s.Log("Checking if DUT woke up from a press on power button")
						err := h.WaitConnect(waitConnectCtx)
						if err == nil {
							s.Fatal("DUT woke up unexpectedly")
						}
						if strings.Contains(err.Error(), context.DeadlineExceeded.Error()) {
							s.Log("Found DUT disconnected, continuing the test")
							continue
						}
					}
				}
				s.Fatal("While setting and checking for the lid state: ", err)
			}
		}
	}

	// Check that when Wilco devices are in deep sleep, or at the off state,
	// waking it would not be possible either by AC, or by opening lid.
	// Expect a timeout in waiting for DUT to reconnect.
	waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, 1*time.Minute)
	defer cancelWaitConnect()
	err = h.WaitConnect(waitConnectCtx)
	switch err.(type) {
	case nil:
		// When tested manually, drallion duts woke up from opening lid.
		// Use the lid_wake_from_power_off config to differentiate them from the others.
		if (!h.Config.LidWakeFromPowerOff && tc.checkLidState) || tc.checkCharger {
			s.Fatal("DUT woke up unexpectedly")
		}
	default:
		if !strings.Contains(err.Error(), context.DeadlineExceeded.Error()) {
			s.Fatal("Unexpected error occurred: ", err)
		}
	}
	if (!h.Config.LidWakeFromPowerOff && tc.checkLidState) || tc.checkCharger {
		s.Logf("DUT remained offline, pressing power button for %s seconds to wake DUT", servo.Dur(h.Config.HoldPwrButtonPowerOn))
		if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.Dur(h.Config.HoldPwrButtonPowerOn)); err != nil {
			s.Fatal("Failed to press power key via servo: ", err)
		}

		waitConnectFromPressPowerCtx, cancelWaitConnectFromPressPower := context.WithTimeout(ctx, 2*time.Minute)
		defer cancelWaitConnectFromPressPower()
		s.Log("Checking that DUT wakes up from a press on power button")
		if err := h.WaitConnect(waitConnectFromPressPowerCtx); err != nil {
			s.Fatal("Failed to reconnect to DUT: ", err)
		}
	}

	if checkTPMRSTLState {
		s.Log("Checking for TPM_RST_L to verify DUT's ap off during deep sleep")
		var (
			foundGpio    = `(0|1\W*)TPM_RST_L`
			paramInvalid = `Parameter\s+(\d+)\s+invalid`
			checkGpio    = `(` + foundGpio + `|` + paramInvalid + `)`
		)
		cmd := "gpioget TPM_RST_L"
		out, err := h.Servo.RunCR50CommandGetOutput(ctx, cmd, []string{checkGpio})
		if err != nil {
			s.Fatalf("Failed to run command %v: %v", cmd, err)
		}
		reMatch := regexp.MustCompile(foundGpio)
		if match := reMatch.FindStringSubmatch(out[0][0]); match == nil {
			s.Fatal("Did not find gpio TPM_RST_L: ", err)
		}
		if !strings.Contains(out[0][0], "1*") {
			s.Fatal("Gpio TPM_RST_L did not change after dut awakened")
		}
	}
}
