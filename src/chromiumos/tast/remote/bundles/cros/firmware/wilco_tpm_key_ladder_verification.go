// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"regexp"
	"strings"
	"time"

	"chromiumos/tast/common/servo"
	"chromiumos/tast/remote/firmware"
	"chromiumos/tast/remote/firmware/fixture"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         WilcoTPMKeyLadderVerification,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify TPM key ladder when device boots into customer diagnostic mode",
		Contacts: []string{
			"chromeos-faft@google.com",
			"cienet-firmware@cienet.corp-partner.google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware", "firmware_unstable"},
		SoftwareDeps: []string{"wilco"},
		Fixture:      fixture.NormalMode,
		Timeout:      20 * time.Minute,
	})
}

func WilcoTPMKeyLadderVerification(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	d := s.DUT()
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}
	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to get config: ", err)
	}

	// Disconnect USB to ensure that recovery screen would be reached.
	s.Log("Powering off the USB")
	if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxOff); err != nil {
		s.Fatal("Failed to set usb mux state to off: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 60*time.Second)
	defer cancel()
	s.Log("Capturing GSC log")
	if err := h.Servo.SetOnOff(ctx, servo.CR50UARTCapture, servo.On); err != nil {
		s.Fatal("Failed to capture GSC UART: ", err)
	}
	defer func(ctx context.Context) {
		if err := h.Servo.SetOnOff(ctx, servo.CR50UARTCapture, servo.Off); err != nil {
			s.Fatal("Failed to disable capture GSC UART: ", err)
		}
	}(cleanupCtx)
	// Read the UART stream just to make sure there isn't buffered data.
	if _, err := h.Servo.GetQuotedString(ctx, servo.CR50UARTStream); err != nil {
		s.Fatal("Failed to read GSC UART: ", err)
	}

	/*
		Referencing the 'board/cr50/board.c' and 'include/tpm_registers.h' files from
		branch cr50_stab, TPM enabled/disabled states are defined as follows:
		TPM_MODE_ENABLED_TENTATIVE = 0, which is the default state when dut boots up.
		TPM_MODE_ENABLED = 1, if tpm mode was set manually.
		TPM_MODE_DISABLED = 2, meaning tpm mode disabled.
		The key ladder states are defined as: "prod", "dev", and "disabled".
		For this test's purposes, we're considering the status declared below.
	*/
	var (
		enabledTPMMode    = "enabled (0)"
		disabledTPMMode   = "disabled (2)"
		enabledKeyLadder  = "prod"
		disabledKeyLadder = "disabled"
	)

	for _, step := range []struct {
		powerCycleDUT        func(ctx context.Context, h *firmware.Helper) error
		expectDUTReconnected bool
		tpmMode              string
		keyLadder            string
	}{
		{
			enterDiagMode,
			false,
			disabledTPMMode,
			disabledKeyLadder,
		},
		{
			func(ctx context.Context, h *firmware.Helper) error {
				return h.Servo.SetPowerState(ctx, servo.PowerStateReset)
			},
			true,
			enabledTPMMode,
			enabledKeyLadder,
		},
	} {
		if err := step.powerCycleDUT(ctx, h); err != nil {
			s.Fatal("While power cycling the DUT: ", err)
		}
		waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, 3*time.Minute)
		defer cancelWaitConnect()
		err := d.WaitConnect(waitConnectCtx)
		switch err.(type) {
		case nil:
			if !step.expectDUTReconnected {
				s.Fatal("Found DUT connected unexpectedly")
			}
		default:
			if step.expectDUTReconnected ||
				(!step.expectDUTReconnected && !strings.Contains(err.Error(), context.DeadlineExceeded.Error())) {
				s.Fatal("Unexpected error occurred: ", err)
			}
		}

		s.Log("Verifying TPM and Key Ladder states")
		output, err := h.Servo.RunCR50CommandGetOutput(ctx, "sysinfo", []string{
			`TPM\s+MODE:\s+(enabled \(\d\)|disabled \(\d\))\s*`,
			`Key\s+Ladder:\s+(prod|dev|disabled)\s*`})
		if err != nil {
			s.Fatal("Failed to run cr50 console sysinfo command: ", err)
		}
		if step.tpmMode != output[0][1] {
			s.Fatalf("Incorrect value, got %s from %s, but wanted %s",
				output[0][1], output[0][0], step.tpmMode)
		}
		if step.keyLadder != output[1][1] {
			s.Fatalf("Incorrect value, got %s from %s, but wanted %s",
				output[1][1], output[1][0], step.keyLadder)
		}
	}
}

func enterDiagMode(ctx context.Context, h *firmware.Helper) error {
	testing.ContextLog(ctx, "Rebooting the DUT to recovery screen")
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateRec); err != nil {
		return errors.Wrap(err, "failed to boot to recovery screen")
	}
	testing.ContextLogf(ctx, "Sleeping for %s (FirmwareScreen)", h.Config.FirmwareScreen)
	if err := testing.Sleep(ctx, h.Config.FirmwareScreen); err != nil {
		return errors.Wrap(err, "failed to sleep")
	}
	testing.ContextLog(ctx, "Pressing F12")
	if err := h.Servo.PressKey(ctx, "<f12>", servo.DurTab); err != nil {
		return errors.Wrap(err, "failed to press the f12 key")
	}
	testing.ContextLog(ctx, "Sleeping for 15 seconds till dut reaches confirmation page")
	if err := testing.Sleep(ctx, 15*time.Second); err != nil {
		return errors.Wrap(err, "failed to sleep")
	}
	testing.ContextLog(ctx, "Pressing power button to enter diagnostics mode")
	if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.DurTab); err != nil {
		return errors.Wrap(err, "failed to press power key")
	}
	testing.ContextLog(ctx, "Waiting for DUT to enter diagnostics mode")
	if err := testing.Sleep(ctx, 15*time.Second); err != nil {
		return errors.Wrap(err, "failed to sleep")
	}
	testing.ContextLog(ctx, "Scanning GSC log to verify entry to diagnostics mode")
	out, err := h.Servo.GetQuotedString(ctx, servo.CR50UARTStream)
	if err != nil {
		return errors.Wrap(err, "failed to read GSC Uart")
	}
	diagModeRe := regexp.MustCompile(`enable diagnostic mode`)
	diagModeMatch := diagModeRe.FindStringSubmatch(out)
	if diagModeMatch == nil {
		return errors.Wrap(err, "did not find match in GSC log about enabling diagnostics mode")
	}
	return nil
}
