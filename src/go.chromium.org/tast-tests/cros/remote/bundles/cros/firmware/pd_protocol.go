// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"

	fwCommon "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: PDProtocol,
		Desc: "USB PD negotiation",
		Contacts: []string{
			"chromeos-faft@google.com", // Owning team list
		},
		BugComponent: "b:194910842", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Vars:         []string{"servo"},
		Fixture:      fixture.NormalMode,
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.SkipOnFormFactor(hwdep.Chromebox)),
		LacrosStatus: testing.LacrosVariantUnneeded,
		// TODO: When stable, move to firmware_pd
		Params: firmware.AddPDPorts([]testing.Param{{
			Val: firmware.PDTestParams{
			},
		}}, []string{"group:firmware", "firmware_pd_unstable"}),
	})
}

// PDProtocol USB PD protocol test
func PDProtocol(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	p := s.Param().(firmware.PDTestParams)

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}

	if err := h.Servo.SetFWWPState(ctx, servo.FWWPStateOn); err != nil {
		s.Fatal("Failed to turn on WP: ", err)
	}

	if err := firmware.SetupPDTester(ctx, h, p.CC, p.DTS, p.RequiredPort); err != nil {
		s.Fatal("Failed to configure Servo for PD testing: ", err)
	}

	// Turn off the USB mux, this will ensure that when we reboot into recovery
	// mode, we will stay at the recovery screen
	if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxOff); err != nil {
		s.Fatal("Failed to turn off USBMux: ", err)
	}

	// Turn on PD communication
	if err := h.Servo.SetPDCommunication(ctx, servo.On); err != nil {
		s.Fatal("Failed to turn off PD communication: ", err)
	}

	// Create a mode switcher and reboot to recover mode without waiting for a
	// connection (because we can't re-connect in recovery mode)
	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		s.Fatal("Failed to create new boot mode switcher: ", err)
	}

	if err := ms.RebootToMode(ctx, fwCommon.BootModeRecovery, firmware.SkipWaitConnect); err != nil {
		s.Fatal("Failed to reboot into recovery mode: ", err)
	}

	// Set the Servo as a sink
	if err := h.Servo.SetPDRole(ctx, servo.PDRoleSnk); err != nil {
		s.Fatal("Failed to set servoV4 to SNK: ", err)
	}

	// Poll the PD state, it should end up on PE_SRC_Disabled or
	// PD_STATE_SRC_DISABLED since we can't be a source in recovery mode.
	if err := checkPEStates(ctx, s, h, map[string]bool{
		"PD_STATE_SRC_DISABLED": true,
		"PE_SRC_Disabled":       true,
	}); err != nil {
		s.Fatal("Failed to verify power state: ", err)
	}

	// Set the Servo as a source
	if err := h.Servo.SetPDRole(ctx, servo.PDRoleSrc); err != nil {
		s.Fatal("Failed to set servoV4 to SNK: ", err)
	}

	// Poll the PD state, it should end up on PE_SNK_Ready or
	// PD_STATE_SNK_READY since we can be a sink in recovery mode.
	if err := checkPEStates(ctx, s, h, map[string]bool{
		"PD_STATE_SNK_READY": true,
		"PE_SNK_Ready":       true,
	}); err != nil {
		s.Fatal("Failed to verify power state: ", err)
	}
}

func checkPEStates(ctx context.Context, s *testing.State, h *firmware.Helper, expectedStates map[string]bool) error {
	// Poll for at least the duration it takes to get to the firmware screen
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		// Get the PD state for the port under test
		state, err := h.Servo.GetDUTPDState(ctx)
		if err != nil {
			return err
		}
		// Verify that we're in one of the expected states
		if _, ok := expectedStates[state.PEStateName]; !ok {
			return errors.Errorf("invalid PE state: %s", state.PEStateName)
		}
		return nil
	}, &testing.PollOptions{Timeout: h.Config.FirmwareScreen}); err != nil {
		// We hit the timeout
		return err
	}
	// We got to the right state in time
	return nil
}
