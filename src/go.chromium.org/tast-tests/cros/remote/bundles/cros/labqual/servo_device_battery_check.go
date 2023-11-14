// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package labqual

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ServoDeviceBatteryCheck,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check battery's last full charge and that battery is charging or atleast 70%",
		Contacts: []string{
			"peep-fleet-infra-sw@google.com",
		},
		BugComponent: "b:1032353", // Chrome Operations > Fleet > Software > OS Fleet Automation
		Attr:         []string{"group:labqual_informational", "group:labqual_stable"},
		SoftwareDeps: []string{"chrome"},
		ServiceDeps:  []string{"tast.cros.power.BatteryService", "tast.cros.firmware.UtilsService"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.Battery()),
		Fixture:      fixture.NormalMode,
	})
}

// ServoDeviceBatteryCheck checks the battery's last full charge and whether the battery is charging
// or at minimum required charge level
func ServoDeviceBatteryCheck(ctx context.Context, s *testing.State) {
	const MinimumBatteryLevel = 70

	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}
	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}

	lastFullCharge, err := getBatteryFullChargeMAH(ctx, h)
	if err != nil {
		s.Fatal("Failed to get last full charge in mAh: ", err)
	}
	s.Logf("Servo Validate Battery Charging: last full charge is %dmAh", lastFullCharge)

	// Verifying battery charging and charge level.
	s.Log("Checking battery information for charging")
	charging, percentage, err := isBatteryChargingAndBatteryLevel(ctx, h)
	if err != nil {
		s.Fatal("Failed to check charging status from power_supply_info: ", err)
	}

	if !charging && percentage < MinimumBatteryLevel {
		s.Fatalf("Device is not charging and charge percentage %f is less than minimum %d", percentage, MinimumBatteryLevel)
	}
	s.Logf("Battery charging is %v and is at %.2f %%", charging, percentage)
}

// isBatteryChargingAndBatteryLevel returns true if battery is charging and the charge percentage.
func isBatteryChargingAndBatteryLevel(ctx context.Context, h *firmware.Helper) (bool, float64, error) {
	stateRegex := `state:(\s+\w+\s?\w+)`
	stateExpMatch := regexp.MustCompile(stateRegex)
	percentageRegex := `percentage:(\s+\w+\s?\w+)`
	percentageExpMatch := regexp.MustCompile(percentageRegex)

	out, err := h.DUT.Conn().CommandContext(ctx, "power_supply_info").Output()
	if err != nil {
		return false, 0, errors.Wrap(err, "failed to retrieve power supply info from DUT")
	}

	stateMatches := stateExpMatch.FindStringSubmatch(string(out))
	if len(stateMatches) < 2 {
		return false, 0, errors.Errorf("failed to match regex %q in %q", stateExpMatch, string(out))
	}

	percentageMatches := percentageExpMatch.FindStringSubmatch(string(out))
	if len(percentageMatches) < 2 {
		return false, 0, errors.Errorf("failed to match regex %q in %q", percentageExpMatch, string(out))
	}
	batteryLevel, err := strconv.ParseFloat(strings.TrimSpace(percentageMatches[1]), 64)
	if err != nil {
		return false, 0, errors.Errorf("failed to parse battery percentage %q", percentageMatches[1])
	}

	return strings.TrimSpace(percentageMatches[1]) != "Discharging", batteryLevel, nil
}

// getBatteryFullChargeMAH returns battery's last full charge
func getBatteryFullChargeMAH(ctx context.Context, h *firmware.Helper) (int, error) {
	var err error = nil
	maxMAH := 0
	testing.Poll(ctx, func(ctx context.Context) error {
		maxMAH, err = h.Servo.GetBatteryFullChargeMAH(ctx)
		if err != nil {
			return err
		}
		return nil
	}, &testing.PollOptions{Timeout: 20 * time.Second, Interval: time.Second})
	if err != nil {
		// Returning -1 along with error to indicate that BatteryFullChargeMAH could not be retrieved
		return -1, err
	}

	return maxMAH, nil
}
