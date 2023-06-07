// Copyright 2023 The ChromiumOS Authors
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
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         WilcoChargingFromUSBC,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify typec charging behavior on wilco devices",
		Contacts: []string{
			"chromeos-faft@google.com",
			"cienet-firmware@cienet.corp-partner.google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware"},
		SoftwareDeps: []string{"wilco"},
		Fixture:      fixture.NormalMode,
		Timeout:      30 * time.Minute,
	})
}

/*
WilcoChargingFromUSBC checks that wilco devices can charge from
type-c chargers. To run this test, we used the following setup:
servo_v4/v4p1 type-c with type-c charger > 27W connected, plus
a servo_micro. Note that with servo_type-c, we've seen ethernet
drop, as also pointed out in ticket b/191951909 comment #36. But,
if we bypassed servo_type-c, and connected the ethernet cable
directly to a Wilco dut's ethernet port, or using a usb-a to ethernet
adapter, the networking was fine.
*/
func WilcoChargingFromUSBC(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to get config: ", err)
	}

	// For debugging purposes, log dut connection type.
	dutConnType, err := h.Servo.GetDUTConnectionType(ctx)
	if err != nil {
		s.Fatal("Failed to find dut connection type: ", err)
	}
	s.Logf("DUT connection type: %s", dutConnType)

	s.Log("Checking for initial power supply info")
	for _, device := range []string{"ac", "battery"} {
		state, err := verifyPowerSupplyInfo(ctx, h, device)
		if err != nil {
			s.Fatalf("Failed to verify initial %s state: %v", device, err)
		}
		s.Logf("Found initial %s state: %s", device, state)
	}

	s.Log("Opening CCD")
	if err := h.OpenCCD(ctx, false, true); err != nil {
		s.Fatal("Failed to open ccd: ", err)
	}

	s.Log("Rebooting the DUT with cold reset")
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateReset); err != nil {
		s.Fatal("Failed to reboot with cold reset: ", err)
	}
	if err := func() error {
		s.Log("Waiting for reconnection to DUT")
		waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, 2*time.Minute)
		defer cancelWaitConnect()
		if err := h.WaitConnect(waitConnectCtx); err != nil {
			return err
		}
		return nil
	}(); err != nil {
		s.Fatal("Failed to reconnect to DUT: ", err)
	}

	s.Log("Checking for power supply info after CCD open")
	for _, device := range []string{"ac", "battery"} {
		state, err := verifyPowerSupplyInfo(ctx, h, device)
		if err != nil {
			s.Fatalf("Failed to verify %s state after ccd open: %v", device, err)
		}
		s.Logf("Found %s state after ccd open: %s", device, state)
	}

	s.Log("Disabling the factory mode")
	if err := h.DUT.Conn().CommandContext(ctx, "gsctool", "-a", "-F", "disable").Run(ssh.DumpLogOnError); err != nil {
		s.Fatal("Failed to run 'gsctool -a -F disable': ", err)
	}

	s.Log("Sleeping for 5 seconds")
	if err := testing.Sleep(ctx, 5*time.Second); err != nil {
		s.Fatal("Failed to sleep for 5 seconds: ", err)
	}

	s.Log("Powering off the DUT")
	if err := h.DUT.Conn().CommandContext(ctx, "poweroff").Start(); err != nil {
		s.Fatal("Failed to run poweroff cmd: ", err)
	}
	s.Log("Waiting for DUT to power off")
	waitUnreachableCtx, cancelUnreachable := context.WithTimeout(ctx, 2*time.Minute)
	defer cancelUnreachable()
	if err := h.DUT.WaitUnreachable(waitUnreachableCtx); err != nil {
		s.Fatal("DUT did not power down: ", err)
	}

	s.Log("Removing charger")
	if err := h.SetDUTPower(ctx, false); err != nil {
		s.Fatal("Unable to remove charger: ", err)
	}

	s.Log("Sleeping for 30 seconds")
	if err := testing.Sleep(ctx, 30*time.Second); err != nil {
		s.Fatal("Failed to sleep for 30 seconds: ", err)
	}

	s.Log("Connecting charger")
	if err := h.SetDUTPower(ctx, true); err != nil {
		s.Fatal("Unable to connect charger: ", err)
	}

	s.Logf("Pressing power button for %s seconds to wake DUT", servo.Dur(h.Config.HoldPwrButtonPowerOn))
	if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.Dur(h.Config.HoldPwrButtonPowerOn)); err != nil {
		s.Fatal("Failed to press power key via servo: ", err)
	}
	if err := func() error {
		s.Log("Waiting for reconnection to DUT")
		waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, 2*time.Minute)
		defer cancelWaitConnect()
		if err := h.WaitConnect(waitConnectCtx); err != nil {
			return err
		}
		return nil
	}(); err != nil {
		s.Fatal("Failed to reconnect to DUT: ", err)
	}

	s.Log("Sleeping for 5 seconds")
	if err := testing.Sleep(ctx, 5*time.Second); err != nil {
		s.Fatal("Failed to sleep for 5 seconds: ", err)
	}

	s.Log("Verifying charger detected and battery charges")
	if _, err := verifyPowerSupplyInfo(ctx, h, "ac", "yes"); err != nil {
		s.Fatal("Failed to verify ac online: ", err)
	}
	if _, err := verifyPowerSupplyInfo(ctx, h, "battery", "Charging", "Fully charged"); err != nil {
		s.Fatal("Failed to verify battery charging: ", err)
	}
}

// verifyPowerSupplyInfo runs the host command 'power_supply_info', matches for the specified
// regexp pattern based on the device name, and then verifies if the device is at the expected
// state. If no arguments are passed in for the expected states, the current state is returned.
func verifyPowerSupplyInfo(ctx context.Context, h *firmware.Helper, device string, expStates ...string) (string, error) {
	var regex string
	switch device {
	case "ac":
		regex = `online:\s+(.+)`
	case "battery":
		regex = `state:\s+(.+)`
	default:
		return "", errors.Errorf("got unknown device %s", device)
	}
	expMatch := regexp.MustCompile(regex)
	out, err := h.DUT.Conn().CommandContext(ctx, "power_supply_info").Output()
	if err != nil {
		return "", errors.Wrap(err, "failed to get power supply info")
	}
	matches := expMatch.FindStringSubmatch(string(out))
	if len(matches) != 2 {
		return "", errors.Errorf("failed to match regex %q in %q", expMatch, string(out))
	}
	currentState := strings.TrimSpace(matches[1])
	if expStates == nil {
		return currentState, nil
	}
	for _, state := range expStates {
		if currentState == state {
			return currentState, nil
		}
	}
	return "", errors.Errorf("unexpected %s state, got: %s, want: %q", device, currentState, expStates)
}
