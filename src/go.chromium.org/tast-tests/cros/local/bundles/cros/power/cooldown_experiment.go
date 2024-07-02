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

const (
	stressTestDuration = 15 * time.Minute
	idleDuration       = 5 * time.Minute
	recordingInterval  = 5 * time.Second
	testTimeout        = stressTestDuration + idleDuration + power.StrictCooldownTimeout + time.Minute
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CooldownExperiment,
		Desc:         "Examine the effectiveness of different cooldown methods",
		BugComponent: "b:1361410",
		Contacts:     []string{"chromeos-platform-power@google.com", "zactu@google.com"},
		Fixture:      setup.PowerNoUINoWiFi,
		Timeout:      testTimeout,
		Attr:         []string{"group:power", "power_daily_misc"},
		Data:         []string{tracing.TBMTracedProbesConfigFile},
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
	testing.ContextLog(ctx, "Started stress-ng workload to stress CPU")

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

	testing.ContextLog(ctx, "Stopped stressing CPU")
	r.EndCheckpoint(stressTestCheckpoint)
	cooldownCheckpoint := r.StartCheckpoint("cooldown")
	testing.ContextLog(ctx, "Cooldown started")

	// We want to upload metrics even if cooldown failed for debugging purposes,
	// thus the cooldown error needs to be delayed to the end of the test.
	cooldownErr := power.StrictCooldown(ctx)

	if cooldownErr != nil {
		testing.ContextLog(ctx, "Continue testing after failing to cooldown CPU with error: ", cooldownErr)
	}

	testing.ContextLog(ctx, "Cooldown ended")
	r.EndCheckpoint(cooldownCheckpoint)
	idleCheckpoint := r.StartCheckpoint("idle")
	testing.ContextLog(ctx, "Idle started")

	// GoBigSleepLint: Measure power metrics when the device is idle.
	if err := testing.Sleep(ctx, idleDuration); err != nil {
		s.Fatal("Failed to sleep while idling: ", err)
	}

	testing.ContextLog(ctx, "Idle ended")
	r.EndCheckpoint(idleCheckpoint)

	if err := r.Finish(ctx); err != nil {
		s.Fatal("Failed to finish recording: ", err)
	}

	if cooldownErr != nil {
		s.Fatal("Test finished with cooldown failure: ", cooldownErr)
	}
}
