// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwsec

import (
	"context"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/dut"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type cCDCapabilitiesRebootECAP struct {
	capState                     servo.CCDCapState
	commandsSucceedWhenCcdOpen   bool
	commandsSucceedWhenCcdLocked bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func: CCDCapabilitiesRebootECAP,
		Desc: "Test to verify RebootECAP CCD capability by using `ecrst` and `sysrst` GSC commands",
		Attr: []string{"group:firmware", "group:hwsec", "firmware_cr50"},
		Contacts: []string{
			"chromeos-faft@google.com",
			"cros-hwsec@google.com",
			"mvertescher@google.com",
		},
		BugComponent: "b:1188704",
		Fixture:      fixture.NormalMode,
		HardwareDeps: hwdep.D(hwdep.GSCUART()),
		SoftwareDeps: []string{"gsc", "reboot"},
		// This test can take a while since the DUT can reboot multiple times and
		// that can be slow for certain devices
		Timeout: 8 * time.Minute,
		Vars:    []string{"servo"},
		Params: []testing.Param{{
			Name: "cap_default",
			Val: cCDCapabilitiesRebootECAP{
				capState:                     servo.CapDefault,
				commandsSucceedWhenCcdOpen:   true,
				commandsSucceedWhenCcdLocked: false,
			},
		}, {
			Name: "cap_always",
			Val: cCDCapabilitiesRebootECAP{
				capState:                     servo.CapAlways,
				commandsSucceedWhenCcdOpen:   true,
				commandsSucceedWhenCcdLocked: true,
			},
		}, {
			Name: "cap_unless_locked",
			Val: cCDCapabilitiesRebootECAP{
				capState:                     servo.CapUnlessLocked,
				commandsSucceedWhenCcdOpen:   true,
				commandsSucceedWhenCcdLocked: false,
			},
		}, {
			Name: "cap_if_opened",
			Val: cCDCapabilitiesRebootECAP{
				capState:                     servo.CapIfOpened,
				commandsSucceedWhenCcdOpen:   true,
				commandsSucceedWhenCcdLocked: false,
			},
		}},
	})
}

func CCDCapabilitiesRebootECAP(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	userParams := s.Param().(cCDCapabilitiesRebootECAP)

	if err := h.OpenCCD(ctx, true, false); err != nil {
		s.Fatal("Failed to open CCD: ", err)
	}

	ccdSettings := map[servo.CCDCap]servo.CCDCapState{"RebootECAP": userParams.capState}
	if err := h.Servo.SetCCDCapability(ctx, ccdSettings); err != nil {
		s.Fatal("Failed to set `RebootECAP` capability state: ", err)
	}

	if err := verifyGscEcrstCommand(ctx, s, userParams.commandsSucceedWhenCcdOpen); err != nil {
		s.Fatal("Failed to verify if the EC can be reset when CCD open: ", err)
	}

	if err := verifyGscSysrstCommand(ctx, s, userParams.commandsSucceedWhenCcdOpen); err != nil {
		s.Fatal("Failed to verify if the AP can be reset when CCD open: ", err)
	}

	if err := h.Servo.LockCCD(ctx); err != nil {
		s.Fatal("Failed to lock CCD: ", err)
	}

	// Open CCD when finished
	defer func() {
		if err := h.OpenCCD(ctx, true, false); err != nil {
			s.Fatal("Failed to open CCD: ", err)
		}
	}()

	if err := verifyGscEcrstCommand(ctx, s, userParams.commandsSucceedWhenCcdLocked); err != nil {
		s.Fatal("Failed to verify if the EC can be reset when CCD locked: ", err)
	}

	if err := verifyGscSysrstCommand(ctx, s, userParams.commandsSucceedWhenCcdLocked); err != nil {
		s.Fatal("Failed to verify if the AP can be reset when CCD locked: ", err)
	}
}

