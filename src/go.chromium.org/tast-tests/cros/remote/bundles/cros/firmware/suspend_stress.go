// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	pb "go.chromium.org/tast-tests/cros/services/cros/firmware"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: SuspendStress,
		Desc: "Suspend and wake stress test",
		Contacts: []string{
			"chromeos-faft@google.com",
			"tij@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Vars:         []string{"firmware.suspendStressFailFast", "firmware.suspendStressIters"},
		TestBedDeps:  tbdep.ServoPresentAndWorking,
		Attr:         []string{"group:firmware", "group:dsp"},
		ServiceDeps:  []string{"tast.cros.firmware.UtilsService", "tast.cros.firmware.TPMService"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		SoftwareDeps: []string{"chrome"},
		Fixture:      fixture.NormalMode,
		Params: []testing.Param{
			{
				Name: "short",
				// 10 iterations takes between 7-15 minutes depending on model and number of errors encountered.
				Timeout:   20 * time.Minute,
				Val:       10,
				ExtraAttr: []string{"firmware_stress", "dsp_small"},
			},
			{
				Name:      "medium",
				Timeout:   400 * time.Minute,
				Val:       250,
				ExtraAttr: []string{"dsp_medium"},
			},
			{
				Name:      "fw_qual_1250",
				Timeout:   900 * time.Minute, // 15 hours to stay below 16 hr limit for more graceful cleanup.
				Val:       1250,
				ExtraAttr: []string{"dsp_large"},
			},
		},
	})
}

const (
	// minSuspendResumeSeconds and maxSuspendResumeSeconds are the range of seconds passed to powerd_dbus_suspend --suspend_for_sec.
	minSuspendResumeSeconds = 5
	maxSuspendResumeSeconds = 15
	// minPostResumeSleepSeconds and maxPostResumeSleepSeconds are the range of seconds to sleep after resuming from suspend and starting the next iteration.
	minPostResumeSleepSeconds = 3
	maxPostResumeSleepSeconds = 8
	// powerdDelaySeconds is the time powerd waits before suspending, i.e. passed to powerd_dbus_suspend --delay.
	powerdDelaySeconds = 3
	// checkFrequency sets how often (in iterations) login, tpm, and ectool are checked. Always checked on last iteration.
	checkFrequency = 100
	// powerStatePadding is extra time to wait for the S3/S0ix power state, log warning if takes longer than this padding accounts for.
	powerStatePadding = 10 * time.Second
	// maxWaitPadding is the maximum amount of additional time to wait for a full suspend + wake, taking longer is a failure cases.
	maxWaitPadding = 40 * time.Second
	// numECLogsToPrint determines how many lines of the ec log to print on failure.
	// Full log for cycle gets saved to tast logs.
	numECLogsToPrint = 100
)

// suspendStressCrashDir is the dir name to save logs to
var suspendStressCrashDir = "suspend_stress_errors"

