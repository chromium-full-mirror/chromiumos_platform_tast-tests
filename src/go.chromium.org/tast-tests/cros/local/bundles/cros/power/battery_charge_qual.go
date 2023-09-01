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

var qualChargeParam = power.ChargeParams{
	MinChargePercentage:   99.0,
	MaxChargePercentage:   100.0,
	DischargeOnCompletion: false,
	IsCustomized:          false,
	IsPowerQual:           true,
}

// Battery should be drained to at least this percentage.
const chargePrepThreshold = 8.0

func init() {
	testing.AddTest(&testing.Test{
		Func:         BatteryChargeQual,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test that battery can be charged or discharged to a certain range",
		BugComponent: "b:1361410", // ChromeOS > Platform > System > Core Power
		Contacts:     []string{"chromeos-platform-power@google.com", "jingmuli@google.com"},
		HardwareDeps: hwdep.D(
			hwdep.Battery(), // Test doesn't run on ChromeOS devices without a battery.
		),
		// Battery should be drained before the test gets started and usually we expect it
		// to finish within 3 hours and at most 4 hours.
		Timeout: 4 * time.Hour,
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

	if status.BatteryPercent > chargePrepThreshold {
		s.Fatalf("Battery should be drained below %.2f%% before the test", chargePrepThreshold)
	}

	r := power.NewRecorder(ctx, 20*time.Second, s.OutDir(), s.TestName())
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
