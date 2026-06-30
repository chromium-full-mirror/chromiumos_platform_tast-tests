// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"strconv"
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
		Func: ECPDCCD,
		Desc: "CCD connection test",
		Contacts: []string{
			"chromeos-faft@google.com", // Owning team list
			"jasonyuan@google.com",     // Test author
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Data:         []string{firmware.ConfigFile},
		Vars:         []string{"servo"},
		TestBedDeps:  tbdep.ServoPresentAndWorking,
		Fixture:      fixture.NormalMode,
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Timeout:      6 * time.Minute,
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
		}},
	})
}

// ECPDCCD verifies that the DUT port can enable CCD with the correct CC voltages
func ECPDCCD(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}

	testParams := s.Param().(firmware.PDTestParams)

	if err := firmware.SetupPDTester(ctx, h, testParams, s.OutDir()); err != nil {
		s.Fatal("Failed to configure Servo for PD testing: ", err)
	}

	if err := h.Servo.SetOnOff(ctx, servo.CCDKeepaliveEn, servo.Off); err != nil {
		s.Fatal("Failed to disable ccd keep alive: ", err)
	}

	//TODO(b/343535169): different gsc chips seems to have different voltage thresholds,
	//                   add more voltage inervals after sorting the different thresholds.
	voltageRange := []int{0, 1000, 1500, 2500}

	if err := h.Servo.ServoCcDac(ctx, 1, "on"); err != nil {
		s.Fatal("Failed to enable servo cc dac: ", err)
	}
	if err := h.Servo.ServoCcDac(ctx, 2, "on"); err != nil {
		s.Fatal("Failed to enable servo cc dac: ", err)
	}
	defer func() {
		// restore normal servo cc functionality
		testing.ContextLog(ctx, "restoring automatic cc control")
		if err := h.Servo.ServoCcDac(ctx, 1, "off"); err != nil {
			s.Fatal("Failed to disable servo cc dac: ", err)
		}
		if err := h.Servo.ServoCcDac(ctx, 2, "off"); err != nil {
			s.Fatal("Failed to disable servo cc dac: ", err)
		}
	}()

	numErrors := 0
	for _, cc1 := range voltageRange {
		if err := h.Servo.ServoCcDac(ctx, 1, strconv.Itoa(cc1)); err != nil {
			s.Fatal("Failed to edit servo cc dac: ", err)
		}

		for _, cc2 := range voltageRange {
			if err := h.Servo.ServoCcDac(ctx, 2, strconv.Itoa(cc2)); err != nil {
				s.Fatal("Failed to edit servo cc dac: ", err)
			}

			ccdExpect := "asserted"
			if cc1 < 500 || cc2 < 500 || cc1 > 2000 || cc2 > 2000 {
				ccdExpect = "deasserted"
			}
			// GoBigSleepLint: getting the CCD State seems to interrupt the CCD process
			//                 so we have to get it right the first time instead of polling
			testing.Sleep(ctx, 5*time.Second)

			ccdMode, err := h.Servo.GetCCDMode(ctx)
			if err != nil {
				s.Fatal("Failed to retrieve ccd status: ", err)
			}

			if ccdMode != ccdExpect {
				s.Errorf("Test case cc1=%d, cc2=%d: Expected ccd state: %s, actual: %s", cc1, cc2, ccdExpect, ccdMode)
				numErrors++
			}
		}
	}

	if numErrors != 0 {
		s.Fatalf("Failed %d testcases", numErrors)
	}
}
