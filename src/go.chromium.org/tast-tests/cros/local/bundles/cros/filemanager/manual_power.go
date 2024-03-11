// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filemanager

import (
	"context"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ManualPower,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Measures power usage for an hour",
		BugComponent: "b:167289",
		Contacts: []string{
			"chromeos-files-syd@google.com",
			"joelhockey@chromium.org",
		},
		Timeout: time.Hour,
	})
}

// ManualPower will collect power metrics for 15 minutes. It expects that
// the device is already logged in.
func ManualPower(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	// Set up with options to match 'powerAsh' in power fixtures.
	// We can match KeyboardBrightness, but not NightLight or DarkTheme
	// since we don't have a valid TestConn.
	options := &setup.PowerTestOptions{
		KeyboardBrightness: setup.SetKbBrightnessToZero,
	}
	cleanup, _, err := setup.PowerTestSetup(ctx, "setup", nil, options)
	if err != nil {
		s.Fatal("Power setup failed: ", err)
	}
	defer func() {
		if s.HasError() {
			cleanup(cleanupCtx)
		}
	}()

	r := power.NewRecorder(ctx, 5*time.Second, s.OutDir(), s.TestName())
	testing.ContextLog(ctx, "Cooldown")
	if err := r.Cooldown(ctx); err != nil {
		s.Error("Cooldown failed: ", err)
	}
	if err := r.Start(ctx); err != nil {
		s.Fatal("Cannot start collecting power metrics: ", err)
	}
	testing.ContextLog(ctx, "Ready")

	// Continue collecting data up until the context deadline occurs, to stop
	// the metric collection run `touch /tmp/stop_power_test` on the DUT and
	// this will schedule the metrics collection to stop in 3 more iterations.
	var sleepTotal = 10000
	fileFound := false
	for i := 0; i < sleepTotal; i++ {
		testing.ContextLogf(ctx, "Recording metrics (%02d/%d)", i+1, sleepTotal)
		// GoBigSleepLint: sleep while we record metrics.
		if err := testing.Sleep(ctx, 1*time.Minute); err != nil {
			s.Error("Failed to sleep: ", err)
			break
		}
		if _, err := os.Stat("/tmp/stop_power_test"); err == nil && !fileFound {
			s.Log("Stop file found")
			fileFound = true
			sleepTotal = i + 3
		}
	}

	if err := r.Finish(cleanupCtx); err != nil {
		s.Error("Cannot finish collecting power metrics: ", err)
	}
	if err := os.Remove("/tmp/stop_power_test"); err != nil {
		s.Log("Failed to remove stop file: ", err)
	}
	testing.ContextLog(cleanupCtx, "Done")
}