func SuspendStress(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}

	if err := h.Servo.RemoveCCDWatchdogs(ctx); err != nil {
		s.Fatal("Failed to remove ccd watchdog: ", err)
	}

	losesPowerOnPDDisconnect, err := h.LosesPowerOnServoPDDisconnect(ctx)
	if err != nil {
		s.Fatal("Failed to check whether servo PD disconnect drops DUT power: ", err)
	}
	reconnectOpts := []firmware.WaitConnectOption{firmware.ResetEthernetDongle}
	if losesPowerOnPDDisconnect {
		reconnectOpts = append(reconnectOpts, firmware.SkipPDRoleSnk)
	}

	suspendStressCrashDir = filepath.Join(s.OutDir(), suspendStressCrashDir)

	// Number of iterations to run stress test for.
	numIters := s.Param().(int)
	// Read test args for override of number of iterations.
	if numItersStr, ok := s.Var("firmware.suspendStressIters"); ok {
		numItersInt, err := strconv.Atoi(numItersStr)
		if err != nil {
			s.Fatalf("Invalid value for var firmware.suspendStressIters: got %q, expected int", numItersStr)
		} else {
			s.Logf("Running %d iters instead of preset %d", numItersInt, numIters)
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

	// If fail fast is set to true, fails immediately at first error, otherwise collects errors over all iterations.
	failFast := false
	if failFastStr, ok := s.Var("firmware.suspendStressFailFast"); ok {
		failFastBool, err := strconv.ParseBool(failFastStr)
		if err != nil {
			s.Fatalf("Invalid value for var firmware.suspendStressFailFast: got %q, expected bool", failFastStr)
		} else {
			failFast = failFastBool
		}
	}

	// Restart UI to ensure no user is logged in, as this will change power state behaviour on suspend.
	if err := h.DUT.Conn().CommandContext(ctx, "restart", "ui").Run(ssh.DumpLogOnError); err != nil {
		s.Fatal("Failed to restart ui before test: ", err)
	}

	failures := make(map[int][]error, numIters)
	failureCount := 0
	if !failFast {
		for i := 0; i < numIters; i++ {
			failures[i] = []error{}
		}
	}

	logFailure := func(msg string, err error, iter int) {
		failureCount++
		if failFast {
			s.Fatalf("%s: %v", msg, err)
		} else {
			s.Logf("ITERATION FAILURE (%d) %s: %v", iter, msg, err)
			failures[iter] = append(failures[iter], errors.Wrap(err, msg))
		}
	}

	// GoBigSleepLint: Original autotest included a stop/start modemfwd as a temporary workaround for misbehaved modemfwd (b/164255562).
	// Since associated issues are fixed, a simple wait should be enough to prevent related errors.
	if err := testing.Sleep(ctx, 2*time.Second); err != nil {
		s.Fatal("Failed to sleep for 2s")
	}

	const powerdLogPath = "/var/log/power_manager/powerd.LATEST"
	s.Logf("Clearing %s", powerdLogPath)
	if err := h.DUT.Conn().CommandContext(ctx, "truncate", "--size=0", powerdLogPath).Run(); err != nil {
		s.Fatal("Failed to clear powerd log file: ", err)
	}

	startTime := time.Now()

	for i := 0; i < numIters; i++ {
		if i > 0 {
			estimatedTimeRemaining := time.Since(startTime) / time.Duration(i) * time.Duration(numIters-i)
			s.Logf("------ Running iteration %d out of %d (%d failures) Time remaining: %s ------", i+1, numIters, failureCount, estimatedTimeRemaining.Round(time.Minute))
		} else {
			s.Logf("------ Running iteration %d out of %d ------", i+1, numIters)
		}
		h.Servo.Echo(ctx, fmt.Sprintf("firmware.SuspendStress iteration %d out of %d", i+1, numIters))

		if err := h.CloseRPCConnection(ctx); err != nil {
			logFailure("Failed to close rpc connection", err, i)
		}

		suspendSeconds := minSuspendResumeSeconds + rand.Intn(maxSuspendResumeSeconds-minSuspendResumeSeconds)

		func() {
			var retErr error
			var ecLogBuf strings.Builder
			readECUART := func() string {
				chunk, err := h.Servo.GetQuotedString(ctx, servo.ECUARTStream)
				if err == nil {
					ecLogBuf.WriteString(chunk)
				}
				return ecLogBuf.String()
			}
			closeUART, err := h.Servo.EnableUARTCapture(ctx, servo.ECUARTCapture)
			if err != nil {
				s.Log("Failed to start ec uart capture: ", err)
			} else {
				// Drain buffered EC UART output from prior iterations.
				_, _ = h.Servo.GetQuotedString(ctx, servo.ECUARTStream)
			}
			defer func() {
				if retErr != nil {
					ecLogs := readECUART()
					tail, numLines := getLogsTail(ecLogs, numECLogsToPrint)
					saveLogsForFailedIter(ctx, h, ecLogs, i)
					testing.ContextLogf(ctx, "Last %d EC Logs from failure: %v", numLines, tail)
				}
				if closeUART != nil {
					if err := closeUART(ctx); err != nil {
						s.Log("Failed to close ec uart capture: ", err)
					}
				}
			}()

			if err := timeSuspendWakeCycle(ctx, h, suspendSeconds, readECUART); err != nil {
				logFailure("Failed waiting for suspend and wake", err, i)
				retErr = err
			}
			waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, 30*time.Second)
			defer cancelWaitConnect()
			err = h.WaitConnect(waitConnectCtx, firmware.FromHibernation)
			if err == nil {
				return
			}

			s.Log("RETRIABLE: Failed to reconnect to DUT after suspend: ", err)
			if losesPowerOnPDDisconnect {
				// fakedisconnect drops Type-C VBUS, cold-rebooting battery-less USB-PD Chromeboxes and rotating powerd.LATEST.
				s.Log("Skipping servo fakedisconnect on USB-PD powered DUT; retrying with ethernet dongle reset")
			} else {
				s.Log("Disconnecting servo for 1s")
				// Try resetting servo usb-c connection and try again.
				h.Servo.RunServoCommand(ctx, "fakedisconnect 100 1000")
			}

			waitConnectCtx, cancelWaitConnect = context.WithTimeout(ctx, 30*time.Second)
			defer cancelWaitConnect()
			if err := testing.Poll(waitConnectCtx, func(ctx context.Context) error {
				return h.WaitConnect(ctx, reconnectOpts...)
			}, &testing.PollOptions{Interval: time.Second}); err != nil {
				logFailure("Failed to reconnnect to DUT after waking and resetting servo usb", err, i)
				retErr = err
			}
		}()

		if i%50 == 0 {
			s.Log("Marking servo in use")
			if err := h.ServoProxy.MarkServoInUse(ctx); err != nil {
				s.Log("Warning: Failed to mark servo in use: ", err)
			}
		}

		if (i+1)%checkFrequency == 0 || i == numIters-1 {
			s.Log("Checking login, TPM, and ECTool on iteration ", i+1)
			// Note the original autotest for power_SuspendStress only ran these at the end of the test, but here it runs every iteration.
			// This slows down the test notably, so if it needs to be run for many iterations, it might be worthwhile temporarily moving this to only run at the end.
			if err := testLoginSuccess(ctx, h); err != nil {
				logFailure("Failed to test Chrome login", err, i)
			}

			if err := testTPM(ctx, h); err != nil {
				logFailure("Failed to test TPM state", err, i)
			}

			if err := testEctool(ctx, h); err != nil {
				logFailure("Failed to test ectool", err, i)
			}
		}

		// if something went wrong during some iteration, reset so other iterations remain independent.
		if len(failures[i]) > 0 {
			// Don't ignore these errors as they likely indicate that DUT is offline after iteration.
			s.Log("Resetting DUT due to errors on this iteration")
			if err := h.Servo.SetPowerState(ctx, servo.PowerStateReset); err != nil {
				s.Fatalf("Failed to reset DUT after failures in suspend at iteration %d: %v", i+1, err)
			}
			s.Log("Reconnecting to DUT")
			func() {
				waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, time.Minute)
				defer cancelWaitConnect()
				if err := testing.Poll(waitConnectCtx, func(ctx context.Context) error {
					return h.WaitConnect(ctx, reconnectOpts...)
				}, &testing.PollOptions{Interval: time.Second}); err != nil {
					s.Fatalf("Failed to reconnect to DUT after reset at iteration %d: %v", i+1, err)
				}
			}()
		}

		if i != numIters-1 {
			resuspendDelay := time.Duration(minPostResumeSleepSeconds+rand.Intn(maxPostResumeSleepSeconds-minPostResumeSleepSeconds)) * time.Second
			s.Logf("Sleeping for %s before next iteration", resuspendDelay)
			// GoBigSleepLint: random duration in range [minPostResumeSleepSeconds, maxPostResumeSleepSeconds) to wait between suspend iterations.
			if err := testing.Sleep(ctx, resuspendDelay); err != nil {
				// Don't ignore this error even without fail fast set as it means context timed out.
				s.Fatalf("Test timed out between suspends on iteration %d: %v", i+1, err)
			}
		}
	}

	s.Log("Errors encountered:")
	for iter, errors := range failures {
		if len(errors) > 0 {
			s.Logf("Iter %d: Had the following failures:", iter+1)
			for _, errMsg := range errors {
				s.Logf("\t%v", errMsg)
			}
		}
	}

	// Save the powerd log for debugging purposes.
	output, err := h.Reporter.CatFile(ctx, powerdLogPath)
	if err != nil {
		s.Error("Failed to read powerd log: ", err)
	}

	matchCount := strings.Count(output, "powerd_suspend returned 0")

	destPath := filepath.Join(s.OutDir(), "powerd.log")
	if err := os.WriteFile(destPath, []byte(output), 0666); err != nil {
		s.Error("Failed to write powerd log: ", err)
	}

	if failureCount > 0 {
		s.Fatalf("Encountered %d errors during execution of stress test and got %d powerd_suspend returned 0 in powerd log, check execution log for details", failureCount, matchCount)
	} else {
		s.Logf("\tNo errors encountered and got %d powerd_suspend returned 0 in powerd log", matchCount)
	}

}

