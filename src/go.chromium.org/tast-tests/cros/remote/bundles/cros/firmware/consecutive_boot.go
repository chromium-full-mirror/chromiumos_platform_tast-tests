// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type consecutiveBootMethod int

const (
	consecutiveBootWithPowerBtn consecutiveBootMethod = iota
	consecutiveBootWithShutdownCmd
)

type argsForConsecutiveBoot struct {
	bootMethod consecutiveBootMethod
	bootMode   string
}

func init() {
	testing.AddTest(&testing.Test{
		Func: ConsecutiveBoot,
		Desc: "Test DUT shuts down and boots to ChromeOS over many iterations",
		Contacts: []string{
			"chromeos-faft@google.com",
			"tij@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware", "firmware_stress"},
		Vars:         []string{"firmware.consecutiveBootIters", "firmware.consecutiveBootCustomCmd"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		// Default 10 iterations typically takes anywhere from 5 - 70 minutes
		// depending on the model and amount of errors encountered.
		Timeout:      30 * time.Hour,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Params: []testing.Param{
			{
				Name:    "power_button_normal_mode",
				Fixture: fixture.NormalMode,
				Val: argsForConsecutiveBoot{
					bootMethod: consecutiveBootWithPowerBtn,
					bootMode:   "normal",
				},
			},
			{
				Name:    "power_button_dev_mode",
				Fixture: fixture.DevModeGBB,
				Val: argsForConsecutiveBoot{
					bootMethod: consecutiveBootWithPowerBtn,
					bootMode:   "developer",
				},
			},
			{
				Name:    "shutdown_cmd_normal_mode",
				Fixture: fixture.NormalMode,
				Val: argsForConsecutiveBoot{
					bootMethod: consecutiveBootWithShutdownCmd,
					bootMode:   "normal",
				},
			},
			{
				Name:    "shutdown_cmd_dev_mode",
				Fixture: fixture.DevModeGBB,
				Val: argsForConsecutiveBoot{
					bootMethod: consecutiveBootWithShutdownCmd,
					bootMode:   "developer",
				},
			},
		},
	})
}

func ConsecutiveBoot(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	testArgs := s.Param().(argsForConsecutiveBoot)
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}
	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}

	numIters := 10
	if numItersStr, ok := s.Var("firmware.consecutiveBootIters"); ok {
		numItersInt, err := strconv.Atoi(numItersStr)
		if err != nil {
			s.Fatalf("Invalid value for var firmware.consecutiveBootIters: got %q, expected int", numItersStr)
		} else {
			numIters = numItersInt
		}
	}

	verifyBootMode := func(mode string) error {
		if mainfwType, err := h.Reporter.CrossystemParam(ctx, reporters.CrossystemParamMainfwType); err != nil {
			return errors.Wrap(err, "failed to get crossystem mainfw_type")
		} else if mainfwType != mode {
			return errors.Errorf("expected mainfw_type to be %s, got %q", mode, mainfwType)
		}
		return nil
	}

	shutdownWithPowerButton := func() error {
		s.Log("Pressing power key until device shuts down")
		if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.Dur(h.Config.HoldPwrButtonPowerOff)); err != nil {
			return errors.Wrap(err, "failed to press power key")
		}
		return nil
	}

	shutdownWithShutdownCmd := func() error {
		s.Log("Sending `/sbin/shutdown -P now` to shutdown dut")
		if err := h.DUT.Conn().CommandContext(ctx, "/sbin/shutdown", "-P", "now").Start(); err != nil {
			return errors.Wrap(err, "failed to run `/sbin/shutdown -P now` cmd")
		}
		return nil
	}

	var shutdownFunc func() error
	if testArgs.bootMethod == consecutiveBootWithPowerBtn {
		shutdownFunc = shutdownWithPowerButton
	} else if testArgs.bootMethod == consecutiveBootWithShutdownCmd {
		shutdownFunc = shutdownWithShutdownCmd
	}

	hasCustomCmd := false
	customCmd, ok := s.Var("firmware.consecutiveBootCustomCmd")
	if ok {
		s.Logf("Custom command %q was provided, it will be run after every reboot", customCmd)
		hasCustomCmd = true
	}

	expectECReboot := false
	if h.Config.Platform == "kukui" || h.Config.Platform == "jacuzzi" {
		expectECReboot = true
	}

	// Based on b/268492022, octopus devices might not be able to power on from S5, wait for G3 in that case.
	// Otherwise power on from S5 to save time waiting for G3.
	expectedStates := []string{"S5", "G3"}
	if h.Config.Platform == "octopus" {
		expectedStates = []string{"G3"}
	}

	getTime := func(ctx context.Context) (int64, error) {
		result, err := h.Servo.RunECCommandGetOutputNoConsoleLogs(ctx, "gettime", []string{`Time:\s+0x(\S+)\s`})
		if err != nil {
			return 0, errors.Wrap(err, "failed to get ec time")
		}
		time, err := strconv.ParseInt(result[0][1], 16, 64)
		if err != nil {
			return 0, errors.Wrap(err, "could not parse")
		}
		return time, nil
	}
	priorECTime, err := getTime(ctx)
	if err != nil {
		s.Fatal("Failed to read EC clock: ", err)
	}

	s.Log("Verifying boot mode is ", testArgs.bootMode)
	if err := verifyBootMode(testArgs.bootMode); err != nil {
		s.Fatal("Failed boot mode check: ", err)
	}

	bootID, err := h.Reporter.BootID(ctx)
	if err != nil {
		s.Fatal("Failed to get boot id: ", err)
	}

	// Counters to track points of failure.
	shutdownFuncFailed := 0
	failToGetG3S5 := 0
	failToPressPowerKey := 0
	failToConnectToDUT := 0
	incorrectBootMode := 0
	badBootID := 0
	unexpectedECReboot := 0
	customCmdFailed := 0
	powerdFailed := 0

	failures := make(map[int][]error, numIters)
	for i := 0; i < numIters; i++ {
		failures[i] = []error{}
	}
	logFailure := func(err error, iter int, errCount *int) {
		s.Logf("Iter %d -- %v", iter+1, err)
		failures[iter] = append(failures[iter], err)
		if errCount != nil {
			*errCount = *errCount + 1
		}
	}

	for i := 0; i < numIters; i++ {
		s.Logf("Running iteration %d out of %d ", i+1, numIters)
		if err := shutdownFunc(); err != nil {
			logFailure(errors.Wrap(err, "error in shutdown func"), i, &shutdownFuncFailed)
		}

		s.Log("Check for G3/S5 powerstate")
		if err := h.WaitForPowerStates(ctx, 500*time.Millisecond, 180*time.Second, expectedStates...); err != nil {
			logFailure(errors.Wrap(err, "failed to get G3/S5 powerstate"), i, &failToGetG3S5)
		}

		s.Log("Pressing power key until device boots")
		if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.Dur(h.Config.HoldPwrButtonPowerOn)); err != nil {
			logFailure(errors.Wrap(err, "failed to press power key"), i, &failToPressPowerKey)
		}

		s.Log("Check for S0 powerstate")
		if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout, "S0"); err != nil {
			logFailure(errors.Wrap(err, "failed to get S0 powerstate"), i, nil)
		}

		// Wrap in func so ctx cancel defer executes immediately after this block.
		func() {
			s.Log("Wait for DUT to connect")
			waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
			defer cancelWaitConnect()
			if err := h.WaitConnect(waitConnectCtx); err != nil {
				logFailure(errors.Wrap(err, "failed to wait for device to connect"), i, &failToConnectToDUT)
			}
		}()

		// Make sure boot mode is preserved over reboot.
		s.Log("Verifying boot mode is ", testArgs.bootMode)
		if err := verifyBootMode(testArgs.bootMode); err != nil {
			logFailure(errors.Wrap(err, "failed boot mode check"), i, &incorrectBootMode)
		}

		s.Log("Verifying boot id changed over reboot")
		if newBootID, err := h.Reporter.BootID(ctx); err != nil {
			logFailure(errors.Wrap(err, "failed to get boot id"), i, &badBootID)
		} else if newBootID == bootID {
			logFailure(errors.Wrap(err, "unexpectedly got same boot id over reboot"), i, &badBootID)
		} else {
			bootID = newBootID
		}

		if !expectECReboot {
			if ecTime, err := getTime(ctx); err != nil {
				logFailure(errors.Wrap(err, "failed to read EC clock"), i, &unexpectedECReboot)
			} else if ecTime < priorECTime {
				logFailure(
					errors.Wrapf(err, "EC reboot detected. Clock was %v but now is %v", priorECTime, ecTime),
					i, &unexpectedECReboot,
				)
			} else {
				priorECTime = ecTime
			}
		}

		if hasCustomCmd {
			s.Logf("Running provided custom command %q after reboot #%d", customCmd, i+1)
			if out, err := h.DUT.Conn().CommandContext(ctx, "sh", "-c", customCmd).CombinedOutput(ssh.DumpLogOnError); err != nil {
				logFailure(errors.Wrap(err, "error running custom cmd"), i, &customCmdFailed)
			} else {
				s.Log("cmd output:", string(out))
			}
		}

		if testArgs.bootMethod == consecutiveBootWithPowerBtn {
			// Wait for powerd to be running so power key press will be recognized.
			if err := testing.Poll(ctx, func(ctx context.Context) error {
				out, err := h.DUT.Conn().CommandContext(ctx, "status", "powerd").Output(ssh.DumpLogOnError)
				if err != nil {
					return errors.Wrap(err, "failed to get powerd status")
				}
				if !strings.Contains(string(out), "powerd start/running") {
					return errors.Errorf("expected powerd to be running, actual status was %q", string(out))
				}
				return nil

			}, &testing.PollOptions{
				Timeout:  15 * time.Second,
				Interval: 2 * time.Second,
			}); err != nil {
				logFailure(errors.Wrap(err, "failed to wait for powerd to start"), i, &powerdFailed)
			}
		}
	}

	numFails := 0
	for _, errors := range failures {
		numFails += len(errors)
	}
	if numFails > 0 {
		s.Logf("Encountered %d errors during execution of stress test:", numFails)
		s.Logf("\tFailed to shutdown:........%d", shutdownFuncFailed)
		s.Logf("\tFailed to reach G3/S5:.....%d", failToGetG3S5)
		s.Logf("\tPower key failed:..........%d", failToPressPowerKey)
		s.Logf("\tFailed to connect to DUT:..%d", failToConnectToDUT)
		s.Logf("\tGot incorrect bootmode:....%d", incorrectBootMode)
		s.Logf("\tBootID did not change:.....%d", badBootID)
		s.Logf("\tUnexpected EC reboot:......%d", unexpectedECReboot)
		s.Logf("\tCustom cmd failed:.........%d", customCmdFailed)
		s.Logf("\tPowerd was not running:....%d", powerdFailed)
		s.Fatalf("ConsecutiveBoot test had %d errors, see logs for details", numFails)
	} else {
		s.Log("No errors encountered")
	}
}
