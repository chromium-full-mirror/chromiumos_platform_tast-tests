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
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ExampleNoUIManualMetrics,
		Desc:         "Collect power metrics when device is in idle with no UI",
		BugComponent: "b:1361410",
		Contacts: []string{
			"chromeos-platform-power@google.com",
			"mqg@chromium.org",
		},
		Timeout: 1*time.Minute + power.RecorderTimeout,
		Params: []testing.Param{{
			Name:    "fastest",
			Fixture: setup.PowerNoUINoWiFi,
			Val: power.TimeParams{
				Interval: 1 * time.Second,
				Total:    10 * time.Second,
			},
		}, {
			Name:    "fast",
			Fixture: setup.PowerNoUINoWiFi,
			Val: power.TimeParams{
				Interval: 5 * time.Second,
				Total:    20 * time.Second,
			},
		}, {
			Name:    "wifi",
			Fixture: setup.PowerNoUIWiFi,
			Val: power.TimeParams{
				Interval: 1 * time.Second,
				Total:    10 * time.Second,
			},
			ExtraAttr: []string{"group:power", "power_daily"},
		}},
	})
}

func ExampleNoUIManualMetrics(ctx context.Context, s *testing.State) {
	// Reserve some time to cleanup, even if it fails due to ctx timeout.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Any test specific setup code should go here.
	interval := s.Param().(power.TimeParams).Interval
	total := s.Param().(power.TimeParams).Total

	r := power.NewRecorder(ctx, interval, s.OutDir(), s.TestName())
	defer r.Close(cleanupCtx)
	if err := r.Cooldown(ctx); err != nil {
		s.Error("Cooldown failed: ", err)
	}
	if err := r.Start(ctx); err != nil {
		s.Fatal("Cannot start collecting power metrics: ", err)
	}

	// Start of main test body. Idle for `total` seconds while reading power
	// metrics every `interval` seconds. Device setup is handled in fixture.
	// This test both serves as an example for future power tests and as a light
	// weight test to test the device setup. Replace this chunk of code with
	// functionality code for future power tests.
	// GoBigSleepLint: sleep to let the device idle.
	if err := testing.Sleep(ctx, total); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}
	// End of main test body.

	if err := r.Finish(ctx); err != nil {
		s.Error("Cannot finish collecting power metrics: ", err)
	}
}
