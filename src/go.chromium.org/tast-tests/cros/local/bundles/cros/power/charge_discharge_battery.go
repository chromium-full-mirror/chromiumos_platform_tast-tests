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
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ChargeDischargeBattery,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test that battery can be charged or discharged to a certain range",
		BugComponent: "b:167191", // ChromeOS > Platform > System > Power
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
				DischargeOnCompletion: true},
			Timeout: 3 * time.Hour,
		}, {
			Name: "charging_measurement_prep",
			Val: power.ChargeParams{
				MinChargePercentage:   7.5,
				MaxChargePercentage:   8.0,
				DischargeOnCompletion: true},
			Timeout: 5 * time.Hour,
		}, {
			Name:    "customization_prep",
			Val:     power.ChargeParams{DischargeOnCompletion: true, Customized: true},
			Timeout: 5 * time.Hour,
		}},
	})
}

func ChargeDischargeBattery(ctx context.Context, s *testing.State) {
	// Reserve some time to cleanup, even if it fails due to ctx timeout.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	var err error
	var minPercent, maxPercent float64

	if s.Param().(power.ChargeParams).Customized {
		minChargePercentStr, ok := s.Var("min_charge_percent")
		if !ok {
			s.Fatal("The min_charge_percent is not provided")
		}

		minPercent, err = strconv.ParseFloat(strings.TrimSpace(string(minChargePercentStr)), 64)
		if err != nil {
			s.Fatalf("Failed to parse min_charge_percent percentage from %q", minChargePercentStr)
		}

		maxChargePercentStr, ok := s.Var("max_charge_percent")
		if !ok {
			s.Fatal("The max_charge_percent is not provided")
		}

		maxPercent, err = strconv.ParseFloat(strings.TrimSpace(string(maxChargePercentStr)), 64)
		if err != nil {
			s.Fatalf("Failed to parse max_charge_percent percentage from %q", maxChargePercentStr)
		}
	} else {
		minPercent = s.Param().(power.ChargeParams).MinChargePercentage
		maxPercent = s.Param().(power.ChargeParams).MaxChargePercentage
	}

	dischargeOnCompletion := s.Param().(power.ChargeParams).DischargeOnCompletion

	r := power.NewRecorder(ctx, 20*time.Second, s.OutDir(), s.TestName())
	defer r.Close(cleanupCtx)

	if err := r.Start(ctx); err != nil {
		s.Fatal("Cannot start collecting power metrics: ", err)
	}

	if err := setup.PrepareBattery(ctx, float64(minPercent), float64(maxPercent), dischargeOnCompletion); err != nil {
		s.Fatal("Failed to charge/discharge DUT: ", err)
	}

	if err := r.Finish(ctx); err != nil {
		s.Error("Cannot finish collecting power metrics: ", err)
	}
}
