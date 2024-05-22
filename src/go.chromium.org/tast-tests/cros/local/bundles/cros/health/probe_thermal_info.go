// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package health

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/croshealthd"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type thermalSensorInfo struct {
	Name               string  `json:"name"`
	TemperatureCelsius float64 `json:"temperature_celsius"`
	Source             string  `json:"source"`
}

type thermalInfo struct {
	ThermalSensors []thermalSensorInfo `json:"thermal_sensors"`
}

// The maximum temperature difference we allow between the value gathered by
// healthd and by tast test. This value is arbitrarily decided, and should be
// changed if tests fail consistently.
const maxTemperatureDelta = 10

func init() {
	testing.AddTest(&testing.Test{
		Func:         ProbeThermalInfo,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check that we can probe cros_healthd for thermal info",
		Contacts: []string{
			"cros-tdm-tpe-eng@google.com",
			"yycheng@google.com",
		},
		BugComponent: "b:982097", // ChromeOS > Platform > Enablement > Health
		// TODO(b/317147710): Promote to critical.
		Attr:         []string{"group:mainline", "informational", "group:criticalstaging"},
		SoftwareDeps: []string{"diagnostics"},
		Fixture:      "crosHealthdRunning",
		// TODO(b/336951497): Resume testing on primus if the bug is solved and FW uprevved.
		HardwareDeps: hwdep.D(hwdep.SkipOnModel("primus")),
	})
}

func ProbeThermalInfo(ctx context.Context, s *testing.State) {
	params := croshealthd.TelemParams{Category: croshealthd.TelemCategoryThermal}
	var thermal thermalInfo
	if err := croshealthd.RunAndParseJSONTelem(ctx, params, s.OutDir(), &thermal); err != nil {
		s.Fatal("Failed to get thermal telemetry info: ", err)
	}

	if err := verifyThermalData(ctx, &thermal); err != nil {
		s.Fatal("Failed to validate thermal data, err: ", err)
	}
}

// verifyThermalData verifies that all sensors are reported by healthd.
func verifyThermalData(ctx context.Context, thermal *thermalInfo) error {
	if err := verifySensorTemperatureRange(&thermal.ThermalSensors); err != nil {
		return err
	}

	expectedSysfsSensors, err := getThermalSensorsFromSysFs()
	if err != nil {
		return err
	}
	expectedEcSensors, err := getThermalSensorsFromEctool(ctx)
	if err != nil {
		return err
	}
	expectedSensors := append(expectedSysfsSensors, expectedEcSensors...)
	if err := verifySensors(thermal.ThermalSensors, expectedSensors); err != nil {
		return err
	}

	return nil
}

// verifySensorTemperatureRange verifies that all temperatures read from sensors are reasonable.
func verifySensorTemperatureRange(sensors *[]thermalSensorInfo) error {
	for _, sensor := range *sensors {
		// Arbitrary value for checking temperature reading is reasonable. Values beyond
		// these limits are most likely caused by sensor failure.
		if sensor.TemperatureCelsius < 0 || sensor.TemperatureCelsius > 100 {
			return errors.Errorf("Thermal reading outside threshold of 0-100 Celsius: %f", sensor.TemperatureCelsius)
		}
	}
	return nil
}

// compareSensors compares each field of thermalSensorInfo, and allows for a variation in `TemperatureCelsius` property.
func compareSensors(v1, v2 thermalSensorInfo) bool {
	if v1.Name != v2.Name {
		return false
	}
	if v1.Source != v2.Source {
		return false
	}
	if math.Abs(v1.TemperatureCelsius-v2.TemperatureCelsius) > maxTemperatureDelta {
		return false
	}
	return true
}

