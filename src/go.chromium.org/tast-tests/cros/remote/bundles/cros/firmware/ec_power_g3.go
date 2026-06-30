// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"regexp"
	"strconv"
	"time"

	fwCommon "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type powerOffMethod int

const (
	shutdownCommand powerOffMethod = iota
	longPowerButtonPress
	powerStateOff
)

type powerG3Params struct {
	PowerOffMethod    powerOffMethod
	PowerStateTimeout time.Duration
	RemovePower       bool
	SetRecScreen      bool
	SetRecMode        bool
	CheckUSB          bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func: ECPowerG3,
		Desc: "Test that DUT goes to G3 powerstate on various types of shutdown",
		Contacts: []string{
			"chromeos-faft@google.com",
			"tij@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		TestBedDeps:  tbdep.ServoPresentAndWorking,
		Attr:         []string{"group:firmware", "firmware_ec", "firmware_meets_kpi", "firmware_stressed", "firmware_ec_ro", "firmware_ec_rw"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Fixture:      fixture.NormalMode,
		Params: []testing.Param{
			{
				Name: "shutdown",
				Val: powerG3Params{
					PowerOffMethod: shutdownCommand,
				},
			},
			{
				Name:      "power_button",
				ExtraAttr: []string{"firmware_enabled", "firmware_bringup"},
				Val: powerG3Params{
					PowerOffMethod: longPowerButtonPress,
				},
			},
			{
				Name:      "power_state",
				ExtraAttr: []string{"firmware_enabled", "firmware_bringup", "group:labqual"},
				Val: powerG3Params{
					PowerOffMethod: powerStateOff,
				},
			},
			{
				Name:             "power_state_usb_plugged_in",
				ExtraTestBedDeps: []string{tbdep.ServoUSBState("NORMAL")},
				ExtraAttr:        []string{"firmware_enabled", "group:labqual"},
				Val: powerG3Params{
					PowerOffMethod: powerStateOff,
					CheckUSB:       true,
				},
				Timeout: 120 * time.Minute,
			},
			{
				Name:              "power_state_snk",
				ExtraAttr:         []string{"firmware_enabled", "firmware_bringup"},
				ExtraHardwareDeps: hwdep.D(hwdep.Battery()),
				Val: powerG3Params{
					PowerOffMethod: powerStateOff,
					RemovePower:    true,
				},
			},
			{
				Name:             "power_state_rec_off",
				ExtraTestBedDeps: []string{tbdep.ServoUSBState("NORMAL")},
				ExtraAttr:        []string{"firmware_enabled", "group:labqual"},
				Val: powerG3Params{
					PowerOffMethod: powerStateOff,
					SetRecMode:     true,
				},
				Timeout: 120 * time.Minute,
			},
			{
				Name:      "power_button_from_ro",
				ExtraAttr: []string{"firmware_enabled", "group:labqual"},
				Val: powerG3Params{
					PowerOffMethod: longPowerButtonPress,
					SetRecScreen:   true,
					// Verify if the power state transitions to G3 after PowerStateTimeout.
					PowerStateTimeout: 11 * time.Second,
				},
				Timeout: 15 * time.Minute,
			},
		},
	})
}

