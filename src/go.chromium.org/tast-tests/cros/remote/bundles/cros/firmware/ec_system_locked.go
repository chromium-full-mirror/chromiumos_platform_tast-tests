// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

// If this test fails, check that CONFIG_SYSTEM_UNLOCKED is disabled.

func init() {
	testing.AddTest(&testing.Test{
		Func: ECSystemLocked,
		Desc: "This test case verifies that changing the FW write protection state has expected effect in EC sysinfo",
		Contacts: []string{
			"chromeos-faft@google.com",
			"jbettis@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware", "firmware_ec"},
		Fixture:      fixture.DevModeGBB,
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Requirements: []string{"sys-fw-0022-v02"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		Timeout:      12 * time.Minute,
	})
}

func ECSystemLocked(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servod")
	}

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to get config")
	}

	// WP should be disabled at test end.
	defer func() {
		s.Log("Restore the FW write protect to disabled")
		if err := setWPState(ctx, h, false); err != nil {
			s.Fatal("Failed to disable wp at test end: ", err)
		}
	}()

	s.Log("Test wp state with WP disabled")
	if err := verifyECWPStatus(ctx, h, false); err != nil {
		s.Fatal("EC lockstate wrong: ", err)
	}

	s.Log("Test wp state with WP enabled")
	if err := verifyECWPStatus(ctx, h, true); err != nil {
		s.Fatal("EC lockstate wrong: ", err)
	}
}

func setWPState(ctx context.Context, h *firmware.Helper, newState bool) error {
	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		return errors.Wrap(err, "creating mode switcher")
	}

	/*
		Some ITE ECs can only clear their WP status on a power-on reset,
		so changing HW WP requires hard ec reboot to take effect, possible cases:
		init HW | init SW | desired | Steps
		    0   |    0    |    0    |  Return
			0   |    0    |    1    |  SW on -> HW on -> Reboot -> Return
			0   |    1    |    0    |  SW off -> Return
			0   |    1    |    1    |  HW on -> Reboot -> Return
			1   |    0    |    0    |  HW off -> Reboot -> Return
			1   |    0    |    1    |  HW off -> Reboot -> SW on -> HW on -> Reboot -> Return
			1   |    1    |    0    |  HW off -> Reboot -> SW off -> Return
			1   |    1    |    1    |  Return
		Case 6 would require 2 reboots here, but since hw and sw wp should always match,
		and both should be disabled at test start/end, we assume they always correspond.
		Then, we only need to consider cases 1, 2, 7, 8.
	*/

	initialState, err := isHWWPEnabled(ctx, h)
	if err != nil {
		return errors.Wrap(err, "failed to get initial write protect state")
	}
	if initialState == newState {
		testing.ContextLogf(ctx, "WP State already %v, no action required", initialState)
		return nil
	} else if newState { // Need to enable WP from disabled state.
		// Enable SW WP before hardware WP.
		testing.ContextLog(ctx, "Setting sw wp to enable")
		if err := h.Servo.RunECCommand(ctx, "flashwp enable"); err != nil {
			return errors.Wrap(err, "failed to set flashwp enable")
		}
		if err := h.Servo.SetFWWPState(ctx, servo.FWWPStateOn); err != nil {
			return errors.Wrap(err, "failed to enable hw write protect")
		}
		testing.ContextLog(ctx, "Rebooting the DUT")
		if err := ms.ModeAwareReboot(ctx, firmware.ColdReset, firmware.AllowGBBForce); err != nil {
			return errors.Wrap(err, "failed to perform mode aware reboot")
		}
	} else { // Need to disable WP from enabled state.
		if err := h.Servo.SetFWWPState(ctx, servo.FWWPStateOff); err != nil {
			return errors.Wrap(err, "failed to disable hw write protect")
		}
		// Reboot after deasserting hardware write protect pin to deactivate
		// write protect. And then remove software write protect flag.
		testing.ContextLog(ctx, "Rebooting the DUT")
		if err := ms.ModeAwareReboot(ctx, firmware.ColdReset, firmware.AllowGBBForce); err != nil {
			return errors.Wrap(err, "failed to perform mode aware reboot")
		}
		// Disable SW WP after hardware WP.
		testing.ContextLog(ctx, "Setting sw wp to disable")
		if err := h.Servo.RunECCommand(ctx, "flashwp disable"); err != nil {
			return errors.Wrap(err, "failed to set flashwp disable")
		}
	}

	currState, err := isHWWPEnabled(ctx, h)
	if err != nil {
		return errors.Wrap(err, "failed to get new write protect state")
	}

	if currState != newState {
		return errors.Errorf("fw wp state after reboot was %v, want %v", currState, newState)
	}
	testing.ContextLog(ctx, "FW write protect state has been successfully set to ", currState)
	return nil
}

