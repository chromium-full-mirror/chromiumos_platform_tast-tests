// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"time"

	"chromiumos/tast/local/power"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ExampleNoUIManualMetrics,
		Desc:         "Collect power metrics when device is in idle with no UI",
		BugComponent: "b:167191",
		Contacts: []string{
			"chromeos-platform-power@google.com",
			"mqg@chromium.org",
		},
		Timeout: 3 * time.Minute,
		Params: []testing.Param{{
			Name:    "fastest",
			Fixture: "powerNoUINoWiFi",
			Val: power.TimeParams{
				Interval: 1 * time.Second,
				Total:    10 * time.Second,
			},
		}, {
			Name:    "fast",
			Fixture: "powerNoUINoWiFi",
			Val: power.TimeParams{
				Interval: 5 * time.Second,
				Total:    20 * time.Second,
			},
		}, {
			Name:    "wifi",
			Fixture: "powerNoUIWiFi",
			Val: power.TimeParams{
				Interval: 1 * time.Second,
				Total:    10 * time.Second,
			},
			ExtraAttr: []string{"group:crosbolt", "crosbolt_perbuild"},
		}},
	})
}

func ExampleNoUIManualMetrics(ctx context.Context, s *testing.State) {
	// Any test specific setup code should go here.
	interval := s.Param().(power.TimeParams).Interval
	total := s.Param().(power.TimeParams).Total

	r, err := power.NewRecorder(ctx, interval, s.OutDir(), s.TestName())
	if err != nil {
		s.Fatal("Cannot create a new Recorder to collect power metrics: ", err)
	}
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
