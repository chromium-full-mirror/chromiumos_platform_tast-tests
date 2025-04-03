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
		Func: ECPDPowerSwap,
		Desc: "Verify USB-C/PD Power Role Swap",
		Contacts: []string{
			"chromeos-faft@google.com",
			"asemjonovs@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Fixture:      fixture.NormalMode,
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Timeout:      20 * time.Minute,
		TestBedDeps:  []string{tbdep.ServoStateWorking},
		Attr:         []string{"group:firmware", "firmware_pd", "firmware_stressed", "firmware_ec_ro", "firmware_ec_rw", "firmware_bios_pdc"},
		Params: []testing.Param{{
			Name:      "normal",
			ExtraAttr: []string{"firmware_enabled", "firmware_meets_kpi"},
			Val: firmware.PDTestParams{
				NumIterations: 1,
				DTS:           firmware.DTSModeOff,
			},
		}, {
			Name: "normal_stress",
			Val: firmware.PDTestParams{
				NumIterations: 20,
				DTS:           firmware.DTSModeOff,
			},
		}, {
			Name:      "flipcc",
			ExtraAttr: []string{"firmware_meets_kpi"},
			Val: firmware.PDTestParams{
				CC:            firmware.CCPolarityFlipped,
				NumIterations: 1,
				DTS:           firmware.DTSModeOff,
			},
		}, {
			Name:      "flipcc_stress",
			ExtraAttr: []string{"firmware_meets_kpi"},
			Val: firmware.PDTestParams{
				CC:            firmware.CCPolarityFlipped,
				NumIterations: 20,
				DTS:           firmware.DTSModeOff,
			},
		}, {
			Name:      "dts",
			ExtraAttr: []string{"firmware_meets_kpi"},
			Val: firmware.PDTestParams{
				DTS:           firmware.DTSModeOn,
				NumIterations: 1,
			},
		}, {
			Name: "dts_stress",
			Val: firmware.PDTestParams{
				DTS:           firmware.DTSModeOn,
				NumIterations: 20,
			},
		}, {
			Name:      "flipcc_dts",
			ExtraAttr: []string{"firmware_meets_kpi"},
			Val: firmware.PDTestParams{
				CC:            firmware.CCPolarityFlipped,
				DTS:           firmware.DTSModeOn,
				NumIterations: 1,
			},
		}, {
			Name: "flipcc_dts_stress",
			Val: firmware.PDTestParams{
				CC:            firmware.CCPolarityFlipped,
				DTS:           firmware.DTSModeOn,
				NumIterations: 20,
			},
		}, {
			Name:      "shutdown",
			ExtraAttr: []string{"firmware_meets_kpi"},
			Val: firmware.PDTestParams{
				Shutdown:      true,
				NumIterations: 1,
				DTS:           firmware.DTSModeOff,
			},
		}, {
			Name:      "suspend",
			ExtraAttr: []string{"firmware_meets_kpi"},
			Val: firmware.PDTestParams{
				Suspend:       true,
				NumIterations: 1,
				DTS:           firmware.DTSModeOff,
			},
		}},
	})
}

const (
	pdStatePollTimeout  time.Duration = 10 * time.Second
	pdStatePollInterval time.Duration = 500 * time.Millisecond
	pdSettleTimeDefault time.Duration = 3 * time.Second
	pdSettleTimeDTSMode time.Duration = 5 * time.Second
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

	if pdState, err := h.Servo.GetDUTPDState(ctx); err != nil {
		s.Fatal("Failed to get PD state: ", err)
	} else {
		curPowerRole = string(pdState.PowerRole)
	}

	for i := 0; i < testParams.NumIterations; i++ {
		testing.ContextLogf(ctx, "[%d] - PD Role Before: %s", i, curPowerRole)
		testing.ContextLogf(ctx, "[%d] - PD Request Power Swap", i)
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
					nowPowerRole = string(pdState.PowerRole)
					testing.ContextLogf(ctx, "[%d] - PD Role After: %s", i, nowPowerRole)
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
				nowPowerRole = string(pdState.PowerRole)
				testing.ContextLogf(ctx, "[%d] - PD Role After: %s", i, nowPowerRole)
				if curPowerRole != nowPowerRole {
					s.Fatal("Unexpected power role swap: ", err)
				}
			} else {
				s.Fatal("Failed to get PD state: ", err)
			}
		}

		var pdSettleTime time.Duration = pdSettleTimeDefault
		if testParams.DTS == firmware.DTSModeOn {
			pdSettleTime = pdSettleTimeDTSMode
		}
		curPowerRole = nowPowerRole
		// GoBigSleepLint: Let PDC settle before initiating next PRS
		if err := testing.Sleep(ctx, pdSettleTime); err != nil {
			s.Fatal("Failed to sleep for PDC settle: ", err)
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