func saveLogsForFailedIter(ctx context.Context, h *firmware.Helper, ecLog string, iter int) error {
	saveDir := filepath.Join(suspendStressCrashDir, fmt.Sprintf("iter_%d", iter))
	if err := os.MkdirAll(saveDir, os.ModePerm); err != nil {
		return errors.Wrap(err, "failed to make dir to save logs")
	}
	ecLogFilePath := filepath.Join(saveDir, "ec.log")
	if err := os.WriteFile(ecLogFilePath, []byte(ecLog), os.ModePerm); err != nil {
		return errors.Wrapf(err, "failed to save ec log to %s", ecLogFilePath)
	}

	// For saving logs from dut, dut needs to up.
	if err := h.EnsureDUTBooted(ctx); err != nil {
		return errors.Wrap(err, "DUT was unable to be booted, failed to save logs from dut")
	}

	fileExists := func(f string) bool {
		return h.DUT.Conn().CommandContext(ctx, "test", "-f", f) == nil
	}

	// Some of these might not exist/are platform specific, save if they exist.
	additionalFilesToLog := []string{
		"/sys/kernel/debug/pmc_core",
		"/sys/fs/pstore/console-ramoops-0",
		"/sys/kernel/debug/pmc_core/slp_s0_residency_usec",
		"/sys/kernel/debug/amd_pmc/s0ix_stats",
		"/sys/kernel/debug/telemetry/s0ix_residency_usec",
	}

	for _, f := range additionalFilesToLog {
		if fileExists(f) {
			// Rename file for saving, retain file path/name for traceability.
			saveFilePath := filepath.Join(saveDir, strings.ReplaceAll(strings.TrimPrefix(f, "/"), "/", "_"))
			if err := linuxssh.GetFile(ctx, h.DUT.Conn(), f, saveFilePath, linuxssh.DereferenceSymlinks); err != nil {
				testing.ContextLogf(ctx, "Failed to save file %s to %s", f, saveFilePath)
			}
		}
	}

	testing.ContextLogf(ctx, "Saved related logs for failed suspend iteration %d to %s", iter, saveDir)
	return nil
}

