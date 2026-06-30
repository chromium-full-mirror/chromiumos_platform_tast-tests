// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/framework/protocol"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ECWatchdog,
		Desc: "Servo based EC watchdog test",
		Contacts: []string{
			"chromeos-faft@google.com",
			"js@semihalf.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		TestBedDeps:  tbdep.ServoPresentAndWorking,
		Attr:         []string{"group:firmware", "firmware_ec", "firmware_stressed", "firmware_meets_kpi", "firmware_ec_ro", "firmware_ec_rw"},
		Fixture:      fixture.NormalMode,
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
	})
}

func isPanicOnWatchdogWarningEnabled(dutFeatures *protocol.DUTFeatures) (bool, error) {
	satisfied, _, err := hwdep.ECBuildConfigOptions("PANIC_ON_WATCHDOG_WARNING", "PLATFORM_EC_PANIC_ON_WATCHDOG_WARNING").Satisfied(dutFeatures.GetHardware())
	if err != nil {
		return false, errors.Wrap(err, "failed to check PANIC_ON_WATCHDOG_WARNING")
	}
	if !satisfied {
		return false, nil
	}

	return true, nil
}

// getPanicInfo attempts to get panicinfo upto 3 times before giving up,
// since EC communication can be flaky on some boards (e.g. Asurada).
func getPanicInfo(ctx context.Context, s *testing.State, h *firmware.Helper) (string, error) {
	var err error
	const retryCount = 3
	for i := 0; i < retryCount; i++ {
		panicInfo, err := firmware.NewECTool(h.DUT, firmware.ECToolNameMain).GetPanicInfo(ctx)
		if err == nil {
			return panicInfo, nil
		}
		s.Logf("Failed to fetch panicinfo: %v, retrying", err)
		// GoBigSleepLint: Wait for EC communication to settle
		testing.Sleep(ctx, 100*time.Millisecond)
	}
	return "", errors.Wrapf(err, "failed to fetch panicinfo after %d retries", retryCount)
}

