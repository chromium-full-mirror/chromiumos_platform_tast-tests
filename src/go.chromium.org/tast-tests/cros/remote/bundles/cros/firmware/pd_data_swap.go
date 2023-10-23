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
		Func: PDDataSwap,
		Desc: "USB PD data role swap test",
		Contacts: []string{
			"chromeos-faft@google.com", // Owning team list
			"keithshort@chromium.org",  // Test author
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		// TODO: When stable, move to firmware_pd.
		Data:         []string{firmware.ConfigFile},
		Attr:         []string{"group:firmware", "firmware_pd_unstable"},
		Vars:         []string{"servo"},
		Fixture:      fixture.NormalMode,
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Timeout:      15 * time.Minute,
		Params: []testing.Param{{
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
		}},
		// TODO: b/194910842 - [faft-pd] Convert firmware_PDDataSwap to TAST
		// Add "shutdown" parameter
	})
}

const (
	pdDataRolePollTimeout  time.Duration = 500 * time.Millisecond
	pdDataRolePollInterval time.Duration = 100 * time.Millisecond
	pdDataRoleSwapCount    int           = 10
)

// dataSwapSrc determines which PD partner initiates the data swap.
type dataSwapSrc int

const (
	dutDataSwap dataSwapSrc = iota
	servoDataSwap
)

// PDDataSwap performs a USB PD data role swap test.
func PDDataSwap(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}

	testParams := s.Param().(firmware.PDTestParams)

	if err := firmware.SetupPDTester(ctx, h, testParams.CC, testParams.DTS, testParams.RequiredPort); err != nil {
		s.Fatal("Failed to configure Servo for PD testing: ", err)
	}

	pdState, err := h.Servo.GetServoPDState(ctx)
	if err != nil {
		s.Fatal("Failed to get Servo PD state: ", err)
	}

	// Verify that the DUT supports data swap, as reported in the servo's partner flags.
	if pdState.PEFlags&servo.PartnerDualRoleData != 0 {
		s.Logf("DUT supports data role swap, attempting %d swaps", pdDataRoleSwapCount)
		var swapSrc dataSwapSrc
		for i := 0; i < pdDataRoleSwapCount; i++ {
			// Every 2 swaps, switch which partner initiates the data role swap.
			if i&2 == 0 {
				swapSrc = dutDataSwap
			} else {
				swapSrc = servoDataSwap
			}
			err := dataRoleSwap(ctx, h, swapSrc)
			if err != nil {
				s.Fatal("Data role swap failed: ", err)
			}
		}
	}
	// TODO: b/194910842 - [faft-pd] Convert firmware_PDDataSwap to TAST
	// Need to verify data role swap is rejected.
}

func dataRoleSwap(ctx context.Context, h *firmware.Helper, swapSrc dataSwapSrc) error {
	// Get the servo's current role.
	pdState, err := h.Servo.GetServoPDState(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get Servo PD state before data swap")
	}

	servoRoleBefore := pdState.DataRole
	testing.ContextLog(ctx, "Servo data role before: ", servoRoleBefore)

	if swapSrc == servoDataSwap {
		// Initiate swap from the servo.
		testing.ContextLog(ctx, "Servo initiates data swap")
		reply, err := h.Servo.ServoSendDataSwapRequest(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to initiate data swap on servo")
		}

		testing.ContextLogf(ctx, "DUT swap response %q", reply)

		if reply == servo.PDCtrlReject {
			// A PD device is allowed to reject a data swap request.
			// The DUT may reject a data swap if it is already in its
			// preferred role.
			// Fall through and perform a swap from the DUT side.
			testing.ContextLog(ctx, "DUT rejected data swap (expected)")

			swapSrc = dutDataSwap
		}
	}

	if swapSrc == dutDataSwap {
		// Initiate swap from the DUT.
		testing.ContextLog(ctx, "DUT initiates data swap")
		err = h.Servo.SendDataSwapRequest(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to initiate data swap on DUT")
		}
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if pdState, err = h.Servo.GetServoPDState(ctx); err == nil {
			if pdState.DataRole == servoRoleBefore {
				return errors.Wrap(err, "failed to switch data role")
			}
		} else {
			return errors.Wrap(err, "failed to get servo PD state after data swap")
		}

		testing.ContextLog(ctx, "Servo data role after: ", pdState.DataRole)
		return nil
	}, &testing.PollOptions{Timeout: pdDataRolePollTimeout, Interval: pdDataRolePollInterval}); err != nil {
		return errors.Wrap(err, "expected data role swap")
	}

	return nil
}
