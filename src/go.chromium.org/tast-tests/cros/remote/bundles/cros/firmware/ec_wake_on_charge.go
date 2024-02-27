// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/powercontrol"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type testArgsForECWakeOnCharge struct {
	formFactor    string
	tabletModeOff string
	hasLid        bool
}

type retriableErr struct {
	*errors.E
}

type debugInformation struct {
	servoConnectionType string
	servoType           string
	hasMicroOrC2D2      bool
	hasCCD              bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ECWakeOnCharge,
		Desc:         "Checks that device will charge when EC is in a low-power mode, as a replacement for manual test 1.4.11",
		LacrosStatus: testing.LacrosVariantUnneeded,
		Contacts: []string{
			"chromeos-faft@google.com",
			"arthur.chuang@cienet.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		// TODO: When stable, change firmware_unstable to a different attr.
		Attr:         []string{"group:firmware", "firmware_unstable", "firmware_bringup"},
		SoftwareDeps: []string{"chrome"},
		Vars:         []string{"board", "model"},
		Fixture:      fixture.NormalMode,
		ServiceDeps:  []string{"tast.cros.firmware.UtilsService"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.Battery()),
		Timeout:      40 * time.Minute,
		Params: []testing.Param{{
			Name:              "chromeslate",
			ExtraHardwareDeps: hwdep.D(hwdep.FormFactor(hwdep.Chromeslate)),
			Val: &testArgsForECWakeOnCharge{
				formFactor: "chromeslate",
				hasLid:     false,
			},
		}, {
			Name:              "convertible",
			ExtraHardwareDeps: hwdep.D(hwdep.FormFactor(hwdep.Convertible)),
			Val: &testArgsForECWakeOnCharge{
				formFactor:    "convertible",
				tabletModeOff: "tabletmode off",
				hasLid:        true,
			},
		}, {
			Name:              "detachable",
			ExtraHardwareDeps: hwdep.D(hwdep.FormFactor(hwdep.Detachable)),
			Val: &testArgsForECWakeOnCharge{
				formFactor:    "detachable",
				tabletModeOff: "basestate attach",
				hasLid:        true,
			},
		}, {
			ExtraHardwareDeps: hwdep.D(hwdep.FormFactor(hwdep.Clamshell)),
			Val: &testArgsForECWakeOnCharge{
				formFactor: "clamshell",
				hasLid:     true,
			},
		}},
	})
}