func getLogsTail(logs string, n int) (string, int) {
	lines := strings.Split(logs, "\n")
	numLines := len(lines)
	start := 0
	if numLines > n {
		start = numLines - n
	}
	lastNLines := lines[start:]
	return strings.Join(lastNLines, "\n"), len(lastNLines)
}

var ecUARTPowerStateRe = regexp.MustCompile(`(?:power state \d+ = ([A-Za-z0-9>-]+)|Power state: [A-Za-z0-9_]+ --> ([A-Za-z0-9_]+))`)

func isSuspendPowerState(st string) bool {
	return strings.Contains(st, "S0i") || st == "S3"
}

func isTransitionalECPowerState(st string) bool {
	return strings.Contains(st, "->") || st == "S0S0ix" || st == "S0ixS0" || st == "S0S3" || st == "S3S0"
}

// checkECUARTPowerCycle inspects captured EC UART output for a completed
// S0ix/S3 -> S0 cycle or an unexpected steady power state (such as G3/S5).
func checkECUARTPowerCycle(ecLog string) (bool, error) {
	matches := ecUARTPowerStateRe.FindAllStringSubmatch(ecLog, -1)
	var states []string
	for _, m := range matches {
		st := m[1]
		if st == "" {
			st = strings.TrimPrefix(m[2], "SYS_POWER_STATE_")
			st = strings.TrimPrefix(st, "AP_POWER_STATE_")
		}
		if isTransitionalECPowerState(st) {
			continue
		}
		if len(states) == 0 || states[len(states)-1] != st {
			states = append(states, st)
		}
	}
	suspendIdx := -1
	for idx, st := range states {
		if !isSuspendPowerState(st) && st != "S0" {
			return false, errors.Errorf("dut in unexpected powerstate %s in EC UART (%v)", st, states)
		}
		if suspendIdx == -1 && isSuspendPowerState(st) {
			suspendIdx = idx
		}
	}
	if suspendIdx != -1 && suspendIdx < len(states)-1 && states[len(states)-1] == "S0" {
		return true, nil
	}
	return false, nil
}

type suspendSnapshot struct {
	swSuccess    int64
	hasHWCounter bool
	hwCounter    int64
}

const readSuspendSnapshotScript = `cat /sys/power/suspend_stats/success && ` +
	`if check_powerd_config --suspend_to_idle >/dev/null 2>&1; then ` +
	`if [ -r /sys/kernel/debug/pmc_core/slp_s0_residency_usec ]; then cat /sys/kernel/debug/pmc_core/slp_s0_residency_usec; ` +
	`elif [ -r /sys/kernel/debug/telemetry/s0ix_residency_usec ]; then cat /sys/kernel/debug/telemetry/s0ix_residency_usec; ` +
	`elif [ -r /sys/kernel/debug/amd_pmc/s0ix_stats ]; then awk '/^S0ix Exit Time:/ {print $4}' /sys/kernel/debug/amd_pmc/s0ix_stats; fi; fi`

func parseSuspendSnapshot(out string) (suspendSnapshot, error) {
	fields := strings.Fields(out)
	if len(fields) == 0 {
		return suspendSnapshot{}, errors.New("empty suspend snapshot")
	}
	sw, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return suspendSnapshot{}, errors.Wrap(err, "parsing suspend_stats/success")
	}
	snap := suspendSnapshot{swSuccess: sw}
	if len(fields) >= 2 {
		hw, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return suspendSnapshot{}, errors.Wrap(err, "parsing hardware s0ix counter")
		}
		snap.hasHWCounter = true
		snap.hwCounter = hw
	}
	return snap, nil
}

