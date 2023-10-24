// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"time"

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
		// TODO: When stable, move to firmware_ec
		Attr:         []string{"group:firmware", "firmware_pd_unstable"},
		Vars:         []string{"servo"},
		Fixture:      fixture.NormalMode,
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.SkipOnFormFactor(hwdep.Chromebox)),
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

// PDProtocol USB PD protocol test
func PDProtocol(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}

	if err := h.Servo.SetFWWPState(ctx, servo.FWWPStateOn); err != nil {
		s.Fatal("Failed to turn on WP: ", err)
	}

	// Start the test with the servo as a source
	if err := h.Servo.SetPDRole(ctx, servo.PDRoleSrc); err != nil {
		s.Fatal("Failed to set servoV4 to SNK: ", err)
	}

	// Wait for PD negotiation to complete
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return h.Servo.RequirePDTester(ctx)
	}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
		s.Fatal("Failed to wait PD negotiation: ", err)
	}

	// Turn off the USB mux, this will ensure that when we reboot into recovery
	// mode, we will stay at the recovery screen
	if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxOff); err != nil {
		s.Fatal("Failed to turn off USBMux: ", err)
	}

	// Require a single PD port
	err := h.Servo.RequireDUTPDInfo(ctx)
	if err != nil {
		s.Fatal("Error in getting PD port info: ", err)
	}

	// Set the Servo as a sink
	if err := h.Servo.SetPDRole(ctx, servo.PDRoleSnk); err != nil {
		s.Fatal("Failed to set servoV4 to SNK: ", err)
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

	// Sleep for 30 seconds to wait for recovery mode
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		// Get the PD state for the port under test
		state, err := h.Servo.GetDUTPDState(ctx)
		if err != nil {
			return err
		}
		// Verify that we're not in either source or sink ready states
		var activePEStates = map[string]bool{
			"PD_STATE_SNK_READY": true,
			"PD_STATE_SRC_READY": true,
			"PE_SNK_Ready":       true,
			"PE_SRC_Ready":       true,
		}
		if _, ok := activePEStates[state.PEStateName]; ok {
			return errors.Errorf("invalid PE state: %s", state.PEStateName)
		}
		return nil
	}, &testing.PollOptions{Timeout: h.Config.FirmwareScreen}); err != nil {
		s.Fatal("Failed to verify PE state: ", err)
	}
}
