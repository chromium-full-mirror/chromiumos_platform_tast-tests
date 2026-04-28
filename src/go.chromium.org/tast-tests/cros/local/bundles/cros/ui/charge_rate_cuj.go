// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast-tests/cros/local/power/util"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const chargeTimeout = 30 * time.Minute

func init() {
	testing.AddTest(&testing.Test{
		Func: ChargeRateCUJ,
		Desc: "Measures the battery charging rate in a fixed interval",
		Contacts: []string{
			"cros-sw-perf@google.com",
			"cienet-development@googlegroups.com",
			"mars.huang@cienet.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:cuj", "cuj_chargerate"},
		HardwareDeps: hwdep.D(
			hwdep.Battery(),
			hwdep.ChromeEC(),
		),
		Fixture: "chromeLoggedIn",
		Timeout: setup.BatteryPreparationTimeout + chargeTimeout,
	})
}

func ChargeRateCUJ(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 20*time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	restartPowerd, err := setup.DisableService(ctx, "powerd")
	if err != nil {
		s.Fatal("Failed to stop powerd: ", err)
	}
	if restartPowerd != nil {
		// Use cleanupCtx to ensure powerd restarts even if test times out.
		defer restartPowerd(cleanupCtx)
	}

	// chargeParams prepares the battery to around 65% within testable range to
	// measure the charging rate.
	chargeParams := power.ChargeParams{
		MinChargePercentage:     64.0,
		MaxChargePercentage:     65.0,
		DischargeOnCompletion:   false,
		IsCustomized:            false,
		IsPowerQual:             false,
		EnableDischargeWatchdog: true,
	}

	if err := prepareBatteryWithHighLoad(ctx, cr, chargeParams); err != nil {
		s.Fatal("Failed to prepare battery: ", err)
	}

	const (
		// targetPercentage is the target battery percentage to measure.
		targetPercentage = 80.0
		// chargeMetricCollectionInterval is the interval for collecting the
		// charging power metric.
		chargeMetricCollectionInterval = time.Minute
	)
	s.Logf("Starting charge phase measurement to %.2f%%", targetPercentage)

	if err := setup.AllowBatteryCharging(ctx); err != nil {
		s.Fatal("Failed to allow charging: ", err)
	}

	if err := setup.WaitUntilPowerSourceChanges(ctx, true); err != nil {
		s.Fatal("Timed out waiting for DUT to start charging: ", err)
	}

	pv := perf.NewValues()
	chargeMetric := perf.Metric{
		Name:      "Battery.ChargeRate",
		Unit:      "percent",
		Direction: perf.BiggerIsBetter,
		Multiple:  true,
		Interval:  fmt.Sprintf("%vs", chargeMetricCollectionInterval.Seconds()),
	}
	totalTimeMetric := perf.Metric{
		Name:      "Battery.TotalChargeTime",
		Unit:      "minute",
		Direction: perf.SmallerIsBetter,
		Multiple:  false,
	}
	lastStatus, err := power.GetStatus(ctx)
	if err != nil {
		s.Fatal("Failed to get status for charging: ", err)
	}
	startTime := time.Now()
	for {
		// GoBigSleepLint: Wait for a fixed interval to measure battery percentage change.
		if err := testing.Sleep(ctx, chargeMetricCollectionInterval); err != nil {
			s.Fatal("Timeout while sleeping: ", err)
		}

		currentStatus, err := power.GetStatus(ctx)
		if err != nil {
			s.Error("Failed to obtain DUT power status: ", err)
			break
		}

		delta := currentStatus.BatteryPercent - lastStatus.BatteryPercent
		pv.Append(chargeMetric, delta)
		s.Logf("Charging: %.2f%%/%vmin (Current: %.2f%%)", delta, chargeMetricCollectionInterval.Minutes(), currentStatus.BatteryPercent)

		if currentStatus.BatteryPercent >= targetPercentage {
			s.Log("Charge target reached")
			break
		}

		lastStatus = currentStatus
	}

	totalMinutes := time.Since(startTime).Minutes()
	pv.Set(totalTimeMetric, totalMinutes)

	if err := pv.Save(s.OutDir()); err != nil {
		s.Error("Failed to save metrics: ", err)
	}
}

// prepareBatteryWithHighLoad ensures the battery is in range, using WebGL
// and max fan if high-load discharge is needed.
func prepareBatteryWithHighLoad(ctx context.Context, cr *chrome.Chrome, params power.ChargeParams) error {
	status, err := power.GetStatus(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get battery status")
	}

	if status.BatteryPercent > params.MaxChargePercentage {
		const webGLURL = "https://webglsamples.org/aquarium/aquarium.html?numFish=30000"
		cleanupCtx := ctx
		ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
		defer cancel()

		conn, err := cr.NewConn(ctx, webGLURL)
		if err != nil {
			return errors.Wrapf(err, "failed to open %s", webGLURL)
		}
		defer conn.Close()
		defer conn.CloseTarget(cleanupCtx)

		if util.GetNumFans(ctx) > 0 {
			if err := power.SetFanMax(ctx); err != nil {
				return errors.Wrap(err, "failed to set fan to max")
			}
			defer power.SetFanAuto(cleanupCtx)
		}
	}

	return setup.PrepareBattery(ctx, params)
}