func didDUTSuspend(pre, post suspendSnapshot) bool {
	if post.swSuccess <= pre.swSuccess {
		return false
	}
	if pre.hasHWCounter || post.hasHWCounter {
		return pre.hasHWCounter && post.hasHWCounter && post.hwCounter > pre.hwCounter
	}
	return true
}

func readDUTSuspendSnapshot(ctx context.Context, h *firmware.Helper) (suspendSnapshot, error) {
	out, err := h.DUT.Conn().CommandContext(ctx, "sh", "-c", readSuspendSnapshotScript).Output()
	if err != nil {
		return suspendSnapshot{}, err
	}
	return parseSuspendSnapshot(string(out))
}

func verifyDUTSuspendedAfterStall(ctx context.Context, h *firmware.Helper, pre suspendSnapshot) (bool, string) {
	waitCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if !h.DUT.Connected(waitCtx) {
		if err := h.WaitConnect(waitCtx, firmware.FromHibernation); err != nil {
			return false, ""
		}
	}
	post, err := readDUTSuspendSnapshot(waitCtx, h)
	if err != nil || !didDUTSuspend(pre, post) {
		return false, ""
	}
	if post.hasHWCounter {
		return true, fmt.Sprintf("suspend_stats %d->%d, hw_s0ix %d->%d", pre.swSuccess, post.swSuccess, pre.hwCounter, post.hwCounter)
	}
	return true, fmt.Sprintf("suspend_stats %d->%d", pre.swSuccess, post.swSuccess)
}

func timeSuspendWakeCycle(ctx context.Context, h *firmware.Helper, suspendSeconds int, readECUART func() string) error {
	preSnap, preSnapErr := readDUTSuspendSnapshot(ctx, h)
	if preSnapErr != nil {
		testing.ContextLog(ctx, "Warning: failed to read pre-suspend snapshot: ", preSnapErr)
	}

	testing.ContextLogf(ctx, "Suspending dut for %ds with delay of %d seconds", suspendSeconds, powerdDelaySeconds)

	// DUT will wake to S0 after suspendSeconds+powerdDelaySeconds, so if S0ix/S3 not detected in that duration + some padding, it failed to suspend.
	expectedSuspendtime := time.Duration(suspendSeconds+powerdDelaySeconds)*time.Second + powerStatePadding
	maxWaitTime := expectedSuspendtime + maxWaitPadding

	// The --suspend_for_sec flag automatically wakes the DUT after the given duration.
	cmdCtx, cancelCmd := context.WithTimeout(ctx, maxWaitTime)
	cmd := h.DUT.Conn().CommandContext(cmdCtx, "powerd_dbus_suspend", fmt.Sprintf("--delay=%d", powerdDelaySeconds), fmt.Sprintf("--suspend_for_sec=%d", suspendSeconds))
	if err := cmd.Start(); err != nil {
		cancelCmd()
		return errors.Wrap(err, "failed to initiate suspend on DUT")
	}
	defer func() {
		cancelCmd()
		_ = cmd.Wait()
	}()
	start := time.Now()

	testing.ContextLog(ctx, "Checking for S0ix or S3 powerstate")

	collectedPowerStates := make([]string, 0)

	prevState := "S0"
	collectedPowerStates = append(collectedPowerStates, prevState)

	suspendStateIdx := -1
	wakeStateIdx := -1
	checkedDUTFallback := false

	timeTaken := time.Since(start)
	var timeToSuspend time.Duration

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		timeTaken = time.Since(start)
		currPowerState, err := h.Servo.GetECSystemPowerState(ctx)
		if err != nil {
			// Could be for any reason, continue polling for power state.
			return errors.Wrap(err, "failed to get power state")
		}

		prevState = collectedPowerStates[len(collectedPowerStates)-1]
		if currPowerState == prevState {
			if suspendStateIdx == -1 && currPowerState == "S0" && timeTaken >= expectedSuspendtime {
				if readECUART != nil {
					completed, uartErr := checkECUARTPowerCycle(readECUART())
					if uartErr != nil {
						return testing.PollBreak(uartErr)
					}
					if completed {
						testing.ContextLogf(ctx, "WARNING: servod polling missed S0ix/S3 window after %s, but EC UART confirmed suspend/wake cycle", timeTaken)
						return nil
					}
				}
				if preSnapErr == nil && !checkedDUTFallback {
					checkedDUTFallback = true
					if verified, detail := verifyDUTSuspendedAfterStall(ctx, h, preSnap); verified {
						testing.ContextLogf(ctx, "WARNING: servod polling missed S0ix/S3 window after %s, but DUT confirmed suspend/wake (%s)", timeTaken, detail)
						return nil
					}
				}
			}
			// No state change, continue polling for power state.
			return errors.Errorf("did not exit from power state %s", prevState)
		}
		collectedPowerStates = append(collectedPowerStates, currPowerState)

		if isSuspendPowerState(currPowerState) {
			suspendStateIdx = len(collectedPowerStates) - 1
			testing.ContextLogf(ctx, "Suspended after %s seconds", timeTaken)
			timeToSuspend = timeTaken
		}

		if suspendStateIdx != -1 && currPowerState == "S0" {
			wakeStateIdx = len(collectedPowerStates) - 1
			if !isSuspendPowerState(prevState) {
				unexpStates := collectedPowerStates[suspendStateIdx:]
				err := errors.Errorf("got unexpected powerstates %v between suspend and wake", unexpStates)
				// DUT is awake, break poll to stop checking power state, report unexpected power state.
				return testing.PollBreak(err)
			}
			// DUT has successfully completed suspend/wake cycle, exit poll.
			testing.ContextLogf(ctx, "Woke from suspend %s seconds after suspending", timeTaken-timeToSuspend)
			return nil
		}

		// Account for power states like S0i3 that show up on AMD.
		if !isSuspendPowerState(currPowerState) && !strings.Contains(currPowerState, "S0") {
			// If unexpected power state encountered, exit poll immediately and report error.
			err := errors.Errorf("dut in unexpected powerstate %s, was expecting only one of S0, S0ix, or S3", currPowerState)
			return testing.PollBreak(err)
		}

		// Keep running the poll, eventually it will either complete or timeout.
		return errors.Errorf("Waiting for suspend/wake cycle to complete, currently in state: %s", currPowerState)

	}, &testing.PollOptions{Timeout: maxWaitTime, Interval: 250 * time.Millisecond}); err != nil {
		if suspendStateIdx == -1 {
			return errors.Wrapf(err, "failed to get S0ix or S3 powerstate at all after suspend, waited for %s (expected time to suspend was less than %s)", maxWaitTime, expectedSuspendtime)
		} else if wakeStateIdx == -1 {
			return errors.Wrapf(err, "dut was able to suspend but failed to wake after suspend in maximum alloted time of %s", maxWaitTime)
		} else {
			return errors.Wrap(err, "failed to poll for full suspend/wake cycle")
		}
	}

	if timeToSuspend > expectedSuspendtime {
		// If it took significantly longer than expected (more than 10 extra seconds from expected suspend time), log a specific warning as it still suspended but took much longer than expected.
		testing.ContextLogf(ctx, "WARNING: DUT eventually suspended but it took %s (which is %s more than the expected time %s). This is unexpected behavior and should not happen", timeTaken, timeTaken-expectedSuspendtime, expectedSuspendtime)
	}

	return nil
}

