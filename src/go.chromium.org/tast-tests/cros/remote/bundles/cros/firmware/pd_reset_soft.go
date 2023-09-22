// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
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
		BugComponent: "b:194910842", // ChromeOS > Platform > Enablement > Firmware > FAFT
		// TODO: When stable, move to firmware_pd.
		Data:         []string{firmware.ConfigFile},
		Attr:         []string{"group:firmware", "firmware_unstable"},
		Vars:         []string{"servo"},
		Fixture:      fixture.NormalMode,
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Timeout:      15 * time.Minute,
	})
}

// PDResetSoft - USB PD soft reset
func PDResetSoft(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}

	if err := firmware.SetupPDTester(ctx, h, firmware.CCPolarityStandard, firmware.DTSModeOn); err != nil {
		s.Fatal("Failed to configure Servo for PD testing: ", err)
	}

	// Gather info on the DUT's USB-PD config
	if err := h.Servo.RequireDUTPDInfo(ctx); err != nil {
		s.Fatal("Could not gather DUT PD info: ", err)
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

	// Attempt to do a power role swap by forcing the EC/DUT to be a source.
	// The DUT may not support this, in which case the swap will fail and we
	// will stop the test early.
	s.Log("Attempting a power role swap")
	if err := h.Servo.SetPDPowerRole(ctx, servo.PDPortUnderTest, "SRC"); err != nil {
		s.Log("EC/DUT cannot swap power roles. End test here: ", err)
		return
	}
	s.Log("Power role swap succeeded. Repeating soft reset test from each side")

	defer func() {
		// Restore the DUT's port back to normal operation (i.e. a sink)
		s.Log("Restoring EC/DUT's port to sink")
		if err := h.Servo.RestorePDPort(ctx, servo.PDPortUnderTest); err != nil {
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