var (
	tabletModeAngleHasChanged bool
	checkedInfo               debugInformation
)

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

	// Increase timeout in getting response from ec uart.
	if err := h.Servo.SetString(ctx, "ec_uart_timeout", "10"); err != nil {
		s.Fatal("Failed to extend ec uart timeout: ", err)
	}
	defer func() {
		testing.ContextLog(ctx, "Restoring ec uart timeout to the default value of 3 seconds")
		if err := h.Servo.SetString(ctx, "ec_uart_timeout", "3"); err != nil {
			s.Fatal("Failed to restore default ec uart timeout: ", err)
		}
	}()

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
		if args.formFactor == "convertible" && tabletModeAngleHasChanged {
			// Check for tablet mode angles, and if they are different
			// than the default values, restore them to default.
			cmd := firmware.NewECTool(s.DUT(), firmware.ECToolNameMain)
			tabletModeAngleInit, hysInit, err := cmd.SaveTabletModeAngles(ctx)
			if err != nil {
				s.Fatal("Failed to read tablet mode angles: ", err)
			} else if tabletModeAngleInit != "180" || hysInit != "20" {
				s.Log("Restoring ectool tablet mode angles to the default settings")
				if err := cmd.ForceTabletModeAngle(ctx, "180", "20"); err != nil {
					s.Fatal("Failed to restore tablet mode angles to the default settings: ", err)
				}
			}
		}
	}(cleanupCtx, args)

	// Monitor servo connection in the background, and attempt to reconnect if the connection drops.
	done := make(chan struct{})
	defer func() {
		done <- struct{}{}
	}()
	go func() {
	monitorServo:
		for {
			select {
			case <-done:
				break monitorServo
			default:
				// GoBigSleepLint: Ping servo periodically to monitor whether it's alive.
				if err := testing.Sleep(ctx, time.Second); err != nil {
					s.Error("Failed to sleep while monitoring servo: ", err)
				}
				if _, err := h.Servo.Echo(ctx, "ping"); err != nil {
					s.Log("Failed to ping servo, reconnecting: ", err)
					err := h.ServoProxy.Reconnect(ctx)
					if err != nil {
						s.Error("Failed to connect to servo: ", err)
					}
				}
			}
		}
	}()
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

		s.Log("Stopping AC Power")
		if err := powercontrol.PlugUnplugCharger(ctx, h, false); err != nil {
			s.Fatal("Failed to stop power supply: ", err)
		}

		// Skip setting the lid state for DUTs that don't have a lid, i.e. Chromeslates.
		if args.hasLid {
			if tc.lidOpen == servo.LidOpenNo {
				// Closing lid might cause servod exit if the CCD watchdog is not removed.
				s.Log("Removing ccd watchdogs")
				if err := h.Servo.RemoveCCDWatchdogs(ctx); err != nil {
					s.Fatal("Failed to remove CCD watchdog: ", err)
				}
			}
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
			if err := hibernateDUT(ctx, h, s.DUT(), checkedInfo.hasMicroOrC2D2, tc.lidOpen, args.formFactor, args.tabletModeOff); err != nil {
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
					// Long press the power button to boot the device to S0.
					durToWakeToS0 = servo.DurLongPress
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

// checkTabletModeStatus checks whether ChromeOS is in tablet mode through the utils service.
func checkTabletModeStatus(ctx context.Context, h *firmware.Helper) (bool, error) {
	if err := h.RequireRPCUtils(ctx); err != nil {
		return false, errors.Wrap(err, "requiring RPC utils")
	}
	if _, err := h.RPCUtils.NewChrome(ctx, &empty.Empty{}); err != nil {
		return false, errors.Wrap(err, "failed to create instance of chrome")
	}
	defer h.RPCUtils.CloseChrome(ctx, &empty.Empty{})
	res, err := h.RPCUtils.EvalTabletMode(ctx, &empty.Empty{})
	if err != nil {
		return false, err
	}
	return res.TabletModeEnabled, nil
}

func checkInformation(ctx context.Context, h *firmware.Helper, info *debugInformation) error {
	var wantInfo = []string{"hasMicroOrC2D2", "hasCCD", "servoType", "servoConnectionType"}
	for _, val := range wantInfo {
		var err error
		switch val {
		case "hasMicroOrC2D2":
			info.hasMicroOrC2D2, err = h.Servo.PreferDebugHeader(ctx)
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
	testing.ContextLogf(ctx, "DUT has micro or c2d2: %t, has CCD: %t, servo type: %s, servo connection type: %s",
		info.hasMicroOrC2D2, info.hasCCD, info.servoType, info.servoConnectionType)
	return nil
}

func ensureClamshellMode(ctx context.Context, h *firmware.Helper, dut *dut.DUT, formFactor, tabletModeOff string) error {
	inTabletMode, err := checkTabletModeStatus(ctx, h)
	if err != nil {
		return errors.Wrap(err, "unable to check for DUT's tablet mode status")
	}
	if inTabletMode {
		testing.ContextLog(ctx, "DUT is in tablet mode. Attempting to turn tablet mode off")
		out, err := h.Servo.RunTabletModeCommandGetOutput(ctx, tabletModeOff)
		if err != nil {
			if formFactor == "convertible" {
				testing.ContextLogf(ctx, "Failed to run %s: %v. Attempting to set rotation angles with ectool instead", tabletModeOff, err)
				cmd := firmware.NewECTool(dut, firmware.ECToolNameMain)
				// Setting tabletModeAngle to 360 will force DUT into clamshell mode.
				if err := cmd.ForceTabletModeAngle(ctx, "360", "0"); err != nil {
					return errors.Wrap(err, "failed to set DUT in clamshell mode")
				}
				tabletModeAngleHasChanged = true
				return nil
			}
			return errors.Wrapf(err, "failed to run %s", tabletModeOff)
		}
		testing.ContextLogf(ctx, "Tablet mode status: %s", out)
	}
	return nil
}

func hibernateDUT(ctx context.Context, h *firmware.Helper, dut *dut.DUT, hasMicroOrC2D2 bool, lidOpen servo.LidOpenValue, formFactor, tabletModeOff string) error {
	switch lidOpen {
	case servo.LidOpenNo:
		if hasMicroOrC2D2 {
			// In cases where lid is closed, and there's a servo_micro or C2D2 connection,
			// use console command to hibernate. Using keyboard presses might trigger DUT
			// to wake, and interrupt lid emulation.
			testing.ContextLog(ctx, "Putting DUT in hibernation with EC console command")
			if err := h.Servo.ECHibernate(ctx, h.Model, servo.UseConsole); err != nil {
				return errors.Wrap(err, "failed to hibernate")
			}
			return nil
		}
		// Note: In most cases, when lid is closed without log-in, power state transitions from S0 to S5,
		// and then eventually to G3, which would be equivalent to long pressing on power to put DUT asleep.
		// However, recent Testhaus results showed that dedede(kracko) remained connected with power state
		// at S0 after lid closed. Attempt power-off if closing lid didn't put DUTs in G3.
		testing.ContextLog(ctx, "Waiting for power state to become G3 or S5")
		if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, 5*time.Minute, "G3", "S5"); err != nil {
			// Sometimes EC becomes unresponsive when lid is closed. But, for this test's
			// purposes, we're more concerned about whether DUT wakes when AC is re-attached.
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
	case servo.LidOpenYes:
		if formFactor == "convertible" || formFactor == "detachable" {
			// For convertibles and detachables, if DUT was left in tablet mode by previous tests,
			// key presses may be inactive. Always turn tablet mode off before using keyboard to hibernate.
			testing.ContextLogf(ctx, "DUT is a %s. Checking tablet mode status before using keyboard to hibernate", formFactor)
			if err := ensureClamshellMode(ctx, h, dut, formFactor, tabletModeOff); err != nil {
				return err
			}
		}
		h.DisconnectDUT(ctx)
		testing.ContextLog(ctx, "Putting DUT in hibernation with key presses")
		if err := h.Servo.ECHibernate(ctx, h.Model, servo.UseKeyboard); err != nil {
			testing.ContextLogf(ctx, "Failed to hibernate: %v. Retry with using EC console command to hibernate", err)
			if err := h.Servo.ECHibernate(ctx, h.Model, servo.UseConsole); err != nil {
				return errors.Wrap(err, "failed to hibernate")
			}
		}
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
		Timeout:  1 * time.Minute,
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
		// Open CCD before talking to the EC.
		if err := h.OpenCCD(ctx, true, true); err != nil {
			return errors.Wrap(err, "failed to open CCD")
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
