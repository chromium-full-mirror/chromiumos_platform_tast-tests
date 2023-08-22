// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/log"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ECPowerButton,
		Desc: "Verify using servo power key results in expected shutdown behaviour",
		Contacts: []string{
			"chromeos-faft@google.com",
			"tij@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware", "firmware_ec", "group:labqual"},
		Requirements: []string{"sys-fw-0022-v02"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Fixture:      fixture.NormalMode,
		Timeout:      15 * time.Minute,
	})
}

const (
	shortPowerKeyPressDur time.Duration = 200 * time.Millisecond
)

func ECPowerButton(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to require config: ", err)
	}

	s.Log("Test power off with power state")
	if err := testRebootWithSettingPowerState(ctx, h); err != nil {
		s.Fatal("Failed to reboot from setting servo powerstate: ", err)
	}

	s.Log("Test power off with powerd on and off")
	if err := testPowerdPowerOff(ctx, h); err != nil {
		s.Fatal("Failed powering off with or without powerd: ", err)
	}

	s.Log("Test that verifies powerd recieved power key press")
	if err := testPowerdReceivedPowerkey(ctx, h); err != nil {
		s.Fatal("Failed to verify powerd sent dbus signal confirming it recieved power key press: ", err)
	}
}

// testPowerdReceivedPowerkey tests a short power key press is forwarded to powerd which will either
// signal aknowledgement of the power key press or result in a shutdown.
func testPowerdReceivedPowerkey(ctx context.Context, h *firmware.Helper) error {
	testing.ContextLog(ctx, "Getting current boot id")
	bootID, err := h.Reporter.BootID(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get boot id")
	}

	powerDBusMonitor, err := log.StartDBusMonitorCollector(
		ctx,
		h.DUT.Conn(),
		"--system",
		"type='signal'",
		"interface='org.chromium.PowerManager'",
		"member='HandlePowerButtonAcknowledgment'",
	)
	if err != nil {
		return errors.Wrap(err, "failed to start dbus-monitor")
	}
	defer powerDBusMonitor.Close()

	testing.ContextLog(ctx, "Pressing power key")
	if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.Dur(shortPowerKeyPressDur)); err != nil {
		return errors.Wrap(err, "failed to press power key on DUT")
	}

	// GoBigSleepLint: Wait a short amount to make sure dbus monitor had enough time to log power key press acknowledgement.
	if err := testing.Sleep(ctx, time.Second); err != nil {
		return errors.Wrap(err, "failed to sleep for 1s")
	}

	var buff bytes.Buffer
	dumpErr := powerDBusMonitor.Dump(&buff)

	testing.ContextLog(ctx, "Checking for G3/S5 powerstate")
	if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, 30*time.Second, "G3", "S5"); err == nil {
		testing.ContextLog(ctx, "DUT powered off from short power key press, pressing power key to reboot")
		if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.Dur(h.Config.HoldPwrButtonPowerOn)); err != nil {
			return errors.Wrap(err, "failed to press power key on DUT")
		}

		testing.ContextLog(ctx, "Waiting for S0 powerstate")
		if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout, "S0"); err != nil {
			return errors.Wrap(err, "failed to get S0 powerstate")
		}

		ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		if err := h.WaitConnect(ctx); err != nil {
			return errors.Wrap(err, "failed connect to DUT after power key on")
		}
		return nil
	}

	if dumpErr != nil {
		return errors.Wrap(err, "failed to dump dbus-monitor logs to buffer")
	}
	logs := buff.String()
	testing.ContextLog(ctx, "DUT did not power off from short power key press, verifying power key press acknowledged by powerd")
	if !strings.Contains(logs, "path=/org/chromium/PowerManager; interface=org.chromium.PowerManager; member=HandlePowerButtonAcknowledgment") {
		return errors.Errorf("failed to detect powerkey dbus signal from power manager: %v", logs)
	}
	testing.ContextLog(ctx, "Power key pressed detected")

	testing.ContextLog(ctx, "Get new boot id, compare to old")
	if newBootID, err := h.Reporter.BootID(ctx); err != nil {
		return errors.Wrap(err, "failed to get current boot id")
	} else if newBootID != bootID {
		return errors.Errorf("boot ID unexpectedly changed from %s to %s", bootID, newBootID)
	}
	return nil
}

// shutdownAndWake shuts down then wakes DUT with power key press.
func shutdownAndWake(ctx context.Context, h *firmware.Helper, shutDownDur time.Duration, expStates ...string) error {
	testing.ContextLogf(ctx, "Pressing power key for %s", shutDownDur)
	if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.Dur(shutDownDur)); err != nil {
		return errors.Wrap(err, "failed to press power key on DUT")
	}
	h.DisconnectDUT(ctx)

	testing.ContextLogf(ctx, "Checking for %v powerstates", expStates)
	if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout, expStates...); err != nil {
		return errors.Wrapf(err, "failed to get %v powerstates", expStates)
	}

	// GoBigSleepLint: If we are expecting S5/G3, we might still get to G3 after S5, so give it a little time before we wake up again.
	if err := testing.Sleep(ctx, time.Second*2); err != nil {
		return errors.Wrap(err, "sleep failed")
	}

	testing.ContextLog(ctx, "Send cmd to EC to wake up from deepsleep")
	h.Servo.RunECCommand(ctx, "help")

	testing.ContextLog(ctx, "Pressing power key (press)")
	if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.DurPress); err != nil {
		return errors.Wrap(err, "failed to press power key on DUT")
	}

	testing.ContextLog(ctx, "Waiting for S0 powerstate")
	if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout, "S0"); err != nil {
		return errors.Wrap(err, "failed to get S0 powerstate")
	}

	testing.ContextLog(ctx, "Waiting for DUT to connect")
	if err := h.WaitConnect(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for DUT to connect")
	}

	return nil
}

