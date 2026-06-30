// Copyright 2025 The ChromiumOS Authors
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
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: PDSBUVoltageDP,
		Desc: "Verify correct SBU voltage while dp-alt mode is enabled",
		Contacts: []string{
			"chromeos-faft@google.com",
			"jasonyuan@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Fixture:      fixture.NormalMode,
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Timeout:      20 * time.Minute,
		TestBedDeps:  tbdep.ServoPresentAndWorking,
		Attr:         []string{"group:firmware", "firmware_pd_unstable"},
		Params: []testing.Param{{
			Name: "normal",
			Val: firmware.PDTestParams{
				DTS: firmware.DTSModeOff,
			},
		}, {
			Name: "normal_snk",
			Val: firmware.PDTestParams{
				PowerRole: firmware.RoleSink,
				DTS:       firmware.DTSModeOff,
			},
		}, {
			Name: "flipcc",
			Val: firmware.PDTestParams{
				CC:  firmware.CCPolarityFlipped,
				DTS: firmware.DTSModeOff,
			},
		}, {
			Name: "flipcc_snk",
			Val: firmware.PDTestParams{
				CC:        firmware.CCPolarityFlipped,
				PowerRole: firmware.RoleSink,
				DTS:       firmware.DTSModeOff,
			},
		}},
	})
}

func PDSBUVoltageDP(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}

	testParams := s.Param().(firmware.PDTestParams)

	if err := firmware.SetupPDTester(ctx, h, testParams, s.OutDir()); err != nil {
		s.Fatal("Failed to configure Servo for PD testing: ", err)
	}

	input := servo.TypeCInfo{DPMode: servo.DPEnable, PinsCDEF: "CD"}
	if err := h.Servo.ServoSetDPConfigs(ctx, &input, servo.MFPrefEnable); err != nil {
		s.Fatal("Failed to set DP alt-mode: ", err)
	}

	testing.ContextLog(ctx, "verifying DP is enabled")
	typecInfo, err := h.Servo.GetTypeCInfo(ctx, h.DUT)
	if err != nil {
		s.Fatal("Failed to retrieve type-c information: ", err)
	}
	if typecInfo.DPMode != servo.DPEnable {
		s.Fatal("Type-c DP did not enable")
	}

	out, err := h.Servo.ServoGetSBU(ctx)
	if err != nil {
		s.Fatal("Failed to retrieve SBU voltage: ", err)
	}

	sbu1 := out[0]
	sbu2 := out[1]
	if testParams.CC == firmware.CCPolarityFlipped {
		sbu1 = out[1]
		sbu2 = out[0]
	}

	if sbu1 < 2000 || sbu1 > 4000 || sbu2 > 500 {
		s.Fatalf("SBU voltage not within acceptable range: SBU1 = %dmV, SBU2 = %dmV", out[0], out[1])
	}
}