// verifySensors checks whether the sensors returned by healthd matches those gathered by tast test.
func verifySensors(gotSensors, expectedSensors []thermalSensorInfo) error {
	// Sort the sensors in order according to it source, then name, then temperature.
	sortOpt := cmpopts.SortSlices(func(a, b thermalSensorInfo) bool {
		if a.Source != b.Source {
			return a.Source < b.Source
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.TemperatureCelsius < b.TemperatureCelsius
	})
	compareOpt := cmp.Comparer(compareSensors)

	if diff := cmp.Diff(expectedSensors, gotSensors, compareOpt, sortOpt); diff != "" {
		return errors.Errorf("Thermal Sensors reported incorrectly: (-got +want) %s", diff)
	}
	return nil
}

// getThermalSensorsFromSysFs returns all sensors reported from SysFs.
func getThermalSensorsFromSysFs() ([]thermalSensorInfo, error) {
	thermalZonePattern := "/sys/class/thermal/thermal_zone*"

	// Iterate and find all the thermal sensors in the system.
	var thermalZoneDirs, err = filepath.Glob(thermalZonePattern)
	if err != nil {
		return nil, err
	}

	var expectedSensors []thermalSensorInfo
	for _, dirName := range thermalZoneDirs {
		sensorName, err := os.ReadFile(filepath.Join(dirName, "type"))
		if err != nil {
			return nil, err
		}
		sensorTempStr, err := os.ReadFile(filepath.Join(dirName, "temp"))
		if err != nil {
			return nil, err
		}
		sensorTemp, err := strconv.ParseFloat(strings.TrimSpace(string(sensorTempStr)), 64)
		if err != nil {
			return nil, err
		}
		// SysFs reports temperature in millidegree Celsius, convert it to Celsius.
		sensorTemp /= 1000

		expectedSensor := thermalSensorInfo{
			Name:               strings.TrimSpace(string(sensorName)),
			TemperatureCelsius: sensorTemp,
			Source:             "SysFs",
		}
		expectedSensors = append(expectedSensors, expectedSensor)
	}

	return expectedSensors, nil
}

// getThermalSensorsFromEctool parses the output of ectool and gets the value of thermal sensors.
func getThermalSensorsFromEctool(ctx context.Context) ([]thermalSensorInfo, error) {
	// Check to see if a Google EC exists. If it does not, return empty slice.
	if _, err := os.Stat("/sys/class/chromeos/cros_ec"); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	// Parse out the sensor name and sensor temperature in Celsius from ectool
	// output. A sample ectool output from running `ectool temps all` looks like:
	//
	// --sensor name -------- temperature -------- ratio (fan_off and fan_max) --
	// Charger               312 K (= 39 C)           0% (313 K and 348 K)
	// PP3300 Regulator      310 K (= 37 C)           0% (313 K and 348 K)
	// DDR and SOC           315 K (= 42 C)          17% (308 K and 348 K)
	// Fan                   313 K (= 40 C)          12% (308 K and 348 K)

	// This regex captures based on the pattern ` _ K (= _ C)`, where _ represents
	// any number. This should capture the sensor name in the first group, and the
	// temperature in Celsius in the second group.
	sensorNameRegex := regexp.MustCompile(`^(.+) \d+ K\s*\(=\s*(\d+) C\)`)

	bStdout, err := testexec.CommandContext(ctx, "ectool", "temps", "all").Output(testexec.DumpLogOnError)
	if err != nil {
		return nil, errors.Wrap(err, "failed to run ectool command")
	}

	var expectedSensors []thermalSensorInfo
	lines := strings.Split(string(bStdout), "\n")
	for idx, line := range lines {
		// Skip the first line and empty lines.
		if idx == 0 || line == "" {
			continue
		}
		match := sensorNameRegex.FindStringSubmatch(line)
		if match == nil {
			return nil, errors.Errorf("Unable to parse EC sensor name and temperature: %q", line)
		}
		sensorName := strings.TrimSpace(match[1])
		sensorTemp, err := strconv.ParseFloat(strings.TrimSpace(match[2]), 64)
		if err != nil {
			return nil, err
		}
		expectedSensor := thermalSensorInfo{
			Name:               sensorName,
			TemperatureCelsius: sensorTemp,
			Source:             "EC",
		}
		expectedSensors = append(expectedSensors, expectedSensor)
	}
	return expectedSensors, nil
}
