// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

// cmd constants for RunAPCommand - these are all U-Boot commands.
const (
	// Using with no additional arguments returns current U-Boot version
	cmdGetVersion string = "version"
	// Reset the board
	cmdReset string = "reset"
)

// Pattern expression for RunCommandGetOutput.
const (
	reVersion string = `U-Boot.*\)`
)

func init() {
	testing.AddTest(&testing.Test{
		Func: AltfwMode, LacrosStatus: testing.LacrosVariantUnneeded,
		Desc: "Test DUT can get into U-Boot using altfw mode",
		Contacts: []string{
			"chromeos-faft@google.com", // Owning team list
			"sjg@chromium.org",         // Test author
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		// TODO: When stable, move to firmware_bios.
		Attr:         []string{"group:firmware", "firmware_unstable"},
		Fixture:      fixture.DevMode,
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.AlternativeFirmware()),
		Timeout:      4 * time.Minute,
	})
}

// getVersion checks the U-Boot version.
func getVersion(ctx context.Context, h *firmware.Helper) (string, error) {
	out, err := h.Servo.RunAPCommandGetOutput(ctx, cmdGetVersion, []string{reVersion})
	if out == nil {
		testing.ContextLog(ctx, "No version found")
		return "", errors.Wrap(err, "no version found")
	}
	testing.ContextLog(ctx, "Got: ", out[0][0])

	return out[0][0], nil
}

// reset resets the board when in U-Boot.
func reset(ctx context.Context, h *firmware.Helper) error {
	err := h.Servo.SetString(ctx, servo.APUARTCmd, cmdReset)
	if err != nil {
		return errors.Wrapf(err, "setting APUARTCmd to %s", cmdReset)
	}

	return nil
}

func AltfwMode(ctx context.Context, s *testing.State) {
	s.Log("Starting test of alffw mode")
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}

	newbp, err := firmware.NewBypasser(ctx, h)
	if err != nil {
		s.Fatal("Failed to create a new bypasser")
	}

	s.Log("Enable dev_boot_altfw")
	if err := h.DUT.Conn().CommandContext(ctx, "crossystem", "dev_boot_altfw=1").Run(); err != nil {
		s.Fatal("Failed to set dev_boot_altfw in crossystem: ", err)
	}
	if err := h.DUT.Conn().CommandContext(ctx, "crossystem", "dev_default_boot=altfw").Run(); err != nil {
		s.Fatal("Failed to set dev_default_boot in crossystem: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 1*time.Minute)
	defer cancel()

	// Restore dev_boot_altfw and dev_default_boot at the end of the test.
	defer func(ctx context.Context) {
		s.Log("Disable dev_boot_altfw")
		if err := h.EnsureDUTBooted(ctx); err != nil {
			s.Fatal("Failed to reconnect to dut: ", err)
		}
		if err := h.DUT.Conn().CommandContext(ctx, "crossystem", "dev_boot_altfw=0").Run(); err != nil {
			s.Fatal("Failed to set dev_boot_altfw in crossystem: ", err)
		}
		if err := h.DUT.Conn().CommandContext(ctx, "crossystem", "dev_default_boot=disk").Run(); err != nil {
			s.Fatal("Failed to set dev_default_boot in crossystem: ", err)
		}
	}(cleanupCtx)

	if err := h.Servo.SetOnOff(ctx, servo.APUARTCapture, servo.On); err != nil {
		s.Fatal("Failed to capture AP UART: ", err)
	}
	defer func() {
		if err := h.Servo.SetOnOff(ctx, servo.APUARTCapture, servo.Off); err != nil {
			s.Fatal("Failed to disable capture AP UART: ", err)
		}
	}()

	s.Log("Drain UART")
	if _, err := h.Servo.GetQuotedString(ctx, servo.APUARTStream); err != nil {
		s.Fatal("Failed to drain UART: ", err)
	}

	s.Log("Reboot")
	rebootCtx, cancel := context.WithTimeout(ctx, time.Second*3)
	defer cancel()
	if err := h.DUT.Conn().CommandContext(rebootCtx, "reboot").Run(); err != nil {
		s.Log("Failed to run reboot command: ", err)
	}

	s.Log("Waiting for DUT to reach the firmware screen")
	if err := h.WaitFirmwareScreen(ctx); err != nil {
		s.Fatal("Failed to get to firmware screen: ", err)
	}

	s.Log("Jump into altfw")
	if err := newbp.BypassAltfwMode(ctx); err != nil {
		s.Fatal("Failed to boot through altfw mode: ", err)
	}

	s.Log("Look for AP uart")
	ubootRe := regexp.MustCompile(`U-Boot 20`)
	if err := h.Servo.PollForRegexp(ctx, servo.APUARTStream, ubootRe, 60*time.Second); err != nil {
		s.Fatal(errors.Wrap(err, "failed to find U-Boot prompt"))
	}

	s.Log("Stop U-Boot autoboot")
	if err := h.Servo.KeypressWithDuration(ctx, servo.Enter, servo.DurShortPress); err != nil {
		s.Fatal("Failed to stop autoboot: ", err)
	}

	s.Log("Drain UART")
	if _, err := h.Servo.GetQuotedString(ctx, servo.APUARTStream); err != nil {
		s.Fatal("Failed to drain UART: ", err)
	}

	s.Log("Check U-Boot version")
	version, _ := getVersion(ctx, h)
	s.Log("Got version: ", version)

	s.Log("Reset and boot")
	if err := reset(ctx, h); err != nil {
		s.Fatal("Failed to reset: ", err)
	}

	s.Log("Waiting for DUT to reach the firmware screen")
	if err := h.WaitFirmwareScreen(ctx); err != nil {
		s.Fatal("Failed to get to firmware screen: ", err)
	}

	if err := newbp.BypassDevMode(ctx); err != nil {
		s.Fatal("Failed to boot through dev mode: ", err)
	}
}