func ECWatchdog(ctx context.Context, s *testing.State) {
	const (
		// Delay of EC power on.
		ecBootDelay = 1000 * time.Millisecond
	)
	var (
		oldBootID               string
		newBootID               string
		err                     error
		cmd                     string
		delay                   time.Duration
		panicInfo               string
		watchdogPanicReason     = regexp.MustCompile(`(?i)dead6664`)
		watchdogWarnPanicReason = regexp.MustCompile(`(?i)dead6668`)
	)

	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servod")
	}

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}

	// Skip checking for EC crashes in this test because it crashes intentionally.
	h.CheckECCrash = false

	panicOnWatchdogWarning, err := isPanicOnWatchdogWarningEnabled(s.Features(""))
	if err != nil {
		s.Log("Unable to determine if panic on watchdog warning is enabled")
	}

	// If panicInfo already contains a watchdog or is corrupt, force a divide zero panic to clear it
	panicInfo, err = getPanicInfo(ctx, s, h)
	if err != nil {
		s.Log("Failed to fetch current panicinfo, attempt to clear")
	}
	if err != nil || watchdogPanicReason.MatchString(panicInfo) || watchdogWarnPanicReason.MatchString(panicInfo) {
		s.Log("Force a divide by zero panic to clear existing watchdog panicinfo")
		// Force a div zero panic to clear any existing watchdog
		if err := h.Servo.RunECCommand(ctx, "crash divzero"); err != nil {
			s.Fatal("Failed to run EC command: ", err)
		}
		delay = time.Duration(ecBootDelay)
		s.Logf("Sleep %s during watchdog reset", delay)
		// GoBigSleepLint: wait for panic to kick in and reset
		if err = testing.Sleep(ctx, delay); err != nil {
			s.Fatal("Failed to sleep during waiting for EC to get up: ", err)
		}
		s.Log("Wait for DUT to reconnect")
		if err = h.DUT.WaitConnect(ctx); err != nil {
			s.Fatal("Failed to reconnect to DUT: ", err)
		}
		if panicInfo, err = getPanicInfo(ctx, s, h); err != nil {
			s.Fatal("Failed to fetch current panicinfo: ", err)
		}
		if watchdogPanicReason.MatchString(panicInfo) || watchdogWarnPanicReason.MatchString(panicInfo) {
			s.Fatal("Failed to clear panicinfo")
		}
		if err := checkProgramCounter(ctx, panicInfo); err != nil {
			s.Log("Unexpected program counter value: ", err)
		}
	}

	if oldBootID, err = h.Reporter.BootID(ctx); err != nil {
		s.Fatal("Failed to fetch current boot ID: ", err)
	}

	s.Log("Trigger an IO sync")
	err = h.DUT.Conn().CommandContext(ctx, "sync").Run(ssh.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed to sync IO on DUT before calling watchdog: ", err)
	}

	// No watchdog test
	noWatchdogDelay := h.Config.ECWatchdogPeriod - h.Config.ECWatchdogLeadingTime*2
	if noWatchdogDelay < 0 {
		s.Fatal("Invalid noWatchdogDelay")
	}
	cmd = fmt.Sprintf("waitms %d", noWatchdogDelay.Milliseconds())
	s.Logf("Short delay %q, expect no reboot", cmd)
	err = h.Servo.RunECCommand(ctx, cmd)
	if err != nil {
		s.Fatal("Failed to send watchdog timer command to EC: ", err)
	}

	delay = noWatchdogDelay + ecBootDelay
	s.Logf("Wait %s", delay)
	// GoBigSleepLint: wait to ensure no reset happened
	if err = testing.Sleep(ctx, delay); err != nil {
		s.Fatal("Failed to sleep during waiting for EC to get up: ", err)
	}
	s.Log("Wait for DUT to reconnect")
	if err = h.DUT.WaitConnect(ctx); err != nil {
		s.Fatal("Failed to reconnect to DUT: ", err)
	}

	if newBootID, err = h.Reporter.BootID(ctx); err != nil {
		s.Fatal("Failed to fetch current boot ID: ", err)
	}
	if newBootID != oldBootID {
		s.Fatal("Unexpected device reboot")
	}
	panicInfo, err = getPanicInfo(ctx, s, h)
	if err != nil {
		s.Fatal("Failed to fetch current panicinfo: ", err)
	}
	if watchdogPanicReason.MatchString(panicInfo) {
		s.Fatal("Unexpected watchdog caused by short wait")
	}
	if watchdogWarnPanicReason.MatchString(panicInfo) {
		s.Log("Unexpected watchdog warning caused by short wait")
	}
	if err := checkProgramCounter(ctx, panicInfo); err != nil {
		s.Log("Unexpected program counter value: ", err)
	}

	// Watchdog warning test
	s.Logf("Deriving watchdog warning delay from ec_watchdog_period(%s) and ec_watchdog_warning_leading_time(%s)", h.Config.ECWatchdogPeriod, h.Config.ECWatchdogLeadingTime)
	watchdogWarnDelay := h.Config.ECWatchdogPeriod - h.Config.ECWatchdogLeadingTime/2
	if watchdogWarnDelay < 0 {
		s.Fatal("Invalid watchdogWarnDelay")
	}
	cmd = fmt.Sprintf("waitms %d", watchdogWarnDelay.Milliseconds())
	if panicOnWatchdogWarning {
		s.Logf("Watchdog warning delay %q, expect panic and reboot", cmd)
	} else {
		s.Logf("Watchdog warning delay %q, expect warning, but no panic or reboot", cmd)
	}
	err = h.Servo.RunECCommand(ctx, cmd)
	if err != nil {
		s.Fatal("Failed to send watchdog timer command to EC: ", err)
	}

	delay = watchdogWarnDelay + ecBootDelay
	s.Logf("Wait %s", delay)
	// GoBigSleepLint: wait to ensure no reset happened
	if err = testing.Sleep(ctx, delay); err != nil {
		s.Fatal("Failed to sleep during waiting for EC to get up: ", err)
	}
	s.Log("Wait for DUT to reconnect")
	if err = h.DUT.WaitConnect(ctx); err != nil {
		s.Fatal("Failed to reconnect to DUT: ", err)
	}

	newBootID, err = h.Reporter.BootID(ctx)
	if err != nil {
		s.Fatal("Failed to fetch current boot ID: ", err)
	}
	panicInfo, err = getPanicInfo(ctx, s, h)
	if err != nil {
		s.Fatal("Failed to fetch current panicinfo: ", err)
	}

	if panicOnWatchdogWarning {
		if newBootID == oldBootID {
			s.Fatal("Watchdog warning failed to trigger expected reboot, old boot ID is the same as new boot ID")
		}
		if !watchdogPanicReason.MatchString(panicInfo) {
			s.Fatal("Watchdog panic reason missing in panicinfo")
		}
	} else {
		if newBootID != oldBootID {
			s.Fatal("Watchdog warning caused unexpected reboot, old boot ID is not the same as new boot ID")
		}
		if watchdogPanicReason.MatchString(panicInfo) {
			s.Fatal("Unexpected watchdog panic caused by watchdog warning")
		}
		if watchdogWarnPanicReason.MatchString(panicInfo) {
			s.Log("Watchdog warning found in panicinfo (expected)")
		} else {
			s.Log("Watchdog warning not found in panicinfo (unexpected, but not a failure)")
		}
	}
	if err := checkProgramCounter(ctx, panicInfo); err != nil {
		s.Log("Unexpected program counter value: ", err)
	}

	// Watchdog panic test
	// On some older platforms, the EC watchdog time gets locked by the
	// RO code when the device shipped. To workaround this, some EC builds
	// reload the watchdog on the first warning, effectively doubling the
	// hardware watchdog timeout.
	//
	// There's no harm in waiting longer for a watchdog, so wait 4 times
	// the EC's configured watchdog period.
	watchdogDelay := h.Config.ECWatchdogPeriod * 4
	s.Log("HW watchdog waiting period ", watchdogDelay)

	cmd = fmt.Sprintf("waitms %d", watchdogDelay.Milliseconds())
	s.Logf("Trigger watchdog event %q", cmd)
	err = h.Servo.RunECCommand(ctx, cmd)
	if err != nil {
		s.Fatal("Failed to send watchdog timer command to EC: ", err)
	}

	delay = watchdogDelay + ecBootDelay
	s.Logf("Sleep %s during watchdog reset", delay)
	// GoBigSleepLint: wait for watchdog to kick in and reset
	if err = testing.Sleep(ctx, delay); err != nil {
		s.Fatal("Failed to sleep during waiting for EC to get up: ", err)
	}
	s.Log("Wait for DUT to reconnect")
	if err = h.DUT.WaitConnect(ctx); err != nil {
		s.Fatal("Failed to reconnect to DUT: ", err)
	}
	if newBootID, err = h.Reporter.BootID(ctx); err != nil {
		s.Fatal("Failed to fetch current boot ID: ", err)
	}
	if newBootID == oldBootID {
		s.Fatal("Failed to reboot trigger watchdog reset, old boot ID is the same as new boot ID")
	}
	panicInfo, err = getPanicInfo(ctx, s, h)
	if err != nil {
		s.Fatal("Failed to fetch current panicinfo: ", err)
	}
	if !watchdogPanicReason.MatchString(panicInfo) {
		s.Fatal("Watchdog panic reason missing in panicinfo")
	}
	if err := checkProgramCounter(ctx, panicInfo); err != nil {
		s.Log("Unexpected program counter value: ", err)
	}

	s.Logf("Boot ID old: %s, new: %s", newBootID, oldBootID)
}

