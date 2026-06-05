// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	fwCommon "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
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
		// TODO(b/427195218): Add servo-exists + servo_state:WORKING after bug resolved.
		TestBedDeps:  []string{tbdep.ServoPresent},
		Attr:         []string{"group:firmware", "firmware_stress"},
		Vars:         []string{"firmware.consecutiveBootIters", "firmware.consecutiveBootCustomCmd"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		// Default 10 iterations typically takes anywhere from 5 - 70 minutes
		// depending on the model and amount of errors encountered.
		Timeout: 30 * time.Hour,
		Params: []testing.Param{
			{
				Name:    "power_button_normal_mode",
				Fixture: fixture.NormalMode,
				Val: argsForConsecutiveBoot{
					bootMethod: consecutiveBootWithPowerBtn,
				},
			},
			{
				Name:    "power_button_dev_mode",
				Fixture: fixture.DevModeGBB,
				Val: argsForConsecutiveBoot{
					bootMethod: consecutiveBootWithPowerBtn,
				},
			},
			{
				Name:    "shutdown_cmd_normal_mode",
				Fixture: fixture.NormalMode,
				Val: argsForConsecutiveBoot{
					bootMethod: consecutiveBootWithShutdownCmd,
				},
			},
			{
				Name:    "shutdown_cmd_dev_mode",
				Fixture: fixture.DevModeGBB,
				Val: argsForConsecutiveBoot{
					bootMethod: consecutiveBootWithShutdownCmd,
				},
			},
		},
	})
}

