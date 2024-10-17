// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package health

import (
	"context"
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

func ProbeCPUTempInfo(ctx context.Context, s *testing.State) {
	params := croshealthd.TelemParams{Category: croshealthd.TelemCategoryCPU}

	var info types.CPUInfo
	if err := croshealthd.RunAndParseJSONTelem(ctx, params, s.OutDir(), &info); err != nil {
		s.Fatal("Failed to run telem command: ", err)
	}

	if err := validateCPUTempData(&info); err != nil {
		s.Fatal("Failed to validate cpu temp data: ", err)
	}
}
