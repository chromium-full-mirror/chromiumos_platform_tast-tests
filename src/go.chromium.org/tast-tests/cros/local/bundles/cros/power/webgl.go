// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/power"
	pm "go.chromium.org/tast-tests/cros/local/power/metrics"
	"go.chromium.org/tast-tests/cros/local/power/setup"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

var webGLDefaultTimeParams = power.TimeParams{Interval: 5 * time.Second, Total: 10 * time.Minute}

func init() {
	testing.AddTest(&testing.Test{
		Func:         WebGL,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Collect power metrics running heavy WebGL load. (400 jellyfishes)",
		BugComponent: "b:1361410",
		Contacts:     []string{"chromeos-platform-power@google.com"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      10*time.Minute + power.RecorderTimeout,
		Params: []testing.Param{{
			Name:    "10min_ash",
			Fixture: "powerAsh",
		}, {
			Name:              "10min_lacros",
			Fixture:           "powerLacros",
			ExtraSoftwareDeps: []string{"lacros"},
		}},
	})
}

func WebGL(ctx context.Context, s *testing.State) {
	// Reserve some time to cleanup, even if it fails due to ctx timeout.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	discharge := s.FixtValue().(setup.PowerUIFixtureData).Discharge
	bt := s.FixtValue().(setup.PowerUIFixtureData).Bt
	cr := s.FixtValue().(setup.PowerUIFixtureData).Cr

	// Open a window with about:blank tab on the target browser.
	conn, _, cleanup, err := browserfixt.SetUpWithURL(ctx, cr, bt, "about:blank")
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
	if err := ash.SetWindowStateAndWait(ctx, tconn, w.ID, ash.WindowStateFullscreen); err != nil {
		s.Fatal("Failed to make the browser window fullscreen: ", err)
	}

	interval := webGLDefaultTimeParams.Interval
	total := webGLDefaultTimeParams.Total

	r := power.NewRecorder(ctx, interval, s.OutDir(), s.TestName(), power.DischargeWatchdogOption(discharge))
	defer r.Close(cleanupCtx)

	// Register test specific metrics.
	r.RegisterMetrics(pm.NewCustomFpsMetrics(func() float64 {
		var fpsText string
		if err := conn.Eval(ctx, "document.getElementById('frameRate').innerHTML", &fpsText); err != nil {
			return 0
		}
		if fps, err := strconv.ParseFloat(fpsText, 64); err == nil {
			return fps
		}
		return 0
	}, "fps"))

	if err := r.Cooldown(ctx); err != nil {
		s.Error("Cooldown failed: ", err)
	}

	// Start of main test body.
	url := "https://storage.googleapis.com/chromiumos-test-assets-public/tast/cros/power/webgl-jellyfish/index.html"
	if err := conn.Navigate(ctx, url); err != nil {
		s.Fatal("Failed to navigate: ", err)
	}

	// Set the number of jellyfish
	if err := conn.Eval(ctx, "document.getElementById('jCount').value=400", nil); err != nil {
		s.Fatal("Failed to increase jellyfish instance count: ", err)
	}

	if err := r.Start(ctx); err != nil {
		s.Fatal("Cannot start collecting power metrics: ", err)
	}

	// GoBigSleepLint: sleep to measure power.
	if err := testing.Sleep(ctx, total); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}
	// End of main test body.

	if err := r.Finish(ctx); err != nil {
		s.Error("Cannot finish collecting power metrics: ", err)
	}

	// Clean up: open blank page.
	if err := conn.Navigate(ctx, "about:blank"); err != nil {
		s.Fatal("Failed to navigate: ", err)
	}
}
