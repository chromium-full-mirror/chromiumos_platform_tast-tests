// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"time"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"

	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/inputs/fixture"
	"go.chromium.org/tast-tests/cros/local/inputs/testserver"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/power/setup"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         QuickInsertPower,
		Desc:         "Collect power metrics when device is in idle with UI",
		BugComponent: "b:1361410",
		Contacts:     []string{"chromeos-power-team@google.com"},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{{
			Fixture: fixture.ClamshellNonVKWithPicker,
			Val:     power.TimeParams{Total: 10 * time.Minute, Interval: 5 * time.Second},
			ExtraAttr: []string{
				"group:power",
				"power_daily",
				"power_weekly",
				"group:release-health",
				"release-health_power",
			},
		}},
		Timeout: 15*time.Minute + power.RecorderTimeout,
	})
}

func QuickInsertPower(ctx context.Context, s *testing.State) {
	// Reserve some time to cleanup, even if it fails due to ctx timeout.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	powerInterval := s.Param().(power.TimeParams).Interval
	powerTotal := s.Param().(power.TimeParams).Total

	cleanup, _, err := setup.PowerTestSetup(ctx, "powerd and multicast disabled", nil, &setup.PowerTestOptions{
		Powerd:    setup.DisablePowerd,
		Multicast: setup.DisableMulticast,
	})
	if err != nil {
		s.Fatal("Failed to disable powerd and multicast: ", err)
	}
	defer cleanup(cleanupCtx)

	cr := s.FixtValue().(fixture.FixtData).Chrome

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get ash tconn: ", err)
	}

	keyboard, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to access keyboard: ", err)
	}
	defer keyboard.Close(cleanupCtx)

	its, err := testserver.LaunchBrowser(ctx, cr, tconn)
	if err != nil {
		s.Fatal("Failed to launch inputs test server: ", err)
	}
	defer its.CloseAll(cleanupCtx)

	ui := uiauto.New(tconn)

	// Dismiss the first use education dialog.
	if err := uiauto.Combine("search GIFs",
		keyboard.AccelAction("Search+F"),
		ui.LeftClick(nodewith.Name("Get started").Role(role.Button).Visible().Onscreen()),
		ui.WaitUntilExists(nodewith.HasClass("Quick Insert").Visible().Onscreen()),
		// Close the window by toggling Quick Insert again
		keyboard.AccelAction("Search+F"),
	)(ctx); err != nil {
		s.Error("Failed dismiss first-use dialog: ", err)
	}

	r := power.NewRecorder(ctx, powerInterval, s.OutDir(), s.TestName())
	defer r.Close(cleanupCtx)

	if err := power.Cooldown(ctx); err != nil {
		s.Error("Cooldown failed: ", err)
	}
	if err := r.Start(ctx); err != nil {
		s.Fatal("Cannot start collecting power metrics: ", err)
	}

	queries := []string{
		"good luck", "high five", "nervous", "aww", "scared",
		"happy", "bored", "sigh", "sad", "good morning", "good night",
		"cat", "dog", "hello", "lol", "lmao", "bye",
		"wow", "amazing", "huh", "woot", "why", "hug",
	}

	// Run through the queries repeatedly until `powerTotal` has elapsed.
	for i, startTime := 0, time.Now(); time.Now().Sub(startTime) < powerTotal; i++ {
		// Open Quick Insert and search the current query.
		query := queries[i%len(queries)]
		if err := uiauto.Combine("search GIFs",
			its.ClearThenClickFieldAndWaitForActive(testserver.ContentEditableInputField),
			keyboard.AccelAction("Search+F"),
			ui.LeftClick(nodewith.HasClass("GifsButton").Visible().Onscreen().First()),
			keyboard.TypeAction(query),
			// Sleep here to let the GIF results load and animate, giving enough time to take at least one power measurement.
			uiauto.Sleep(powerInterval),
		)(ctx); err != nil {
			s.Error("Failed to search GIFs: ", err)
		}
	}

	// End of main test body.
	if err := r.Finish(ctx); err != nil {
		s.Error("Cannot finish collecting power metrics: ", err)
	}

	if err := power.SaveScreenshot(ctx, cr); err != nil {
		s.Error("Failed to take screenshot: ", err)
	}
}