func ECPowerG3(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}
	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to get fw-testing-config: ", err)
	}
	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		s.Fatal("Failed to create mode switcher: ", err)
	}

	tc := s.Param().(powerG3Params)

	if tc.CheckUSB {
		cs := s.CloudStorage()
		if err := h.SetupUSBKey(ctx, cs); err != nil {
			s.Fatal("USBKey not working: ", err)
		}
	}

	if tc.RemovePower {
		s.Log("Removing charger")
		if err := h.SetDUTPower(ctx, false); err != nil {
			s.Fatal("Failed to remove charger: ", err)
		}
		if err := h.Servo.RemoveCCDWatchdogs(ctx); err != nil {
			s.Fatal("Failed to remove watchdog main: ", err)
		}
	}

	slowG3 := false
	if tc.SetRecScreen {
		// Some devices passed fw qual before power_button_from_ro is required(b/434814366).
		// We enlarge the wait time for these firmware version.
		// `ectool version` in older firmware only output commit hash and do not
		// contain build version like 15194.190.0 so we read ap firmware version.
		roVersion, _, err := h.Reporter.GetFWRORWVersion(ctx)
		if err != nil {
			s.Fatal("Failed to determine RO AP version: ", err)
		}
		re := regexp.MustCompile(`\b(\d+)\.(\d+).(\d+)\b`)

		match := re.FindStringSubmatch(roVersion)
		branchVersion, err := strconv.Atoi(match[2])
		if err == nil && len(match) > 0 && match[1] == "15194" && branchVersion <= 190 {
			slowG3 = true
		}
		s.Log("Booting the DUT to the recovery screen")
		if err := ms.EnableRecMode(ctx, servo.PowerStateRec, servo.USBMuxOff); err != nil {
			s.Fatal("Failed to boot to recovery screen: ", err)
		}
	}

	if tc.SetRecMode {
		s.Log("Rebooting into recovery mode")
		if err := ms.RebootToMode(ctx, fwCommon.BootModeRecovery); err != nil {
			s.Fatal("Failed to reboot into recovery mode: ", err)
		}
	}

	switch tc.PowerOffMethod {
	case shutdownCommand:
		s.Log("Shut down DUT")
		cmd := h.DUT.Conn().CommandContext(ctx, "/sbin/shutdown", "-P", "now")
		if err := cmd.Start(); err != nil {
			s.Fatal("Failed to shut down DUT: ", err)
		}
	case longPowerButtonPress:
		s.Log("Long press power button for ", h.Config.HoldPwrButtonNoPowerdShutdown)
		if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.Dur(h.Config.HoldPwrButtonNoPowerdShutdown)); err != nil {
			s.Fatal("Failed to power off DUT with long press of the power button: ", err)
		}
	case powerStateOff:
		s.Log("Power state off")
		if err := h.Servo.SetPowerState(ctx, servo.PowerStateOff); err != nil {
			s.Fatal("Failed to power off DUT with power state off: ", err)
		}
	}

	h.DisconnectDUT(ctx)
	s.Log("Check for G3 powerstate")
	powerStateTimeout := firmware.PowerStateTimeout
	if tc.PowerStateTimeout > 0 {
		if slowG3 {
			powerStateTimeout = 30 * time.Second
		} else {
			powerStateTimeout = tc.PowerStateTimeout
		}
	}
	if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, powerStateTimeout, "G3"); err != nil {
		s.Fatal("Failed to get G3 powerstate: ", err)
	}

	if tc.PowerOffMethod == powerStateOff {
		s.Log("Power state off")
		if err := h.Servo.SetPowerState(ctx, servo.PowerStateOff); err != nil {
			s.Fatal("Failed to power off DUT with power state off: ", err)
		}
		//GoBigSleepLint: Verify that power state off doesn't power back on by mistake.
		testing.Sleep(ctx, 10*time.Second)
		if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout, "G3"); err != nil {
			s.Fatal("Failed to get G3 powerstate: ", err)
		}
		if tc.CheckUSB {
			s.Log("Setting DFP mode")
			if err := h.Servo.SetDUTPDDataRole(ctx, servo.DFP); err != nil {
				s.Logf("Failed to set pd data role to DFP: %.400s", err)
			}
			s.Log("Inserting a valid USB to DUT")
			if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxDUT); err != nil {
				s.Fatal("Failed to set USBMux: ", err)
			}
		}
		s.Log("Power DUT back on with power state on")
		if err := h.Servo.SetPowerState(ctx, servo.PowerStateOn); err != nil {
			s.Fatal("Failed to power on DUT with power state on: ", err)
		}
	} else {
		closeUART, err := h.Servo.EnableUARTCapture(ctx, servo.ECUARTCapture)
		if err != nil {
			s.Fatal("Failed to enable capture EC UART: ", err)
		}
		defer func() {
			if err := closeUART(ctx); err != nil {
				s.Fatal("Failed to disable capture EC UART: ", err)
			}
		}()

		apUnresponsiveMsg := regexp.MustCompile(`MKBP: The AP is failing to respond despite being powered on`)
		for i := 0; i < 3; i++ {
			s.Log("Attempting to power DUT back on with power button press of ", h.Config.HoldPwrButtonPowerOn)
			if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.Dur(h.Config.HoldPwrButtonPowerOn)); err != nil {
				s.Fatalf("Failed to power on DUT by pressing power button for hold_pwr_button_poweron (%s): %v", h.Config.HoldPwrButtonPowerOn, err)
			}
			found, err := h.Servo.PollForRegexp(ctx, servo.ECUARTStream, apUnresponsiveMsg, 1*time.Minute)
			if err != nil {
				s.Fatal("GSC output parsing failed: ", err)
			}
			if found {
				pollForPowerStateG3 := func(ctx context.Context) (bool, error) {
					type g3NotFoundErr struct {
						*errors.E
					}
					if err := testing.Poll(ctx, func(ctx context.Context) error {
						currPowerState, err := h.Servo.GetECSystemPowerState(ctx)
						if err != nil {
							return errors.Wrap(err, "failed to get power state")
						}
						if currPowerState != "G3" {
							return &g3NotFoundErr{errors.Errorf("expected power state G3, but got power state %q", currPowerState)}
						}
						return nil
					}, &testing.PollOptions{Interval: 5 * time.Second, Timeout: 30 * time.Second}); err != nil {
						if _, ok := errors.Unwrap(err).(*g3NotFoundErr); ok {
							return false, nil
						}
						return false, errors.Wrap(err, "failed to get power state")
					}
					return true, nil
				}
				isG3PowerState, err := pollForPowerStateG3(ctx)
				if err != nil {
					s.Fatal("Failed to get power state G3: ", err)
				}
				if isG3PowerState {
					s.Logf("Captured %q and got power state G3. Retry pressing power button", apUnresponsiveMsg)
				} else {
					s.Logf("Captured %q but did not get power state G3. Continue reconnecting to the DUT", apUnresponsiveMsg)
					break
				}
			} else {
				s.Logf("Did not capture %q. Continue reconnecting to the DUT", apUnresponsiveMsg)
				break
			}
		}
	}
	waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
	defer cancelWaitConnect()
	if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
		currPowerState, stateErr := h.Servo.GetECSystemPowerState(ctx)
		if stateErr != nil {
			s.Fatalf("Failed to reconnect to dut: %v, and failed to check power state: %v", err, stateErr)
		}
		s.Fatalf("Failed to reconnect to dut: %v, got power state: %v", err, currPowerState)
	}

	if tc.RemovePower {
		s.Log("Connecting charger")
		if err := h.SetDUTPower(ctx, true); err != nil {
			s.Fatal("Failed to connect charger: ", err)
		}
		// Restoring power with servo_v4 can cause ethernet failure, so reconnect afterwards
		if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
			s.Fatal("Failed to reconnect to DUT after restarting: ", err)
		}
	}

	if h.DUT != nil {
		if bootMode, err := h.Reporter.CurrentBootMode(ctx); err != nil {
			s.Fatal("Failed to get boot mode: ", err)
		} else if bootMode != fwCommon.BootModeNormal {
			s.Fatal("Unexpected boot mode: ", bootMode)
		}
	}
}
