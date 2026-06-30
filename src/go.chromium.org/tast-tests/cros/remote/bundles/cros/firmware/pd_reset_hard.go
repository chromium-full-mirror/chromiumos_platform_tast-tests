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
		Func: PDResetHard,
		Desc: "USB PD hard reset test",
		Contacts: []string{
			"chromeos-faft@google.com", // Owning team list
			"scollyer@google.com",      // Test author
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
			Name:      "normal",
			ExtraAttr: []string{"firmware_enabled"},
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

func executeHardReset(ctx context.Context, s *testing.State, count int) error {
	h := s.FixtValue().(*fixture.Value).Helper

	for i := 0; i < count; i++ {

		// Servo (PD Tester) initiated hard reset
		testing.ContextLog(ctx, "Servo initiated hard reset attempt ", i+1)
		if err := h.Servo.TriggerServoPDHardReset(ctx); err != nil {
			return errors.Wrap(err, "Servo initiated hard reset failed")
		}

		// EC/DUT initiates hard reset
		testing.ContextLog(ctx, "DUT initiated hard reset attempt ", i+1)
		if err := h.Servo.TriggerPDHardReset(ctx); err != nil {
			return errors.Wrap(err, "DUT initiated hard reset failed")
		}
	}
	return nil
}

// PDResetHard - USB PD hard reset
func PDResetHard(ctx context.Context, s *testing.State) {
	const iterationCount = 3

	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}

	testParams := s.Param().(firmware.PDTestParams)

	if err := firmware.SetupPDTester(ctx, h, testParams, s.OutDir()); err != nil {
		s.Fatal("Failed to configure Servo for PD testing: ", err)
	}

	//
	// Move on to the actual Hard Reset test
	//

	// Test Hard Reset with DUT as power role SNK
	if err := executeHardReset(ctx, s, iterationCount); err != nil {
		s.Fatal("DUT power role SNK hard reset test failed: ", err)
	}

	// If shut down, the DUT will refuse a power role swap to be a source. End the
	// test here.
	if testParams.Shutdown {
		s.Log("Skipping hard reset with DUT as source. Restoring DUT power")

		if err := h.Servo.SetPowerState(ctx, servo.PowerStateOn); err != nil {
			testing.ContextLog(ctx, "Failed to power on DUT: ", err)
		}
		if err := h.WaitConnect(ctx); err != nil {
			s.Fatal("Failed to boot after test: ", err)
		}

		return
	}

	// Servo and DUT haven't implemented the oriented debug accessory mode.
	// When servo is a sink in DTS mode, the DUT has no way to detect the CC
	// polarity and all CC communication will fail. So skip the hard reset
	// test with the DUT as source if we're in DTS mode.
	if testParams.DTS == firmware.DTSModeOn {
		s.Log("Skip testing with DUT as source in DTS mode")
		return
	}

	// Attempt to do a power role swap by forcing the EC/DUT to be a source.
	// The DUT may not support this, in which case the swap will fail and we
	// will stop the test early.
	s.Log("Attempting a power role swap")
	if err := h.Servo.SetPDPowerRole(ctx, "SRC"); err != nil {
		s.Fatal("EC/DUT cannot swap power roles. End test here: ", err)
	}
	s.Log("Power role swap succeeded. Repeating hard reset test from each side")

	defer func() {
		// Restore the DUT's port back to normal operation (i.e. a sink)
		s.Log("Restoring EC/DUT's port to sink")
		if err := h.Servo.RestorePDPort(ctx); err != nil {
			s.Fatal("Could not restore EC/DUT port: ", err)
		}
	}()

	// Test Hard Reset with DUT as power role SRC
	if err := executeHardReset(ctx, s, iterationCount); err != nil {
		s.Fatal("DUT power role SRC hard reset test failed: ", err)
	}
}