func testTPM(ctx context.Context, h *firmware.Helper) (reterr error) {
	tpmExpectedPermanentFlags := make(map[string]map[string]string, 2)
	tpmExpectedPermanentFlags["1.2"] = map[string]string{
		"disable":                      "0",
		"ownership":                    "1",
		"deactivated":                  "0",
		"physicalPresenceHWEnable":     "0",
		"physicalPresenceCMDEnable":    "1",
		"physicalPresenceLifetimeLock": "1",
		"nvLocked":                     "1",
	}
	tpmExpectedPermanentFlags["2.0"] = map[string]string{
		"inLockout": "0",
	}

	tpmExpectedVolatileFlags := make(map[string]map[string]string, 2)
	tpmExpectedVolatileFlags["1.2"] = map[string]string{
		"deactivated":          "0",
		"physicalPresence":     "0",
		"physicalPresenceLock": "1",
		"bGlobalLock":          "1",
	}
	tpmExpectedVolatileFlags["2.0"] = map[string]string{
		"phEnable":   "0",
		"shEnable":   "1",
		"ehEnable":   "1",
		"phEnableNV": "1",
	}

	tpmExpectedSpacePermissions := make(map[string]map[string]string, 2)
	tpmExpectedSpacePermissions["1.2"] = map[string]string{
		"0x1007": `0x8001`,
		"0x1008": `0x1`,
	}
	tpmExpectedSpacePermissions["2.0"] = map[string]string{
		"0x1007": `0x60054c01`,
		"0x1008": `(0x60050001|0x60054001)`,
	}

	logMapVals := func(m map[string]string) {
		for k, v := range m {
			testing.ContextLogf(ctx, "  %v: %v", k, v)
		}
	}

	if err := h.RequireTPMServiceClient(ctx); err != nil {
		return errors.Wrap(err, "failed TPM Service Client")
	}

	if _, err := h.TPMServiceClient.NewHelper(ctx, &empty.Empty{}); err != nil {
		return errors.Wrap(err, "failed to create a helper")
	}
	defer func() {
		if _, err := h.TPMServiceClient.CloseHelper(ctx, &empty.Empty{}); err != nil {
			if reterr != nil {
				testing.ContextLog(ctx, "Failed to close helper")
			} else {
				reterr = errors.Wrap(err, "failed to close helper")
			}
		}
	}()

	if _, err := h.TPMServiceClient.StopDaemons(ctx, &empty.Empty{}); err != nil {
		return errors.Wrap(err, "failed to create a helper")
	}
	defer func() {
		if _, err := h.TPMServiceClient.StartDaemons(ctx, &empty.Empty{}); err != nil {
			if reterr != nil {
				testing.ContextLog(ctx, "Failed to start tpm daemons")
			} else {
				reterr = errors.Wrap(err, "failed to start tpm daemons")
			}
		}
	}()

	tpmVersion, err := h.TPMServiceClient.TPMVersion(ctx, &empty.Empty{})
	if err != nil {
		return errors.Wrap(err, "failed to get TPM version")
	}
	testing.ContextLog(ctx, "TPM Version: ", tpmVersion.Version)

	vfFlags, err := h.TPMServiceClient.GetAllVolatileFlags(ctx, &empty.Empty{})
	if err != nil {
		return errors.Wrap(err, "failed to get TPM volatile flags")
	}
	logMapVals(vfFlags.Flags)

	for flag, expVal := range tpmExpectedVolatileFlags[tpmVersion.Version] {
		if val, ok := vfFlags.Flags[flag]; !ok {
			testing.ContextLogf(ctx, "Expected flag %v to be in volatile flag output, but wasn't, instead got:", flag)
			logMapVals(vfFlags.Flags)
			return errors.Errorf("expected flag %v to be in `tpmc getvf` output but wasn't", flag)
		} else if val != expVal {
			return errors.Errorf("expected flag %v to have value %v but got %v", flag, expVal, val)
		}
	}

	pfFlags, err := h.TPMServiceClient.GetAllPermanentFlags(ctx, &empty.Empty{})
	if err != nil {
		return errors.Wrap(err, "failed to get TPM permanent flags")
	}
	logMapVals(pfFlags.Flags)

	for flag, expVal := range tpmExpectedPermanentFlags[tpmVersion.Version] {
		if val, ok := pfFlags.Flags[flag]; !ok {
			testing.ContextLogf(ctx, "Expected flag %v to be in permanent flag output, but wasn't, instead got:", flag)
			logMapVals(pfFlags.Flags)
			return errors.Errorf("expected flag %v to be in `tpmc getpf` output but wasn't", flag)
		} else if val != expVal {
			return errors.Errorf("expected flag %v to have value %v but got %v", flag, expVal, val)
		}
	}

	for space, expPerm := range tpmExpectedSpacePermissions[tpmVersion.Version] {
		spacePermission, err := h.TPMServiceClient.GetSpacePermissions(ctx, &pb.TPMSpacePermission{Space: space})
		if err != nil {
			return errors.Wrapf(err, "failed to get permissions for space %v", space)
		}

		match := regexp.MustCompile(expPerm).FindStringSubmatch(spacePermission.Permission)
		if match == nil {
			return errors.Errorf("expected space %v to have permissions %v but got %v", space, expPerm, spacePermission.Permission)
		}
	}
	return nil
}

