// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package powercontrol

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/services/cros/security"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
)

// ChromeOSLogin performs login to DUT.
func ChromeOSLogin(ctx context.Context, dut *dut.DUT, rpcHint *testing.RPCHint) error {
	cl, err := rpc.Dial(ctx, dut, rpcHint)
	if err != nil {
		return errors.Wrap(err, "failed to connect to the RPC service on the DUT")
	}
	defer cl.Close(ctx)
	client := security.NewBootLockboxServiceClient(cl.Conn)
	if _, err := client.NewChromeLogin(ctx, &empty.Empty{}); err != nil {
		return errors.Wrap(err, "failed to start Chrome")
	}
	return nil
}

// IsPrevSleepStateAvailable performas a basic check that cbmem returns a value for
// prev_sleep_state in its output. Not all platforms return prev_sleep_state, so
// we can't use ValidatePrevSleepState in tests on those platforms. Here we introduce
// a check that prev_sleep_state is included in cbmem output so we can check the
// values on platforms that do support it.
func IsPrevSleepStateAvailable(ctx context.Context, dut *dut.DUT) (bool, error) {
	// Command to check previous sleep state.
	const cmd = "cbmem -c | grep 'prev_sleep_state' | tail -1"
	out, err := dut.Conn().CommandContext(ctx, "sh", "-c", cmd).Output()
	if err != nil {
		return false, errors.Wrapf(err, "failed to execute %q command", cmd)
	}

	got := strings.TrimSpace(string(out))

	return len(got) > 0, nil
}

// ValidatePrevSleepState sleep state from cbmem command output.
// NOTE: This method currently is only valid on Intel SoCs, as they are the
// only SoC that output prev_sleep_state from cbmem - See b/252884546#6 for
// the details.
func ValidatePrevSleepState(ctx context.Context, dut *dut.DUT, sleepStateValue int) error {
	// Command to check if UFS controller is disabled.
	const ufs_cmd = "cbmem -c | grep -i 'Disabling UFS'"
	out_ufs, err_ufs := dut.Conn().CommandContext(ctx, "sh", "-c", ufs_cmd).Output()
	if err_ufs != nil {
		if !strings.Contains(string(err_ufs.Error()), "Process exited with status 1") {
			return errors.Wrapf(err_ufs, "failed to execute %q command", ufs_cmd)
		}
	}
	
	if len(out_ufs) != 0 {
		got_ufs := strings.TrimSpace(string(out_ufs))
		want_ufs := fmt.Sprintf("Disabling UFS")

		if strings.Contains(got_ufs, want_ufs) {
			sleepStateValue = 0
			testing.ContextLog(ctx, "Warm reboot has happened after cold reboot to disable UFS controller.")
		}
	}

	// Command to check previous sleep state.
	const cmd = "cbmem -c | grep 'prev_sleep_state' | tail -1"
	return testing.Poll(ctx, func(ctx context.Context) error {
		out, err := dut.Conn().CommandContext(ctx, "sh", "-c", cmd).Output()
		if err != nil {
			return errors.Wrapf(err, "failed to execute %q command", cmd)
		}

		got := strings.TrimSpace(string(out))
		want := fmt.Sprintf("prev_sleep_state %d", sleepStateValue)

		if !strings.Contains(got, want) {
			return errors.Errorf("unexpected sleep state = got %q, want %q", got, want)
		}
		return nil
	}, &testing.PollOptions{Timeout: 1 * time.Minute, Interval: 1 * time.Second})
}

// Shutdown sends shutdown command to DUT.
func Shutdown(ctx context.Context, dut *dut.DUT) error {
	powerOffCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := dut.Conn().CommandContext(powerOffCtx, "shutdown", "-h", "now").Run(); err != nil && !errors.Is(err, context.DeadlineExceeded) {
		return errors.Wrap(err, "failed to execute shutdown command")
	}
	return nil
}

// ShutdownAndWaitForPowerState verifies powerState(S5 or G3) after shutdown.
func ShutdownAndWaitForPowerState(ctx context.Context, pxy *servo.Proxy, dut *dut.DUT, powerState string) error {
	if err := Shutdown(ctx, dut); err != nil {
		return err
	}
	sdCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := dut.WaitUnreachable(sdCtx); err != nil {
		return errors.Wrap(err, "failed to wait for unreachable")
	}
	return testing.Poll(ctx, func(ctx context.Context) error {
		got, err := pxy.Servo().GetECSystemPowerState(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get EC power state")
		}
		if want := powerState; got != want {
			return errors.Errorf("unexpected DUT EC power state = got %q, want %q", got, want)
		}
		return nil
	}, &testing.PollOptions{Timeout: 35 * time.Second})
}

