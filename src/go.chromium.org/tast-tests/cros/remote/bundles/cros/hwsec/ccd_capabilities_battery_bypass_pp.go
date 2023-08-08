// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwsec

import (
	"context"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type cCDCapabilitiesBatteryBypassPPParam struct {
	capState                       servo.CCDCapState
	isBatteryConnected             bool
	expectPhysicalPresenceRequired bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func: CCDCapabilitiesBatteryBypassPP,
		Desc: "Test to verify BatteryBypassPP CCD capability locks out restricted console commands",
		Attr: []string{"group:firmware", "group:hwsec", "firmware_cr50"},
		Contacts: []string{
			"cros-hwsec@google.com",
			"mvertescher@google.com",
			"chromeos-faft@google.com",
		},
		BugComponent: "b:1188704",
		Fixture:      fixture.NormalMode,
		HardwareDeps: hwdep.D(hwdep.GSCUART()),
		SoftwareDeps: []string{"gsc", "reboot"},
		Timeout:      2 * time.Minute,
		Vars:         []string{"servo"},
		Params: []testing.Param{{
			Name: "cap_default_and_battery_connected",
			Val: cCDCapabilitiesBatteryBypassPPParam{
				capState:                       servo.CapDefault,
				isBatteryConnected:             true,
				expectPhysicalPresenceRequired: true,
			},
		}, {
			Name: "cap_default_and_battery_disconnected",
			Val: cCDCapabilitiesBatteryBypassPPParam{
				capState:                       servo.CapDefault,
				isBatteryConnected:             false,
				expectPhysicalPresenceRequired: false,
			},
		}, {
			Name: "cap_always_and_battery_connected",
			Val: cCDCapabilitiesBatteryBypassPPParam{
				capState:                       servo.CapAlways,
				isBatteryConnected:             true,
				expectPhysicalPresenceRequired: true,
			},
		}, {
			Name: "cap_always_and_battery_disconnected",
			Val: cCDCapabilitiesBatteryBypassPPParam{
				capState:                       servo.CapAlways,
				isBatteryConnected:             false,
				expectPhysicalPresenceRequired: false,
			},
		}, {
			Name:              "cap_unless_locked_and_battery_connected",
			ExtraHardwareDeps: hwdep.D(hwdep.HasGSCCr50()),
			Val: cCDCapabilitiesBatteryBypassPPParam{
				capState:                       servo.CapUnlessLocked,
				isBatteryConnected:             true,
				expectPhysicalPresenceRequired: true,
			},
		}, {
			Name:              "cap_unless_locked_and_battery_disconnected",
			ExtraHardwareDeps: hwdep.D(hwdep.HasGSCCr50()),
			Val: cCDCapabilitiesBatteryBypassPPParam{
				capState:                       servo.CapUnlessLocked,
				isBatteryConnected:             false,
				expectPhysicalPresenceRequired: true,
			},
		}, {
			Name: "cap_if_opened_and_battery_connected",
			Val: cCDCapabilitiesBatteryBypassPPParam{
				capState:                       servo.CapIfOpened,
				isBatteryConnected:             true,
				expectPhysicalPresenceRequired: true,
			},
		}, {
			Name: "cap_if_opened_and_battery_disconnected",
			Val: cCDCapabilitiesBatteryBypassPPParam{
				capState:                       servo.CapIfOpened,
				isBatteryConnected:             false,
				expectPhysicalPresenceRequired: true,
			},
		}},
	})
}

