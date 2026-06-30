// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/common/typecutils"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: PDPinNegotiation,
		Desc: "Tests if the DUT properly handles pin negotiations for DP alt mode",
		Contacts: []string{
			"chromeos-faft@google.com", // Owning team list
			"jasonyuan@google.com",     // Test author
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Data:         []string{firmware.ConfigFile},
		Vars:         []string{"servo"},
		TestBedDeps:  tbdep.ServoPresentAndWorking,
		SoftwareDeps: []string{"chrome"},
		Fixture:      fixture.NormalMode,
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.Battery(), hwdep.TypecStatus()),
		Timeout:      60 * time.Minute,
		Attr:         []string{"group:firmware", "firmware_pd", "firmware_ec_ro", "firmware_ec_rw", "firmware_bios_pdc"},
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
	pinsCDEF  string
	mfPref    servo.MultiFunctionPref
	pinExpect string
}

func verifyPins(input, output *(servo.TypeCInfo), pin string) error {
	if input.DPMode != output.DPMode {
		return errors.Errorf("incorrect DP activity, expected %d, got %d", input.DPMode, output.DPMode)
	}

	// TODO: b/371041395 track which pin is supposed to be selected in cases where multiple are supported.
	if len(output.PinsCDEF) == 0 {
		return errors.Errorf("no pin assignment found, expected %s", input.PinsCDEF)
	}
	if pin != output.PinsCDEF {
		return errors.Errorf("incorrect pin assignment, expected %c, got %s", input.PinsCDEF[0], output.PinsCDEF)
	}

	return nil
}

func PDPinNegotiation(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}

	testParams := s.Param().(firmware.PDTestParams)

	if err := firmware.SetupPDTester(ctx, h, testParams, s.OutDir()); err != nil {
		s.Fatal("Failed to configure Servo for PD testing: ", err)
	}

	pinsRange := []pinsMF{
		pinsMF{"C", servo.MFPrefDisable, "C"},
		pinsMF{"D", servo.MFPrefEnable, "D"},
		pinsMF{"CD", servo.MFPrefDisable, "C"},
		pinsMF{"CD", servo.MFPrefEnable, "D"},
	}

	for _, pins := range pinsRange {
		testing.ContextLogf(ctx, "testing DP mode: pins=%s, MF pref=%d", pins.pinsCDEF, int(pins.mfPref))
		input := servo.TypeCInfo{DPMode: servo.DPEnable, PinsCDEF: pins.pinsCDEF}

		if err := h.Servo.ServoSetDPConfigs(ctx, &input, pins.mfPref); err != nil {
			s.Fatal("Failed to set DP alt-mode: ", err)
		}

		if err := h.WaitConnect(ctx, firmware.SkipPDRoleSnk); err != nil {
			s.Fatal("Failed to establish connection after enabling DP alt-mode: ", err)
		}

		testing.ContextLog(ctx, "retrieving type-c information")
		if err := typecutils.CheckForDPAltMode(ctx, h.DUT, pins.pinExpect, h.Servo.DUTPDPort()); err != nil {
			testing.ContextLogf(ctx, "Could not find dp connection through svid: %s", err)

			typecInfo, err := h.Servo.GetTypeCInfo(ctx, h.DUT)
			if err != nil {
				s.Fatal("Failed to retrieve type-c information: ", err)
			}
			if err := verifyPins(&input, typecInfo, pins.pinExpect); err != nil {
				s.Fatal("Could not retrieve assigned DP setting: ", err)
			}

			testing.ContextLog(ctx, "successfully validated dp connection using ectools")
		}
	}

	input := servo.TypeCInfo{DPMode: servo.DPDisable, PinsCDEF: "C"}

	if err := h.Servo.ServoSetDPConfigs(ctx, &input, servo.MFPrefDisable); err != nil {
		s.Fatal("Failed to set DP alt-mode: ", err)
	}

	if err := h.WaitConnect(ctx, firmware.SkipPDRoleSnk); err != nil {
		s.Fatal("Failed to establish connection: ", err)
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