// WaitForSuspendState verifies powerState(S0ix or S3).
func WaitForSuspendState(ctx context.Context, h *firmware.Helper) error {
	testing.ContextLog(ctx, "Wait for power state to become S0ix or S3")
	return testing.Poll(ctx, func(ctx context.Context) error {
		state, err := h.Servo.GetECSystemPowerState(ctx)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to get power state"))
		}
		if state != "S0ix" && state != "S3" {
			return errors.New("power state is " + state)
		}
		return nil
	}, &testing.PollOptions{Interval: 1 * time.Second, Timeout: 30 * time.Second})
}

// PowerOntoDUT performs power normal press to wake DUT.
func PowerOntoDUT(ctx context.Context, pxy *servo.Proxy, dut *dut.DUT) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		waitCtx, cancel := context.WithTimeout(ctx, 40*time.Second)
		defer cancel()
		if err := pxy.Servo().KeypressWithDuration(ctx, servo.PowerKey, servo.DurPress); err != nil {
			return errors.Wrap(err, "failed to power normal press")
		}
		if err := dut.WaitConnect(waitCtx); err != nil {
			return errors.Wrap(err, "failed to wait connect DUT")
		}
		return nil
	}, &testing.PollOptions{Timeout: 3 * time.Minute, Interval: 30 * time.Second})
}

// PowerOnDutWithRetry performs power normal press to wake DUT. Retries if it fails.
func PowerOnDutWithRetry(ctx context.Context, pxy *servo.Proxy, dut *dut.DUT) error {
	if err := PowerOntoDUT(ctx, pxy, dut); err != nil {
		testing.ContextLog(ctx, "Unable to wake up DUT. Retrying")
		return PowerOntoDUT(ctx, pxy, dut)
	}
	return nil
}

// PerformSuspendStressTest performs suspend stress test for suspendStressTestCounter cycles.
func PerformSuspendStressTest(ctx context.Context, dut *dut.DUT, suspendStressTestCounter int) error {
	const (
		zeroPrematureWakes    = "Premature wakes: 0"
		zeroSuspendFailures   = "Suspend failures: 0"
		zeroFirmwareLogErrors = "Firmware log errors: 0"
		zeroS0ixErrors        = "s0ix errors: 0"
	)

	testing.ContextLog(ctx, "Wait for a suspend test without failures")
	zeroSuspendErrors := []string{zeroPrematureWakes, zeroSuspendFailures, zeroFirmwareLogErrors, zeroS0ixErrors}
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		stressOut, err := dut.Conn().CommandContext(ctx, "suspend_stress_test", "-c", "1").Output()
		if err != nil {
			return errors.Wrap(err, "failed to execute suspend_stress_test -c 1 command")
		}

		for _, errMsg := range zeroSuspendErrors {
			if !strings.Contains(string(stressOut), errMsg) {
				return errors.Errorf("expect zero failures for %q, got %q", errMsg, string(stressOut))
			}
		}
		return nil
	}, &testing.PollOptions{Timeout: 15 * time.Second}); err != nil {
		return err
	}

	testing.ContextLogf(ctx, "Run: suspend_stress_test -c %d", suspendStressTestCounter)
	counterValue := fmt.Sprintf("%d", suspendStressTestCounter)
	stressOut, err := dut.Conn().CommandContext(ctx, "suspend_stress_test", "-c", counterValue).Output()
	if err != nil {
		return errors.Wrap(err, "failed to execute suspend_stress_test -c 10 command")
	}

	for _, errMsg := range zeroSuspendErrors {
		if !strings.Contains(string(stressOut), errMsg) {
			return errors.Errorf("failed: expect zero failures for %q, got %q", errMsg, string(stressOut))
		}
	}
	return nil
}

// SlpAndC10PackageValues returns SLP counter value and C10 package value.
func SlpAndC10PackageValues(ctx context.Context, dut *dut.DUT) (int, string, error) {
	var c10PackageRe = regexp.MustCompile(`C10 : ([A-Za-z0-9]+)`)
	const (
		slpS0File     = "/sys/kernel/debug/pmc_core/slp_s0_residency_usec"
		pkgCstateFile = "/sys/kernel/debug/pmc_core/package_cstate_show"
	)

	slpOpSetInBytes, err := linuxssh.ReadFile(ctx, dut.Conn(), slpS0File)
	if err != nil {
		return 0, "", errors.Wrap(err, "failed to get SLP counter value")
	}

	slpOpSetValue, err := strconv.Atoi(strings.TrimSpace(string(slpOpSetInBytes)))
	if err != nil {
		return 0, "", errors.Wrap(err, "failed to convert type string to integer")
	}

	pkgOpSetOutput, err := linuxssh.ReadFile(ctx, dut.Conn(), pkgCstateFile)
	if err != nil {
		return 0, "", errors.Wrap(err, "failed to get package cstate value")
	}

	matchSetValue := c10PackageRe.FindStringSubmatch(string(pkgOpSetOutput))
	if matchSetValue == nil {
		return 0, "", errors.New("failed to match pre PkgCstate value")
	}
	pkgOpSetValue := matchSetValue[1]

	return slpOpSetValue, pkgOpSetValue, nil
}

