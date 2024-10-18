// Copyright 2024 The ChromiumOS Authors
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
		Func: PDPinNegotiation,
		Desc: "Tests if the DUT properly handles pin negotiations",
		Contacts: []string{
			"chromeos-faft@google.com", // Owning team list
			"jasonyuan@google.com",     // Test author
		},
		BugComponent: "b:299174993", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Data:         []string{firmware.ConfigFile},
		Vars:         []string{"servo"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      fixture.NormalMode,
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.Battery()),
		Timeout:      60 * time.Minute,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Attr:         []string{"group:firmware", "firmware_pd_unstable"},
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
			Name: "normal_snk",
			Val: firmware.PDTestParams{
				DTS:       firmware.DTSModeOff,
				PowerRole: firmware.RoleSink,
			},
		}, {
			Name: "flipcc_snk",
			Val: firmware.PDTestParams{
				CC:        firmware.CCPolarityFlipped,
				DTS:       firmware.DTSModeOff,
				PowerRole: firmware.RoleSink,
			},
		}},
	})
}

type pinsMF struct {
	pinsCDEF string
	mfPref   servo.MultiFunctionPref
}

func PDPinNegotiation(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}

	testParams := s.Param().(firmware.PDTestParams)

	if err := firmware.SetupPDTester(ctx, h, testParams); err != nil {
		s.Fatal("Failed to configure Servo for PD testing: ", err)
	}

	pinsRange := []pinsMF{
		pinsMF{"C", servo.MFPrefDisable},
		pinsMF{"D", servo.MFPrefEnable},
		pinsMF{"CD", servo.MFPrefDisable},
		pinsMF{"CD", servo.MFPrefEnable},
	}

	for _, pins := range pinsRange {
		testing.ContextLogf(ctx, "testing DP mode: pins=%s, MF pref=%d", pins.pinsCDEF, int(pins.mfPref))
		input := servo.TypeCInfo{DPMode: servo.DPEnable, PinsCDEF: pins.pinsCDEF}

		if err := h.Servo.ServoSetDPConfigs(ctx, &input, pins.mfPref); err != nil {
			s.Fatal("Failed to set DP alt-mode: ", err)
		}
		testing.ContextLog(ctx, "retrieving type-c information")
		typecInfo, err := h.Servo.GetTypeCInfo(ctx, h.DUT)
		if err != nil {
			s.Fatal("Failed to retrieve type-c information: ", err)
		}
		if err := h.Servo.VerifyPins(&input, typecInfo, pins.mfPref); err != nil {
			s.Fatal("Could not retrieve assigned DP setting: ", err)
		}
	}

	input := servo.TypeCInfo{DPMode: servo.DPDisable, PinsCDEF: "C"}

	if err := h.Servo.ServoSetDPConfigs(ctx, &input, servo.MFPrefDisable); err != nil {
		s.Fatal("Failed to set DP alt-mode: ", err)
	}
	testing.ContextLog(ctx, "retrieving type-c information")
	typecInfo, err := h.Servo.GetTypeCInfo(ctx, h.DUT)
	if err != nil {
		s.Fatal("Failed to retrieve type-c information: ", err)
	}
	if typecInfo.DPMode != servo.DPDisable {
		s.Fatal("Type-c DP did not disable")
	}

}
