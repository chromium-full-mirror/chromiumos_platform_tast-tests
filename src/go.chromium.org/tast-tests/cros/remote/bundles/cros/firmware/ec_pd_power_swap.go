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
			Val:  firmware.PDTestParams{},
		}, {
			Name: "flipcc",
			Val: firmware.PDTestParams{
				CC: firmware.CCPolarityFlipped,
			},
		}, {
			Name: "dtsoff",
			Val: firmware.PDTestParams{
				DTS: firmware.DTSModeOff,
			},
		}, {
			Name: "flipcc_dtsoff",
			Val: firmware.PDTestParams{
				CC:  firmware.CCPolarityFlipped,
				DTS: firmware.DTSModeOff,
			},
		}, {
			Name: "shutdown",
			Val: firmware.PDTestParams{
				Shutdown: true,
			},
		}, {
			Name: "suspend",
			Val: firmware.PDTestParams{
				Suspend: true,
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
	var powerSwapSupported bool

	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}

	testParams := s.Param().(firmware.PDTestParams)

	if err := firmware.SetupPDTester(ctx, h, testParams); err != nil {
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

	if pdState, err := h.Servo.GetDUTPDState(ctx); err != nil {
		s.Fatal("Failed to get PD state: ", err)
	} else {
		testing.ContextLogf(ctx, "PD state before: %#v", pdState)
		testing.ContextLog(ctx, "PD Role before: ", pdState.PowerRole)
		curPowerRole = string(pdState.PowerRole)
	}

	if dutResponseMsg, err := h.Servo.ServoSendPowerSwapRequest(ctx); err != nil {
		if powerSwapSupported {
			s.Fatal("Send Power Swap failed: ", err)
		}
	} else if powerSwapSupported && dutResponseMsg != servo.PDCtrlAccept ||
		!powerSwapSupported && dutResponseMsg != servo.PDCtrlReject {
		s.Fatalf("Expected PRS support = %t, but DUT responded %q", powerSwapSupported, dutResponseMsg)
	}

	if powerSwapSupported {
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
	} else {
		// GoBigSleepLint: Check power role after timeout and confirm no power swap occurs
		if err := testing.Sleep(ctx, pdStatePollTimeout); err != nil {
			s.Fatal("Failed to sleep: ", err)
		}
		if pdState, err := h.Servo.GetDUTPDState(ctx); err == nil {
			testing.ContextLogf(ctx, "PD state after: %#v", pdState)
			testing.ContextLog(ctx, "PD Role after: ", pdState.PowerRole)
			nowPowerRole = string(pdState.PowerRole)
			if curPowerRole != nowPowerRole {
				s.Fatal("Unexpected power role swap: ", err)
			}
		} else {
			s.Fatal("Failed to get PD state: ", err)
		}
	}

	if powerSwapSupported {
		if err := h.Servo.RestorePDPort(ctx); err != nil {
			s.Fatal("Failed to restore PD: ", err)
		}
	}

	if testParams.Shutdown {
		if err := h.Servo.SetPowerState(ctx, servo.PowerStateOn); err != nil {
			testing.ContextLog(ctx, "Failed to power on DUT: ", err)
		}
		if err := h.WaitConnect(ctx); err != nil {
			s.Fatal("Failed to boot after test: ", err)
		}
	}
}
