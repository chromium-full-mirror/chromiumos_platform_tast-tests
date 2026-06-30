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
		Func: PDDUTSrcCaps,
		Desc: "Verify DUT has correct source capabilities",
		Contacts: []string{
			"chromeos-faft@google.com",
			"jasonyuan@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > baseOS > Firmware > FAFT
		Fixture:      fixture.NormalMode,
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Timeout:      20 * time.Minute,
		TestBedDeps:  tbdep.ServoPresentAndWorking,
		// TODO(b/364648498): Add "group:firmware_pd" tag once the test is stable.
		Attr: []string{"group:firmware", "firmware_pd_unstable"},

		Params: []testing.Param{{
			Name: "dtson_src",
			Val: firmware.PDTestParams{
				DTS: firmware.DTSModeOn,
			},
		}, {
			Name: "flipcc_dtson_src",
			Val: firmware.PDTestParams{
				CC:  firmware.CCPolarityFlipped,
				DTS: firmware.DTSModeOn,
			},
		}, {
			Name: "normal_src",
			Val: firmware.PDTestParams{
				DTS: firmware.DTSModeOff,
			},
		}, {
			Name: "flipcc_src",
			Val: firmware.PDTestParams{
				CC:  firmware.CCPolarityFlipped,
				DTS: firmware.DTSModeOff,
			},
		}, {
			Name: "dtson_snk",
			Val: firmware.PDTestParams{
				PowerRole: firmware.RoleSink,
				DTS:       firmware.DTSModeOn,
			},
		}, {
			Name: "flipcc_dtson_snk",
			Val: firmware.PDTestParams{
				CC:        firmware.CCPolarityFlipped,
				PowerRole: firmware.RoleSink,
				DTS:       firmware.DTSModeOn,
			},
		}, {
			Name: "normal_snk",
			Val: firmware.PDTestParams{
				DTS:       firmware.DTSModeOff,
				PowerRole: firmware.RoleSink,
			},
		}, {
			Name: "flipcc_normal_snk",
			Val: firmware.PDTestParams{
				CC:        firmware.CCPolarityFlipped,
				DTS:       firmware.DTSModeOff,
				PowerRole: firmware.RoleSink,
			},
		}, {
			Name: "shutdown",
			Val: firmware.PDTestParams{
				DTS:      firmware.DTSModeOff,
				Shutdown: true,
			},
		}, {
			Name: "suspend",
			Val: firmware.PDTestParams{
				DTS:     firmware.DTSModeOff,
				Suspend: true,
			},
		}},
	})
}

func PDDUTSrcCaps(ctx context.Context, s *testing.State) {
	var powerSwapSupported bool

	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}

	testParams := s.Param().(firmware.PDTestParams)

	if err := firmware.SetupPDTester(ctx, h, testParams, s.OutDir()); err != nil {
		s.Fatal("Failed to configure Servo for PD testing: ", err)
	}

	if dualRole, err := h.Servo.GetDUTDualRoleState(ctx, servo.PDPortUnderTest); dualRole != servo.USBPdDualRoleOn {
		if err != nil {
			s.Fatal("Get DualRole failed: ", err)
		}
		testing.ContextLog(ctx, "Power Swap support not advertised by DUT")
		powerSwapSupported = false
	} else {
		powerSwapSupported = true
	}

	srccaps, err := h.Servo.GetDUTSrcCaps(ctx)

	if powerSwapSupported {
		if len(srccaps) == 0 {
			s.Fatal("Retrieved DUT source caps returned empty: ", err)
		}

		for _, cap := range srccaps {
			testing.ContextLogf(ctx, "srccap output is: %dmV/%dmA", cap.Voltage, cap.Current)
			if cap.Voltage != 5000 || cap.Current != 3000 {
				s.Fatalf("DUT source cap is Invalid: srccap is %dmV/%dmA", cap.Voltage, cap.Current)
			}
		}
	} else {
		for _, cap := range srccaps {
			testing.ContextLogf(ctx, "srccap output is: %dmV/%dmA", cap.Voltage, cap.Current)
		}
		if len(srccaps) != 0 {
			s.Fatalf("DUT returned source capabilities while sourcing is disabled: %dmV/%dmA", srccaps[0].Voltage, srccaps[0].Current)
		}
	}

	if testParams.Shutdown || testParams.Suspend {
		if err := h.Servo.SetPowerState(ctx, servo.PowerStateOn); err != nil {
			testing.ContextLog(ctx, "Failed to power on DUT: ", err)
		}
		if err := h.WaitConnect(ctx); err != nil {
			s.Fatal("Failed to boot after test: ", err)
		}
	}
}
