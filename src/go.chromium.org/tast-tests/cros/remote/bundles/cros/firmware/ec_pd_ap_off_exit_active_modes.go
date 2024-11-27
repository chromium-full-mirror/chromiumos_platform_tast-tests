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
		Func: ECPDApOffExitActiveModes,
		Desc: "Tests if the DUT exits alternate modes when AP transitions to off",
		Contacts: []string{
			"chromeos-faft@google.com", // Owning team list
			"bszpila@google.com",       // Test author
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Data:         []string{firmware.ConfigFile},
		Vars:         []string{"servo"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      fixture.NormalMode,
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Timeout:      60 * time.Minute,
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
			ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel("boxy")),
		}, {
			Name: "flipcc_snk",
			Val: firmware.PDTestParams{
				CC:        firmware.CCPolarityFlipped,
				DTS:       firmware.DTSModeOff,
				PowerRole: firmware.RoleSink,
			},
			ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel("boxy")),
		}},
	})
}

// ECPDApOffExitActiveModes Enables DP mode with CD pins and turns off the DUT to check
// if EC reports DP as disabled after transitioning to G3.
func ECPDApOffExitActiveModes(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}

	testParams := s.Param().(firmware.PDTestParams)

	if err := firmware.SetupPDTester(ctx, h, testParams); err != nil {
		s.Fatal("Failed to configure Servo for PD testing: ", err)
	}

	// Enable dp mode
	input := servo.TypeCInfo{DPMode: servo.DPEnable, PinsCDEF: "CD"}
	if err := h.Servo.ServoSetDPConfigs(ctx, &input, servo.MFPrefEnable); err != nil {
		s.Fatal("Failed to set DP alt-mode: ", err)
	}

	// Check if DP is on
	testing.ContextLog(ctx, "Verifying DP is enabled")
	typecInfo, err := h.Servo.GetTypeCByECCommand(ctx)
	if err != nil {
		s.Fatal("Failed to retrieve type-c information: ", err)
	}
	if typecInfo.DPMode != servo.DPEnable {
		s.Fatal("Type-c DP did not enable")
	}

	// Turn off the DUT
	firmware.ShutdownDUT(ctx, h)

	// Check if DP is off
	testing.ContextLog(ctx, "Verifying DP is disabled")
	typecInfo, err = h.Servo.GetTypeCByECCommand(ctx)
	if err != nil {
		s.Fatal("Failed to retrieve type-c information: ", err)
	}
	if typecInfo.DPMode != servo.DPDisable {
		s.Fatal("Type-c DP did not disable")
	}

	// Turn DUT back on
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateOn); err != nil {
		testing.ContextLog(ctx, "Failed to power on DUT: ", err)
	}
	if err := h.WaitConnect(ctx); err != nil {
		s.Fatal("Failed to boot after test: ", err)
	}

	// Check if DP is back on
	testing.ContextLog(ctx, "Verifying DP is enabled")
	typecInfo, err = h.Servo.GetTypeCByECCommand(ctx)
	if err != nil {
		s.Fatal("Failed to retrieve type-c information: ", err)
	}
	if typecInfo.DPMode != servo.DPEnable {
		s.Fatal("Type-c DP did not enable")
	}
}
