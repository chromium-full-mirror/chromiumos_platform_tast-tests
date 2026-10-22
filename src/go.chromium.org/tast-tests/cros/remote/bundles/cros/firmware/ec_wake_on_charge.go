// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/powercontrol"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type testArgsForECWakeOnCharge struct {
	hasLid bool
}

type retriableErr struct {
	*errors.E
}

type debugInformation struct {
	servoConnectionType string
	servoType           string
	hasMicroOrC2D2      bool
	hasServoMicro       bool
	hasCCD              bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func: ECWakeOnCharge,
		Desc: "Checks that device will charge when EC is in a low-power mode, as a replacement for manual test 1.4.11",
		Contacts: []string{
			"chromeos-faft@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		TestBedDeps:  tbdep.ServoPresentAndWorking,
		// TODO: When stable, change firmware_unstable to a different attr.
		Attr:         []string{"group:firmware", "firmware_unstable", "firmware_bringup"},
		Vars:         []string{"board", "model"},
		Fixture:      fixture.NormalMode,
		ServiceDeps:  []string{"tast.cros.firmware.UtilsService"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.Battery()),
		Timeout:      40 * time.Minute,
		Params: []testing.Param{{
			Name:              "chromeslate",
			ExtraHardwareDeps: hwdep.D(hwdep.FormFactor(hwdep.Chromeslate)),
			Val: &testArgsForECWakeOnCharge{
				hasLid: false,
			},
		}, {
			Name:              "convertible",
			ExtraHardwareDeps: hwdep.D(hwdep.FormFactor(hwdep.Convertible)),
			Val: &testArgsForECWakeOnCharge{
				hasLid: true,
			},
		}, {
			Name:              "detachable",
			ExtraHardwareDeps: hwdep.D(hwdep.FormFactor(hwdep.Detachable)),
			Val: &testArgsForECWakeOnCharge{
				hasLid: true,
			},
		}, {
			ExtraHardwareDeps: hwdep.D(hwdep.FormFactor(hwdep.Clamshell)),
			Val: &testArgsForECWakeOnCharge{
				hasLid: true,
			},
		}},
	})
}

var checkedInfo debugInformation

