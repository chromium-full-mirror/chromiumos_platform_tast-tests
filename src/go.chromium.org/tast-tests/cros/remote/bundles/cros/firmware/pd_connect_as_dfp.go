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
		Func: PDConnectAsDFP,
		Desc: "USB PD expect dut to swap from UFP to DFP",
		Contacts: []string{
			"chromeos-faft@google.com", // Owning team list
			"bszpila@google.com",       // Test author
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Data:         []string{firmware.ConfigFile},
		Vars:         []string{"servo"},
		Fixture:      fixture.NormalMode,
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Timeout:      15 * time.Minute,
		TestBedDeps:  tbdep.ServoPresentAndWorking,
		Attr:         []string{"group:firmware", "firmware_pd_unstable"},
		Params: []testing.Param{{
			Name: "normal",
			Val: firmware.PDTestParams{
				DTS:       firmware.DTSModeOff,
				PowerRole: firmware.RoleSource,
			},
			ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel("boxy")),
		}, {
			Name: "flipcc",
			Val: firmware.PDTestParams{
				CC:        firmware.CCPolarityFlipped,
				DTS:       firmware.DTSModeOff,
				PowerRole: firmware.RoleSource,
			},
			ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel("boxy")),
		}, {
			Name: "dts",
			Val: firmware.PDTestParams{
				DTS:       firmware.DTSModeOn,
				PowerRole: firmware.RoleSource,
			},
			ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel("boxy")),
		}, {
			Name: "flipcc_dts",
			Val: firmware.PDTestParams{
				CC:        firmware.CCPolarityFlipped,
				DTS:       firmware.DTSModeOn,
				PowerRole: firmware.RoleSource,
			},
			ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel("boxy")),
		}},
	})
}

const (
	pdDataRoleSwapTimeout time.Duration = 3 * time.Second
)

func PDConnectAsDFP(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	testParams := firmware.PDTestParams{}

	// Connect servo as source
	if err := firmware.SetupPDTester(ctx, h, testParams, s.OutDir()); err != nil {
		s.Fatal("Failed to configure Servo for PD testing: ", err)
	}

	// Set servo to reject data role swap
	if err := h.Servo.ServoSetDataSwapReject(ctx, true); err != nil {
		s.Fatal("Failed to set servo DRS policy: ", err)
	}

	// Go back to default state in case any errors occur during the test
	defer h.Servo.ServoSetDataSwapReject(ctx, false)

	// Reset the connection
	if err := h.Servo.ServoCcOff(ctx); err != nil {
		s.Fatal("Failed to disconnect servo from the DUT: ", err)
	}
	if err := h.Servo.ServoCcOn(ctx); err != nil {
		s.Fatal("Failed to connect servo to the DUT: ", err)
	}

	// GoBigSleepLint: Give some time for servo to connect with DUT
	if err := testing.Sleep(ctx, pdDataRoleSwapTimeout); err != nil {
		s.Fatal("Sleep failed: ", err)
	}

	// Make sure servo is DFP
	if pdState, err := h.Servo.GetServoPDState(ctx); err != nil {
		s.Fatal("Failed to get Servo PD state: ", err)
	} else if pdState.DataRole != servo.DataRoleDFP {
		s.Fatal("Servo should be a DFP, instead is ", pdState.DataRole)
	}

	// Set servo to accept data role swap
	if err := h.Servo.ServoSetDataSwapReject(ctx, false); err != nil {
		s.Fatal("Failed to set servo DRS policy: ", err)
	}

	// Reset the connection
	if err := h.Servo.ServoCcOff(ctx); err != nil {
		s.Fatal("Failed to disconnect servo from the DUT: ", err)
	}
	if err := h.Servo.ServoCcOn(ctx); err != nil {
		s.Fatal("Failed to connect servo to the DUT: ", err)
	}

	// GoBigSleepLint: Give some time for DUT to reset connection and send DRS request
	if err := testing.Sleep(ctx, pdDataRoleSwapTimeout); err != nil {
		s.Fatal("Sleep failed: ", err)
	}

	// Check that servo is UFP - that means DRS was sent by DUT and servo accepted
	if pdState, err := h.Servo.GetServoPDState(ctx); err != nil {
		s.Fatal("Failed to get Servo PD state: ", err)
	} else if pdState.DataRole != servo.DataRoleUFP {
		s.Fatal("Servo should be a UFP")
	}
}
