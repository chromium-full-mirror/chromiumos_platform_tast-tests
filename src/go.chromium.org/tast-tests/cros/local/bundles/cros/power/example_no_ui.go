// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ExampleNoUI,
		Desc:         "Collect power metrics when device is in idle with no UI",
		BugComponent: "b:1361410",
		Contacts:     []string{"chromeos-platform-power@google.com"},
		// Disabled because this is an example test for other tests to follow.
		// Attr:      []string{"group:mainline", "informational"},
		Timeout: 1 * time.Minute,
		Fixture: setup.PowerMetricsNoUI,
	})
}

func ExampleNoUI(ctx context.Context, s *testing.State) {
	// Start of main test body. Idle for 10 seconds while reading power metrics
	// every second (handled in the fixture). This both serves as an example for
	// future power tests and as a light weight test to test the device setup.
	// Replace this chunk of code with functionality code for future power
	// tests.
	// GoBigSleepLint: sleep to let the device idle.
	if err := testing.Sleep(ctx, 10*time.Second); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}
	// End of main test body.
}
