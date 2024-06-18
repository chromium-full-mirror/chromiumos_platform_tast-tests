// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"time"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"

	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/power/setup"
)

var exampleUITimeParams = power.TimeParams{Interval: 1 * time.Second, Total: 10 * time.Second}
var exampleUI5minTimeParams = power.TimeParams{Interval: 5 * time.Second, Total: 300 * time.Second}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ExampleUI,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Collect power metrics when device is in idle with UI",
		BugComponent: "b:1361410",
		Contacts:     []string{"chromeos-platform-power@google.com"},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{{
			Name:      "ash_kbbl",
			Fixture:   setup.PowerAshKbbl,
			ExtraAttr: []string{"group:power", "power_daily", "power_weekly"},
			Val:       exampleUITimeParams,
			Timeout:   1*time.Minute + power.RecorderTimeout,
		}, {
			Name:    "ash",
			Fixture: setup.PowerAsh,
			Val:     exampleUI5minTimeParams,
			Timeout: 6*time.Minute + power.RecorderTimeout,
		}, {
			Name:    "ash_dark",
			Fixture: setup.PowerAshDark,
			Val:     exampleUI5minTimeParams,
			Timeout: 6*time.Minute + power.RecorderTimeout,
		}, {
			Name:    "ash_night",
			Fixture: setup.PowerAshNightlight,
			Val:     exampleUI5minTimeParams,
			Timeout: 6*time.Minute + power.RecorderTimeout,
		}, {
			Name:    "ash_gaia",
			Fixture: setup.PowerAshGAIA,
			Val:     exampleUITimeParams,
			Timeout: 1*time.Minute + power.RecorderTimeout,
		}, {
			Name:    "ash_arc",
			Fixture: setup.PowerAshARC,
			Val:     exampleUITimeParams,
			Timeout: 1*time.Minute + power.RecorderTimeout,
		}, {
			Name:              "lacros_kbbl",
			Fixture:           setup.PowerLacrosKbbl,
			ExtraSoftwareDeps: []string{"lacros"},
			ExtraAttr:         []string{"group:power", "power_daily", "power_weekly"},
			Val:               exampleUITimeParams,
			Timeout:           1*time.Minute + power.RecorderTimeout,
		}, {
			Name:              "lacros",
			Fixture:           setup.PowerLacros,
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               exampleUI5minTimeParams,
			Timeout:           6*time.Minute + power.RecorderTimeout,
		}, {
			Name:              "lacros_dark",
			Fixture:           setup.PowerLacrosDark,
			ExtraSoftwareDeps: []string{"lacros"},
			ExtraAttr:         []string{"group:power", "power_daily", "power_weekly"},
			Val:               exampleUI5minTimeParams,
			Timeout:           6*time.Minute + power.RecorderTimeout,
		}, {
			Name:              "lacros_night",
			Fixture:           setup.PowerLacrosNightlight,
			ExtraSoftwareDeps: []string{"lacros"},
			ExtraAttr:         []string{"group:power", "power_daily", "power_weekly"},
			Val:               exampleUI5minTimeParams,
			Timeout:           6*time.Minute + power.RecorderTimeout,
		}, {
			Name:              "lacros_gaia",
			Fixture:           setup.PowerLacrosGAIA,
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               exampleUITimeParams,
			Timeout:           1*time.Minute + power.RecorderTimeout,
		}, {
			Name:              "lacros_arc",
			Fixture:           setup.PowerLacrosARC,
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               exampleUITimeParams,
			Timeout:           1*time.Minute + power.RecorderTimeout,
		}},
	})
}

func ExampleUI(ctx context.Context, s *testing.State) {
	// Reserve some time to cleanup, even if it fails due to ctx timeout.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	discharge := s.FixtValue().(setup.PowerUIFixtureData).Discharge
	bt := s.FixtValue().(setup.PowerUIFixtureData).Bt
	cr := s.FixtValue().(setup.PowerUIFixtureData).Cr
	interval := s.Param().(power.TimeParams).Interval
	total := s.Param().(power.TimeParams).Total

	const blankPageContents = `data:text/html,
		<html>
			<body>
				<style media='(prefers-color-scheme: dark)'>
					body { background: black;}
				</style>
			</body>
		</html>`

	// Render an entirely white or black blank page in the browser in accordance with the OS theme.
	conn, _, cleanup, err := browserfixt.SetUpWithURL(ctx, cr, bt, blankPageContents)
	if err != nil {
		s.Fatal("Failed to open a blank new tab: ", err)
	}
	defer cleanup(cleanupCtx)
	defer conn.Close()
	defer conn.CloseTarget(cleanupCtx)

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

	r := power.NewRecorder(ctx, interval, s.OutDir(), s.TestName(), power.DischargeWatchdogOption(discharge))
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

	if err := power.SaveScreenshot(ctx, cr); err != nil {
		s.Error("Failed to take screenshot: ", err)
	}
}
