// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/services/cros/power"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ECCharging, LacrosStatus: testing.LacrosVariantUnneeded, Desc: "Servo based EC charging control test",
		Contacts: []string{
			"chromeos-faft@google.com",
			"js@semihalf.com",
			"epeers@google.com",
			"jbettis@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		// TODO: When stable, change firmware_unstable to a different attr.
		Attr:         []string{"group:firmware", "firmware_unstable"},
		Fixture:      "bootModeNormal",
		SoftwareDeps: []string{"chrome"},
		ServiceDeps: []string{
			"tast.cros.ui.PowerMenuService",
			"tast.cros.browser.ChromeService",
			"tast.cros.power.BatteryService",
		},
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.Battery()),
		Timeout:      time.Hour,
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
	if chargingInt(cs["batt.current"], "mA") <= 0 {
		s.Fatalf("DUT is not charging (battery current below/equal to zero: %s)", cs["batt.current"])
	}
	if chargingInt(cs["chg.voltage"], "mV") <= 0 {
		s.Fatalf("DUT is not charging (charger voltage below/equal to zero: %s)", cs["chg.voltage"])
	}

	if (chargingInt(cs["batt.desired_current"], "mA") < 100) &&
		(chargingInt(cs["batt.state_of_charge"], "%") < 100) {
		s.Fatalf("Trickle charging battery? Need more discharge? (desired current: %s)",
			cs["batt.desired_current"])
	}

	// check the requested vs actual values.
	if float32(chargingInt(cs["chg.voltage"], "mV")) > 1.05*float32(chargingInt(cs["batt.desired_voltage"], "mV")) {
		s.Fatalf("Charger target voltage is too high. (target: %s, battery: %s)",
			cs["chg.voltage"], cs["batt.desired_voltage"])
	}
	if float32(chargingInt(cs["chg.current"], "mA")) > 1.05*float32(chargingInt(cs["batt.desired_current"], "mA")) {
		s.Fatalf("Charger target current is too high. (target: %s, battery: %s)",
			cs["chg.current"], cs["batt.desired_current"])
	}

	if float32(chargingInt(cs["batt.voltage"], "mV")) > 1.05*float32(chargingInt(cs["chg.voltage"], "mV")) {
		s.Fatalf("Battery actual voltage is too high. (battery: %s, charger: %s)",
			cs["batt.voltage"], cs["chg.voltage"])
	}
	if float32(chargingInt(cs["batt.current"], "mA")) > 1.05*float32(chargingInt(cs["chg.current"], "mA")) {
		s.Fatalf("Battery actual current is too high. (battery: %s, charger: %s)",
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

func disconnectCharger(ctx context.Context, h *firmware.Helper, s *testing.State) {
	s.Log("Stopping power supply")
	if err := h.SetDUTPower(ctx, false); err != nil {
		s.Fatal("Failed to remove charger: ", err)
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		cs, err := firmware.GetChargingState(ctx, h)
		if err != nil {
			s.Fatal("Failed querying EC for charge state: ", err)
		}
		if cs["global.ac"] != "0" {
			return errors.New("Charger is not disconnected yet")
		}
		return nil
	}, &testing.PollOptions{Timeout: time.Minute, Interval: time.Second}); err != nil {
		s.Fatal("Failed to disconnect charger: ", err)
	}
}

func connectCharger(ctx context.Context, h *firmware.Helper, s *testing.State) {
	s.Log("Starting power supply")
	if err := h.SetDUTPower(ctx, true); err != nil {
		s.Fatal("Failed to attach charger: ", err)
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		cs, err := firmware.GetChargingState(ctx, h)
		if err != nil {
			s.Fatal("Failed querying EC for charge state: ", err)
		}
		if cs["global.ac"] != "1" {
			return errors.New("Charger is not attached yet")
		} else if chargingInt(cs["batt.current"], "mA") < 0 {
			return errors.New("Battery still supplying current")
		}

		return nil
	}, &testing.PollOptions{Timeout: time.Minute, Interval: time.Second}); err != nil {
		s.Fatal("Failed to connect charger: ", err)
	}
}

// ECCharging discharges the DUT then checks its voltages
// and current to determine its charging circuitry and EC
// reporting is working as intended
func ECCharging(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}

	cleanup := setupChan(ctx, h, s) // Make things quiet.
	defer cleanup()

	cl, err := rpc.Dial(ctx, h.DUT, s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(ctx)

	// Disable Charge Limit for this test, since it messes with detecting if the
	// battery is correctly charging while plugged in.
	client := power.NewBatteryServiceClient(cl.Conn)
	if _, err := client.New(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to start Battery Service Client: ", err)
	}
	defer client.Close(ctx, &empty.Empty{})
	if _, err := client.StopChargeLimit(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to stop Charge Limit: ", err)
	}

	// Dewatt requested 0mA at 95%. 94% had a request. Picked 93 for a bit of margin.
	targetDischarge := 93

	if getBatteryPercent(ctx, h, s) > targetDischarge {
		s.Log("Initiating battery discharging")
		disconnectCharger(ctx, h, s)

		// As the firmware test with bootModeNormal does not receive
		// browser services on its initialization, we cannot easily
		// use Chrome for battery drain procedure. Instead, we can
		// simply spawn stress-ng (which seems to be available in
		// base rootfs) for specified amount of time.
		// See also battery_service.go:DrainBattery
		const stressingScript = `
			cd /tmp; stress-ng --cpu 32 --timeout 1m
	`
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			s.Log("Stressing CPU to discharge battery")
			if err := h.DUT.Conn().CommandContext(ctx, "bash", "-c", stressingScript).Run(); err != nil {
				s.Fatal("Failed to discharge battery using CPU stress: ", err)
			}

			currentBatteryPercent := getBatteryPercent(ctx, h, s)
			if currentBatteryPercent > targetDischarge {
				return errors.Errorf("Not enough battery discharged: %d, want %d", currentBatteryPercent, targetDischarge)
			}

			return nil
			// poll at 1s since the stress script will block progress for 1 minute
		}, &testing.PollOptions{Timeout: time.Hour, Interval: time.Second}); err != nil {
			s.Fatal("Failed to discharge battery : ", err)
		}

		s.Log("Whew! That was stressful. Go back to charging")
		connectCharger(ctx, h, s)
	}

	checkCharging(ctx, h, s)

}
