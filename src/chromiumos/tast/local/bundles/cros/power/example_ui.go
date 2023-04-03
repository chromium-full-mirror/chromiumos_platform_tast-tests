// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"time"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/power"
	"chromiumos/tast/local/power/setup"
	"chromiumos/tast/testing"
)

const (
	// Interval between data points for power metrics.
	interval = 1 * time.Second
	// Total test time to collect power metrics.
	total = 10 * time.Second
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ExampleUI,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Collect power metrics when device is in idle with UI",
		BugComponent: "b:167191",
		Contacts:     []string{"chromeos-platform-power@google.com"},
		// Disabled because this is an example test for other tests to follow.
		// Attr:      []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      3 * time.Minute,
		Params: []testing.Param{{
			Name:    "ash",
			Fixture: "powerAsh",
		}, {
			Name:              "lacros",
			Fixture:           "powerLacros",
			ExtraSoftwareDeps: []string{"lacros"},
		}},
	})
}

func ExampleUI(ctx context.Context, s *testing.State) {
	// Reserve some time to cleanup, even if it fails due to ctx timeout.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	bt := s.FixtValue().(setup.PowerUIFixtureData).Bt
	cr := s.FixtValue().(setup.PowerUIFixtureData).Cr

	// Open a window with about:blank tab on the target browser.
	conn, _, cleanup, err := browserfixt.SetUpWithURL(ctx, cr, bt, "about:blank")
	if err != nil {
		s.Fatal("Failed to open a blank new tab: ", err)
	}
	defer cleanup(cleanupCtx)
	defer conn.Close()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get ash tconn: ", err)
	}

	w, err := ash.WaitForAnyWindow(ctx, tconn, ash.BrowserTypeMatch(bt))
	if err != nil {
		s.Fatal("Failed to open a browser window: ", err)
	}
	if err := ash.SetWindowStateAndWait(ctx, tconn, w.ID, ash.WindowStateMaximized); err != nil {
		s.Fatal("Failed to maximize the browser window: ", err)
	}

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
