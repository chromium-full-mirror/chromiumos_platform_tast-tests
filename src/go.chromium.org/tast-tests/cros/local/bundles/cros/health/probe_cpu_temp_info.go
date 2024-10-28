// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package health

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/health/types"

	"go.chromium.org/tast-tests/cros/local/croshealthd"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ProbeCPUTempInfo,
		Desc: "Check that we can probe cros_healthd for CPU temperature info",
		Contacts: []string{
			"cros-tdm-tpe-eng@google.com",
			"pohengchen@google.com",
		},
		BugComponent: "b:982097", // ChromeOS > Platform > Enablement > Health
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"diagnostics"},
		Fixture:      "crosHealthdRunning",
		Timeout:      3 * time.Minute,
	})
}

// verifyCPUTempRange verifies that all temperatures read from sensors are reasonable.
func verifyCPUTempRange(tempChannels *[]types.TemperatureChannelInfo) error {
	for _, tempChannel := range *tempChannels {
		// Arbitrary value for checking temperature reading is reasonable. Values beyond
		// these limits are most likely caused by sensor failure.
		if tempChannel.TemperatureCelsius < 0 || tempChannel.TemperatureCelsius > 100 {
			return errors.Errorf("CPU temperature reading outside threshold of 0-100 Celsius: %d", tempChannel.TemperatureCelsius)
		}
	}
	return nil
}

func validateCPUTempData(info *types.CPUInfo) error {
	// Check CpuInfo has at least one CPU temperature channel
	if len(info.TemperatureChannels) == 0 {
		return errors.New("invalid TemperatureChannels (empty)")
	}

	return verifyCPUTempRange(&info.TemperatureChannels)
}

// getTempsFromHwmon returns CPU temp data in degree Celsius.
// The data is from /sys/class/hwmon, which is the old data source for HealthD.
func getTempsFromHwmon(ctx context.Context) ([]float64, error) {
	const hwmonTempPattern = "/sys/class/hwmon/hwmon*/temp*_input"
	var temps []float64
	// Iterate and find all the thermal sensors in the system.
	hwmonTempFiles, err := filepath.Glob(hwmonTempPattern)
	if err != nil {
		return nil, errors.Wrapf(err, "Hwmon pattern %q is malformed", hwmonTempPattern)
	}

	for _, tempFile := range hwmonTempFiles {
		sensorTempStr, err := os.ReadFile(tempFile)
		if err != nil {
			testing.ContextLogf(ctx, "Unable to read tempature string from %q", tempFile)
			continue
		}
		sensorTemp, err := strconv.ParseInt(strings.TrimSpace(string(sensorTempStr)), 10, 32)
		if err != nil {
			testing.ContextLogf(ctx, "Unable to parse %q from %q into integer", strings.TrimSpace(string(sensorTempStr)), tempFile)
			continue
		}
		// Hwmon reports temperature in millidegree Celsius, convert it to Celsius.
		temps = append(temps, float64(sensorTemp)/1000)
	}

	return temps, nil
}

// verifyTempsWithHwmon make sure the difference in CPU temps data between
// thermal zones and Hwmon is not too large.
//
// This is a temporary test since we recently change the data source
// and want to ensure the difference won't be too large.
func verifyTempsWithHwmon(ctx context.Context, info *types.CPUInfo) error {
	tempsFromHwmon, err := getTempsFromHwmon(ctx)
	if err != nil {
		return err
	}

	if len(tempsFromHwmon) == 0 {
		// Seen as pass if there is no data available from Hwmon
		return nil
	}
	var sumTempFromHwmon float64 = 0.0
	for _, temp := range tempsFromHwmon {
		sumTempFromHwmon += temp
	}
	avgTempFromHwmon := sumTempFromHwmon / float64(len(tempsFromHwmon))

	var sumTempFromThermalZone float64 = 0.0
	for _, tempChannel := range info.TemperatureChannels {
		sumTempFromThermalZone += float64(tempChannel.TemperatureCelsius)
	}
	avgTempFromThermalZone := sumTempFromThermalZone / float64(len(info.TemperatureChannels))

	if diff := math.Abs(avgTempFromThermalZone - avgTempFromHwmon); diff > 10 {
		// Hwmon may not contain any sensor near cpu.
		// As a result, the increase of avg cpu temp is expected
		// after we change to use thermal zone as the data source.
		return errors.Errorf("CPU temperature diff is too large, got diff %f (thermal_zone=%f, hwmon=%f), want < 10", diff, avgTempFromThermalZone, avgTempFromHwmon)
	}
	return nil
}

func ProbeCPUTempInfo(ctx context.Context, s *testing.State) {
	params := croshealthd.TelemParams{Category: croshealthd.TelemCategoryCPU}

	var info types.CPUInfo
	if err := croshealthd.RunAndParseJSONTelem(ctx, params, s.OutDir(), &info); err != nil {
		s.Fatal("Failed to run telem command: ", err)
	}

	if err := validateCPUTempData(&info); err != nil {
		s.Fatal("Failed to validate cpu temp data: ", err)
	}

	if err := verifyTempsWithHwmon(ctx, &info); err != nil {
		s.Fatal("Failed to verify temp with Hwmon: ", err)
	}
}
