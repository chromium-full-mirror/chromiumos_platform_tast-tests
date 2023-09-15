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
		Func: PDDataSwap,
		Desc: "USB PD data role swap test",
		Contacts: []string{
			"chromeos-faft@google.com", // Owning team list
			"keithshort@chromium.org",  // Test author
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		// TODO: When stable, move to firmware_pd.
		Data:         []string{firmware.ConfigFile},
		Attr:         []string{"group:firmware", "firmware_unstable"},
		Vars:         []string{"servo"},
		Fixture:      fixture.NormalMode,
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Timeout:      15 * time.Minute,
	})
}

// PDDataSwap USB PD data role swap test
func PDDataSwap(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}

	// TODO: move common PD test requirements into separate go file.

	// From firmware_test.py->setup_pdtester:
	// 1. Verify servo includes PD tester (v4, v4p1), and debug connection (micro, c2d2)
	// 2. Ensure battery is at 10% or greater
	//	a. Start charging if needed and wait
	// 3. Disable dts_mode on the PD tester

	// PD tests require both a servo V4 connection and servo debug connection.
	if err := h.Servo.RequirePDTester(ctx); err != nil {
		s.Fatal("Servo configuration does not support PD testing: ", err)
	}
	s.Log("Has PDTester")

	err := h.Servo.RequireDUTPDInfo(ctx)
	if err != nil {
		s.Fatal("Error in getting PD port info: ", err)
	}

	// Note - servo has 2 PD ports.  Port 0 is the connection to the charger
	// port 1 connects to the DUT.
	var pdState *servo.PDState
	pdState, err = h.Servo.GetServoPDState(ctx)
	if err != nil {
		s.Fatal("Failed to get Servo PD state: ", err)
	}

	s.Log("Servo PD info:")
	s.Logf("  Port       %d", pdState.Port)
	s.Logf("  Polarity   %q", pdState.Polarity)
	s.Logf("  Connection %q", pdState.Connection)
	s.Logf("  PowerRole  %q", pdState.PowerRole)
	s.Logf("  DataRole   %q", pdState.DataRole)
	s.Logf("  PEState    %d", pdState.PEState)
	s.Logf("  Flags      0x%x", pdState.Flags)

	hasBattery := h.Config.HasECCapability(firmware.ECBattery)
	s.Log("ECCapBattery: ", hasBattery)

	if hasBattery {
		if err := firmware.ChargeToLevel(ctx, h, 10, 10*time.Minute); err != nil {
			s.Fatal("Cannot start PD test: ", err)
		}

		// FIXME - have ChargeToLevel return current state
		cs, err := firmware.GetChargingState(ctx, h)
		if err != nil {
			s.Fatal("Failed to read charging state: ", err)
		}

		s.Log("Battery capacity at test start: ", cs["batt.state_of_charge"])
	}

	// TODO: Implement actual PD data role swap test
}
