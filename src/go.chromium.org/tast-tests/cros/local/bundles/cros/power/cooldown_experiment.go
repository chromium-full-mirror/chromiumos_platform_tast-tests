// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"runtime"
	"time"

	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast-tests/cros/local/tracing"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type experimentConfig struct {
	useThermal     bool
	cooldownParams power.ThermalCooldownParams
}

// Takes about 15 minutes to saturate DUT's thermal solution and monitors
// power metrics while idling for 5 minutes to observe any anomalies.
const (
	stressTestDuration = 15 * time.Minute
	idleDuration       = 5 * time.Minute
	recordingInterval  = 5 * time.Second
	cooldownTimeout    = 5 * time.Minute
)

// Taking 60 regular samples over a period of 1 minute to verify the device
// is cooled down to a steady state.
const (
	thermalSamples   = 60
	samplingInterval = 1 * time.Second
)

// Three levels of cooldown to try and understand the best
// parameters that cools down fast and reliable.
var relaxCooldownParams = power.ThermalCooldownParams{
	SampleCount:          thermalSamples,
	Interval:             samplingInterval,
	Timeout:              cooldownTimeout,
	MaxStandardDeviation: 4.0,
}

var mediumCooldownParams = power.ThermalCooldownParams{
	SampleCount:          thermalSamples,
	Interval:             samplingInterval,
	Timeout:              cooldownTimeout,
	MaxStandardDeviation: 2.0,
}

var strictCooldownParams = power.ThermalCooldownParams{
	SampleCount:          thermalSamples,
	Interval:             samplingInterval,
	Timeout:              cooldownTimeout,
	MaxStandardDeviation: 1.0,
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         CooldownExperiment,
		Desc:         "Examine the effectiveness of different cooldown methods",
		BugComponent: "b:1361410",
		Contacts:     []string{"chromeos-platform-power@google.com", "zactu@google.com"},
		Fixture:      setup.PowerNoUINoWiFi,
		Timeout:      30 * time.Minute,
		Attr:         []string{"group:power", "power_daily_misc"},
		Data:         []string{tracing.TBMTracedProbesConfigFile},
		Params: []testing.Param{{
			Name: "original",
			Val: experimentConfig{
				useThermal: false,
			},
		}, {
			Name: "relax",
			Val: experimentConfig{
				useThermal:     true,
				cooldownParams: relaxCooldownParams,
			},
		}, {
			Name: "medium",
			Val: experimentConfig{
				useThermal:     true,
				cooldownParams: mediumCooldownParams,
			},
		}, {
			Name: "strict",
			Val: experimentConfig{
				useThermal:     true,
				cooldownParams: strictCooldownParams,
			},
		}},
	})
}

// CooldownExperiment collect power metrics to examine effectiveness
// of the current and thermal cooldown methods.
func CooldownExperiment(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	r := power.NewRecorder(ctx, recordingInterval, s.OutDir(), s.TestName())
	defer r.Close(cleanupCtx)

	r.EnableTracing(s.DataPath(tracing.TBMTracedProbesConfigFile), "trace.data")

	if err := r.Start(ctx); err != nil {
		s.Fatal("Failed to start recording: ", err)
	}

	stressTestCheckpoint := r.StartCheckpoint("stressTestCPU")

	stopStressTest, err := setup.StressCPU(ctx, runtime.NumCPU(), "/tmp")
	if err != nil {
		s.Fatal("Failed to start stressing CPU: ", err)
	}

	// GoBigSleepLint: Stressing CPU until the heat-sink is saturated.
	if err := testing.Sleep(ctx, stressTestDuration); err != nil {
		s.Fatal("Failed to sleep while stressing CPU: ", err)
	}

	if err := stopStressTest(cleanupCtx); err != nil {
		s.Fatal("Failed to stop stressing CPU: ", err)
	}

	r.EndCheckpoint(stressTestCheckpoint)
	cooldownCheckpoint := r.StartCheckpoint("cooldown")

	// We want to upload metrics even if cooldown failed for debugging purposes,
	// thus the cooldown error needs to be delayed to the end of the test.
	conf := s.Param().(experimentConfig)
	var cooldownErr error
	if conf.useThermal {
		cooldownErr = power.ThermalCooldown(ctx, conf.cooldownParams)
	} else {
		cooldownErr = power.Cooldown(ctx)
	}

	if cooldownErr != nil {
		testing.ContextLog(ctx, "Continue testing after failing to cooldown CPU with error: ", cooldownErr)
	}

	r.EndCheckpoint(cooldownCheckpoint)
	idleCheckpoint := r.StartCheckpoint("idle")

	// GoBigSleepLint: Measure power metrics when the device is idle.
	if err := testing.Sleep(ctx, idleDuration); err != nil {
		s.Fatal("Failed to sleep while idling: ", err)
	}

	r.EndCheckpoint(idleCheckpoint)

	if err := r.Finish(ctx); err != nil {
		s.Fatal("Failed to finish recording: ", err)
	}

	if cooldownErr != nil {
		s.Fatal("Test finished with cooldown failure: ", cooldownErr)
	}
}
