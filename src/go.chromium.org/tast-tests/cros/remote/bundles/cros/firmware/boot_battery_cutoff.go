// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/futility"
	"go.chromium.org/tast-tests/cros/common/flashrom"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const (
	// Sleep time is set to 60 seconds due to SMP batteries requirement.
	sleepDuration = 60 * time.Second
	// Expected error messages when the EC is unresponsive.
	errmsg1 = "Timed out waiting for interfaces to become available"
	errmsg2 = "No data was sent from the pty"
	errmsg3 = "EC: Timeout waiting for response."
)

type reconnectErr struct {
	*errors.E
}

func init() {
	testing.AddTest(&testing.Test{
		Func: BootBatteryCutoff,
		Desc: "Verify if system can boot after battery cutoff",
		Contacts: []string{
			"chromeos-faft@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		// TODO(b/427195218): Add servo-exists + servo_state:WORKING after bug resolved.
		TestBedDeps:  []string{tbdep.ServoPresent},
		Attr:         []string{"group:firmware", "firmware_ec", "firmware_stressed", "firmware_meets_kpi", "firmware_ec_ro", "firmware_ec_rw"},
		Fixture:      fixture.NormalMode,
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.Battery()),
		Params: []testing.Param{{
			Name:              "chromeslate",
			ExtraHardwareDeps: hwdep.D(hwdep.FormFactor(hwdep.Chromeslate)),
			Val:               true,
		}, {
			ExtraHardwareDeps: hwdep.D(hwdep.SkipOnFormFactor(hwdep.Chromeslate)),
			Val:               false,
		}},
		Timeout: 20 * time.Minute,
	})
}