func ECWakeOnCharge(ctx context.Context, s *testing.State) {

	h := s.FixtValue().(*fixture.Value).Helper

	board, _ := s.Var("board")
	model, _ := s.Var("model")
	h.OverridePlatform(ctx, board, model)

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to get config: ", err)
	}

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}

	// At start of test, save some information that may
	// be useful for debugging.
	if err := checkInformation(ctx, h, &checkedInfo); err != nil {
		s.Fatal("Unable to log information at start of test: ", err)
	}

	// To prevent leaving DUT in an offline state, open lid or perform a cold reset at the end of test.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Minute)
	defer cancel()

	args := s.Param().(*testArgsForECWakeOnCharge)
	defer func(ctx context.Context, args *testArgsForECWakeOnCharge) {
		if args.hasLid {
			if err := h.Servo.OpenLid(ctx); err != nil {
				s.Fatal("Failed to set lid state: ", err)
			}
		}
		if err := h.EnsureDUTBooted(ctx); err != nil {
			s.Fatal("Failed to ensure DUT booted: ", err)
		}
	}(cleanupCtx, args)

	for _, tc := range []struct {
		lidOpen servo.LidOpenValue
	}{
		{servo.LidOpenYes},
		{servo.LidOpenNo},
	} {
		// Only repeat the test in lid closed when device has a lid.
		if tc.lidOpen == servo.LidOpenNo && !args.hasLid {
			break
		}

		s.Log("Removing ccd watchdogs before power changes")
		if err := h.Servo.RemoveCCDWatchdogs(ctx); err != nil {
			s.Fatal("Failed to remove CCD watchdog: ", err)
		}

		s.Log("Stopping AC Power")
		if err := powercontrol.PlugUnplugCharger(ctx, h, false); err != nil {
			s.Fatal("Failed to stop power supply: ", err)
		}

		// Restart UI to ensure no user is logged in, as this will change power state behaviour on lid close.
		if err := h.DUT.Conn().CommandContext(ctx, "restart", "ui").Run(ssh.DumpLogOnError); err != nil {
			s.Fatal("Failed to restart ui before test: ", err)
		}

		// Set EC hibernation delay before closing the lid (which triggers powerd shutdown).
		// On CCD-only and C2D2 setups (!hasServoMicro), lid_open:no is either emulated in EC RAM via
		// 'lidclose' or driven via GSC console 'CCD_REC_LID_SWITCH', which does not hold LID_OPEN low
		// across EC hibernation and Z-state. Only arm hibdelay when lid is open or when servo_micro
		// drives the physical lid_open GPIO expander.
		if h.Config.Hibernate && (tc.lidOpen == servo.LidOpenYes || checkedInfo.hasServoMicro) {
			ecTool := firmware.NewECTool(s.DUT(), firmware.ECToolNameMain)
			s.Log("Setting EC hibernation delay to 5s via ectool")
			if err := ecTool.SetHibernationDelay(ctx, 5); err != nil {
				s.Fatal("Failed to set EC hibernation delay: ", err)
			}
		}

		// Skip setting the lid state for DUTs that don't have a lid, i.e. Chromeslates.
		if args.hasLid {
			s.Logf("-------------Test with lid open: %s-------------", tc.lidOpen)
			if err := testing.Poll(ctx, func(ctx context.Context) error {
				if err := h.Servo.SetStringAndCheck(ctx, servo.LidOpen, string(tc.lidOpen)); err != nil {
					// This error may be temporary.
					if strings.Contains(err.Error(), "No data was sent from the pty") ||
						strings.Contains(err.Error(), "Timed out waiting for interfaces to become available") {
						return err
					}
					return testing.PollBreak(err)
				}
				return nil
			}, &testing.PollOptions{Timeout: 3 * time.Minute, Interval: 1 * time.Second}); err != nil {
				s.Fatal("Failed to set lid state: ", err)
			}
			// There's a chance that CCD would close when lid closed.
			// Open CCD to prevent errors from running EC commands.
			if tc.lidOpen == servo.LidOpenNo {
				if err := h.OpenCCD(ctx, true, true); err != nil {
					s.Fatal("Failed to open CCD after closing lid: ", err)
				}
			}
		}

		if h.Config.Hibernate {
			if err := hibernateDUT(ctx, h, checkedInfo.hasServoMicro, tc.lidOpen); err != nil {
				s.Fatal("Failed to hibernate DUT: ", err)
			}
		} else {
			// For DUTs that do not support the ec hibernation command, when lid is open, we could use
			// a long power button press instead to put DUT in deep sleep. But, when lid is closed without
			// log-in, power state will eventually reach G3.
			if tc.lidOpen == servo.LidOpenYes {
				s.Logf("Long pressing on power key for %s to put DUT into deep sleep mode", h.Config.HoldPwrButtonPowerOff)
				if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.Dur(h.Config.HoldPwrButtonPowerOff)); err != nil {
					s.Fatal("Failed to set a KeypressControl by servo: ", err)
				}
			}
			s.Log("Waiting for power state to become G3 or S3")
			if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, 1*time.Minute, "G3", "S3"); err != nil {
				s.Fatal("Failed to get powerstates at G3 or S3: ", err)
			}
		}

		s.Log("Reconnecting AC")
		if err := checkECWakesFromACReconnected(ctx, h, string(tc.lidOpen)); err != nil {
			_, ok := err.(*retriableErr)
			if ok && !h.Config.ACOnCanWakeApFromUlp {
				s.Log("AC on event cannot wake AP, putting AP back to SO with power button")
				if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout, "G3"); err != nil {
					s.Fatal("Failed to get G3 power state: ", err)
				}
				durToWakeToS0 := servo.DurTab
				if h.Config.Platform == "kukui" {
					durToWakeToS0 = servo.Dur(h.Config.HoldPwrButtonPowerOn)
				}
				if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, durToWakeToS0); err != nil {
					s.Fatal("Failed to press power button: ", err)
				}
				s.Log("Waiting for DUT to reconnect")
				waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
				defer cancelWaitConnect()

				if err := h.WaitConnect(waitConnectCtx); err != nil {
					s.Fatal("Failed to reconnect to DUT: ", err)
				}
			} else {
				s.Fatal("Failed to wake EC from AC reconnected: ", err)
			}
		}

		if h.Config.Hibernate && tc.lidOpen == servo.LidOpenYes {
			if err := h.VerifyECHibernateWakeResetFlags(ctx); err != nil {
				s.Fatal("Failed to verify EC reset flags after wake on charge: ", err)
			}
		}

		// Testhaus results showed that on Nami DUTs, when tested with lid closed,
		// lid's state changed after waking up from hibernation. More research
		// might be needed, but for now skip on checking lid state for Nami devices.
		if args.hasLid && h.Config.Platform != "nami" {
			// Verify DUT's lid current state remains the same as the initial state.
			s.Log("Checking lid state hasn't changed")
			lidStateFinal, err := h.Servo.LidOpenState(ctx)
			if err != nil {
				s.Fatal("Failed to check the final lid state: ", err)
			}
			if lidStateFinal != string(tc.lidOpen) {
				s.Fatalf("DUT's lid_open state has changed from %s to %s", tc.lidOpen, lidStateFinal)
			}
		}
	}
}

