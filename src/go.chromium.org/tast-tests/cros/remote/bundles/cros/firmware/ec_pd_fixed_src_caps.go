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
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ECPDFixedSrcCaps,
		Desc: "Tests if the DUT properly advertises and sources fixed VBUS voltage of 5V/1.5A to a Sink",
		Contacts: []string{
			"chromeos-faft@google.com", // Owning team list
			"bszpila@google.com",       // Test author
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Data:         []string{firmware.ConfigFile},
		Vars:         []string{"servo"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      fixture.NormalMode,
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Timeout:      5 * time.Minute,
		TestBedDeps:  tbdep.ServoPresentAndWorking,
		Attr:         []string{"group:firmware", "firmware_pd_unstable"},
		Params: []testing.Param{{
			Name: "normal_snk",
			Val: firmware.PDTestParams{
				DTS:       firmware.DTSModeOff,
				PowerRole: firmware.RoleSink,
			},
			ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel("boxy")),
		}, {
			Name: "flipcc_snk",
			Val: firmware.PDTestParams{
				CC:        firmware.CCPolarityFlipped,
				DTS:       firmware.DTSModeOff,
				PowerRole: firmware.RoleSink,
			},
			ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel("boxy")),
		}, {
			Name: "dts_snk",
			Val: firmware.PDTestParams{
				DTS:       firmware.DTSModeOn,
				PowerRole: firmware.RoleSink,
			},
			ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel("boxy")),
		}, {
			Name: "flipcc_dts_snk",
			Val: firmware.PDTestParams{
				CC:        firmware.CCPolarityFlipped,
				DTS:       firmware.DTSModeOn,
				PowerRole: firmware.RoleSink,
			},
			ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel("boxy")),
		}},
	})
}

const (
	reconnectionTimeout = 1 * time.Second
	minConnV            = 200
	minRdV              = 700
	maxRdV              = 1160
)

// ECPDFixedSrcCaps connects the servo in sink role and checks if DUT is sourcing 5V/1.5A.
// This action doesn't need support of any PD comm. Here, the test just expects DUT applying
// the correct Type-C advertisement, i.e. Rp_1A5 on both CC lines. Since the servo applies Rd
// on one or both CC lines (depending on the DTS mode), the voltage levels of the CC lines should
// fall within the expected ranges defined in the USB Type-C spec.
func ECPDFixedSrcCaps(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}

	testParams := s.Param().(firmware.PDTestParams)

	if err := firmware.SetupPDTester(ctx, h, testParams, s.OutDir()); err != nil {
		s.Fatal("Failed to configure Servo for PD testing: ", err)
	}

	// Reset connection to be able to measure initially advertised current limit
	if err := h.Servo.ServoCcOff(ctx); err != nil {
		s.Fatal("Failed to disconnect servo from the DUT: ", err)
	}
	if err := h.Servo.ServoCcSnk(ctx); err != nil {
		s.Fatal("Failed to connect servo to the DUT: ", err)
	}

	// Poll for voltage change
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		var cc1VStr, cc2VStr string
		var err error

		if cc1VStr, cc2VStr, err = h.Servo.DUTCCVoltageMV(ctx); err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to measure CC voltage"))
		}

		cc1V, err := strconv.Atoi(cc1VStr)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "cc1 contains incorrect value: "+cc1VStr))
		}

		cc2V, err := strconv.Atoi(cc2VStr)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "cc2 contains incorrect value: "+cc2VStr))
		}

		// The servo emulates both the PD partner and the Type-C cable. A typical Type-C cable
		// (the non-DTS mode case) only connects one CC line to the PD partner; the other one is
		// either left open or pulled down by Ra (an eMarker cable). So, by default, the servo
		// leaves one of the CC lines open.
		if cc1V < minConnV || cc2V < minConnV {
			return errors.New("CC voltage lower than connection detect threshold")
		}
		return nil
	}, &testing.PollOptions{Timeout: reconnectionTimeout}); err != nil {
		s.Fatal("timed out waiting for servo to connect as a sink: ", err)
	}

	// Measure the voltage
	if cc1VStr, cc2VStr, err := h.Servo.DUTCCVoltageMV(ctx); err != nil {
		s.Fatal("Failed to measure CC voltage: ", err)
	} else {
		cc1V, err := strconv.Atoi(cc1VStr)
		if err != nil {
			s.Fatal("cc1 contains incorrect value: "+cc1VStr, err)
		}

		cc2V, err := strconv.Atoi(cc2VStr)
		if err != nil {
			s.Fatal("cc2 contains incorrect value: "+cc2VStr, err)
		}

		if (cc1V < minRdV || cc1V > maxRdV) && (cc2V < minRdV || cc2V > maxRdV) {
			s.Fatal("CC voltage doesn't corespond to 1.5A: ", cc1VStr, ", ", cc2VStr)
		}
	}

	// Check power role
	dutPDState, err := h.Servo.GetDUTPDState(ctx)
	if err != nil {
		s.Fatal("Failed to get DUT PD State: ", err)
	}

	if dutPDState.PowerRole != servo.PowerRoleSRC {
		s.Fatal("DUT should have SRC power role")
	}
}
