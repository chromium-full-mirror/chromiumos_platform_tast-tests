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
		Data:         []string{firmware.ConfigFile},
		Vars:         []string{"servo"},
		Fixture:      fixture.NormalMode,
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Timeout:      15 * time.Minute,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Params: firmware.AddPDPorts([]testing.Param{{
			Name: "normal",
			Val:  firmware.PDTestParams{},
		}, {
			Name: "normal_snk",
			Val: firmware.PDTestParams{
				PowerRole: firmware.RoleSink,
			},
			ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel("boxy")),
		}, {
			Name: "flipcc",
			Val: firmware.PDTestParams{
				CC: firmware.CCPolarityFlipped,
			},
		}, {
			Name: "flipcc_snk",
			Val: firmware.PDTestParams{
				CC:        firmware.CCPolarityFlipped,
				PowerRole: firmware.RoleSink,
			},
			ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel("boxy")),
		}, {
			Name: "dtsoff",
			Val: firmware.PDTestParams{
				DTS: firmware.DTSModeOff,
			},
		}, {
			Name: "dtsoff_snk",
			Val: firmware.PDTestParams{
				DTS:       firmware.DTSModeOff,
				PowerRole: firmware.RoleSink,
			},
			ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel("boxy")),
		}, {
			Name: "flipcc_dtsoff",
			Val: firmware.PDTestParams{
				CC:  firmware.CCPolarityFlipped,
				DTS: firmware.DTSModeOff,
			},
		}, {
			Name: "flipcc_dtsoff_snk",
			Val: firmware.PDTestParams{
				CC:        firmware.CCPolarityFlipped,
				DTS:       firmware.DTSModeOff,
				PowerRole: firmware.RoleSink,
			},
			ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel("boxy")),
		}, {
			Name: "shutdown",
			Val: firmware.PDTestParams{
				Shutdown: true,
			},
		}}, []string{"group:firmware", "firmware_pd"}),
	})
}

const (
	pdDataRolePollTimeout  time.Duration = 5 * time.Second
	pdDataRolePollInterval time.Duration = 100 * time.Millisecond
)

// PDDataSwap requests a single data role swap from the servo. The test is successful if the swap
// works or is rejected but the servo stays in UFP. In practice the servo should already be UFP,
// so all the swaps will be rejected. This is the only scenario that is actually supported as
// ChromeOS always wants to be in the DFP role.
func PDDataSwap(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}

	testParams := s.Param().(firmware.PDTestParams)

	if err := firmware.SetupPDTester(ctx, h, testParams); err != nil {
		s.Fatal("Failed to configure Servo for PD testing: ", err)
	}

	if err := dataRoleSwap(ctx, h); err != nil {
		s.Error("Data role swap failed: ", err)
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

// dataRoleSwap tests data role swaps from servo.
// As the DUT always wants to be DFP (servo is UFP), we count it as success if the swap is successful, or if it is rejected and servo is UFP.
func dataRoleSwap(ctx context.Context, h *firmware.Helper) error {
	// Get the servo's current role.
	pdState, err := h.Servo.GetServoPDState(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get Servo PD state before data swap")
	}

	servoRoleBefore := pdState.DataRole
	testing.ContextLog(ctx, "Servo data role before: ", servoRoleBefore)

	// Initiate swap from the servo.
	reply, err := h.Servo.ServoSendDataSwapRequest(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to initiate data swap on servo")
	}

	testing.ContextLogf(ctx, "DUT swap response %q", reply)

	if reply == servo.PDCtrlReject && servoRoleBefore == servo.DataRoleUFP {
		// A PD device is allowed to reject a data swap request.
		// The DUT may reject a data swap if it is already in its
		// preferred role. The DUT's preferred role should always be DFP.
		testing.ContextLog(ctx, "DUT rejected data swap (expected)")
		// GoBigSleepLint: Sleep for pdDataRolePollTimeout to make sure the data role doesn't spontaneously change.
		if err := testing.Sleep(ctx, pdDataRolePollTimeout); err != nil {
			return errors.Wrap(err, "sleep failed")
		}
		if pdState, err = h.Servo.GetServoPDState(ctx); err == nil {
			if pdState.DataRole != servo.DataRoleUFP {
				return errors.Errorf("incorrect role got %q, want %q", pdState.DataRole, servo.DataRoleUFP)
			}
		} else {
			return errors.Wrap(err, "failed to get servo PD state after rejected data swap")
		}

		testing.ContextLog(ctx, "Servo data role after: ", pdState.DataRole)
		return nil
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