func testLoginSuccess(ctx context.Context, h *firmware.Helper) error {
	if err := h.RequireRPCUtils(ctx); err != nil {
		return errors.Wrap(err, "failed requiring RPC utils")
	}

	// Automatically logs in unless specified otherwise, so if there is an error logging in, it will be caught here.
	testing.ContextLog(ctx, "Create new Chrome and login")
	if _, err := h.RPCUtils.NewChrome(ctx, &empty.Empty{}); err != nil {
		return errors.Wrap(err, "failed to create instance of chrome")
	}

	testing.ContextLog(ctx, "Close Chrome")
	if _, err := h.RPCUtils.CloseChrome(ctx, &empty.Empty{}); err != nil {
		return errors.Wrap(err, "failed to close instance of chrome")
	}

	return nil
}

func testEctool(ctx context.Context, h *firmware.Helper) error {
	ec := firmware.NewECTool(h.DUT, firmware.ECToolNameMain)

	testing.ContextLog(ctx, "Polling for 10s to get battery info from EC")
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		out, err := h.Servo.RunECCommandGetOutput(ctx, "battery", []string{`(Command 'battery' not found or ambiguous|Status:\s+0x[0-9A-Fa-f]+)`})
		if err != nil {
			return errors.Wrap(err, "failed to check for presence of battery cmd in EC")
		} else if strings.Contains(out[0][0], "Command 'battery' not found") {
			testing.ContextLog(ctx, "Battery not available/testable")
		} else {
			if err := testECToolBattery(ctx, h, ec); err != nil {
				testing.PollBreak(err)
			}
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: 3 * time.Second}); err != nil {
		errors.Wrap(err, "failed testing ectool battery")
	}

	if err := testECToolFanspeed(ctx, h, ec); err != nil {
		errors.Wrap(err, "failed testing getting/setting fanspeed with ectool")
	}

	if err := testECToolSensorTemps(ctx, h, ec); err != nil {
		errors.Wrap(err, "failed testing getting/setting fanspeed with ectool")
	}

	return nil
}

