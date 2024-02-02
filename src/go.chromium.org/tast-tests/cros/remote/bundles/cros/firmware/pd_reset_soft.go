// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
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
		// TODO: When stable, move to firmware_pd.
		Data:         []string{firmware.ConfigFile},
		Attr:         []string{"group:firmware", "firmware_pd_unstable"},
		Vars:         []string{"servo"},
		Fixture:      fixture.NormalMode,
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Timeout:      15 * time.Minute,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Params: []testing.Param{{
			Name: "normal",
			Val: firmware.PDTestParams{
				CC:       firmware.CCPolarityStandard,
				DTS:      firmware.DTSModeOn,
				Shutdown: false,
			},
		}, {
			Name: "flipcc",
			Val: firmware.PDTestParams{
				CC:       firmware.CCPolarityFlipped,
				DTS:      firmware.DTSModeOn,
				Shutdown: false,
			},
		}, {
			Name: "dtsoff",
			Val: firmware.PDTestParams{
				CC:       firmware.CCPolarityStandard,
				DTS:      firmware.DTSModeOff,
				Shutdown: false,
			},
		}, {
			Name: "flipcc_dtsoff",
			Val: firmware.PDTestParams{
				CC:       firmware.CCPolarityFlipped,
				DTS:      firmware.DTSModeOff,
				Shutdown: false,
			},
		}, {
			Name: "shutdown",
			Val: firmware.PDTestParams{
				CC:       firmware.CCPolarityStandard,
				DTS:      firmware.DTSModeOn,
				Shutdown: true,
			},
		}},
	})
}

// PDResetSoft - USB PD soft reset
func PDResetSoft(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}

	testParams := s.Param().(firmware.PDTestParams)

	if err := firmware.SetupPDTester(ctx, h, testParams.CC, testParams.DTS, testParams.RequiredPort); err != nil {
		s.Fatal("Failed to configure Servo for PD testing: ", err)
	}

	if testParams.Shutdown {
		if err := firmware.ShutdownDUT(ctx, h); err != nil {
			s.Fatal("Could not shut down DUT: ", err)
		}
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
	s.Log("Attempting EC/DUT-initiated soft reset")
	if err := h.Servo.TriggerPDSoftReset(ctx); err != nil {
		s.Fatal("EC-initiated soft reset did not succeed: ", err)
	}

	// Testing soft resets after a power role swap (DUT is SRC) is not currently
	// supported.
	if testParams.Shutdown {
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
	s.Log("Attempting EC/DUT-initiated soft reset")
	if err := h.Servo.TriggerPDSoftReset(ctx); err != nil {
		s.Fatal("EC-initiated soft reset did not succeed after swapping power roles: ", err)
	}

}
