// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

var chargeQualPrepChargeParam = power.ChargeParams{
	// In general, it shouldn't take 5 hours long to drain
	// the battery to ~8%.
	MaxBatteryPreparationTime: 5 * time.Hour,
	MinChargePercentage:       7.5,
	MaxChargePercentage:       8.0,
	DischargeOnCompletion:     true,
	IsCustomized:              false,
	IsPowerQual:               false,
}

var qualChargeParam = power.ChargeParams{
	MaxBatteryPreparationTime: 4 * time.Hour,
	MinChargePercentage:       99.0,
	MaxChargePercentage:       100.0,
	DischargeOnCompletion:     false,
	IsCustomized:              false,
	IsPowerQual:               true,
	SkipDegradationAdjustment: true,
}

const (
	// metricCollectionInterval is the interval for collecting power metrics.
	metricCollectionInterval = 20 * time.Second
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         BatteryChargeQual,
		Desc:         "Measure battery charging speed in Active S0 idle state with default screen brightness",
		BugComponent: "b:1361410", // ChromeOS > Platform > System > Core Power
		Contacts:     []string{"chromeos-power-team@google.com"},
		HardwareDeps: hwdep.D(
			hwdep.Battery(), // Test doesn't run on ChromeOS devices without a battery.
		),
		Fixture: "powerAsh",
		// Timeout is set to the sum of maximum time to prepare the battery and
		// maximum time to charge the battery.
		Timeout: chargeQualPrepChargeParam.MaxBatteryPreparationTime + qualChargeParam.MaxBatteryPreparationTime,
	})
}

func BatteryChargeQual(ctx context.Context, s *testing.State) {
	// Reserve some time to cleanup, even if it fails due to ctx timeout.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	status, err := power.GetStatus(ctx)
	if err != nil {
		s.Fatal("Failed to obtain DUT power status: ", err)
	}

	if status.BatteryPercent > chargeQualPrepChargeParam.MaxChargePercentage {
		testing.ContextLogf(ctx, "Discharging battery to around %.1f%% prior to test",
			chargeQualPrepChargeParam.MaxChargePercentage)
		if err := setup.PrepareBattery(ctx, chargeQualPrepChargeParam); err != nil {
			s.Fatal("Failed to discharge DUT battery to predefined state: ", err)
		}
	}

	restartPowerd, err := setup.DisableService(ctx, "powerd")
	if err != nil {
		s.Fatal("Failed to stop powerd: ", err)
	}
	if restartPowerd != nil {
		// Use cleanupCtx to ensure powerd restarts even if test times out.
		defer restartPowerd(cleanupCtx)
	}

	r := power.NewRecorder(ctx, metricCollectionInterval, s.OutDir(), s.TestName())
	defer r.Close(cleanupCtx)
	if err := r.Start(ctx); err != nil {
		s.Fatal("Cannot start collecting power metrics: ", err)
	}

	if err := setup.PrepareBattery(ctx, qualChargeParam); err != nil {
		s.Fatal("Failed to charge DUT: ", err)
	}

	if err := r.Finish(ctx); err != nil {
		s.Error("Cannot finish collecting power metrics: ", err)
	}
}