func BootBatteryCutoff(ctx context.Context, s *testing.State) {
	ffIsChromeslate := s.Param().(bool)

	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to get config: ", err)
	}

	// For debugging purposes, log servo and dut connection type.
	servoType, err := h.Servo.GetServoType(ctx)
	if err != nil {
		s.Fatal("Failed to find servo type: ", err)
	}
	s.Logf("Servo type: %s", servoType)

	dutConnType, err := h.Servo.GetDUTConnectionType(ctx)
	if err != nil {
		s.Fatal("Failed to find dut connection type: ", err)
	}
	s.Logf("DUT connection type: %s", dutConnType)

	hasMicroOrC2D2, err := h.Servo.PreferDebugHeader(ctx)
	if err != nil {
		s.Fatal("PreferDebugHeader: ", err)
	}

	// This function will disconnect the charger, send the command Batterycutoff, and wait for 60 seconds.
	sendingBatteryCutoff := func(ctx context.Context) error {
		// Disconnect Charger.
		s.Log("Stopping power supply")
		if err := firmware.PollToSetChargerStatus(ctx, h, false); err != nil {
			if h.RPM != nil {
				return errors.Wrap(err, "failed to remove charger using rpm")
			}
			return errors.Wrap(err, "failed to remove charger")
		}
		s.Log("Charger is removed")

		// Remove CCD watchdog for servod not to close when power supply is stopped after sending batterycutoff command.
		s.Log("Disabling CCD watchdog")
		if err := h.Servo.RemoveCCDWatchdogs(ctx); err != nil {
			s.Fatal("Failed to remove CCD watchdog: ", err)
		}

		// Make sure DUT is connected before sending command over ssh.
		if err := h.WaitConnect(ctx, firmware.ResetEthernetDongle); err != nil {
			return errors.Wrap(err, "failed to connect to DUT")
		}
		// Send batterycutoff command.
		s.Log("Sending batterycutoff command")
		if err := s.DUT().Conn().CommandContext(ctx, "sync").Run(ssh.DumpLogOnError); err != nil {
			return errors.Wrap(err, "failed to run sync command")
		}
		unreachableCtx, close := context.WithTimeout(ctx, 10*time.Second)
		defer close()
		cmd := s.DUT().Conn().CommandContext(unreachableCtx, "ectool", "batterycutoff")
		if err := cmd.Start(); err != nil {
			return errors.Wrap(err, "failed to send batterycutoff command")
		}
		err := cmd.Wait(ssh.DumpLogOnError)
		s.Log("ectool batterycutoff: ", err)

		// Verify the DUT becomes unresponsive.
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			_, err := h.Servo.RunECCommandGetOutput(ctx, "version", []string{`.`})
			if err == nil {
				return errors.Wrap(err, "EC is still active after Batterycutoff")
			}
			if !strings.Contains(err.Error(), errmsg1) && !strings.Contains(err.Error(), errmsg2) && !strings.Contains(err.Error(), errmsg3) {
				return errors.Wrap(err, "unexpected EC error")
			}
			return nil
		}, &testing.PollOptions{Timeout: 60 * time.Second, Interval: 3 * time.Second}); err != nil {
			s.Fatal("EC did not become unresponsive: ", err)
		}
		s.Log("EC is unresponsive")

		// Wait for a 60-second-delay after sending the batterycutoff command.
		s.Logf("Sleep for %s", sleepDuration)
		// GoBigSleepLint: Test requirement on SMP battery.
		if err := testing.Sleep(ctx, sleepDuration); err != nil {
			return errors.Wrap(err, "failed to sleep")
		}
		return nil
	}

	// This function will try to reconnect to the DUT and check the system power state to assure DUT has booted.
	confirmBoot := func(ctx context.Context, wakeByAC bool) error {
		// Wait for a connection to the DUT.
		s.Log("Wait for SSH to DUT")
		waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
		defer cancelWaitConnect()

		if err := h.WaitConnect(waitConnectCtx, firmware.FromHibernation, firmware.ResetEthernetDongle); err != nil {
			if wakeByAC && errors.Is(err, context.DeadlineExceeded) {
				return &reconnectErr{E: errors.New("timed out reconnecting DUT. Attempting a press on power button")}
			}
			return errors.Wrap(err, "failed to reconnect to DUT")
		}
		return nil
	}

	// Open CCD at the end of the test if it's locked. Also, disable write protections
	// that were enabled during this test, so that other tests would not be affected
	// later (i.e. setting gbb flags to change boot mode).
	var (
		hardwareWPEnabled   bool
		ecSoftwareWPEnabled bool
		apSoftwareWPEnabled bool
	)
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Minute)
	defer cancel()

	defer func(ctx context.Context, hardwareWPEnabled, apSoftwareWPEnabled, ecSoftwareWPEnabled *bool) {
		// Cr50 goes to sleep when battery is disconnected, and when DUT wakes, CCD state might be locked.
		if hasCCD, err := h.Servo.HasCCD(ctx); err != nil {
			s.Fatal("While checking if servo has a CCD connection: ", err)
		} else if hasCCD {
			s.Log("Ensuring CCD is open")
			if err := h.OpenCCD(ctx, false, false); err != nil {
				s.Fatal("Failed to set CCD open: ", err)
			}
		}
		if *hardwareWPEnabled {
			s.Log("Disabling hardware write protect")
			if err := h.Servo.SetFWWPState(ctx, servo.FWWPStateForceOff); err != nil {
				s.Fatal("Failed to disable hardware write protect: ", err)
			}
			s.Log("Rebooting DUT to ensure hardware WP disabled")
			if err := h.Servo.SetPowerState(ctx, servo.PowerStateReset); err != nil {
				s.Fatal("Faild to reset DUT: ", err)
			}
			h.DisconnectDUT(ctx)
			if err := h.WaitConnect(ctx, firmware.ResetEthernetDongle); err != nil {
				s.Fatal("Failed to reconnect to DUT: ", err)
			}
		}
		if *apSoftwareWPEnabled {
			s.Log("Disabling ap software write protect")
			futilityInstance, err := futility.NewLocalBuilder(h.DUT).Build()
			if err != nil {
				s.Fatal("Failed to create futility instance: ", err)
			}

			if _, err := futilityInstance.SetWP(ctx, false); err != nil {
				s.Fatal("Failed to set AP wp to disable: ", err)
			}
		}
		if *ecSoftwareWPEnabled {
			s.Log("Disabling ec software write protect")
			if err := s.DUT().Conn().CommandContext(ctx, "ectool", "flashprotect", "disable").Run(ssh.DumpLogOnError); err != nil {
				s.Log("Error in running 'ectool flashprotect disable', got error: ", err)
				if err := verifyECSoftwareWPStatus(ctx, s, false); err != nil {
					s.Fatal("While verifying EC wp state: ", err)
				}
			}
		}
	}(cleanupCtx, &hardwareWPEnabled, &apSoftwareWPEnabled, &ecSoftwareWPEnabled)

	// Check ec and ap software write protect status.
	// Enable write protections before battery cutoff.
	for _, programmer := range []flashrom.Programmer{flashrom.ProgrammerEc, flashrom.ProgrammerHost} {
		var flashromConfig flashrom.Config
		flashromInstance, ctx, shutdown, _, err := flashromConfig.
			FlashromInit("").
			ProgrammerInit(programmer, "").
			SetDut(s.DUT()).
			Probe(ctx)
		defer func() {
			if err := shutdown(); err != nil {
				s.Error("Failed to shutdown flashromInstance: ", err)
			}
		}()
		if err != nil {
			s.Fatal("Flashrom probe failed, unable to build flashrom instance: ", err)
		}

		wpStatus, out, err := flashromInstance.SoftwareWriteProtectStatus(ctx)
		if err != nil {
			s.Fatalf("Failed to check for %s write protection: %v. Output is %s", programmer, err, string(out))
		}
		if !wpStatus {
			s.Logf("Enabling %s software write protect", programmer)
			switch programmer {
			case "ec":
				if err := s.DUT().Conn().CommandContext(ctx, "ectool", "flashprotect", "enable").Run(ssh.DumpLogOnError); err != nil {
					s.Log("Error in running 'ectool flashprotect enable', got error: ", err)
					if err := verifyECSoftwareWPStatus(ctx, s, true); err != nil {
						s.Fatal("While verifying EC wp state: ", err)
					}
				}
				ecSoftwareWPEnabled = true
			case "host":
				futilityInstance, err := futility.NewLocalBuilder(h.DUT).Build()
				if err != nil {
					s.Fatal("Failed to create futility instance: ", err)
				}

				if _, err := futilityInstance.SetWP(ctx, true); err != nil {
					s.Fatal("Failed to set AP wp to enable: ", err)
				}
				apSoftwareWPEnabled = true
			}
		}
	}

	// Check and enable hardware write protect.
	hardwareWP, err := h.Servo.GetString(ctx, servo.FWWPState)
	if err != nil {
		s.Fatal("Failed to get write protect state: ", err)
	}
	if servo.FWWPStateValue(hardwareWP) != servo.FWWPStateForceOn {
		s.Log("Enabling hardware write protect")
		if err := h.Servo.SetFWWPState(ctx, servo.FWWPStateForceOn); err != nil {
			s.Fatal("Failed to enable hardware write protect: ", err)
		}
		hardwareWPEnabled = true
	}

	// Send battery cutoff and check EC is unresponsive.
	if err := sendingBatteryCutoff(ctx); err != nil {
		s.Fatal("Failed to send Batterycutoff command and wait: ", err)
	}

	// Connect charger.
	s.Log("Starting power supply")
	if err := firmware.PollToSetChargerStatus(ctx, h, true); err != nil {
		s.Fatal("Failed to attach the charger: ", err)
	}
	s.Log("Charger attached")

	// Confirm a successful boot.
	if err := confirmBoot(ctx, true); err != nil {
		if _, ok := err.(*reconnectErr); ok {
			s.Log("Context error: ", err.(*reconnectErr))
			// Power button is another wake up pin to wake DUT from deep sleep.
			// If re-connecting charger fails in waking up DUT, try with a press
			// on power to fully wake DUT from G3 into S0.
			if err := wakeDUTS0(ctx, h); err != nil {
				s.Fatal("Unable to reconnect to DUT: ", err)
			}
		} else {
			s.Fatal("Failed to boot: ", err)
		}
	}
	s.Log("DUT booted succesfully")

	// If is a CHROMESLATE and a micro-servo is connected, repeat the test but wake up the DUT with power button.
	if ffIsChromeslate && hasMicroOrC2D2 {
		s.Log("Performing extra steps for CHROMESLATE")
		// Send battery cutoff and check EC is unresponsive.
		if err := sendingBatteryCutoff(ctx); err != nil {
			s.Fatal("Failed to send Batterycutoff command and wait: ", err)
		}

		// Attempt to boot DUT by pressing power button.
		s.Log("Pressing power key")
		if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.Dur(h.Config.HoldPwrButtonPowerOff)); err != nil {
			s.Fatal("Failed to press power button")
		}

		// Confirm a successful boot.
		if err := confirmBoot(ctx, false); err != nil {
			s.Fatal("Failed to boot: ", err)
		}
		s.Log("DUT booted succesfully")
	} else if ffIsChromeslate && !hasMicroOrC2D2 {
		// During this test, the EC will become unresponsive and a micro-servo will be required to press the powerkey.
		s.Log("WARNING: DUT is a chromeslate but no micro-servo is present")
	}
}

