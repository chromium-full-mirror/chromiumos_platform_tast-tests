// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"regexp"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/testing"
)

const (
	// Default number of GSC Reboots performed during test. Can be overridden through var.
	defaultRebootAttempts = 5
)

var (
	apROPasses *regexp.Regexp = regexp.MustCompile(`apro result \(20\) : pass`)
)

func init() {
	testing.AddTest(&testing.Test{
		Func: APROVerificationReboot,
		Desc: "Test DUT can pass AP RO verification on device with MP signed firmware",
		Contacts: []string{
			"chromeos-faft@google.com", // Owning team list
			"jettrink@chromium.org",    // Test author
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware", "firmware_mp"},
		Fixture:      fixture.DevModeWPEnabledZeroGBB,
		Timeout:      5 * time.Minute,
		Vars:         []string{"reboots", "block_unverified_ro"},
	})
}

func APROVerificationReboot(ctx context.Context, s *testing.State) {
	s.Log("Starting test AP RO verification test")
	h := s.FixtValue().(*fixture.Value).Helper

	bp, err := firmware.NewBypasser(ctx, h)
	if err != nil {
		s.Fatal("Failed to create a new bypasser")
	}

	// Use overridden reboots value if present
	rebootAttempts := defaultRebootAttempts
	if valueStr, ok := s.Var("reboots"); ok {
		value, err := strconv.ParseUint(valueStr, 10, 8)
		if err != nil {
			s.Fatal("Invalid 'reboots' var value: ", valueStr)
		}
		rebootAttempts = int(value)
	}

	// If block_unverified_ro var is true, then set AllowUnverifiedRo to Default(Never)
	if valueStr, ok := s.Var("block_unverified_ro"); ok {
		value, err := strconv.ParseBool(valueStr)
		if err != nil {
			s.Fatal("Invalid 'block_unverified_ro' var value: ", valueStr)
		}
		if value {
			// We need to open CCD, which should be instantaneous since we are in test lab mode, before
			// we can change the CCD setting.
			if err := h.Servo.OpenCCD(ctx); err != nil {
				s.Fatal("Failed to open ccd: ", err)
			}
			if err := h.Servo.RunCR50Command(ctx, "ccd set AllowUnverifiedRo Default"); err != nil {
				s.Fatal("Failed to disable AllowUnverifiedRo: ", err)
			}
			s.Log("Set AllowUnverifiedRo to Default! This will prevent AP boot")
			// Ensure that AllowUnverifiedRo is back to always to help recover lab devices
			defer func() {
				s.Log("Set AllowUnverifiedRo back to Always")
				if err := h.Servo.OpenCCD(ctx); err != nil {
					s.Fatal("Failed to open ccd: ", err)
				}
				if err := h.Servo.RunCR50Command(ctx, "ccd set AllowUnverifiedRo Always"); err != nil {
					s.Fatal("Failed to enable AllowUnverifiedRo: ", err)
				}
			}()
		}
	}

	// We need to enable HWWP at GSC boot since we are actually rebooting the GSC in this
	// test and we want HWWP to stay enabled. Note that if HWWP is ever disabled, and the
	// Test OS image reboots, it "helpfully" disables software write protect, which we do
	// not want for this test.
	s.Log("Setting HWWP enabled at GSC (re)boot")
	if err := h.Servo.RunCR50Command(ctx, "wp enable atboot"); err != nil {
		s.Fatal("Failed to enable write protect at boot: ", err)
	}

	s.Logf("Starting %d GSC reboot attempts ", rebootAttempts)
	for i := 0; i < rebootAttempts; i++ {
		s.Log("Reboot attempt ", i)

		// Reboot GSC with 1000ms delay, this should give enough time for ssh command to return without
		// having a timeout error. Still don't error in case it is marginal.
		rebootCtx, cancel := context.WithTimeout(ctx, time.Second*3)
		defer cancel()
		if err := h.DUT.Conn().CommandContext(rebootCtx, "gsctool", "-a", "--reboot", "1000").Run(); err != nil {
			s.Log("Failed to run gsctool reboot command: ", err)
		}

		// We should be polling UART console for keyboard ready before the above reboot finishes since
		// there is a 1000ms delay.
		s.Log("Waiting until keyboard is ready")
		if err := h.Servo.WaitFirmwareKeyboard(ctx, h.Config.FirmwareScreen); err != nil {
			s.Fatal("Failed to get to firmware screen: ", err)
		}

		// Since we are running MP signed AP FW, we need to be in dev mode to run a Test OS image.
		s.Log("Get passed dev mode screen")
		if err := bp.BypassDevMode(ctx); err != nil {
			s.Fatal("Failed to boot through dev mode: ", err)
		}

		rebootCtx, cancel = context.WithTimeout(ctx, h.Config.DelayRebootToPing)
		defer cancel()
		if err = h.WaitConnect(rebootCtx); err != nil {
			s.Fatal("Device did not come back up after gsctool reboot: ", err)
		}

		s.Log("Ensure that AP RO verification status is success")
		out, err := h.DUT.Conn().CommandContext(ctx, "gsctool", "-aB").Output()
		if err != nil {
			s.Fatal("Failed to run gsctool -aB command: ", err)
		}

		if apROPasses.Match(out) {
			s.Log("AP RO verification succeeded")
		} else {
			s.Errorf("AP RO verification attempt %d did not pass : %s", i, string(out))
		}
	}
}