// enablePowerd either disables or re-enables powerd service.
func enablePowerd(ctx context.Context, h *firmware.Helper, status bool) error {
	startOrStop := "start"
	if !status {
		startOrStop = "stop"
	}

	startStopJob := func(ctx context.Context, job string, missingOk bool) error {
		if status, err := h.DUT.Conn().CommandContext(ctx, "status", job).Output(); err == nil {
			if strings.Contains(string(status), startOrStop) {
				testing.ContextLogf(ctx, "Job %q already has status %q", job, startOrStop)
				return nil
			}
		}
		cmd := h.DUT.Conn().CommandContext(ctx, startOrStop, job)
		stderr, _ := cmd.StderrPipe()
		if err := cmd.Start(); err != nil {
			return errors.Wrapf(err, "failed to run '%s %s' cmd on DUT", startOrStop, job)
		}
		scanner := bufio.NewScanner(stderr)
		errMsg := ""
		for scanner.Scan() {
			errMsg = fmt.Sprintf("%s\n%s", errMsg, scanner.Text())
		}
		if err := cmd.Wait(); err != nil {
			// If stopping already stopped job or starting already running job, ignore error.
			if strings.Contains(errMsg, "Job is already running") || strings.Contains(errMsg, "Unknown instance") {
				return nil
			}
			if missingOk && strings.Contains(errMsg, "Unknown job:") {
				return nil
			}
			return errors.Wrapf(err, "failed to %s job %v, got error: %s", startOrStop, job, errMsg)
		}
		return nil
	}

	if status {
		if err := startStopJob(ctx, "powerd", false); err != nil {
			return errors.Wrap(err, "failed to start powerd")
		}
	}

	if err := startStopJob(ctx, "fwupd", true); err != nil {
		return errors.Wrapf(err, "failed to %v fwupd", startOrStop)
	}

	if !status {
		if err := startStopJob(ctx, "powerd", false); err != nil {
			return errors.Wrap(err, "failed to stop powerd")
		}
	}

	return nil
}

// testPowerdPowerOff tests rebooting with power button with powerd service stopped.
func testPowerdPowerOff(ctx context.Context, h *firmware.Helper) (reterr error) {
	testing.ContextLog(ctx, "stopping powerd")
	if err := enablePowerd(ctx, h, false); err != nil {
		return errors.Wrap(err, "failed to stop powerd")
	}
	// Make sure fwupd and powerd are running again after test.
	defer func() {
		if err := h.EnsureDUTBooted(ctx); err != nil {
			if reterr != nil {
				testing.ContextLog(ctx, "Failed to ensure dut booted: ", err)
			} else {
				reterr = errors.Wrap(err, "failed to ensure dut booted")
			}
			return
		}
		if err := enablePowerd(ctx, h, true); err != nil {
			if reterr != nil {
				testing.ContextLog(ctx, "Failed to restart powerd after test end: ", err)
			} else {
				reterr = errors.Wrap(err, "failed to restart powerd after test end")
			}
			return
		}
	}()

	powerdDur := h.Config.HoldPwrButtonPowerOff
	noPowerdDur := h.Config.HoldPwrButtonNoPowerdShutdown

	if err := shutdownAndWake(ctx, h, noPowerdDur, "G3"); err != nil {
		return errors.Wrap(err, "failed shut down and wake with no powerd")
	}

	testing.ContextLog(ctx, "starting powerd")
	if err := enablePowerd(ctx, h, true); err != nil {
		return errors.Wrap(err, "failed to start powerd")
	}

	if err := shutdownAndWake(ctx, h, powerdDur, "S5", "G3"); err != nil {
		return errors.Wrap(err, "failed shut down and wake with powerd")
	}

	return nil
}

// testRebootWithSettingPowerState tests the DUT can power off and power on with power key.
func testRebootWithSettingPowerState(ctx context.Context, h *firmware.Helper) error {
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateOff); err != nil {
		return errors.Wrap(err, "failed to set 'power_state' to 'off'")
	}

	testing.ContextLog(ctx, "Waiting for G3 or S5 powerstate")
	if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout, "G3", "S5"); err != nil {
		return errors.Wrap(err, "failed to get G3 or S5 powerstate")
	}

	testing.ContextLog(ctx, "Pressing power key to turn on DUT")
	if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.Dur(h.Config.HoldPwrButtonPowerOn)); err != nil {
		return errors.Wrap(err, "failed to press power key on DUT")
	}

	testing.ContextLog(ctx, "Waiting for S0 powerstate")
	if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout, "S0"); err != nil {
		return errors.Wrap(err, "failed to get S0 powerstate")
	}

	ctx, cancel := context.WithTimeout(ctx, 6*time.Minute)
	defer cancel()
	if err := h.WaitConnect(ctx); err != nil {
		return errors.Wrap(err, "failed connect to DUT after power key on")
	}
	return nil
}
