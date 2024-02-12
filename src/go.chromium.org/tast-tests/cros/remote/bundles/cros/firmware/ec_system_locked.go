// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ECSystemLocked,
		Desc: "This test case verifies that changing the FW write protection state has expected effect in EC sysinfo",
		Contacts: []string{
			"chromeos-faft@google.com",
			"jbettis@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware", "firmware_ec"},
		Fixture:      fixture.DevMode,
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Requirements: []string{"sys-fw-0022-v02"},
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

func ECSystemLocked(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servod")
	}

	state, err := h.Servo.GetString(ctx, servo.FWWPState)
	if err != nil {
		s.Fatal("Failed to get initial write protect state: ", err)
	}
	initialState, err := verifyFWWriteProtectState(state)
	if err != nil {
		s.Fatal("Failed to recognize initial FW WP state: ", err)
	}
	defer func() {
		s.Log("Restore the original FW write protect state")
		setFWWriteProtectStateAndReboot(ctx, h, s, initialState)
	}()
	s.Log("FW initial write protect state: ", state)
	if err = verifySysinfoLocked(ctx, h, initialState); err != nil {
		s.Error("EC lockstate wrong: ", err)
	}

	setFWWriteProtectStateAndReboot(ctx, h, s, !initialState)
	if err = verifySysinfoLocked(ctx, h, !initialState); err != nil {
		s.Error("EC lockstate wrong: ", err)
	}
}

func setFWWriteProtectStateAndReboot(ctx context.Context, h *firmware.Helper, s *testing.State, newFWWriteProtectState bool) {
	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		s.Fatal("Creating mode switcher: ", err)
	}

	if newFWWriteProtectState {
		// enable SW WP before hardware WP
		if err := h.Servo.RunECCommand(ctx, "flashwp enable"); err != nil {
			s.Fatal("Failed to enable flashwp: ", err)
		}
		if err := h.Servo.SetFWWPState(ctx, servo.FWWPStateOn); err != nil {
			s.Fatal("Failed to disable firmware write protect: ", err)
		}
		s.Log("Rebooting the EC")
		if err := h.Servo.RunECCommand(ctx, "reboot hard"); err != nil {
			s.Fatal("Failed to reboot ec: ", err)
		}
	} else {
		if err := h.Servo.SetFWWPState(ctx, servo.FWWPStateOff); err != nil {
			s.Fatal("Failed to disable firmware write protect: ", err)
		}
		s.Log("Rebooting the DUT")
		// Reboot after deasserting hardware write protect pin to deactivate
		// write protect. And then remove software write protect flag.
		// Some ITE ECs can only clear their WP status on a power-on reset,
		// no software-initiated reset will do.
		if err := ms.ModeAwareReboot(ctx, firmware.ColdReset); err != nil {
			s.Fatal("Failed to perform mode aware reboot: ", err)
		}
		// disable SW WP after hardware WP
		if err := h.Servo.RunECCommand(ctx, "flashwp disable"); err != nil {
			s.Fatal("Failed to disable flashwp: ", err)
		}
	}
	waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, 2*time.Minute)
	defer cancelWaitConnect()
	if err := h.WaitConnect(waitConnectCtx); err != nil {
		s.Fatal("Failed to reconnect to the DUT: ", err)
	}
	state, err := h.Servo.GetString(ctx, servo.FWWPState)
	if err != nil {
		s.Fatal("Failed to get write protect state: ", err)
	}
	stateAfterReboot, err := verifyFWWriteProtectState(state)
	if err != nil {
		s.Fatal("Failed to recognize FW WP state after reboot: ", err)
	}
	if stateAfterReboot != newFWWriteProtectState {
		s.Fatalf("FW WP state after reboot got %q, want %v", state, newFWWriteProtectState)
	}
	s.Log("FW write protect state has been successfully set to ", state)
}

func verifyFWWriteProtectState(state string) (bool, error) {
	switch servo.FWWPStateValue(state) {
	case servo.FWWPStateOn:
		return true, nil
	case servo.FWWPStateOff:
		return false, nil
	default:
		return false, errors.New("invalid FW WP state: " + state)
	}
}

func verifySysinfoLocked(ctx context.Context, h *firmware.Helper, expectLocked bool) error {
	out, err := h.Servo.RunECCommandGetOutput(ctx, "sysinfo", []string{`Flags:\s+(locked|unlocked)[^\n]*\n`})
	if err != nil {
		return errors.Wrap(err, "sysinfo failed")
	}
	if expectLocked && out[0][1] != "locked" {
		return errors.Errorf("sysinfo reported wrong flags, got %v want %v", out[0][1], "locked")
	}
	if !expectLocked && out[0][1] != "unlocked" {
		return errors.Errorf("sysinfo reported wrong flags, got %v want %v", out[0][1], "unlocked")
	}
	return nil
}
