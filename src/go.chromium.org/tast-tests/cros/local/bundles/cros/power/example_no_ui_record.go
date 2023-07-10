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
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ExampleNoUIRecord,
		Desc:         "Collect power metrics with main test code in Record() when device is in idle with no UI",
		BugComponent: "b:167191",
		Contacts:     []string{"chromeos-platform-power@google.com"},
		// Disabled because this is an example test for other tests to follow.
		// Attr:      []string{"group:mainline", "informational"},
		Fixture: setup.PowerNoUINoWiFi,
		Timeout: 3 * time.Minute,
	})
}

// ExampleNoUIRecord is the test func to show how to use Record().
func ExampleNoUIRecord(ctx context.Context, s *testing.State) {
	// Reserve some time to cleanup, even if it fails due to ctx timeout.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	const interval = 1 * time.Second // Power metrics collect interval.

	r := power.NewRecorder(ctx, interval, s.OutDir(), s.TestName())
	defer r.Close(cleanupCtx)

	if err := r.Record(ctx, func(ctx context.Context) error {
		// Start of main test body. Idle for the defined duration while reading
		// power metrics with the defined interval. This both serves as an
		// example for future power tests and as a light weight test to test the
		// device setup. Replace this chunk of code with functionality code for
		// future power tests.
		const duration = 10 * time.Second // Total duration to stay in idle.
		// GoBigSleepLint: Measure power metrics when the device is idle.
		if err := testing.Sleep(ctx, duration); err != nil {
			return errors.Wrap(err, "failed to sleep")
		}
		// End of main test body.
		return nil
	}); err != nil {
		s.Fatal("Failed to record: ", err)
	}
}