func checkInformation(ctx context.Context, h *firmware.Helper, info *debugInformation) error {
	var wantInfo = []string{"hasMicroOrC2D2", "hasServoMicro", "hasCCD", "servoType", "servoConnectionType"}
	for _, val := range wantInfo {
		var err error
		switch val {
		case "hasMicroOrC2D2":
			info.hasMicroOrC2D2, err = h.Servo.PreferDebugHeader(ctx)
		case "hasServoMicro":
			info.hasServoMicro, err = h.Servo.HasServoMicro(ctx)
		case "hasCCD":
			info.hasCCD, err = h.Servo.HasCCD(ctx)
		case "servoType":
			info.servoType, err = h.Servo.GetServoType(ctx)
		case "servoConnectionType":
			info.servoConnectionType, err = h.Servo.GetString(ctx, servo.DUTConnectionType)
		}
		if err != nil {
			return errors.Wrapf(err, "failed to check for %s", val)
		}
	}
	testing.ContextLogf(ctx, "DUT has micro or c2d2: %t, has servo_micro: %t, has CCD: %t, servo type: %s, servo connection type: %s",
		info.hasMicroOrC2D2, info.hasServoMicro, info.hasCCD, info.servoType, info.servoConnectionType)
	return nil
}

func hibernateDUT(ctx context.Context, h *firmware.Helper, hasServoMicro bool, lidOpen servo.LidOpenValue) error {
	if lidOpen == servo.LidOpenNo && !hasServoMicro {
		// Without servo_micro's physical GPIO expander, lid_open:no is not held low across
		// EC hibernation and Z-state. Wait for G3 or S5 only.
		testing.ContextLog(ctx, "Waiting for power state to become G3 or S5")
		if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, 5*time.Minute, "G3", "S5"); err != nil {
			if strings.Contains(err.Error(), "No data was sent from the pty") ||
				strings.Contains(err.Error(), "Timed out waiting for interfaces to become available") {
				testing.ContextLog(ctx, "DUT appears to be completely offline. We're okay as long as reconnecting power resumes it")
				return nil
			}
			if h.DUT.Connected(ctx) {
				testing.ContextLog(ctx, "Found dut still connected after lid closed, attempting power-off")
				if err := h.DUT.Conn().CommandContext(ctx, "poweroff").Start(); err != nil {
					return errors.Wrap(err, "failed to run poweroff cmd")
				}
				if err := h.DUT.WaitUnreachable(ctx); err != nil {
					return errors.Wrap(err, "failed to wait for DUT unreachable")
				}
				return nil
			}
			return errors.Wrap(err, "failed to get powerstates at G3 or S5")
		}
		return nil
	}

	switch lidOpen {
	case servo.LidOpenNo:
		// Closing the lid without login triggers powerd shutdown to G3. If the DUT is
		// still reachable after 30s, explicitly power it off.
		testing.ContextLog(ctx, "Waiting for DUT to shut down after lid close")
		waitCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		if err := h.DUT.WaitUnreachable(waitCtx); err != nil {
			testing.ContextLog(ctx, "DUT still reachable after lid close, running poweroff")
			if err := h.DUT.Conn().CommandContext(ctx, "poweroff").Start(); err != nil {
				return errors.Wrap(err, "failed to run poweroff cmd")
			}
			if err := h.DUT.WaitUnreachable(ctx); err != nil {
				return errors.Wrap(err, "failed to wait for DUT unreachable")
			}
		}
	case servo.LidOpenYes:
		testing.ContextLog(ctx, "Powering off AP to G3 for EC hibernation")
		if err := h.DUT.Conn().CommandContext(ctx, "poweroff").Start(); err != nil {
			return errors.Wrap(err, "failed to run poweroff cmd")
		}
		if err := h.DUT.WaitUnreachable(ctx); err != nil {
			return errors.Wrap(err, "failed to wait for DUT unreachable")
		}
	}
	h.DisconnectDUT(ctx)
	// GoBigSleepLint: Wait for the 5s G3 hibernation delay to expire and EC to enter hibernation.
	if err := testing.Sleep(ctx, 15*time.Second); err != nil {
		return errors.Wrap(err, "failed to sleep waiting for EC hibernation")
	}
	if err := h.Servo.CheckUnresponsiveEC(ctx); err != nil {
		return errors.Wrap(err, "while verifying whether EC is unresponsive after hibernating DUT")
	}
	return nil
}

