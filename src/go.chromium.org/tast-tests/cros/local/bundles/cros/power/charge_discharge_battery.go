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
		Desc:         "This test is used to charge battery to a certain range before running other (power) tests",
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
				MinChargePercentage:   40.0,
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
		}, {
			Name: "power_qual_prep_80_browsing",
			Val: power.ChargeParams{
				MinChargePercentage:   78.0,
				MaxChargePercentage:   80.0,
				DischargeOnCompletion: true,
				IsCustomized:          false,
				IsPowerQual:           false,
			},
			Timeout: 3 * time.Hour,
		}, {
			Name: "power_qual_prep_75_browsing_heavy",
			Val: power.ChargeParams{
				MinChargePercentage:   73.0,
				MaxChargePercentage:   75.0,
				DischargeOnCompletion: true,
				IsCustomized:          false,
				IsPowerQual:           false,
			},
			Timeout: 3 * time.Hour,
		}, {
			Name: "power_qual_prep_68_vpb_vp9",
			Val: power.ChargeParams{
				MinChargePercentage:   66.0,
				MaxChargePercentage:   68.0,
				DischargeOnCompletion: true,
				IsCustomized:          false,
				IsPowerQual:           false,
			},
			Timeout: 3 * time.Hour,
		}, {
			Name: "power_qual_prep_61_vpb_h264",
			Val: power.ChargeParams{
				MinChargePercentage:   59.0,
				MaxChargePercentage:   61.0,
				DischargeOnCompletion: true,
				IsCustomized:          false,
				IsPowerQual:           false,
			},
			Timeout: 3 * time.Hour,
		}, {
			Name: "power_qual_prep_53_vc",
			Val: power.ChargeParams{
				MinChargePercentage:   51.0,
				MaxChargePercentage:   53.0,
				DischargeOnCompletion: true,
				IsCustomized:          false,
				IsPowerQual:           false,
			},
			Timeout: 3 * time.Hour,
		}, {
			Name: "power_qual_prep_35_55_arc",
			Val: power.ChargeParams{
				MinChargePercentage:   35.0,
				MaxChargePercentage:   55.0,
				DischargeOnCompletion: true,
				IsCustomized:          false,
				IsPowerQual:           false,
			},
			Timeout: 3 * time.Hour,
		}, {
			Name: "power_qual_prep_30_40_informational",
			Val: power.ChargeParams{
				MinChargePercentage:   30.0,
				MaxChargePercentage:   40.0,
				DischargeOnCompletion: true,
				IsCustomized:          false,
				IsPowerQual:           false,
			},
			Timeout: 3 * time.Hour,
		}, {
			Name: "power_test_prep_lab",
			Val: power.ChargeParams{
				MinChargePercentage:   65.0,
				MaxChargePercentage:   70.0,
				DischargeOnCompletion: true,
				IsCustomized:          false,
				IsPowerQual:           false,
			},
			Timeout: 3 * time.Hour,
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
