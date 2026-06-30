// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"time"

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
		Func: PDResetSoft,
		Desc: "USB PD soft reset test",
		Contacts: []string{
			"chromeos-faft@google.com", // Owning team list
			"honscheid@google.com",     // Test author
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Data:         []string{firmware.ConfigFile},
		Attr:         []string{"group:firmware", "firmware_pd", "firmware_meets_kpi", "firmware_stressed", "firmware_ec_ro", "firmware_ec_rw", "firmware_bios_pdc"},
		Vars:         []string{"servo"},
		TestBedDeps:  tbdep.ServoPresentAndWorking,
		Fixture:      fixture.NormalMode,
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Timeout:      15 * time.Minute,
		Params: []testing.Param{{
			Name: "normal",
			Val: firmware.PDTestParams{
				DTS: firmware.DTSModeOff,
			},
		}, {
			Name: "flipcc",
			Val: firmware.PDTestParams{
				CC:  firmware.CCPolarityFlipped,
				DTS: firmware.DTSModeOff,
			},
		}, {
			Name: "dts",
			Val: firmware.PDTestParams{
				DTS: firmware.DTSModeOn,
			},
		}, {
			Name: "flipcc_dts",
			Val: firmware.PDTestParams{
				CC:  firmware.CCPolarityFlipped,
				DTS: firmware.DTSModeOn,
			},
		}, {
			Name: "shutdown",
			Val: firmware.PDTestParams{
				Shutdown: true,
				DTS:      firmware.DTSModeOff,
			},
		}},
	})
}

const (
	pdResetPollTimeout  time.Duration = 10 * time.Second
	pdResetPollInterval time.Duration = 500 * time.Millisecond
)

// PDResetSoft - USB PD soft reset
func PDResetSoft(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}

	testParams := s.Param().(firmware.PDTestParams)

	if err := firmware.SetupPDTester(ctx, h, testParams, s.OutDir()); err != nil {
		s.Fatal("Failed to configure Servo for PD testing: ", err)
	}

	//
	// Move on to the actual Soft Reset test
	//

	// Servo (PD Tester) initiates soft reset
	s.Log("Attempting Servo-initiated soft reset")
	if err := h.Servo.TriggerServoPDSoftReset(ctx); err != nil {
		s.Fatal("Servo-initiated soft reset did not succeed: ", err)
	}

	// EC/DUT initiates soft reset
	if h.Servo.IsDUTPDSoftResetSupported() {
		s.Log("Attempting EC/DUT-initiated soft reset")
		if err := h.Servo.TriggerPDSoftReset(ctx); err != nil {
			s.Fatal("EC-initiated soft reset did not succeed: ", err)
		}
	} else {
		s.Log("Skipping DUT-initiated soft reset as unsupported operation")
	}

	// If shut down, the DUT will refuse a power role swap to be a source. End the
	// test here.
	if testParams.Shutdown {
		s.Log("Skipping soft reset with DUT as source. Restoring DUT power")

		if err := h.Servo.SetPowerState(ctx, servo.PowerStateOn); err != nil {
			testing.ContextLog(ctx, "Failed to power on DUT: ", err)
		}
		if err := h.WaitConnect(ctx); err != nil {
			s.Fatal("Failed to boot after test: ", err)
		}

		return
	}

	// Attempt to do a power role swap by forcing the EC/DUT to be a source.
	// The DUT may not support this, in which case the swap will fail and we
	// will stop the test early.
	s.Log("Attempting a power role swap")
	if err := h.Servo.SetPDPowerRole(ctx, "SRC"); err != nil {
		s.Log("EC/DUT cannot swap power roles. End test here: ", err)
		return
	}

	// If the swap was accepted, wait for the port to be ready
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if pdState, err := h.Servo.GetDUTPDState(ctx); err == nil {
			if !pdState.IsSourceReady() {
				return errors.Wrap(err, "PD state is not ready")
			}
		} else {
			return errors.Wrap(err, "failed to get PD state")
		}

		return nil
	}, &testing.PollOptions{Timeout: pdResetPollTimeout, Interval: pdResetPollInterval}); err != nil {
		s.Fatal("Expected PD power swap: ", err)
	}
	s.Log("Power role swap succeeded. Repeating soft reset test from each side")

	defer func() {
		// Restore the DUT's port back to normal operation (i.e. a sink)
		s.Log("Restoring EC/DUT's port to sink")
		if err := h.Servo.RestorePDPort(ctx); err != nil {
			s.Fatal("Could not restore EC/DUT port: ", err)
		}
	}()

	// Repeat the previous tests - Servo (PD Tester) initiates soft reset
	s.Log("Attempting Servo-initiated soft reset")
	if err := h.Servo.TriggerServoPDSoftReset(ctx); err != nil {
		s.Fatal("Servo-initiated soft reset did not succeed after swapping power roles: ", err)
	}

	// EC/DUT initiates soft reset
	if h.Servo.IsDUTPDSoftResetSupported() {
		s.Log("Attempting EC/DUT-initiated soft reset")
		if err := h.Servo.TriggerPDSoftReset(ctx); err != nil {
			s.Fatal("EC-initiated soft reset did not succeed after swapping power roles: ", err)
		}
	} else {
		s.Log("Skipping DUT-initiated soft reset as unsupported operation")
	}
}