func checkProgramCounter(ctx context.Context, panicInfo string) error {
	if strings.Contains(panicInfo, "No panic data") {
		return nil
	}

	pcMap := map[string]*regexp.Regexp{
		"cortex-m0 or risc-v": regexp.MustCompile(`pc\s*[\:]\s*([0-9a-fA-F]*)\s`), // Looks like: `... pc :{hex val or empty}\n`
		"nds32":               regexp.MustCompile(`(?i)IPC\s*([0-9a-fA-F]*)\s`),   // Looks like: `IPC {hex val or empty} ...`
	}
	for arch, regex := range pcMap {
		match := regex.FindStringSubmatch(panicInfo)
		if match == nil {
			continue
		}
		if len(match) < 2 || match[1] == "" {
			return errors.Errorf("program counter was unexpectedly blank, expected non-zero value: %v", panicInfo)
		}
		pc, err := strconv.ParseInt(match[1], 16, 64)
		if err != nil {
			return errors.Wrapf(err, "failed to parse program counter value from match %v to int", match[1])
		}
		testing.ContextLogf(ctx, "Found program counter value (arch: %v): %v (%v)", arch, pc, strings.TrimSpace(match[0]))
		if pc == 0 {
			return errors.Errorf("program counter was unexpectedly 0, expected non-zero value: %v", panicInfo)
		}
		return nil
	}
	return errors.Errorf("unable to find value for program counter in panic info, unsupported architecture or bad regex: %v", panicInfo)
}
