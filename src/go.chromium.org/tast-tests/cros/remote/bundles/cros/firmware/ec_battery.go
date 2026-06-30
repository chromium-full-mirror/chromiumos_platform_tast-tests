// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ECBattery,
		Desc: "Check battery temperature, voltage, and current readings",
		Contacts: []string{
			"chromeos-faft@google.com",
			"jbettis@chromium.org",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT

		TestBedDeps:  tbdep.ServoPresentAndWorking,
		Attr:         []string{"group:firmware", "firmware_ec", "firmware_stressed", "firmware_meets_kpi", "firmware_ec_ro", "firmware_ec_rw"},
		Fixture:      fixture.NormalMode,
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.Battery()),
	})
}

func abs(i int) int {
	if i < 0 {
		return -i
	}
	return i
}

func ECBattery(ctx context.Context, s *testing.State) {
	const (
		BatteryVoltageReadingFPTemplate     = "/sys/class/power_supply/%s/voltage_now"
		BatteryCurrentReadingFPTemplate     = "/sys/class/power_supply/%s/current_now"
		VoltageMVErrorMargin                = 300
		CurrentMAErrorMargin                = 300
		BatteryTemperatureCelsiusUpperBound = 70 // Temperature in Celsius
		BatteryTemperatureCelsiusLowerBound = 0  // Temperature in Celsius
		BatteryNameLookupScript             = `
			for path in $(grep -ilH --color=no Battery /sys/class/power_supply/*/type) ; do
				batteryName=$(basename $(dirname $path))
				if [   -e /sys/class/power_supply/$batteryName/status \
					-a -e /sys/class/power_supply/$batteryName/voltage_now \
					-a -e /sys/class/power_supply/$batteryName/current_now ] ; then
					echo $batteryName
					break
				fi
			done
		`
	)

	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servod")
	}

	s.Log("Checking for battery info in sysfs")
	batteryNameOut, err := h.DUT.Conn().CommandContext(ctx, "bash", "-c", BatteryNameLookupScript).Output()
	if err != nil {
		s.Fatal("Failed to retrieve battery info from sysfs: ", err)
	}

	batteryName := bytes.TrimSuffix(batteryNameOut, []byte("\n"))
	if batteryName == nil {
		s.Fatal("Cannot find battery in sysfs or device does not have battery installed")
	}

	s.Log("Battery name is ", string(batteryName))
	batteryVoltageFP := fmt.Sprintf(BatteryVoltageReadingFPTemplate, batteryName)
	batteryCurrentFP := fmt.Sprintf(BatteryCurrentReadingFPTemplate, batteryName)

	type comparisonTestCase struct {
		metric       string
		unit         string
		servoControl servo.IntControl
		sysfsPath    string
		errorMargin  int
	}

	for _, tc := range []comparisonTestCase{
		{"voltage", "mV", servo.BatteryVoltageMV, batteryVoltageFP, VoltageMVErrorMargin},
		{"current", "mA", servo.BatteryCurrentMA, batteryCurrentFP, CurrentMAErrorMargin},
	} {
		s.Logf("Checking if %s from sysfs matches servo", tc.metric)
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			servoReading, err := h.Servo.GetInt(ctx, tc.servoControl)
			if err != nil {
				return errors.Wrapf(err, "failed to read battery %s from servo", tc.metric)
			}

			kernelReadingOut, err := h.DUT.Conn().CommandContext(ctx, "cat", tc.sysfsPath).Output()
			if err != nil {
				return errors.Wrapf(err, "failed to read battery %s from servo", tc.metric)
			}
			kernelReadingOut = bytes.TrimSuffix(kernelReadingOut, []byte("\n"))
			kernelReading, err := strconv.Atoi(string(kernelReadingOut))
			if err != nil {
				return errors.Wrapf(err, "failed to parse kernel %s reading value %s", tc.metric, kernelReadingOut)
			}

			// Kernel gives values in micro-units, convert to milli-units here.
			kernelReading = kernelReading / 1000

			s.Logf("Battery %s reading from kernel: %d%s", tc.metric, kernelReading, tc.unit)
			s.Logf("Battery %s reading from servo: %d%s", tc.metric, servoReading, tc.unit)

			if abs(abs(servoReading)-abs(kernelReading)) > tc.errorMargin {
				return errors.Errorf("reading from servo (%d%s) and kernel (%d%s) mismatch beyond %d%s error margin",
					servoReading, tc.unit, kernelReading, tc.unit, tc.errorMargin, tc.unit)
			}
			return nil
		}, &testing.PollOptions{Timeout: time.Minute, Interval: 10 * time.Second}); err != nil {
			s.Fatalf("Failed to verify %s after retries: %v", tc.metric, err)
		}
	}

	s.Log("Checking if battery temperature is reasonable")
	batteryTemperature, err := h.Servo.GetFloat(ctx, servo.BatteryTemperatureCelsius)
	if err != nil {
		s.Fatal("Failed to read battery temperature from servo: ", err)
	}
	s.Log("Battery temperature: ", batteryTemperature, " C")

	if batteryTemperature > BatteryTemperatureCelsiusUpperBound ||
		batteryTemperature < BatteryTemperatureCelsiusLowerBound {
		s.Fatalf("Abnormal battery temperature %0.2f (should be within %d-%d C)",
			batteryTemperature,
			BatteryTemperatureCelsiusLowerBound,
			BatteryTemperatureCelsiusUpperBound)
	}
}