func ConsecutiveBoot(ctx context.Context, s *testing.State) {
	pv := s.FixtValue().(*fixture.Value)
	h := pv.Helper
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

	type stressInfo struct {
		Iterations int `json:"iterations"`
	}

	var si stressInfo
	si.Iterations = numIters
	jsonData, err := json.Marshal(si)
	if err != nil {
		s.Fatal("Failed to marshal json: ", err)
	}
	err = os.WriteFile(filepath.Join(s.OutDir(), "stress_info.json"), jsonData, 0666)
	if err != nil {
		s.Fatal("Failed to write file: ", err)
	}

	verifyBootMode := func(mode fwCommon.BootMode) error {
		if curr, err := h.Reporter.CurrentBootMode(ctx); err != nil {
			return errors.Wrap(err, "failed to get current boot mode")
		} else if curr != mode {
			return errors.Errorf("expected mainfw_type to be %s, got %q", mode, curr)
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
		shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		// io.EOF errors are fine, that just means the shutdown happened so fast, ssh didn't get a chance to return success.
		// context.DeadlineExceeded errors are also fine.
		if err := h.DUT.Conn().CommandContext(shutdownCtx, "/sbin/shutdown", "-P", "now").Start(); err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, context.DeadlineExceeded) {
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
		var ecTime int64
		err := testing.Poll(ctx, func(ctx context.Context) error {
			result, err := h.Servo.RunECCommandGetOutputNoConsoleLogs(ctx, "gettime", []string{`Time:\s+0x(\S+)\s`})
			if err != nil {
				return errors.Wrap(err, "failed to get ec time")
			}
			ecTime, err = strconv.ParseInt(result[0][1], 16, 64)
			if err != nil {
				return errors.Wrap(err, "could not parse")
			}
			return nil
		}, &testing.PollOptions{Timeout: 30 * time.Second})
		return ecTime, err
	}
	priorECTime, err := getTime(ctx)
	if err != nil {
		s.Fatal("Failed to read EC clock: ", err)
	}

	s.Log("Verifying boot mode is ", pv.BootMode)
	if err := verifyBootMode(pv.BootMode); err != nil {
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
	failToGetS0 := 0
	failToConnectToDUT := 0
	incorrectBootMode := 0
	badBootID := 0
	unexpectedECReboot := 0
	customCmdFailed := 0
	powerdFailed := 0
	numFails := 0
	ecCrashes := 0

	failures := make(map[int][]error, numIters)
	for i := 0; i < numIters; i++ {
		failures[i] = []error{}
	}
	logFailure := func(err error, iter int, errCount *int) {
		s.Logf("Iter %d -- %v", iter+1, err)
		failures[iter] = append(failures[iter], err)
		if errCount != nil {
			*errCount++
		}
		numFails++
	}

	startTime := time.Now()

	for i := 0; i < numIters; i++ {
		if i > 0 {
			estimatedTimeRemaining := time.Since(startTime) / time.Duration(i) * time.Duration(numIters-i)
			s.Logf("------ Running iteration %d out of %d (%d failures) Time remaining: %s ------", i+1, numIters, numFails, estimatedTimeRemaining.Round(time.Minute))
		} else {
			s.Logf("------ Running iteration %d out of %d ------", i+1, numIters)
		}
		h.Servo.Echo(ctx, fmt.Sprintf("%s iteration %d out of %d", s.TestName(), i+1, numIters))

		checkCrash := true
		if err := h.UpdateECCrashCache(ctx); err != nil {
			s.Log("Failed to cache ec crashes before starting iteration ", i+1)
			checkCrash = false
		}

		if err := testing.Poll(ctx, func(ctx context.Context) error {
			if err := shutdownFunc(); err != nil {
				s.Log("Error in shutdown func: ", err)
				return errors.Wrap(err, "error in shutdown func")
			}

			s.Log("Check for G3/S5 powerstate")
			if err := h.WaitForPowerStates(ctx, 500*time.Millisecond, 25*time.Second, expectedStates...); err != nil {
				s.Log("Failed to get G3/S5 powerstate: ", err)
				return errors.Wrap(err, "failed to get G3/S5 powerstate")
			}

			return nil
		}, &testing.PollOptions{
			// In case the DUT enters the automatic critical update procedure,
			// retrying the shutdown() and WaitForPowerStates() for ten minutes might help.
			Timeout:  10 * time.Minute,
			Interval: 5 * time.Second, // Wait for 5 seconds to prevent too many logs are recorded in the EC console.
		}); err != nil {
			s.Log("Failed to shutdown and wait for G3/S5 powerstate")
			if strings.Contains(err.Error(), "error in shutdown func") {
				logFailure(err, i, &shutdownFuncFailed)
			} else if strings.Contains(err.Error(), "failed to get G3/S5 powerstate") {
				logFailure(err, i, &failToGetG3S5)
			} else {
				s.Error("Unexpected error: ", err)
			}
		}

		s.Log("Pressing power key until device boots")
		if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.Dur(h.Config.HoldPwrButtonPowerOn)); err != nil {
			logFailure(errors.Wrap(err, "failed to press power key"), i, &failToPressPowerKey)
		}

		s.Log("Check for S0 powerstate")
		if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout, "S0"); err != nil {
			logFailure(errors.Wrap(err, "failed to get S0 powerstate"), i, &failToGetS0)
		}

		// Wrap in func so ctx cancel defer executes immediately after this block.
		func() {
			reconnectTimeout := h.Config.DelayRebootToPing
			if pv.BootMode == fwCommon.BootModeDev {
				reconnectTimeout = reconnectTimeout + h.Config.FirmwareScreen + firmware.DevScreenShortDelay
			}
			s.Log("Wait for DUT to connect")
			waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, reconnectTimeout)
			defer cancelWaitConnect()
			if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
				logFailure(errors.Wrap(err, "failed to wait for device to connect"), i, &failToConnectToDUT)
			}
		}()

		// Make sure boot mode is preserved over reboot.
		s.Log("Verifying boot mode is ", pv.BootMode)
		if err := verifyBootMode(pv.BootMode); err != nil {
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
			ecTime, err := getTime(ctx)
			if err != nil {
				logFailure(errors.Wrap(err, "failed to read EC clock"), i, &unexpectedECReboot)
			} else if ecTime < priorECTime {
				logFailure(
					errors.Wrapf(err, "EC reboot detected. Clock was %v but now is %v", priorECTime, ecTime),
					i, &unexpectedECReboot,
				)
			}
			priorECTime = ecTime
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

		if checkCrash {
			s.Log("Checking for unexpected EC crashes over reboot")
			crashes, err := h.GetNewECCrashes(ctx)
			if err != nil {
				s.Log("Failed to check for new ec crashes on iteration ", i+1)
			}
			if len(crashes) != 0 {
				logFailure(errors.New("caught unexpected EC crash"), i, &ecCrashes)
				for crashName := range crashes {
					logPath := firmware.ECCrashBaseDir + crashName + ".eccrash"
					out, err := h.Reporter.CatFile(ctx, logPath)
					if err != nil {
						s.Logf("Failed to read .eccrash file %s to print in log", logPath)
					}
					s.Logf("!!!WARNING: Detected unexpected EC crash on iteration %d!!!: %v", i+1, string(out))
				}
			}
		}
	}

	if numFails > 0 {
		s.Logf("Encountered %d errors during execution of stress test:", numFails)
		s.Logf("\tFailed to shutdown:........%d", shutdownFuncFailed)
		s.Logf("\tFailed to reach G3/S5:.....%d", failToGetG3S5)
		s.Logf("\tPower key failed:..........%d", failToPressPowerKey)
		s.Logf("\tFailed to reach S0:........%d", failToGetS0)
		s.Logf("\tFailed to connect to DUT:..%d", failToConnectToDUT)
		s.Logf("\tGot incorrect bootmode:....%d", incorrectBootMode)
		s.Logf("\tBootID did not change:.....%d", badBootID)
		s.Logf("\tUnexpected EC reboot:......%d", unexpectedECReboot)
		s.Logf("\tCustom cmd failed:.........%d", customCmdFailed)
		s.Logf("\tPowerd was not running:....%d", powerdFailed)
		s.Logf("\tUnexpected EC crashes:.....%d", ecCrashes)
		for iter, errors := range failures {
			if len(errors) > 0 {
				s.Logf("Iter %d: Had the following failures:", iter+1)
				for _, errMsg := range errors {
					s.Logf("\t%v", errMsg)
				}
			}
		}
		s.Fatalf("ConsecutiveBoot test had %d errors, see logs for details", numFails)
	} else {
		s.Log("No errors encountered")
	}
}