// wakeDUTS0 will check DUT's power state if replugging AC fails to fully awake DUT from battery cutoff.
// If DUT is at G3 or S5 power button will be pressed to advance it to S0.
func wakeDUTS0(ctx context.Context, h *firmware.Helper) error {
	retryCtx, cancelRetry := context.WithTimeout(ctx, 3*time.Minute)
	defer cancelRetry()

	if err := h.OpenCCD(ctx, false, false); err != nil {
		return errors.Wrap(err, "open ccd")
	}

	// Check if DUT is at G3. If DUT is in G3, use power button to boot it into S0.
	testing.ContextLog(retryCtx, "Checking if power state is at G3 or S5")
	if err := h.WaitForPowerStates(retryCtx, firmware.PowerStateInterval, 1*time.Minute, "G3", "S5"); err != nil {
		// For debugging purposes, if EC power state is found to be S0, check for the AP state.
		checkPowerState := func() string {
			testing.ContextLog(ctx, "Checking for the DUT's power state")
			state, err := h.Servo.GetECSystemPowerState(ctx)
			if err != nil {
				testing.ContextLog(ctx, "Error getting power state: ", err)
				return "unknown"
			}
			return state
		}
		value := checkPowerState()
		if value == "S0" {
			apPower, screenState, err := h.Servo.GetAPState(ctx)
			if err != nil {
				testing.ContextLog(ctx, "Failed to get ap power and screen state from cr50 console: ", err)
			}
			return errors.Wrapf(err, "found DUT's power state at S0, power of ap: %s, status of screen: %s", apPower, screenState)
		}
		return errors.Wrapf(err, "DUT disconnected due to other reasons, found power state %s", value)
	}
	testing.ContextLogf(retryCtx, "Pressing power button for %s to wake DUT into S0 from G3 or S5", h.Config.HoldPwrButtonPowerOn)
	if err := h.Servo.KeypressWithDuration(retryCtx, servo.PowerKey, servo.Dur(h.Config.HoldPwrButtonPowerOn)); err != nil {
		return errors.Wrap(err, "failed to press power button")
	}
	testing.ContextLog(retryCtx, "Waiting for power state S0")
	if err := h.WaitForPowerStates(retryCtx, firmware.PowerStateInterval, 1*time.Minute, "S0"); err != nil {
		return errors.Wrap(err, "unable to get power state at S0")
	}
	return nil
}