func testECToolSensorTemps(ctx context.Context, h *firmware.Helper, ec *firmware.ECTool) (reterr error) {
	testing.ContextLog(ctx, "Testing sensor temperatures")
	tempInfo, err := ec.TempsInfo(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get tempsinfo from ectool")
	}

	for sensor, idx := range tempInfo {
		temp, err := ec.Temps(ctx, idx)
		if err != nil {
			return errors.Wrapf(err, "failed to get temp for %v sensor from ectool", sensor)
		}
		testing.ContextLogf(ctx, "EC reports sensor %q has temperature %v K", sensor, temp)
		if tempInC := temp - 273; tempInC < 0 || tempInC > 100 {
			return errors.Errorf("sensor %q had unexpected abnormal temperature %v", sensor, temp)
		}
	}

	return nil
}

func testECToolFanspeed(ctx context.Context, h *firmware.Helper, ec *firmware.ECTool) (reterr error) {
	// Original autotest hardware_EC had target RPM at 10,000 in 3s but some models were unable to achieve this.
	targetFanRPM := 5000
	speedMargin := 200

	// Don't set fan speed to 5000 if already running at higher speed to makes sure fan speed isn't reduced on a device running at high temp.
	testing.ContextLog(ctx, "Polling for 15s for current fan temp")
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		fanSpeeds, err := ec.GetFanRPM(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get fanspeed from ectool")
		}
		for fan, speed := range fanSpeeds {
			if speed > targetFanRPM-speedMargin {
				return errors.Errorf("fan %v is running too fast (%v rpm) for test to run", fan, speed)
			}
		}
		return nil
	}, &testing.PollOptions{Timeout: 15 * time.Second, Interval: 3 * time.Second}); err != nil {
		return err
	}

	testing.ContextLogf(ctx, "Setting fanspeed to %d rpm", targetFanRPM)
	if err := ec.SetFanRPM(ctx, targetFanRPM); err != nil {
		return errors.Wrap(err, "failed to set fan speed with ectool")
	}
	defer func() {
		testing.ContextLog(ctx, "Resetting the fanspeed back to auto")
		if err := ec.AutoFanCtrl(ctx); err != nil {
			if reterr != nil {
				testing.ContextLog(ctx, "Failed to set fanspeed back to auto control")
			} else {
				reterr = errors.Wrap(err, "failed to set fanspeed back to auto control")
			}
			testing.ContextLog(ctx, "Rebooting DUT to set fanspeed back to auto")
			if err := h.DUT.Reboot(ctx); err != nil {
				testing.ContextLog(ctx, "Failed to reboot DUT")
			}
		}
	}()

	testing.ContextLogf(ctx, "Polling for 15s for fan to reach to %d+-%d rpm", targetFanRPM, speedMargin)
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		fanSpeeds, err := ec.GetFanRPM(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get fanspeed from ectool")
		}
		for fan, speed := range fanSpeeds {
			if speed > targetFanRPM+speedMargin || speed < targetFanRPM-speedMargin {
				return errors.Errorf("expected fan %d to have speed %d+-%d but had speed %d", fan, targetFanRPM, speedMargin, speed)
			}
		}
		return nil
	}, &testing.PollOptions{Timeout: 15 * time.Second, Interval: 3 * time.Second}); err != nil {
		return err
	}

	return nil
}

func testECToolBattery(ctx context.Context, h *firmware.Helper, ec *firmware.ECTool) error {
	testing.ContextLog(ctx, "Testing battery temp")
	tempInfo, err := ec.TempsInfo(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get tempsinfo from ectool")
	}

	if idx, ok := tempInfo["Battery"]; !ok {
		testing.ContextLog(ctx, "EC does not report battery temp")
	} else {
		temp, err := ec.Temps(ctx, idx)
		if err != nil {
			return errors.Wrap(err, "failed to get battery temp from ectool")
		}
		testing.ContextLogf(ctx, "EC reports battery temperature is %v K", temp)
	}

	testing.ContextLog(ctx, "Attempting to query for battery info")
	if battInfo, err := ec.Battery(ctx); err != nil {
		return errors.Wrapf(err, "failed to get battery info from ectool, got output: %v", battInfo)
	}

	return nil
}
