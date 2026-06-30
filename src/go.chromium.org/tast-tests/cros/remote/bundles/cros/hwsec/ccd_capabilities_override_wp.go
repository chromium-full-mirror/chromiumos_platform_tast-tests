// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwsec

import (
	"context"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type cCDCapabilitiesOverrideWP struct {
	capState                       servo.CCDCapState
	wpCommandSucceedsWhenCCDOpen   bool
	wpCommandSucceedsWhenCCDLocked bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func: CCDCapabilitiesOverrideWP,
		Desc: "Test to verify OverrideWP CCD capability controls access to the `wp` console command",
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
		TestBedDeps:  tbdep.ServoPresentAndWorking,
		Params: []testing.Param{{
			Name: "cap_default",
			Val: cCDCapabilitiesOverrideWP{
				capState:                       servo.CapDefault,
				wpCommandSucceedsWhenCCDOpen:   true,
				wpCommandSucceedsWhenCCDLocked: false,
			},
		}, {
			Name: "cap_always",
			Val: cCDCapabilitiesOverrideWP{
				capState:                       servo.CapAlways,
				wpCommandSucceedsWhenCCDOpen:   true,
				wpCommandSucceedsWhenCCDLocked: true,
			},
		}, {
			Name:              "cap_unless_locked",
			ExtraHardwareDeps: hwdep.D(hwdep.HasGSCCr50()),
			Val: cCDCapabilitiesOverrideWP{
				capState:                       servo.CapUnlessLocked,
				wpCommandSucceedsWhenCCDOpen:   true,
				wpCommandSucceedsWhenCCDLocked: false,
			},
		}, {
			Name: "cap_if_opened",
			Val: cCDCapabilitiesOverrideWP{
				capState:                       servo.CapIfOpened,
				wpCommandSucceedsWhenCCDOpen:   true,
				wpCommandSucceedsWhenCCDLocked: false,
			},
		}},
	})
}

func CCDCapabilitiesOverrideWP(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	userParams := s.Param().(cCDCapabilitiesOverrideWP)

	if err := h.OpenCCD(ctx, true, false); err != nil {
		s.Fatal("Failed to open CCD: ", err)
	}

	originalFWWPState, err := h.Servo.GetString(ctx, servo.FWWPState)
	if err != nil {
		s.Fatal("Failed to get FW WP state: ", err)
	}

	// Restore the original FW WP state after the test has finished
	defer func() {
		// Convert state to a value Servo can set
		if originalFWWPState == "off" {
			originalFWWPState = string(servo.FWWPStateForceOff)
		} else if originalFWWPState == "on" {
			originalFWWPState = string(servo.FWWPStateForceOn)
		}

		if err := setFWWPState(ctx, s, originalFWWPState); err != nil {
			s.Fatal("Failed to restore FW WP state: ", err)
		}
	}()

	if err := resetFWWPState(ctx, s); err != nil {
		s.Fatal("Failed reset FW WP state: ", err)
	}

	ccdSettings := map[servo.CCDCap]servo.CCDCapState{"OverrideWP": userParams.capState}
	if err := h.Servo.SetCCDCapability(ctx, ccdSettings); err != nil {
		s.Fatal("Failed to set `OverrideWP` capability state: ", err)
	}

	if err := verifyGscWpCommand(ctx, s, userParams.wpCommandSucceedsWhenCCDOpen); err != nil {
		s.Fatal("Failed verify GSC `wp` command behavior: ", err)
	}

	if err := resetFWWPState(ctx, s); err != nil {
		s.Fatal("Failed reset FW WP state: ", err)
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

	if err := verifyGscWpCommand(ctx, s, userParams.wpCommandSucceedsWhenCCDLocked); err != nil {
		s.Fatal("Failed verify GSC `wp` command behavior: ", err)
	}
}

// verifyGscWpCommand attempts to use the GSC `wp` command to set WP to on then
// off to verify that the command is working properly. Servo is used to
// independently verify the WP state set by the command. If `expectSuccess` is
// false, the `wp` command is expected to fail and the WP is not checked via
// Servo.
func verifyGscWpCommand(ctx context.Context, s *testing.State, expectSuccess bool) error {
	if err := runGscWpCommand(ctx, s, true, expectSuccess); err != nil {
		return errors.Wrap(err, "failed to run GSC `wp` command to enable write protect")
	}
	if expectSuccess {
		if err := verifyFWWPState(ctx, s, true); err != nil {
			return errors.Wrap(err, "failed to verify FW WP state as on")
		}
	}

	if err := runGscWpCommand(ctx, s, false, expectSuccess); err != nil {
		return errors.Wrap(err, "failed to run GSC `wp` command to disabled write protect")
	}
	if expectSuccess {
		if err := verifyFWWPState(ctx, s, false); err != nil {
			return errors.Wrap(err, "failed to verify FW WP state as off")
		}
	}
	return nil
}

// resetFWWPState uses Servo to reset the FW WP state.
func resetFWWPState(ctx context.Context, s *testing.State) error {
	return setFWWPState(ctx, s, "reset")
}

// setFWWPState uses Servo to set the FW WP state.
func setFWWPState(ctx context.Context, s *testing.State, value string) error {
	h := s.FixtValue().(*fixture.Value).Helper
	testing.ContextLogf(ctx, "Setting %q to %q", servo.FWWPState, value)
	return h.Servo.SetString(ctx, servo.FWWPState, value)
}

// runGscWpCommand attempts to call the GSC write protect command with an
// argument to set the WP to on or off. An `expectSuccess` parameter is required
// so success can be verified. An error is returned if this function fails.
func runGscWpCommand(ctx context.Context, s *testing.State, wpOn, expectSuccess bool) error {
	h := s.FixtValue().(*fixture.Value).Helper
	command := "wp on"
	if !wpOn {
		command = "wp off"
	}
	regex := "Flash WP:"
	failureRegex := "Access Denied"
	if !expectSuccess {
		regex = failureRegex
	}

	if err := h.Servo.CheckGSCCommandOutput(ctx, command, []string{regex}); err != nil {
		return errors.Wrap(err, "failed to match GSC command output, expected command `"+command+"` to succeed = "+strconv.FormatBool(expectSuccess))
	}
	return nil
}

// verifyFWWPState checks that the FW WP state returned via Servo matches the
// `expectOn` parameter.
func verifyFWWPState(ctx context.Context, s *testing.State, expectOn bool) error {
	isFWWPStateEnabled, err := getFWWPState(ctx, s)
	if err != nil {
		return errors.Wrap(err, "failed to get FW WP state")
	}
	if isFWWPStateEnabled != expectOn {
		return errors.Wrap(err, "expected FW WP state = "+strconv.FormatBool(expectOn)+", but found state = "+strconv.FormatBool(isFWWPStateEnabled))
	}
	return nil
}

// getFWWPState uses Servo to get the FW WP state, either `true` if enabled or
// `false` if disabled.
func getFWWPState(ctx context.Context, s *testing.State) (bool, error) {
	h := s.FixtValue().(*fixture.Value).Helper

	state, err := h.Servo.GetString(ctx, servo.FWWPState)
	if err != nil {
		return true, errors.Wrap(err, "failed to get FW WP state value")
	}

	switch servo.FWWPStateValue(state) {
	// Strings returned when using Servo to force FW WP on/off
	case servo.FWWPStateForceOn:
		return true, nil
	case servo.FWWPStateForceOff:
		return false, nil
	// Strings returned when using GSC to force FW WP on/off
	case "on":
		return true, nil
	case "off":
		return false, nil
	default:
		return false, errors.New("Invalid FW WP state: " + state)
	}
}