// verifyECSoftwareWPStatus checks that the ec write protection status is the
// expected value using flashrom. Some DUTs, such as Nautilus and Nautiluslte,
// failed the ectool command, but their wp status from flashrom showed otherwise.
func verifyECSoftwareWPStatus(ctx context.Context, s *testing.State, expected bool) (retErr error) {
	var flashromConfig flashrom.Config
	flashromInstance, ctx, shutdown, _, err := flashromConfig.
		FlashromInit("").
		ProgrammerInit(flashrom.ProgrammerEc, "").
		SetDut(s.DUT()).
		Probe(ctx)
	defer func() {
		if err := shutdown(); err != nil {
			if retErr == nil {
				retErr = errors.Wrap(err, "failed to shutdown flashromInstance")
			} else {
				testing.ContextLog(ctx, "Failed to shutdown flashromInstance: ", err)
			}
		}
	}()
	if err != nil {
		return errors.Wrap(err, "flashrom probe failed, unable to build flashrom instance")
	}
	wpStatus, out, err := flashromInstance.SoftwareWriteProtectStatus(ctx)
	if err != nil {
		return errors.Wrapf(err, "failed to collect the ec write protection status with flashrom. Output is %s", string(out))
	}

	if wpStatus != expected {
		return errors.Wrapf(err, "EC software write protect not %t, got output: %s", expected, string(out))
	}
	testing.ContextLog(ctx, "WARNING: ectool returned a non-zero exit, but the wp status changed as expected")
	return nil
}
