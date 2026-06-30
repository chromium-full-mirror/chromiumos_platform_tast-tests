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
		Func: PDDataSwap,
		Desc: "USB PD data role swap test",
		Contacts: []string{
			"chromeos-faft@google.com", // Owning team list
			"keithshort@chromium.org",  // Test author
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Data:         []string{firmware.ConfigFile},
		Vars:         []string{"servo"},
		TestBedDeps:  tbdep.ServoPresentAndWorking,
		Fixture:      fixture.NormalMode,
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Timeout:      15 * time.Minute,
		Attr:         []string{"group:firmware", "firmware_pd", "firmware_meets_kpi", "firmware_stressed", "firmware_ec_ro", "firmware_ec_rw", "firmware_bios_pdc"},
		Params: []testing.Param{{
			Name:      "normal",
			ExtraAttr: []string{"firmware_enabled"},
			Val: firmware.PDTestParams{
				DTS: firmware.DTSModeOff,
			},
		}, {
			Name: "normal_snk",
			Val: firmware.PDTestParams{
				PowerRole: firmware.RoleSink,
				DTS:       firmware.DTSModeOff,
			},
			ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel("boxy")),
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
			ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel("boxy")),
		}, {
			Name: "dts",
			Val: firmware.PDTestParams{
				DTS: firmware.DTSModeOn,
			},
		}, {
			Name: "dts_snk",
			Val: firmware.PDTestParams{
				DTS:       firmware.DTSModeOn,
				PowerRole: firmware.RoleSink,
			},
			ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel("boxy")),
		}, {
			Name: "flipcc_dts",
			Val: firmware.PDTestParams{
				CC:  firmware.CCPolarityFlipped,
				DTS: firmware.DTSModeOn,
			},
		}, {
			Name: "flipcc_dts_snk",
			Val: firmware.PDTestParams{
				CC:        firmware.CCPolarityFlipped,
				DTS:       firmware.DTSModeOn,
				PowerRole: firmware.RoleSink,
			},
			ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel("boxy")),
		}, {
			Name: "shutdown",
			Val: firmware.PDTestParams{
				Shutdown: true,
				DTS:      firmware.DTSModeOff,
			},
		}},
	})
}

const (
	pdDataRolePollTimeout time.Duration = 5 * time.Second
)

// PDDataSwap requests a single data role swap from the servo. The test is successful if the swap
// is rejected and the servo stays in UFP. In practice the servo always is in UFP, even when
// connected as source - in such scenario, before we get to this function, DUT will automatically
// request DRS which servo will always accept. Thus all the swaps in this test should be rejected.
// This is the only scenario that is actually supported as ChromeOS always wants to be in the DFP role.
func PDDataSwap(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}

	testParams := s.Param().(firmware.PDTestParams)

	if err := firmware.SetupPDTester(ctx, h, testParams, s.OutDir()); err != nil {
		s.Fatal("Failed to configure Servo for PD testing: ", err)
	}

	if err := dataRoleSwap(ctx, h); err != nil {
		s.Error("Data role swap failed: ", err)
	}

	// Reset the servo's power role.
	if err := h.Servo.ServoCcOff(ctx); err != nil {
		s.Error("Cannot force CC off on Servo: ", err)
	}

	if err := h.Servo.SetPDRole(ctx, servo.PDRoleSrc); err != nil {
		s.Error("Failed to set pd role: ", err)
	}
	if err := h.Servo.SetPDCommunication(ctx, servo.On); err != nil {
		s.Error("Failed to enable pd comms: ", err)
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
// As the DUT should already be a DFP and always wants to be DFP (servo is UFP),
// we count it as success if DRS is rejected and servo is UFP.
func dataRoleSwap(ctx context.Context, h *firmware.Helper) error {
	// Get the servo's current role.
	pdState, err := h.Servo.GetServoPDState(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get Servo PD state before data swap")
	}

	// Make sure servo is UFP
	if pdState.DataRole != servo.DataRoleUFP {
		return errors.New("DUT failed to automatically switch to DFP")
	}

	// Initiate swap from the servo.
	reply, err := h.Servo.ServoSendDataSwapRequest(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to initiate data swap on servo")
	}

	testing.ContextLogf(ctx, "DUT swap response %q", reply)

	if reply == servo.PDCtrlReject {
		// A PD device is allowed to reject a data swap request.
		// The DUT should reject a data swap if it is already in its
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

	if err := h.Servo.RestorePDDataRole(ctx); err != nil {
		return errors.Wrap(err, "failed to restore DUT to DFP")
	}

	return errors.New("DUT did not reject the swap response")
}
