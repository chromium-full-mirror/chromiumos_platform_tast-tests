// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: DevModeStress,
		Desc: "Test mode aware reboot and suspend preserve dev mode over several iterations",
		Contacts: []string{
			"chromeos-faft@google.com",
			"tij@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware", "firmware_stress"},
		Vars:         []string{"firmware.DevModeStressIters"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Timeout:      60 * time.Minute,
		Fixture:      fixture.DevMode,
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

func DevModeStress(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}

	numIters := 30
	if numItersStr, ok := s.Var("firmware.DevModeStressIters"); ok {
		numItersInt, err := strconv.Atoi(numItersStr)
		if err != nil {
			s.Fatalf("Invalid value for var firmware.DevModeStressIters: got %q, expected int", numItersStr)
		} else {
			numIters = numItersInt
		}
	}

	verifyBootMode := func() error {
		if mainfwType, err := h.Reporter.CrossystemParam(ctx, reporters.CrossystemParamMainfwType); err != nil {
			return errors.Wrap(err, "failed to get crossystem mainfw_type")
		} else if mainfwType != "developer" {
			return errors.Errorf("expected mainfw_type to be 'developer', got %q", mainfwType)
		}

		if devswBoot, err := h.Reporter.CrossystemParam(ctx, reporters.CrossystemParamDevswBoot); err != nil {
			return errors.Wrap(err, "failed to get crossystem devsw_boot")
		} else if devswBoot != "1" {
			return errors.Errorf("expected devsw_boot to be 1, got %s", devswBoot)
		}
		return nil
	}

	// Fixture ensures initially in dev mode, so for first iteration boot mode doesn't need to be checked.
	for i := 0; i < numIters; i++ {
		s.Logf("Running iteration %d out of %d ", i+1, numIters)

		s.Log("Performing mode aware reboot")
		ms, err := firmware.NewModeSwitcher(ctx, h)
		if err != nil {
			s.Fatal("Failed to create mode switcher: ", err)
		}
		if err := ms.ModeAwareReboot(ctx, firmware.WarmReset); err != nil {
			s.Fatal("Failed to perform mode aware reboot: ", err)
		}

		s.Log("Verifying boot mode is developer")
		if err := verifyBootMode(); err != nil {
			s.Fatal("Failed boot mode check: ", err)
		}

		s.Log("Suspending DUT")
		cmd := h.DUT.Conn().CommandContext(ctx, "powerd_dbus_suspend", "--delay=5")
		if err := cmd.Start(); err != nil {
			s.Fatal("Failed to suspend DUT: ", err)
		}

		s.Log("Checking for S0ix or S3 powerstate")
		if err := h.WaitForPowerStates(ctx, 1*time.Second, 90*time.Second, "S0ix", "S3"); err != nil {
			s.Fatal("Failed to get S0ix or S3 powerstate: ", err)
		}

		s.Log("Sleeping for 5 seconds")
		// GoBigSleepLint: Wait in suspend for some time suspend.
		if err := testing.Sleep(ctx, 5*time.Second); err != nil {
			s.Fatal("Failed to sleep waiting for suspend: ", err)
		}

		s.Log("Pressing power key to wake device")
		if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.DurPress); err != nil {
			s.Fatal("Failed to press power key: ", err)
		}

		s.Log("Wait for DUT to connect")
		if err := h.WaitConnect(ctx); err != nil {
			s.Fatal("Failed to wait for device to connect: ", err)
		}

		s.Log("Verifying boot mode is developer")
		if err := verifyBootMode(); err != nil {
			s.Fatal("Failed boot mode check: ", err)
		}
	}
}
