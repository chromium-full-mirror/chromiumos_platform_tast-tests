// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"

	fwCommon "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/tbdep"
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
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Vars:         []string{"servo"},
		// TODO(b/427195218): Add servo-exists + servo_state:WORKING after bug resolved.
		TestBedDeps:  []string{tbdep.ServoPresent},
		Fixture:      fixture.NormalMode,
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.SkipOnFormFactor(hwdep.Chromebox)),
		Attr:         []string{"group:firmware", "firmware_pd", "firmware_meets_kpi", "firmware_stressed", "firmware_ec_ro"},
		Params: []testing.Param{{
			Val: firmware.PDTestParams{
				DTS: firmware.DTSModeOff,
			},
		}},
	})
}

// PDProtocol USB PD protocol test
func PDProtocol(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	p := s.Param().(firmware.PDTestParams)

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}

	if err := h.Servo.SetFWWPState(ctx, servo.FWWPStateForceOn); err != nil {
		s.Fatal("Failed to turn on WP: ", err)
	}

	if err := firmware.SetupPDTester(ctx, h, p, s.OutDir()); err != nil {
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

	defer func() {
		if err := h.Servo.SetPowerState(ctx, servo.PowerStateWarmReset); err != nil {
			s.Fatal("Failed to reset DUT: ", err)
		}
		if err := h.WaitConnect(ctx); err != nil {
			s.Fatal("Failed to establish connection after resetting DUT: ", err)
		}
	}()

	// Set the Servo as a sink
	if err := h.Servo.SetPDRole(ctx, servo.PDRoleSnk); err != nil {
		s.Fatal("Failed to set servoV4 to SNK: ", err)
	}

	// GoBigSleepLint: We need to check that we don't reach PE_SRC_Ready or
	// PD_STATE_SRC_READY. Instead of polling (since we don't care about the
	// states in between) we just sleep for the 'FirmwareScreen' duration which
	// is the time required to get to the firmware screen.
	if err := testing.Sleep(ctx, h.Config.FirmwareScreen); err != nil {
		s.Fatal("Failed to sleep during firmware screen: ", err)
	}

	// Check the PD state; it should NOT be source-ready since we cannot be a
	// source in the recovery screen
	state, err := h.Servo.GetDUTPDState(ctx)
	if err != nil {
		s.Fatal("Failed to check current PD state: ", err)
	}
	if state.IsSourceReady() {
		s.Fatalf("Power state must not be source-ready. Got state %q", state.GetStateName())
	}

	// Set the Servo as a source, renegotiation should start and the PE
	// state should end up as 'ready'
	if err := h.Servo.SetPDRole(ctx, servo.PDRoleSrc); err != nil {
		s.Fatal("Failed to set servoV4 to source: ", err)
	}

	// Poll the DUT PD state, it should end up sink-ready since we can be a sink
	// in recovery mode. We can poll here, because as soon as the state reaches
	// 'ready', we can call the test a success.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		// Get the PD state for the port under test
		state, err := h.Servo.GetDUTPDState(ctx)
		if err != nil {
			return err
		}
		// Keep polling until we are sink-ready
		if !state.IsSinkReady() {
			return errors.Errorf("PD state is not sink-ready: %q", state.GetStateName())
		}
		return nil
	}, &testing.PollOptions{Timeout: h.Config.FirmwareScreen}); err != nil {
		// We hit the timeout
		s.Fatal("Failed to verify power state: ", err)
	}
}
