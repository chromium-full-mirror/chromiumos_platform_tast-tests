// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"fmt"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	pb "go.chromium.org/tast-tests/cros/services/cros/firmware"

	"go.chromium.org/tast/core/errors"
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
		Vars:         []string{"firmware.suspendStressFailFast"},
		Attr:         []string{"group:firmware"},
		ServiceDeps:  []string{"tast.cros.firmware.UtilsService", "tast.cros.firmware.TPMService"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		SoftwareDeps: []string{"chrome"},
		Fixture:      fixture.NormalMode,
		Params: []testing.Param{
			{
				Name:      "short",
				Timeout:   30 * time.Minute,
				Val:       25,
				ExtraAttr: []string{"firmware_stress"},
			},
			{
				Name:    "medium",
				Timeout: 300 * time.Minute,
				Val:     250,
			},
			{
				Name:    "long",
				Timeout: 3000 * time.Minute,
				Val:     2500,
			},
		},
	})
}

const (
	minResuspendResumeTime = 2
	maxResuspendResumeTime = 8 // Sets time range between [minResuspendResumeTime, minResuspendResumeTime+maxResuspendResumeTime).
	suspendDuration        = 15
	powerdDelayDur         = 3
)

func SuspendStress(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}

	if err := h.Servo.RemoveCCDWatchdogs(ctx); err != nil {
		s.Fatal("Failed to remove ccd watchdog: ", err)
	}

	// Number of iterations to run stress test for.
	numIters := s.Param().(int)

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

	s.Log("Resetting DUT to clear any existing chrome sessions")
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateReset); err != nil {
		s.Fatal("Failed to reset DUT: ", err)
	}
	s.Log("Reconnecting to DUT")
	if err := h.WaitConnect(ctx); err != nil {
		s.Fatal("Failed to reconnect to DUT after reset at iteration: ", err)
	}

	failures := make(map[int][]error, numIters)
	if !failFast {
		for i := 0; i < numIters; i++ {
			failures[i] = []error{}
		}
	}

	logFailure := func(msg string, err error, iter int) {
		if failFast {
			s.Fatalf("%s: %v", msg, err)
		} else {
			s.Log(msg)
			failures[iter] = append(failures[iter], errors.Wrap(err, msg))
		}
	}

	// GoBigSleepLint: Original autotest included a stop/start modemfwd as a temporary workaround for misbehaved modemfwd (b/164255562).
	// Since associated issues are fixed, a simple wait should be enough to prevent related errors.
	if err := testing.Sleep(ctx, 2*time.Second); err != nil {
		s.Fatal("Failed to sleep for 2s")
	}

	for i := 0; i < numIters; i++ {
		s.Logf("Running iteration %d out of %d ", i+1, numIters)

		s.Logf("Suspending dut for %d seconds", suspendDuration)
		// The --wakup_timeout automatically unsuspends after given time.
		cmd := h.DUT.Conn().CommandContext(ctx, "powerd_dbus_suspend", fmt.Sprintf("--delay=%d", powerdDelayDur), fmt.Sprintf("--suspend_for_sec=%d", suspendDuration))
		if err := cmd.Start(); err != nil {
			logFailure("Failed to initiate suspend on DUT", err, i)
		}

		h.DisconnectDUT(ctx)

		s.Log("Checking for S0ix or S3 powerstate")
		if err := h.WaitForPowerStates(ctx, 250*time.Millisecond, time.Duration(suspendDuration+powerdDelayDur)*time.Second, "S0ix", "S3"); err != nil {
			logFailure("Failed to get S0ix or S3 powerstate after suspend", err, i)
		}

		// The --suspend_for_sec means it should automatically go back to S0.
		s.Log("Checking for S0 powerstate")
		if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout, "S0"); err != nil {
			logFailure("Failed to get S0 powerstate after waking from suspend", err, i)
		}

		if err := h.WaitConnect(ctx); err != nil {
			logFailure("Failed to reconnnect to DUT after waking from suspend", err, i)
		}

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

		// if something went wrong during some iteration, reset so other iterations remain independent.
		if len(failures[i]) > 0 {
			// Don't ignore these errors as they likely indicate that DUT is offline after iteration.
			s.Log("Resetting DUT due to errors on this iteration")
			if err := h.Servo.SetPowerState(ctx, servo.PowerStateReset); err != nil {
				s.Fatalf("Failed to reset DUT after failures in suspend at iteration %d: %v", i+1, err)
			}
			s.Log("Reconnecting to DUT")
			if err := h.WaitConnect(ctx); err != nil {
				s.Fatalf("Failed to reconnect to DUT after reset at iteration %d: %v", i+1, err)
			}
		}

		resuspendDelay := time.Duration(minResuspendResumeTime+rand.Intn(maxResuspendResumeTime)) * time.Second
		s.Logf("Sleeping for %s before next iteration", resuspendDelay)
		// GoBigSleepLint: random duration in range [minResuspendResumeTime, minResuspendResumeTime+maxResuspendResumeTime) to wait between suspend iterations.
		if err := testing.Sleep(ctx, resuspendDelay); err != nil {
			// Don't ignore this error even without fail fast set as it means context timed out.
			s.Fatalf("Test timed out between suspends on iteration %d: %v", i+1, err)
		}
	}

	s.Log("Errors encountered:")
	numFails := 0
	for iter, errors := range failures {
		numFails += len(errors)
		if len(errors) > 0 {
			s.Logf("Iter %d: Had the following failures:", iter+1)
			for _, errMsg := range errors {
				s.Logf("\t%v", errMsg)
			}
		}
	}
	if numFails > 0 {
		s.Fatalf("Encountered %d errors during execution of stress test, check execution log for details", numFails)
	} else {
		s.Log("\tNo errors encountered")
	}

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

	h.DisconnectDUT(ctx)
	if err := h.WaitConnect(ctx); err != nil {
		return errors.Wrap(err, "failed to connect to DUT")
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
	h.DisconnectDUT(ctx)
	if err := h.WaitConnect(ctx); err != nil {
		return errors.Wrap(err, "failed to connect to DUT")
	}

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
