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
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/power"
	pm "go.chromium.org/tast-tests/cros/local/power/metrics"
	"go.chromium.org/tast-tests/cros/local/power/setup"
)

var exampleHistTimeParams = power.TimeParams{Interval: 5 * time.Second, Total: 30 * time.Second}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ExampleHistogram,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Collect browser histogram metrics",
		BugComponent: "b:1361410",
		Contacts:     []string{"chromeos-platform-power@google.com", "mqg@google.com"},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{{
			Name:    "ash",
			Fixture: setup.PowerAsh,
			Val:     exampleHistTimeParams,
			Timeout: 1*time.Minute + power.RecorderTimeout,
		}, {
			Name:    "lacros",
			Fixture: setup.PowerLacros,
			Val:     exampleHistTimeParams,
			Timeout: 1*time.Minute + power.RecorderTimeout,
		}},
	})
}

func ExampleHistogram(ctx context.Context, s *testing.State) {
	// Reserve some time to cleanup, even if it fails due to ctx timeout.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	discharge := s.FixtValue().(setup.PowerUIFixtureData).Discharge
	bt := s.FixtValue().(setup.PowerUIFixtureData).Bt
	cr := s.FixtValue().(setup.PowerUIFixtureData).Cr
	interval := s.Param().(power.TimeParams).Interval
	total := s.Param().(power.TimeParams).Total

	// Open a Google search window.
	url := "https://www.google.com/search?q=chromium"
	conn, br, cleanup, err := browserfixt.SetUpWithURL(ctx, cr, bt, url)
	if err != nil {
		s.Fatal("Failed to open a new tab: ", err)
	}
	defer cleanup(cleanupCtx)
	defer conn.Close()
	defer conn.CloseTarget(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get ash tconn: ", err)
	}
	bTconn, err := br.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get browser tconn: ", err)
	}

	w, err := ash.WaitForAnyWindow(ctx, tconn, ash.BrowserTypeMatch(bt))
	if err != nil {
		s.Fatal("Failed to open a browser window: ", err)
	}
	if err := ash.SetWindowStateAndWait(ctx, tconn, w.ID, ash.WindowStateMaximized); err != nil {
		s.Fatal("Failed to maximize the browser window: ", err)
	}

	ui := uiauto.New(tconn)
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Error with creating EventWriter from keyboard: ", err)
	}

	r := power.NewRecorder(ctx, interval, s.OutDir(), s.TestName(), power.DischargeWatchdogOption(discharge))
	defer r.Close(cleanupCtx)
	// Register two histogram metrics: one has samples along the timeline, and
	// the other one only has an average value.
	//
	// Examples of an timeline based metric:
	// "histogram.EventLatencyKeyPressedTotalLatency": {
	//   "summary": {
	//     "units": "us",
	//     "improvement_direction": "down",
	//     "type": "list_of_scalar_values",
	//     "interval": "t",
	//     "values": [
	//       49558.72222222222,
	//       48571.11111111111,
	//       50601.5,
	//       53118.681818181816,
	//       49765.13636363636,
	//       54704.63157894737
	//     ]
	//   }
	// }
	//
	// Example of the average metric:
	// "histogram.EventLatencyMousePressedTotalLatency": {
	//   "average": {
	//     "units": "us",
	//     "improvement_direction": "down",
	//     "type": "scalar",
	//     "value": 71725.53846153847
	//   }
	// }
	r.RegisterMetrics(
		pm.NewHistogramMetrics(bTconn, []string{"EventLatency.KeyPressed.TotalLatency"}),
		pm.NewHistogramAverageMetrics(bTconn, []string{"EventLatency.MousePressed.TotalLatency"}),
	)
	if err := r.Cooldown(ctx); err != nil {
		s.Error("Cooldown failed: ", err)
	}
	if err := r.Start(ctx); err != nil {
		s.Fatal("Cannot start collecting power metrics: ", err)
	}

	// Start of main test body.
	loopSleep := 1000 * time.Millisecond
	endTime := time.Now().Add(total)
	clearButton := nodewith.Role(role.Button).Name(" Clear").First()
	searchBox := nodewith.Role(role.TextFieldWithComboBox).Name("Search").First()
	clearAndClick := uiauto.NamedCombine("clear and click", ui.LeftClick(clearButton), ui.LeftClick(searchBox))
	lastMouseClickTime := time.Now()
	clickFunc := clearAndClick
	for time.Now().Before(endTime) {
		if err := uiauto.Combine("click and type",
			clickFunc,
			kb.TypeAction("chromium"),
		)(ctx); err != nil {
			s.Fatal("Failed to do ui operations: ", err)
		}
		// To demonstrate that the "average" metric works when metric snapshot
		// interval is smaller than metric generation interval, do the mouse
		// click less frequently at a larger interval.
		if time.Now().Sub(lastMouseClickTime) <= 2*interval {
			clickFunc = func(ctx context.Context) error { return nil }
		} else {
			clickFunc = clearAndClick
			lastMouseClickTime = time.Now()
		}
		if time.Now().After(endTime) {
			break
		}
		// GoBigSleepLint: sleep before another round of UI operations.
		if err := testing.Sleep(ctx, loopSleep); err != nil {
			s.Fatal("Failed to sleep: ", err)
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