// AssertSLPAndC10 asserts the SLP  and pkgC10 counter value post Resume with SLP counter
// value before Suspend.
func AssertSLPAndC10(slpOpSetPre, slpOpSetPost int, pkgOpSetPre, pkgOpSetPost string) error {
	if slpOpSetPre == slpOpSetPost {
		return errors.Errorf("failed: SLP counter value %q should be different from the one before suspend %q", slpOpSetPost, slpOpSetPre)
	}

	if slpOpSetPost == 0 {
		return errors.Errorf("failed SLP counter value must be non-zero, got: %q", slpOpSetPost)
	}

	if pkgOpSetPre == pkgOpSetPost {
		return errors.Errorf("Failed: Package C10 value %q must be different from the one before suspend %q", pkgOpSetPost, pkgOpSetPre)
	}

	if pkgOpSetPost == "0x0" || pkgOpSetPost == "0" {
		return errors.New("Failed: Package C10 should be non-zero")
	}
	return nil
}

// ValidateG3PowerState verify power state G3 after shutdown.
func ValidateG3PowerState(ctx context.Context, pxy *servo.Proxy) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		pwrState, err := pxy.Servo().GetECSystemPowerState(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get ec power state")
		}
		if pwrState != "G3" {
			return errors.New("DUT not in G3 state")
		}
		return nil
	}, &testing.PollOptions{Timeout: 30 * time.Second})
}

// VerifyPowerdConfigSuspendValue verifies whether DUT is in expected suspend state
// with given expectedConfigValue.
func VerifyPowerdConfigSuspendValue(ctx context.Context, dut *dut.DUT, expectedConfigValue int) error {
	powerdConfigCmd := "check_powerd_config --suspend_to_idle; echo $?"
	configValue, err := dut.Conn().CommandContext(ctx, "bash", "-c", powerdConfigCmd).Output(ssh.DumpLogOnError)
	if err != nil {
		return errors.Wrap(err, "failed to execute power config check command")
	}
	got, err := strconv.Atoi(strings.TrimSpace(string(configValue)))
	if err != nil {
		return errors.Wrap(err, "failed to convert string to integer")
	}
	if want := expectedConfigValue; got != want {
		return errors.Errorf("unexpected suspend state value: got %d, want %d", got, want)
	}
	return nil
}

// PlugUnplugCharger performs plugging/unplugging of charger via servo.
func PlugUnplugCharger(ctx context.Context, h *firmware.Helper, isPowerPlugged bool) error {
	chargerStatus := ""
	if isPowerPlugged {
		testing.ContextLog(ctx, "Starting power supply")
		chargerStatus = "not attached"
	} else {
		testing.ContextLog(ctx, "Stopping power supply")
		chargerStatus = "attached"
	}
	if err := h.SetDUTPower(ctx, isPowerPlugged); err != nil {
		return errors.Wrap(err, "failed to remove charger")
	}
	getChargerPollOptions := testing.PollOptions{Timeout: 10 * time.Second}
	return testing.Poll(ctx, func(ctx context.Context) error {
		if attached, err := h.Servo.GetChargerAttached(ctx); err != nil {
			return err
		} else if isPowerPlugged != attached {
			return errors.Errorf("charger is still %q - use Servo V4 Type-C or supply RPM vars", chargerStatus)
		}
		return nil
	}, &getChargerPollOptions)
}

// PerformPowerdbusSuspend peforms DUT suspend with powerd_dbus_suspend command.
func PerformPowerdbusSuspend(ctx context.Context, dut *dut.DUT, pxy *servo.Proxy) error {
	powerOffCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := dut.Conn().CommandContext(powerOffCtx, "powerd_dbus_suspend").Run(); err != nil && !errors.Is(err, context.DeadlineExceeded) {
		return errors.Wrap(err, "failed to suspend DUT with powerd_dbus_suspend command")
	}
	sdCtx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	if err := dut.WaitUnreachable(sdCtx); err != nil {
		return errors.Wrap(err, "failed to wait for unreachable")
	}
	return nil
}

// PerformColdboot performs coldboot and power normal press to wake DUT.
func PerformColdboot(ctx context.Context, dut *dut.DUT, pxy *servo.Proxy) error {
	powerOffCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := dut.Conn().CommandContext(powerOffCtx, "shutdown", "-h", "now").Run(); err != nil && !errors.Is(err, context.DeadlineExceeded) {
		return errors.Wrap(err, "failed to execute shutdown command")
	}
	sdCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := dut.WaitUnreachable(sdCtx); err != nil {
		return errors.Wrap(err, "failed to wait for unreachable")
	}

	if err := PowerOntoDUT(ctx, pxy, dut); err != nil {
		return errors.Wrap(err, "failed to power-on DUT")
	}
	return nil
}
