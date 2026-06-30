// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"time"

	fwCommon "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: RecToDevUntrusted,
		Desc: "Verify untrusted keys from an attached keyboard have no effect on the firmware screen",
		Contacts: []string{
			"chromeos-firmware@google.com"},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		TestBedDeps:  tbdep.ServoPresentAndWorking,
		Attr:         []string{"group:firmware", "firmware_bios", "firmware_enabled", "firmware_meets_kpi", "firmware_stressed", "firmware_bios_ro", "firmware_bios_rw"},
		Fixture:      fixture.NormalMode,
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.SkipOnFormFactor(hwdep.Chromebox, hwdep.Chromebase)),
		Timeout:      20 * time.Minute,
	})
}

func RecToDevUntrusted(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to get config: ", err)
	}

	// This test is not applicable for devices with h.Config.RecButtonDevSwitch
	// or h.Config.PowerButtonDevSwitch because there is no "Confirm" menu item
	// for them.
	if h.Config.RecButtonDevSwitch || h.Config.PowerButtonDevSwitch {
		s.Fatal("Unexpectedly found one of the RecButtonDevSwitch and PowerButtonDevSwitch devices")
	}

	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		s.Fatal("Creating mode switcher: ", err)
	}

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}
	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}

	if err := h.Reporter.ClearEventlog(ctx); err != nil {
		s.Fatal("Failed to clear event log: ", err)
	}

	if err := ms.EnableRecMode(ctx, servo.PowerStateRec, servo.USBMuxOff); err != nil {
		s.Fatal("Failed to enable recovery mode: ", err)
	}

	if h.HasAPFwState {
		closeUART, err := h.Servo.EnableUARTCapture(ctx, servo.ECUARTCapture)
		if err != nil {
			s.Fatal("Failed to enable capture EC UART: ", err)
		}
		defer func() {
			if err := closeUART(ctx); err != nil {
				s.Error("Failed to cancel capture EC UART: ", err)
			}
		}()
	}

	if err := reachToDevScreen(ctx, h); err != nil {
		s.Fatal("Failed to reach the TO_DEV screen: ", err)
	}

	if err := enterDevModeUntrusted(ctx, h); err != nil {
		s.Fatal("Failed to test untrusted entry to dev mode: ", err)
	}

	s.Log("Rebooting the DUT with a cold reset")
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateReset); err != nil {
		s.Fatal("Failed to cold reset the DUT: ", err)
	}

	s.Log("Waiting for DUT to reconnect")
	waitConnectOps := []firmware.WaitConnectOption{firmware.ResetEthernetDongle}
	waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
	defer cancelWaitConnect()
	if err := h.WaitConnect(waitConnectCtx, waitConnectOps...); err != nil {
		// Log the current power state when reset fails.
		if h.Config.ChromeEC {
			currPowerState, powerStateErr := h.Servo.GetECSystemPowerState(ctx)
			if powerStateErr != nil {
				s.Error("Failed to check powerstate: ", powerStateErr)
			}
			s.Fatalf("Failed to reconnect to DUT: %v, the current power state is %q", err, currPowerState)
		}
		s.Fatal("Failed to reconnect to DUT: ", err)
	}

	s.Log("Verifying DUT in normal mode")
	currentMode, err := h.Reporter.CurrentBootMode(ctx)
	if err != nil {
		s.Fatal("Failed to get current boot mode: ", err)
	}
	if currentMode != fwCommon.BootModeNormal {
		s.Fatalf("Expecting DUT in normal mode, but got: %s", currentMode)
	}

	s.Log("Checking event log to verify the previous boot was recovery mode")
	foundEvents, err := h.Reporter.EventlogList(ctx)
	if err != nil {
		s.Fatal("Failed to find events: ", err)
	}
	expectedBootModes := []reporters.EventlogBootMode{reporters.ManualRecovery, reporters.NormalMode}
	if err := h.Reporter.CheckBootModes(ctx, foundEvents, expectedBootModes); err != nil {
		s.Fatal("Failed to check for the expected boot modes: ", err)
	}
}

func reachToDevScreen(ctx context.Context, h *firmware.Helper) (retErr error) {
	switch h.Config.ModeSwitcherType {
	case firmware.TabletDetachableSwitcher:
		retErr = h.Servo.SetInt(ctx, servo.VolumeUpDownHold, 100)
	case firmware.MenuSwitcher, firmware.KeyboardDevSwitcher:
		retErr = h.Servo.KeypressWithDuration(ctx, servo.CtrlD, servo.DurTab)
	default:
		retErr = errors.Errorf("unable to determine presses to get to the TO_DEV screen, got mode switcher type: %s", h.Config.ModeSwitcherType)
	}

	if h.HasAPFwState {
		testing.ContextLog(ctx, "Waiting for DUT to reach the TO_DEV screen")
		if err := h.DetectFirmwareScreen(ctx, h.Config.FirmwareScreen, fwCommon.RecoveryToDev); err != nil {
			retErr = errors.Join(retErr, err)
		}
	}

	return retErr
}

func enterDevModeUntrusted(ctx context.Context, h *firmware.Helper) error {
	// For LMUI, USB keys <uparrow> and <downarrow> are untrusted.
	if h.Config.ModeSwitcherType == firmware.TabletDetachableSwitcher {
		if err := h.Servo.PressUSBKey(ctx, "<uparrow>", servo.DurTab); err != nil {
			return err
		}
		// GoBigSleepLint: Simulate a specific speed of key press.
		if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
			return errors.Wrapf(err, "failed to sleep for %s", h.Config.KeypressDelay)
		}
	}
	// For MUI and LCUI, USB key "<enter>" is untrusted.
	if err := h.Servo.PressUSBKey(ctx, "<enter>", servo.DurTab); err != nil {
		return err
	}

	if h.HasAPFwState {
		// The window should pop up, and the screen ID should remain RecoveryToDev.
		testing.ContextLog(ctx, "Detecting the RecoveryToDev screen")
		if err := h.DetectFirmwareScreen(ctx, h.Config.FirmwareScreen, fwCommon.RecoveryToDev); err != nil {
			return errors.Wrap(err, "failed to detect RecoveryToDev screen")
		}
	}

	testing.ContextLog(ctx, "Verifying DUT doesn't boot because of the untrusted inputs")
	waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, firmware.DevScreenTimeout+h.Config.DelayRebootToPing)
	defer cancelWaitConnect()

	err := h.WaitConnect(waitConnectCtx)
	switch err.(type) {
	case nil:
		return errors.New("found DUT awake unexpectedly")
	default:
		if !errors.As(err, &context.DeadlineExceeded) {
			return errors.Wrap(err, "found unexpected error")
		}
	}
	return nil
}