func CCDCapabilitiesBatteryBypassPP(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	userParams := s.Param().(cCDCapabilitiesBatteryBypassPPParam)

	if err := h.OpenCCD(ctx, false, false); err != nil {
		s.Fatal("Failed to open CCD: ", err)
	}

	// Make sure that CCD capabilities are in their default states
	if err := h.Servo.RunCR50Command(ctx, "ccd reset"); err != nil {
		s.Fatal("Failed to reset CCD: ", err)
	}

	// Open CCD and reset to factory when finished
	defer func() {
		if err := h.OpenCCD(ctx, true, true); err != nil {
			s.Fatal("Failed to open CCD: ", err)
		}
	}()

	// In addition to setting `BatteryBypassPP` based on the test being run,
	// always make sure that `OpenNoTPMWipe` and `UnlockNoReboot` are set.
	ccdSettings := map[servo.CCDCap]servo.CCDCapState{
		"BatteryBypassPP": userParams.capState,
		"OpenNoTPMWipe":   servo.CapAlways,
		"UnlockNoReboot":  servo.CapAlways,
	}
	if err := h.Servo.SetCCDCapability(ctx, ccdSettings); err != nil {
		s.Fatal("Failed to set CCD capability states: ", err)
	}

	// Set the "batteryConnected" state accordingly
	if userParams.isBatteryConnected {
		if err := h.Servo.CheckGSCCommandOutput(ctx, "bpforce connect", []string{"batt pres:"}); err != nil {
			s.Fatal("Failed to run `bpforce connect`: ", err)
		}
	} else {
		if err := h.Servo.CheckGSCCommandOutput(ctx, "bpforce disconnect", []string{"batt pres:"}); err != nil {
			s.Fatal("Failed to run `bpforce disconnect`: ", err)
		}
	}

	if err := h.Servo.LockCCD(ctx); err != nil {
		s.Fatal("Failed to lock CCD: ", err)
	}

	testIfPhysicalPresenceIsRequired(ctx, s, userParams.expectPhysicalPresenceRequired)
}

func testIfPhysicalPresenceIsRequired(ctx context.Context, s *testing.State, expectPhysicalPresenceRequired bool) {
	if err := testIfPhysicalPresenceRequiredForCCDOpen(ctx, s, expectPhysicalPresenceRequired); err != nil {
		s.Fatal("Failed verify physical presence for CCD open: ", err)
	}

	// GoBigSleepLint: Sleep 5 seconds here to wait for the physical presence
	// press state machine to timeout and reset so other tests don't fail.
	testing.Sleep(ctx, 5*time.Second)

	if isRequired, err := isPhysicalPresenceRequiredForGsctool(ctx, s); expectPhysicalPresenceRequired != isRequired {
		s.Fatal("Failed to verify if `gsctool` requires physical presence or not: ", err)
	}
}

// testIfPhysicalPresenceRequiredForCCDOpen checks if physical presence is
// required for `ccd open` with a hint about whether it is expected or not.
func testIfPhysicalPresenceRequiredForCCDOpen(ctx context.Context, s *testing.State, shouldBeRequired bool) error {
	h := s.FixtValue().(*fixture.Value).Helper
	regex := "Console unlock allowed|(?i)CCD Opened"
	if shouldBeRequired {
		// If `ccd open` is allowed, the GSC will prompt for the user to press.
		// If `ccd open` is not allowed, an access denied string will be
		// printed.
		regex = "Press the physical button now!|Access Denied"
	}

	if err := h.Servo.CheckGSCCommandOutput(ctx, "ccd open", []string{regex}); err != nil {
		return errors.Wrap(err, "expected `ccd open` to check for physical presense = "+strconv.FormatBool(shouldBeRequired))
	}
	return nil
}

// isPhysicalPresenceRequiredForGsctool checks if physical presence is required
// for `gsctool` by attempting to disable factory mode.
func isPhysicalPresenceRequiredForGsctool(ctx context.Context, s *testing.State) (bool, error) {
	if err := gsctoolDisableFactoryMode(ctx, s); err != nil {
		return true, err
	}
	return false, nil
}

// gsctoolDisableFactoryMode calls `gsctool -a -F disable` to disable factory mode.
func gsctoolDisableFactoryMode(ctx context.Context, s *testing.State) error {
	h := s.FixtValue().(*fixture.Value).Helper
	if _, err := h.DUT.Conn().CommandContext(ctx, "gsctool", "-a", "-F", "disable").Output(ssh.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to run 'gsctool -aF disable'")
	}
	return nil
}
