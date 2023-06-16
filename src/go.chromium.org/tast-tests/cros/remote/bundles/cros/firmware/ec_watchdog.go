// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"fmt"
	"time"

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
		// TODO: When stable, change firmware_unstable to a different attr and add linto@chromium.org to gerrit review.
		Attr:         []string{"group:firmware", "firmware_unstable"},
		Fixture:      fixture.NormalMode,
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
	})
}

func ECWatchdog(ctx context.Context, s *testing.State) {
	const (
		// Small delay to make ensure watchdog does not kick in too early.
		noWatchdogDelayMS = 500
		noWatchdogDelay   = noWatchdogDelayMS * time.Millisecond
		// Delay of spin-wait in ms. Nuvoton boards set the hardware watchdog to
		// 3187.5ms and also sets a timer to 2200ms. Set the timeout long enough to
		// exceed the hardware watchdog timer because the timer isn't 100% reliable.
		// If there are other platforms that use a longer watchdog timeout, this
		// may need to be adjusted.
		watchdogDelayMS = 3700
		watchdogDelay   = watchdogDelayMS * time.Millisecond
		// Delay of EC power on.
		ecBootDelay = 1000 * time.Millisecond
	)
	var (
		oldBootID string
		newBootID string
		err       error
		cmd       string
		delay     time.Duration
	)

	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servod")
	}

	if oldBootID, err = h.Reporter.BootID(ctx); err != nil {
		s.Fatal("Failed to fetch current boot ID: ", err)
	}

	s.Log("Trigger an IO sync")
	err = h.DUT.Conn().CommandContext(ctx, "sync").Run(ssh.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed to sync IO on DUT before calling watchdog: ", err)
	}

	// No reboot test

	cmd = fmt.Sprintf("waitms %d", noWatchdogDelayMS)
	s.Logf("Short delay %q, expect no reboot", cmd)
	err = h.Servo.RunECCommand(ctx, cmd)
	if err != nil {
		s.Fatal("Failed to send watchdog timer command to EC: ", err)
	}

	delay = time.Duration(noWatchdogDelay + ecBootDelay)
	s.Logf("Wait %s", delay)
	// GoBigSleepLint: wait to ensure no reset happened
	if err = testing.Sleep(ctx, delay); err != nil {
		s.Fatal("Failed to sleep during waiting for EC to get up: ", err)
	}

	if newBootID, err = h.Reporter.BootID(ctx); err != nil {
		s.Fatal("Failed to fetch current boot ID: ", err)
	}
	if newBootID != oldBootID {
		s.Fatal("Unexpected device reboot")
	}

	// Reboot test

	cmd = fmt.Sprintf("waitms %d", watchdogDelayMS)
	s.Logf("Trigger watchdog event %q", cmd)
	err = h.Servo.RunECCommand(ctx, cmd)
	if err != nil {
		s.Fatal("Failed to send watchdog timer command to EC: ", err)
	}

	delay = time.Duration(watchdogDelay + ecBootDelay)
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

	s.Logf("Boot ID old: %s, new: %s", newBootID, oldBootID)
}