func verifyGscEcrstCommand(ctx context.Context, s *testing.State, expectSuccess bool) error {
	h := s.FixtValue().(*fixture.Value).Helper
	oldID, err := dut.ReadBootID(ctx, h.DUT.Conn())
	if err != nil {
		return errors.Wrap(err, "failed to read boot id")
	}

	if err := runGscEcrstPulseCommand(ctx, s, expectSuccess); err != nil {
		return errors.Wrap(err, "failed to run GSC ecrst pulse command")
	}

	// Make sure the DUT has booted before trying to connect again
	testing.ContextLog(ctx, "Waiting to ensure DUT booted after `ecrst` pulse")
	if err := h.EnsureDUTBooted(ctx); err != nil {
		return errors.Wrap(err, "failed to ensure DUT booted after running `ecrst pulse` GSC command")
	}
	testing.ContextLog(ctx, "DUT booted successfully")

	if expectSuccess {
		// TODO(mvertescher): We need to make sure that the EC actually rebooted
		// here, but for now we can just check that the AP rebooted because this
		// also occurs when running `ecrst pulse`.
		newID, err := dut.ReadBootID(ctx, h.DUT.Conn())
		if err != nil {
			return errors.Wrap(err, "failed to read boot id")
		}
		if oldID == newID {
			return errors.New("failed to reset EC using `ecrst` command")
		}
	}

	return nil
}

// verifyGscSysrstCommand calls sysrst on the GSC that resets the AP and
// verifies that the reset occurred by checking the boot ID of the AP.
func verifyGscSysrstCommand(ctx context.Context, s *testing.State, expectSuccess bool) error {
	h := s.FixtValue().(*fixture.Value).Helper
	oldID, err := dut.ReadBootID(ctx, h.DUT.Conn())
	if err != nil {
		return errors.Wrap(err, "failed to read boot id")
	}

	if err := runGscSysrstPulseCommand(ctx, s, expectSuccess); err != nil {
		return errors.Wrap(err, "failed to run GSC `sysrst pulse` command")
	}

	// Make sure the DUT has booted before trying to connect again
	testing.ContextLog(ctx, "Waiting to ensure DUT booted after `sysrst` pulse")
	if err := h.EnsureDUTBooted(ctx); err != nil {
		return errors.Wrap(err, "failed to ensure DUT booted after running `sysrst pulse` GSC command")
	}
	testing.ContextLog(ctx, "DUT booted successfully")

	if expectSuccess {
		newID, err := dut.ReadBootID(ctx, h.DUT.Conn())
		if err != nil {
			return errors.Wrap(err, "failed to read boot id")
		}
		if oldID == newID {
			return errors.New("failed to reset AP using `sysrst` command")
		}
	}

	return nil
}

// runGscEcrstPulseCommand attempts to call the GSC ecrst command with an
// argument to pulse for 1000ms. An `expectSuccess` parameter is required so
// success can be verified. An error is returned if this function fails.
func runGscEcrstPulseCommand(ctx context.Context, s *testing.State, expectSuccess bool) error {
	h := s.FixtValue().(*fixture.Value).Helper
	command := "ecrst pulse 1000"
	regex := "Pulsing EC reset|"
	failureRegex := "Access Denied"
	if !expectSuccess {
		regex = failureRegex
	}

	if err := h.Servo.CheckGSCCommandOutput(ctx, command, []string{regex}); err != nil {
		return errors.Wrap(err, "failed to match GSC command output, expected command `"+command+"` to succeed = "+strconv.FormatBool(expectSuccess))
	}
	return nil
}

// runGscSysrstPulseCommand attempts to call the GSC sysrst command with an
// argument to pulse for 1000ms. An `expectSuccess` parameter is required so
// success can be verified. An error is returned if this function fails.
func runGscSysrstPulseCommand(ctx context.Context, s *testing.State, expectSuccess bool) error {
	h := s.FixtValue().(*fixture.Value).Helper
	command := "sysrst pulse 1000"
	regex := "Pulsing AP reset|"
	failureRegex := "Access Denied"
	if !expectSuccess {
		regex = failureRegex
	}

	if err := h.Servo.CheckGSCCommandOutput(ctx, command, []string{regex}); err != nil {
		return errors.Wrap(err, "failed to match GSC command output, expected command `"+command+"` to succeed = "+strconv.FormatBool(expectSuccess))
	}
	return nil
}
