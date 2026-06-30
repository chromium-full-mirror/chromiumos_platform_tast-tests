// Copyright 2025 The ChromiumOS Authors
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
		Func: ECPDExitActiveModes,
		Desc: "Tests if the DUT exits alternate modes",
		Contacts: []string{
			"chromeos-faft@google.com",  // Owning team list
			"bszpila@google.com",        // Test author
			"kamilplucinski@google.com", // Test author
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Data:         []string{firmware.ConfigFile},
		Vars:         []string{"servo"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      fixture.NormalMode,
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Timeout:      60 * time.Minute,
		TestBedDeps:  tbdep.ServoPresentAndWorking,
		Attr:         []string{"group:firmware", "firmware_pd_unstable"},
		Params: []testing.Param{{
			Name: "normal",
			Val: firmware.PDTestParams{
				DTS: firmware.DTSModeOff,
			},
		}, {
			Name: "flipcc",
			Val: firmware.PDTestParams{
				CC:  firmware.CCPolarityFlipped,
				DTS: firmware.DTSModeOff,
			},
		}, {
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
		}},
	})
}

// ECPDExitActiveModes perform two tests:
// Enables DP mode with CD pins and check if EC reports DP as disabled after turns off the DUT.
// Enables DP mode with CD pins and check if EC reports DP as disabled after AP direct it to.
func ECPDExitActiveModes(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	subTests := map[string]func(context.Context, *testing.State, *firmware.Helper) error{
		"AP off triger exit of alt modes": ecPDApOffExitActiveModes,
		"AP direct EC to exit alt modes":  ecPDApDirectExitActiveModes,
	}

	for testName, exitActiveModesTest := range subTests {
		s.Run(ctx, testName, func(ctx context.Context, s *testing.State) {
			if err := configurePDTesting(ctx, s, h); err != nil {
				s.Fatalf("%s: Failed to configure servod PD testing: %v", testName, err)
			}
			if err := enableDPMode(ctx, s, h); err != nil {
				s.Fatalf("%s: Failed to enable DP mode: %v", testName, err)
			}
			if err := exitActiveModesTest(ctx, s, h); err != nil {
				s.Fatalf("%s: Failed to exit PD Mode: %v", testName, err)
			}
		})
	}
}

func ecPDApOffExitActiveModes(ctx context.Context, s *testing.State, h *firmware.Helper) error {
	testing.ContextLog(ctx, "Running ECPDApOffExitActiveModes")

	// Turn off the DUT
	firmware.ShutdownDUT(ctx, h)

	// Check if DP is off
	testing.ContextLog(ctx, "Verifying DP is disabled")
	typecInfo, err := h.Servo.GetTypeCByECCommand(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to retrieve type-c information")
	}
	if typecInfo.DPMode != servo.DPDisable {
		return errors.Wrap(err, "type-c DP did not disable")
	}

	// Turn DUT back on
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateOn); err != nil {
		testing.ContextLog(ctx, "Failed to power on DUT: ", err)
	}
	if err := h.WaitConnect(ctx); err != nil {
		return errors.Wrap(err, "failed to boot after test")
	}
	return nil
}

func ecPDApDirectExitActiveModes(ctx context.Context, s *testing.State, h *firmware.Helper) error {
	testing.ContextLog(ctx, "Running ECPDApDirectExitActiveModes")

	// Exit all modes
	portNumber := h.Servo.DUTPDPort()
	dut := h.DUT
	if err := dut.Conn().CommandContext(ctx, "ectool", "typeccontrol", strconv.Itoa(portNumber), "0").Run(); err != nil {
		return errors.Wrap(err, "failed to run Alt Mode command")
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		// Check if DP is off
		testing.ContextLog(ctx, "Verifying DP is disabled")
		typecInfo, err := h.Servo.GetTypeCByECCommand(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to retrieve type-c information")
		}
		if typecInfo.DPMode != servo.DPDisable {
			return errors.Wrap(err, "type-c DP did not disable")
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: 1 * time.Second}); err != nil {
		return errors.Wrap(err, "failed to get cable connected port number")
	}
	return nil
}

func enableDPMode(ctx context.Context, s *testing.State, h *firmware.Helper) error {
	// Enable dp mode
	input := servo.TypeCInfo{DPMode: servo.DPEnable, PinsCDEF: "CD"}
	if err := h.Servo.ServoSetDPConfigs(ctx, &input, servo.MFPrefEnable); err != nil {
		return errors.Wrap(err, "failed to set DP mode to enable")
	}

	return verifyDPMode(ctx, s, h)
}

func verifyDPMode(ctx context.Context, s *testing.State, h *firmware.Helper) error {
	// Check if DP is on
	testing.ContextLog(ctx, "Verifying DP is enabled")
	typecInfo, err := h.Servo.GetTypeCByECCommand(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get DP mode status")
	}
	if typecInfo.DPMode != servo.DPEnable {
		return errors.Wrap(err, "failed to enable DP mode")
	}
	return nil
}

func configurePDTesting(ctx context.Context, s *testing.State, h *firmware.Helper) error {
	if err := h.RequireConfig(ctx); err != nil {
		return errors.Wrap(err, "failed to create config")
	}

	testParams := s.Param().(firmware.PDTestParams)

	if err := firmware.SetupPDTester(ctx, h, testParams, s.OutDir()); err != nil {
		return errors.Wrap(err, "failed to configure servo for PD testing")
	}
	return nil
}