// checkECWakesFromACReconnected plugs the charger, probes the ec for
// response, and expect the DUT to reconnect if the lid was open, or
// remain at G3 if the lid was closed. It is called to wake the DUT
// from hibernation.
func checkECWakesFromACReconnected(ctx context.Context, h *firmware.Helper, lidOpen string) error {
	// getChargerPollOptions sets the time to retry the GetChargerAttached command.
	var getChargerPollOptions = testing.PollOptions{
		Timeout:  3 * time.Minute,
		Interval: 1 * time.Second,
	}
	// getPDRolePollOptions sets the time to retry the GetPDRole command.
	var getPDRolePollOptions = testing.PollOptions{
		Timeout:  10 * time.Second,
		Interval: 1 * time.Second,
	}
	// runECPollOptions sets the time to retry the RunECCommandGetOutput command.
	var runECPollOptions = testing.PollOptions{
		Timeout:  1 * time.Minute,
		Interval: 1 * time.Second,
	}
	if err := h.SetDUTPower(ctx, true); err != nil {
		return errors.Wrap(err, "failed to connect charger")
	}
	if checkedInfo.servoConnectionType == "type-c" {
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			role, err := h.Servo.GetPDRole(ctx)
			if err != nil {
				return errors.Wrap(err, "failed to retrieve USB PD role for servo")
			}
			if role != servo.PDRoleSrc {
				return errors.Wrapf(err, "setting pd role to src, but got: %q", role)
			}
			return nil
		}, &getPDRolePollOptions); err != nil {
			return err
		}
	}
	// Verify EC console is responsive.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		// Cr50 goes to sleep during hibernation, and when DUT wakes, CCD state might be locked.
		// Open CCD before talking to the EC if CCD is present.
		if checkedInfo.hasCCD {
			shortCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			if err := h.OpenCCD(shortCtx, true, true); err != nil {
				return errors.Wrap(err, "failed to open CCD")
			}
		}
		if _, err := h.Servo.RunECCommandGetOutput(ctx, "version", []string{`.`}); err != nil {
			return errors.Wrap(err, "EC is not responsive after reconnecting power supply to DUT")
		}
		return nil
	}, &runECPollOptions); err != nil {
		return errors.Wrap(err, "failed to wait for EC to become responsive")
	}
	testing.ContextLog(ctx, "EC is responsive")

	// Verify that DUT is charging with power supply connected.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		ok, err := h.Servo.GetChargerAttached(ctx)
		testing.ContextLogf(ctx, "GetChargerAttached: ok=%v err=%v", ok, err)
		if err != nil {
			return errors.Wrap(err, "error checking whether charger is attached")
		}
		if !ok {
			return errors.New("DUT is not charging after connecting the power supply")
		}
		return nil
	}, &getChargerPollOptions); err != nil {
		return errors.Wrap(err, "failed to check for charger after connecting power")
	}
	testing.ContextLog(ctx, "DUT is charging")

	if lidOpen == string(servo.LidOpenYes) {
		testing.ContextLog(ctx, "Waiting for DUT to power ON")
		waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
		defer cancelWaitConnect()

		var opts []firmware.WaitConnectOption
		opts = append(opts, firmware.FromHibernation)
		err := h.WaitConnect(waitConnectCtx, opts...)
		if errors.As(err, &context.DeadlineExceeded) {
			// For some devices, the ac on event cannot wake the AP.
			// Return a retriableErr and allow the test to continue by
			// booting up the device with a short power button press.
			return &retriableErr{E: errors.Wrap(err, "timeout in reconnecting to DUT")}
		}
		return err
	}
	// When lid is closed, DUTs remain in G3 after waking up from
	// hibernation with charger reconnected.
	testing.ContextLog(ctx, "Checking for power state at G3")
	if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout, "G3"); err != nil {
		return errors.Wrap(err, "failed to get power state at G3")
	}
	return nil
}