func isHWWPEnabled(ctx context.Context, h *firmware.Helper) (bool, error) {
	state, err := h.Servo.GetString(ctx, servo.FWWPState)
	if err != nil {
		return false, errors.Wrap(err, "failed to get initial write protect state")
	}
	switch servo.FWWPStateValue(state) {
	case servo.FWWPStateOn:
		return true, nil
	case servo.FWWPStateOff:
		return false, nil
	default:
		return false, errors.New("invalid FW WP state: " + state)
	}
}

func verifyECWPStatus(ctx context.Context, h *firmware.Helper, wp bool) error {

	if err := setWPState(ctx, h, wp); err != nil {
		return errors.Wrapf(err, "failed to set wp to %v", wp)
	}

	// Stop interrupting logs/console spam, not essential.
	testing.ContextLog(ctx, "Powering off AP")
	if err := h.Servo.RunECCommand(ctx, "apshutdown"); err != nil {
		testing.ContextLog(ctx, "Failed to shutdown ap: ", err)
	}
	defer func() {
		testing.ContextLog(ctx, "Restarting AP")
		if err := h.Servo.RunECCommand(ctx, "powerbtn"); err != nil {
			testing.ContextLog(ctx, "Failed to restart: ", err)
		}
		if err := h.WaitConnect(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to boot to connect to DUT")
		}
	}()

	testing.ContextLog(ctx, "Checking lock state in sysinfo")
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		out, err := h.Servo.RunECCommandGetOutput(ctx, "sysinfo", []string{`Flags:\s+(locked|unlocked)[^\n]*\n`})
		if err != nil {
			return errors.Wrap(err, "sysinfo failed")
		}
		if wp && out[0][1] != "locked" {
			return errors.Errorf("sysinfo reported wrong flags, got %v want %v", out[0][1], "locked")
		}
		if !wp && out[0][1] != "unlocked" {
			return errors.Errorf("sysinfo reported wrong flags, got %v want %v", out[0][1], "unlocked")
		}
		return nil
	}, &testing.PollOptions{Timeout: 5 * time.Second, Interval: 500 * time.Millisecond}); err != nil {
		return errors.Wrap(err, "looking for sysinfo")
	}

	testing.ContextLog(ctx, "Checking wp_gpio_asserted state in flashinfo")
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		out, err := h.Servo.RunECCommandGetOutput(ctx, "flashinfo", []string{`Flags:.*\n.*\n`})
		if out == nil || err != nil {
			testing.ContextLogf(ctx, "Failed to get flashinfo output, got: %v with err %v", out, err)
			return errors.Wrap(err, "failed to get output from flashinfo")
		} else if err != nil {
			testing.ContextLog(ctx, "flashinfo failed to be fully parsed, trying anyway")
		}
		flashinfoOutput := strings.Join(out[0], "\n")
		testing.ContextLog(ctx, "flashinfo: ", flashinfoOutput)
		oldPattern := regexp.MustCompile(`Flags:.*\n`)
		newPattern := regexp.MustCompile(`wp_gpio_asserted:\s+(ON|OFF)\s*`)

		if newMatch := newPattern.FindStringSubmatch(flashinfoOutput); newMatch != nil {
			testing.ContextLog(ctx, "New format match: ", newMatch)
			if wp && newMatch[1] != "ON" {
				return errors.New("flashinfo reported wp_gpio_asserted OFF when expecting ON, check wp gpio config")
			}
			if !wp && newMatch[1] != "OFF" {
				return errors.New("flashinfo reported wp_gpio_asserted ON when expecting OFF, check wp gpio config")
			}
		} else if oldMatch := oldPattern.FindStringSubmatch(flashinfoOutput); oldMatch != nil {
			testing.ContextLog(ctx, "Old format match: ", oldMatch)
			if wp && !strings.Contains(oldMatch[0], "wp_gpio_asserted") {
				return errors.New("flashinfo reported wp_gpio_asserted OFF when expecting ON, check wp gpio config")
			}
			if !wp && strings.Contains(oldMatch[0], "wp_gpio_asserted") {
				return errors.New("flashinfo reported wp_gpio_asserted ON when expecting OFF, check wp gpio config")
			}
		} else {
			return errors.Errorf("failed to parse flashinfo output, got: %v", flashinfoOutput)
		}

		return nil
	}, &testing.PollOptions{Timeout: 5 * time.Second, Interval: 1 * time.Second}); err != nil {
		return errors.Wrap(err, "looking for flashinfo")
	}

	return nil
}
