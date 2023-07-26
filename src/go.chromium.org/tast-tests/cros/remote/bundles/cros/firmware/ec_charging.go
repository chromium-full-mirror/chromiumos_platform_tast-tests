// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"strconv"
	"strings"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ECCharging, LacrosStatus: testing.LacrosVariantUnneeded, Desc: "Servo based EC charging control test",
		Contacts: []string{
			"chromeos-faft@google.com",
			"js@semihalf.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		// TODO: When stable, change firmware_unstable to a different attr and add linto@chromium.org to gerrit review.
		Attr:         []string{"group:firmware", "firmware_unstable"},
		Fixture:      "bootModeNormal",
		SoftwareDeps: []string{"chrome"},
		ServiceDeps:  []string{"tast.cros.ui.PowerMenuService"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.Battery()),
	})
}

func chargingInt(raw, suffix string) (value int) {
	raw = strings.TrimSuffix(raw, suffix)
	value, _ = strconv.Atoi(raw)
	return value
}

// setupChan sets the channel / reporting from EC to be quiet.
func setupChan(ctx context.Context, h *firmware.Helper, s *testing.State) func() {
	if err := h.Servo.RunECCommand(ctx, "chan save"); err != nil {
		s.Fatal("Failed to send 'chan save' to EC: ", err)
	}

	if err := h.Servo.RunECCommand(ctx, "chan 0"); err != nil {
		s.Fatal("Failed to send 'chan 0' to EC: ", err)
	}

	return func() {
		s.Log("Restoring channel messaging")
		if err := h.Servo.RunECCommand(ctx, "chan restore"); err != nil {
			s.Fatal("Failed to send 'chan restore' to EC: ", err)
		}
	}
}

func checkCharging(ctx context.Context, h *firmware.Helper, s *testing.State) {
	cs, err := firmware.GetChargingState(ctx, h)
	if err != nil {
		s.Fatal("Failed querying EC: ", err)
	}
	if cs["global.ac"] != "1" {
		s.Fatal("DUT is not plugged to AC charger")
	}

	if cs["global.state"] != "charge" {
		s.Fatal("DUT is not charging (DUT is on AC but does not report charging)")
	}
	if chargingInt(cs["batt.current"], "mA") < 0 {
		s.Fatal("DUT is not charging (batterry current below zero)")
	}
	if (chargingInt(cs["batt.desired_current"], "mA") < 100) &&
		(chargingInt(cs["batt.state_of_charge"], "%") < 100) {
		s.Fatalf("Trickling charging battery? Need more discharge? (desired current: %s)",
			cs["batt.desired_current"])
	}

	// check the requested vs actual values.
	if float32(chargingInt(cs["chg.voltage"], "mV")) >= 1.05*float32(chargingInt(cs["batt.desired_voltage"], "mV")) {
		s.Fatalf("Charger target voltage is too high. (target: %s, battery: %s)",
			cs["chg.voltage"], cs["batt.desired_voltage"])
	}
	if float32(chargingInt(cs["chg.current"], "mA")) >= 1.05*float32(chargingInt(cs["batt.desired_current"], "mA")) {
		s.Fatalf("Charger target current is too high. (target: %s, battery: %s)",
			cs["chg.current"], cs["batt.desired_current"])
	}

	if float32(chargingInt(cs["batt.voltage"], "mV")) >= 1.05*float32(chargingInt(cs["chg.voltage"], "mV")) {
		s.Fatalf("Battery actual voltage is too high. (battery: %s, charger: %s",
			cs["batt.voltage"], cs["chg.voltage"])
	}
	if float32(chargingInt(cs["batt.current"], "mA")) >= 1.05*float32(chargingInt(cs["chg.current"], "mA")) {
		s.Fatalf("Battery actual current is too high. (battery: %s, charger: %s",
			cs["batt.current"], cs["chg.current"])
	}

}

func getBatteryPercent(ctx context.Context, h *firmware.Helper, s *testing.State) int {
	cs, err := firmware.GetChargingState(ctx, h)
	if err != nil {
		s.Fatal("Failed querying EC: ", err)
	}
	return chargingInt(cs["batt.state_of_charge"], "%")

}

// ECCharging discharges the DUT then checks its voltages
// and current to determine its charging circuitry and EC
// reporting is working as intended
func ECCharging(ctx context.Context, s *testing.State) {
	const (
		// TrickleChargingThreshold is the current in mA below which is classified as a trickle charge.
		TrickleChargingThreshold = 100
	)

	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}

	cleanup := setupChan(ctx, h, s) // Make things quiet.
	defer cleanup()

	if getBatteryPercent(ctx, h, s) > 95 {
		// TODO: Convert this code to either poll against target battery percent
		// or use the power facilities (requires test conv)
		s.Log("Initiating battery discharging")
		if err := h.Servo.SetPDRole(ctx, servo.PDRoleSnk); err != nil {
			s.Fatal("Failed to initialize battery discharging: ", err)
		}

		// As the firmware test with bootModeNormal does not receive
		// browser services on its initialization, we cannot easily
		// use Chrome for battery drain procedure. Instead, we can
		// simply spawn stress-ng (which seems to be available in
		// base rootfs) for specified amount of time
		// In the future, it might be more valuable to just create
		// the dedicated stressing service on DUT which will also
		// allow to monitor the battery status live

		const stressingScript = `
			cd /tmp; stress-ng --cpu 32 --timeout 4m
	`
		/*
			s.Log("Stressing CPU to discharge battery")
			if err := h.DUT.Conn().CommandContext(ctx, "bash", "-c", stressingScript).Run(); err != nil {
				s.Fatal("Failed to discharge battery using CPU stress: ", err)
			}
		*/
		s.Log("Whew! That was stressful. Go back to charging")
		if err := h.Servo.SetPDRole(ctx, servo.PDRoleSrc); err != nil {
			s.Fatal("Failed to start charging: ", err)
		}

	}

	checkCharging(ctx, h, s)

}
