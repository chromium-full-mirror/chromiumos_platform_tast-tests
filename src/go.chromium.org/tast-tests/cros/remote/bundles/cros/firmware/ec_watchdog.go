// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
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
		Attr:         []string{"group:firmware", "firmware_ec"},
		Fixture:      fixture.NormalMode,
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Requirements: []string{"sys-fw-0022-v02"},
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
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

	// If panicInfo already contains a watchdog, force a divide zero panic to clear it
	panicInfo, err = firmware.NewECTool(h.DUT, firmware.ECToolNameMain).GetPanicInfo(ctx)
	if err != nil {
		s.Fatal("Failed to fetch current panicinfo: ", err)
	}
	if watchdogPanicReason.MatchString(panicInfo) || watchdogWarnPanicReason.MatchString(panicInfo) {
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
		if panicInfo, err = firmware.NewECTool(h.DUT, firmware.ECToolNameMain).GetPanicInfo(ctx); err != nil {
			s.Fatal("Failed to fetch current panicinfo: ", err)
		}
		if watchdogPanicReason.MatchString(panicInfo) || watchdogWarnPanicReason.MatchString(panicInfo) {
			s.Fatal("Failed to clear panicinfo")
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
	panicInfo, err = firmware.NewECTool(h.DUT, firmware.ECToolNameMain).GetPanicInfo(ctx)
	if err != nil {
		s.Fatal("Failed to fetch current panicinfo: ", err)
	}
	if watchdogPanicReason.MatchString(panicInfo) {
		s.Fatal("Unexpected watchdog panicinfo caused by short wait")
	}
	if watchdogWarnPanicReason.MatchString(panicInfo) {
		s.Fatal("Unexpected watchdog warning caused by watchdog warning")
	}

	// Watchdog warning test
	s.Logf("Deriving watchdog warning delay from ec_watchdog_period(%s) and ec_watchdog_warning_leading_time(%s)", h.Config.ECWatchdogPeriod, h.Config.ECWatchdogLeadingTime)
	watchdogWarnDelay := h.Config.ECWatchdogPeriod - h.Config.ECWatchdogLeadingTime/2
	if watchdogWarnDelay < 0 {
		s.Fatal("Invalid watchdogWarnDelay")
	}
	cmd = fmt.Sprintf("waitms %d", watchdogWarnDelay.Milliseconds())
	s.Logf("Watchdog warning delay %q, expect warning, but no panic or reboot", cmd)
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

	if newBootID, err = h.Reporter.BootID(ctx); err != nil {
		s.Fatal("Failed to fetch current boot ID: ", err)
	}
	if newBootID != oldBootID {
		s.Fatal("Unexpected device reboot caused by watchdog warning")
	}

	panicInfo, err = firmware.NewECTool(h.DUT, firmware.ECToolNameMain).GetPanicInfo(ctx)
	if err != nil {
		s.Fatal("Failed to fetch current panicinfo: ", err)
	}
	if watchdogPanicReason.MatchString(panicInfo) {
		s.Fatal("Unexpected watchdog panicinfo caused by watchdog warning")
	}
	if watchdogWarnPanicReason.MatchString(panicInfo) {
		s.Log("Watchdog warning found in panicinfo (expected)")
	} else {
		s.Log("Watchdog warning not found in panicinfo (unexpected, but not a failure)")
	}

	// Watchdog panic test
	watchdogDelay := h.Config.ECWatchdogPeriod * 2
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
	panicInfo, err = firmware.NewECTool(h.DUT, firmware.ECToolNameMain).GetPanicInfo(ctx)
	if err != nil {
		s.Fatal("Failed to fetch current panicinfo: ", err)
	}
	if !watchdogPanicReason.MatchString(panicInfo) {
		s.Fatal("Watchdog panic reason missing in panicinfo")
	}

	s.Logf("Boot ID old: %s, new: %s", newBootID, oldBootID)
}
