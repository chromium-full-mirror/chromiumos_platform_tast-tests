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
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ECPDPowerSwap,
		Desc:         "Verify USB-C/PD Power Role Swap",
		LacrosStatus: testing.LacrosVariantUnneeded,
		Contacts: []string{
			"chromeos-faft@google.com",
			"asemjonovs@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Fixture:      fixture.NormalMode,
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Timeout:      20 * time.Minute,
		Params: firmware.AddPDPorts([]testing.Param{{
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
		}}, []string{"group:firmware", "firmware_pd"}),
	})
}

const (
	pdStatePollTimeout  time.Duration = 10 * time.Second
	pdStatePollInterval time.Duration = 500 * time.Millisecond
)

func ECPDPowerSwap(ctx context.Context, s *testing.State) {
	var curPowerRole string
	var nowPowerRole string

	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}

	testParams := s.Param().(firmware.PDTestParams)

	if testParams.Shutdown {
		if err := firmware.ShutdownDUT(ctx, h); err != nil {
			s.Fatal("Could not shut down DUT: ", err)
		}
	}

	if err := firmware.SetupPDTester(ctx, h, testParams.CC, testParams.DTS, testParams.RequiredPort); err != nil {
		s.Fatal("Failed to configure Servo for PD testing: ", err)
	}

	if dualRole, err := h.Servo.GetDUTDualRoleState(ctx, servo.PDPortUnderTest); dualRole != servo.USBPdDualRoleOn {
		if err != nil {
			s.Fatal("Get DualRole failed: ", err)
		}
		testing.ContextLog(ctx, "Power Swap support not advertised by DUT")
	}

	if pdState, err := h.Servo.GetDUTPDState(ctx); err != nil {
		s.Fatal("Failed to get PD state: ", err)
	} else {
		testing.ContextLogf(ctx, "PD state before: %#v", pdState)
		testing.ContextLog(ctx, "PD Role before: ", pdState.PowerRole)
		curPowerRole = string(pdState.PowerRole)
	}

	if err := h.Servo.SendPowerSwapRequest(ctx); err != nil {
		s.Fatal("Send Power Swap failed: ", err)
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if pdState, err := h.Servo.GetDUTPDState(ctx); err == nil {
			testing.ContextLogf(ctx, "PD state after: %#v", pdState)
			testing.ContextLog(ctx, "PD Role after: ", pdState.PowerRole)
			nowPowerRole = string(pdState.PowerRole)
			if curPowerRole == nowPowerRole {
				return errors.Wrap(err, "failed to switch power role")
			}
		} else {
			return errors.Wrap(err, "failed to get PD state")
		}

		return nil
	}, &testing.PollOptions{Timeout: pdStatePollTimeout, Interval: pdStatePollInterval}); err != nil {
		s.Fatal("Expected PD power swap: ", err)
	}

	if err := h.Servo.RestorePDPort(ctx); err != nil {
		s.Fatal("Failed to restore PD: ", err)
	}
}
