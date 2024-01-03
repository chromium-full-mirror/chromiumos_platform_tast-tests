// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ChargeDischargeBattery,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test that battery can be charged or discharged to a certain range",
		BugComponent: "b:1361410", // ChromeOS > Platform > System > Core Power
		Contacts:     []string{"chromeos-platform-power@google.com", "jingmuli@google.com"},
		HardwareDeps: hwdep.D(
			hwdep.Battery(), // Test doesn't run on ChromeOS devices without a battery.
		),
		// min_charge_percent is the lower end of the target range.
		// max_charge_percent is the higher end of the target range.
		Vars: []string{
			"min_charge_percent",
			"max_charge_percent",
		},
		Params: []testing.Param{{
			Name: "power_test_prep",
			Val: power.ChargeParams{
				MinChargePercentage:   30.0,
				MaxChargePercentage:   97.0,
				DischargeOnCompletion: true,
				IsCustomized:          false,
				IsPowerQual:           false,
			},
			Timeout: 3 * time.Hour,
		}, {
			Name: "power_qual_prep",
			Val: power.ChargeParams{
				MinChargePercentage:   79.0,
				MaxChargePercentage:   81.0,
				DischargeOnCompletion: true,
				IsCustomized:          false,
				IsPowerQual:           false,
			},
			Timeout: 3 * time.Hour,
		}, {
			Name: "charge_qual_prep",
			Val: power.ChargeParams{
				MinChargePercentage:   7.5,
				MaxChargePercentage:   8.0,
				DischargeOnCompletion: true,
				IsCustomized:          false,
				IsPowerQual:           false},
			Timeout: 5 * time.Hour,
		}, {
			Name: "customization_prep",
			Val: power.ChargeParams{
				DischargeOnCompletion: true,
				IsCustomized:          true,
				IsPowerQual:           false},
			Timeout: 5 * time.Hour,
		}},
	})
}

func ChargeDischargeBattery(ctx context.Context, s *testing.State) {
	var chargeParam = s.Param().(power.ChargeParams)

	if chargeParam.IsCustomized {
		minChargePercentStr, ok := s.Var("min_charge_percent")
		if !ok {
			s.Fatal("The min_charge_percent is not provided")
		}

		minPercent, err := strconv.ParseFloat(strings.TrimSpace(string(minChargePercentStr)), 64)
		if err != nil {
			s.Fatalf("Failed to parse min_charge_percent percentage from %q", minChargePercentStr)
		}
		maxChargePercentStr, ok := s.Var("max_charge_percent")
		if !ok {
			s.Fatal("The max_charge_percent is not provided")
		}

		maxPercent, err := strconv.ParseFloat(strings.TrimSpace(string(maxChargePercentStr)), 64)
		if err != nil {
			s.Fatalf("Failed to parse max_charge_percent percentage from %q", maxChargePercentStr)
		}

		chargeParam.MinChargePercentage = float64(minPercent)
		chargeParam.MaxChargePercentage = float64(maxPercent)
	}

	if restartPowerd, err := setup.DisableService(ctx, "powerd"); err == nil {
		if restartPowerd != nil {
			defer restartPowerd(ctx)
		}
	} else {
		testing.ContextLogf(ctx, "Failed to stop powerd: %v; Still prepare battery to target range even though it might take longer", err)
	}

	if err := setup.PrepareBattery(ctx, chargeParam); err != nil {
		s.Fatal("Failed to charge/discharge DUT: ", err)
	}
}
