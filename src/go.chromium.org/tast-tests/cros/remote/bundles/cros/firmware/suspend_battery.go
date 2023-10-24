// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/suspend"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const (
	requiredBatteryPercent = 75
	minBatteryPercent      = 50
	defaultCycles          = 100
	chargeCheckInterval    = time.Minute
	chargeCheckTimeout     = time.Hour
	batteryLevelTimeout    = 20 * time.Second // default servo comm timeout is 10s, battery check requires two
	batteryLevelInterval   = time.Second

	varCycles = "cycles"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: SuspendBattery,
		Desc: "Tests that the DUT suspends and resumes properly while on battery power",
		Contacts: []string{
			"chromeos-faft@google.com",
			"robertzieba@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		// TODO: When stable, change firmware_unstable to a different attr.
		Attr:         []string{"group:firmware", "firmware_unstable"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.Battery()),
		Fixture:      fixture.NormalMode,
		Timeout:      3 * time.Hour, // Allow time for the battery to potentially charge up
		Vars:         []string{varCycles},
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

func SuspendBattery(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}

	// Ensure that the DUT has enough battery power to run the test
	s.Logf("Waiting for battery to reach %d%%", requiredBatteryPercent)
	if err := waitForCharge(ctx, h, requiredBatteryPercent); err != nil {
		s.Fatalf("Failed to reach target %d%%, %s", requiredBatteryPercent, err.Error())
	}

	// Parse our vars
	suspendCycles := defaultCycles
	if v, ok := s.Var(varCycles); ok {
		newCycles, err := strconv.Atoi(v)
		if err != nil {
			s.Fatalf("Failed to parse %s from string %s", varCycles, v)
		}

		suspendCycles = newCycles
	}

	// Create our suspend context
	suspendContext, err := suspend.NewContext(ctx, h)
	if err != nil {
		s.Fatal("Failed to create suspendContext: ", err)
	}
	defer suspendContext.Close()

	// Run our cycles
	for i := 0; i < suspendCycles; i++ {
		s.Logf("Suspend cycling: %d/%d", i+1, suspendCycles)
		previousCount, err := suspendContext.GetKernelSuspendCount()
		if err != nil {
			s.Fatal("Failed to get kernel suspend count: ", err)
		}

		s.Log("Suspending DUT")
		if err := suspendContext.SuspendDUTAllTypes(suspend.DefaultSuspendArgs()); err != nil {
			s.Fatal("Failed to suspend DUT: ", err)
		}

		s.Log("Waking DUT")
		if err := suspendContext.WakeDUT(suspend.DefaultWakeArgs()); err != nil {
			s.Fatal("Failed to wake DUT: ", err)
		}

		// Check that the kernel registered one suspension
		suspendCount, err := suspendContext.GetKernelSuspendCount()
		if err != nil {
			s.Fatal("Failed to get kernel suspend count: ", err)
		}
		if suspendCount != previousCount+1 {
			s.Fatalf("Mismatch in kernel suspend counts, previous: %d, current: %d", previousCount, suspendCount)
		}

		//Charge up if we've dipped below our minimum battery level
		pct, err := getBatteryPct(ctx, h)
		if err != nil {
			s.Fatal("Failed to get battery level")
		}

		if pct < minBatteryPercent {
			s.Logf("Waiting for battery to reach %d%%", requiredBatteryPercent)
			waitForCharge(ctx, h, requiredBatteryPercent)
		}
	}
}

func waitForCharge(ctx context.Context, h *firmware.Helper, target int) error {
	// Make sure AC power is connected
	// The original setting will be restored automatically when the test ends
	if err := h.SetDUTPower(ctx, true); err != nil {
		return err
	}

	err := testing.Poll(ctx, func(ctx context.Context) error {
		pct, err := getBatteryPct(ctx, h)
		if err != nil {
			// Failed to get battery level so stop trying
			return testing.PollBreak(err)
		}

		if pct < target {
			return errors.Errorf("Current battery charge is %d%%, required %d%%", pct, target)
		}

		return nil
	}, &testing.PollOptions{Timeout: chargeCheckTimeout, Interval: chargeCheckInterval})

	if err != nil {
		return err
	}

	// Disable Servo power to DUT
	if err := h.SetDUTPower(ctx, false); err != nil {
		return err
	}

	return nil
}

func getBatteryPct(ctx context.Context, h *firmware.Helper) (int, error) {
	// Attempt to determine the battery percentage
	// Each servo communication attempt is retried to account for any transient
	// communication problems
	var err error = nil
	currentMAH := 0
	maxMAH := 0

	testing.Poll(ctx, func(ctx context.Context) error {
		currentMAH, err = h.Servo.GetBatteryChargeMAH(ctx)
		if err != nil {
			return err
		}

		maxMAH, err = h.Servo.GetBatteryFullChargeMAH(ctx)
		if err != nil {
			return err
		}

		return nil

	}, &testing.PollOptions{Timeout: batteryLevelTimeout, Interval: batteryLevelInterval})

	if err != nil {
		return -1, err
	}

	return int(100 * float32(currentMAH) / float32(maxMAH)), nil
}
