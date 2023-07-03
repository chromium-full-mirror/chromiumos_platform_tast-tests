// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"regexp"
	"strings"
	"time"

	fwCommon "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
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
		Params: []testing.Param{{
			Name:    "verify_recovery_screen",
			Val:     true,
			Timeout: 40 * time.Minute,
		}, {
			Val:     false,
			Timeout: 30 * time.Minute,
		}},
	})
}

func WilcoTPMKeyLadderVerification(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	checkRecoveryScreenWorks := s.Param().(bool)
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
	type testCase int
	const (
		startNormalWithAC testCase = iota
		startNormalWithoutAC
		startNormalWithACReboot
		startDevWithAC
		startDevWithoutAC
	)
	type testSteps struct {
		startMode            fwCommon.BootMode
		connectAC            bool
		setupDUT             func(ctx context.Context, h *firmware.Helper) error
		expectDUTReconnected bool
		tpmMode              string
		keyLadder            string
	}
	runRecoveryScreenToDiagnostics := map[testCase]testSteps{
		startNormalWithAC: testSteps{
			startMode:            fwCommon.BootModeNormal,
			connectAC:            true,
			setupDUT:             enterDiagMode,
			expectDUTReconnected: false,
			tpmMode:              disabledTPMMode,
			keyLadder:            disabledKeyLadder,
		},
		startNormalWithACReboot: testSteps{
			startMode:            fwCommon.BootModeNormal,
			connectAC:            true,
			setupDUT:             rebootWithColdReset,
			expectDUTReconnected: true,
			tpmMode:              enabledTPMMode,
			keyLadder:            enabledKeyLadder,
		},
		startNormalWithoutAC: testSteps{
			startMode:            fwCommon.BootModeNormal,
			connectAC:            false,
			setupDUT:             enterDiagMode,
			expectDUTReconnected: false,
			tpmMode:              disabledTPMMode,
			keyLadder:            disabledKeyLadder,
		},
		startDevWithAC: testSteps{
			startMode:            fwCommon.BootModeDev,
			connectAC:            true,
			setupDUT:             enterDiagMode,
			expectDUTReconnected: false,
			tpmMode:              disabledTPMMode,
			keyLadder:            disabledKeyLadder,
		},
		startDevWithoutAC: testSteps{
			startMode:            fwCommon.BootModeDev,
			connectAC:            false,
			setupDUT:             enterDiagMode,
			expectDUTReconnected: false,
			tpmMode:              disabledTPMMode,
			keyLadder:            disabledKeyLadder,
		},
	}
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}
	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		s.Fatal("Creating mode switcher: ", err)
	}
	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to get config: ", err)
	}

	s.Log("Connecting charger")
	if err := h.SetDUTPower(ctx, true); err != nil {
		s.Fatal("Unable to connect charger: ", err)
	}
	if err := h.CheckChgFrmPwrSuppInfo(ctx, true); err != nil {
		s.Fatal("Failed to check charger: ", err)
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

	var runCases []testSteps
	if checkRecoveryScreenWorks {
		runCases = []testSteps{
			runRecoveryScreenToDiagnostics[startNormalWithAC],
			runRecoveryScreenToDiagnostics[startNormalWithoutAC],
			runRecoveryScreenToDiagnostics[startDevWithAC],
			runRecoveryScreenToDiagnostics[startDevWithoutAC],
		}
	} else {
		runCases = []testSteps{
			runRecoveryScreenToDiagnostics[startNormalWithAC],
			runRecoveryScreenToDiagnostics[startNormalWithACReboot],
		}
	}
	type dutStates struct {
		dutConnected  bool
		acIsConnected bool
		bootMode      fwCommon.BootMode
	}
	currentState := dutStates{dutConnected: true, acIsConnected: true, bootMode: fwCommon.BootModeNormal}
	defer func() {
		if !currentState.dutConnected {
			if err := rebootWithColdReset(ctx, h); err != nil {
				s.Fatal("Failed to cold reset DUT: ", err)
			}
		}
		s.Log("Connecting charger")
		if err := h.SetDUTPower(ctx, true); err != nil {
			s.Fatal("Unable to connect charger: ", err)
		}
		if err := h.CheckChgFrmPwrSuppInfo(ctx, true); err != nil {
			s.Fatal("Failed to check charger: ", err)
		}
	}()
	for _, step := range runCases {
		// Reboot DUT to ensure that it is awake, prior to verifying
		// boot mode and AC status.
		if (step.connectAC != currentState.acIsConnected ||
			step.startMode != currentState.bootMode) && !currentState.dutConnected {
			if err := rebootWithColdReset(ctx, h); err != nil {
				s.Fatal("Failed to cold reset DUT: ", err)
			}
		}
		if step.connectAC != currentState.acIsConnected {
			s.Logf("Setting charger connect: %t", step.connectAC)
			if err := h.SetDUTPower(ctx, step.connectAC); err != nil {
				s.Fatalf("Failed to set charger to %t", step.connectAC)
			}
			if err := h.CheckChgFrmPwrSuppInfo(ctx, step.connectAC); err != nil {
				s.Fatal("Failed to check charger: ", err)
			}
		}
		currentState.acIsConnected = step.connectAC
		if step.startMode != currentState.bootMode {
			if err = ms.RebootToMode(ctx, step.startMode); err != nil {
				s.Fatalf("Failed to set up %s mode: %v", step.startMode, err)
			}
		}
		currentState.bootMode = step.startMode

		if err := step.setupDUT(ctx, h); err != nil {
			s.Fatal("While setting up DUT: ", err)
		}
		waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
		defer cancelWaitConnect()
		err := h.WaitConnect(waitConnectCtx)
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
		currentState.dutConnected = step.expectDUTReconnected

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
	// As mentioned in ticket b/200305070, comment #5, when charger is disconnected,
	// booting Wilco machines with servo.PowerStateRec only works if the machine starts
	// from the power-off state.
	testing.ContextLogf(ctx, "Pressing power button for %s to put DUT in deep sleep", h.Config.HoldPwrButtonPowerOff)
	if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.Dur(h.Config.HoldPwrButtonPowerOff)); err != nil {
		return errors.Wrap(err, "failed to hold power button")
	}
	testing.ContextLog(ctx, "Waiting for DUT to power OFF")
	waitUnreachableCtx, cancelUnreachable := context.WithTimeout(ctx, h.Config.ShutdownTimeout)
	defer cancelUnreachable()

	if err := h.DUT.WaitUnreachable(waitUnreachableCtx); err != nil {
		return errors.Wrap(err, "dut did not power down")
	}
	testing.ContextLog(ctx, "Rebooting the DUT to recovery screen")
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateRec); err != nil {
		return errors.Wrap(err, "failed to boot to recovery screen")
	}
	testing.ContextLogf(ctx, "Sleeping for %s (FirmwareScreen)", h.Config.FirmwareScreen)
	// GoBigSleepLint: Sleeping for model specific time.
	if err := testing.Sleep(ctx, h.Config.FirmwareScreen); err != nil {
		return errors.Wrap(err, "failed to sleep")
	}
	// Following servo.PowerStateRec, verify that dut boots to a firmware screen,
	// which should presumably be the recovery screen.
	apPower, screen, err := h.Servo.GetAPState(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to check the AP state")
	}
	if apPower != "on" || screen != "F" {
		return errors.Errorf("failed to boot to recovery screen. Found AP: %s%s", apPower, screen)
	}
	// Read the UART stream just to make sure there isn't buffered data.
	if _, err := h.Servo.GetQuotedString(ctx, servo.CR50UARTStream); err != nil {
		return errors.Wrap(err, "failed to read GSC UART")
	}
	testing.ContextLog(ctx, "Pressing F12")
	if err := h.Servo.PressKey(ctx, "<f12>", servo.DurTab); err != nil {
		return errors.Wrap(err, "failed to press the f12 key")
	}
	testing.ContextLog(ctx, "Sleeping for 15 seconds till dut reaches confirmation page")
	// GoBigSleepLint: Sleeping for a specific delay to wait for the confirmation page.
	if err := testing.Sleep(ctx, 15*time.Second); err != nil {
		return errors.Wrap(err, "failed to sleep")
	}
	testing.ContextLog(ctx, "Pressing power button to enter diagnostics mode")
	if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.DurTab); err != nil {
		return errors.Wrap(err, "failed to press power key")
	}
	testing.ContextLog(ctx, "Waiting for DUT to enter diagnostics mode")
	// GoBigSleepLint: Sleeping for a specific delay to ensure entry to the diagnostics mode.
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

func rebootWithColdReset(ctx context.Context, h *firmware.Helper) error {
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateReset); err != nil {
		return err
	}
	waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
	defer cancelWaitConnect()
	if err := h.WaitConnect(waitConnectCtx); err != nil {
		return errors.Wrap(err, "failed to reconnect to DUT")
	}
	return nil
}
