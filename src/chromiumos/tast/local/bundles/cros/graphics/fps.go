// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package graphics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"time"

	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/chrome/display"
	"chromiumos/tast/local/graphics"
	"chromiumos/tast/local/input"
	"chromiumos/tast/testing"
	"chromiumos/tast/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         FPS,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Measure frames per second and check it is close to expected fps",
		BugComponent: "b:1021073", // ChromeOS > Platform > Graphics > Compositor
		Contacts:     []string{"chromeos-gfx-compositor@google.com"},
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"chrome", "no_chrome_dcheck", "no_qemu"},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Data:         []string{"fps.html"},
		Timeout:      5 * time.Minute,
		Params: []testing.Param{{
			Val:     browser.TypeAsh,
			Fixture: "chromeGraphics",
		}, {
			Name:              "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			// TODO(crbug.com/1430065): Remove informational when it become stable.
			ExtraAttr: []string{"informational"},
			Val:       browser.TypeLacros,
			Fixture:   "chromeGraphicsLacros",
		}},
	})
}

func FPS(ctx context.Context, s *testing.State) {
	const (
		// Collect statistics for 5 seconds.
		collectTime = 5 * time.Second

		// Trim 10% outliers of measurements.
		trimPercent = 10
		// Accept up to 0.2 fps margin over the reference fps.
		margin = 0.2
		// Accept up to 0.2 fps standard deviation.
		maxStddev = 0.2
		// Minimum number of fps samples required
		minSamples = 30
	)

	// Open web page with constantly changing content to defeat PSR.
	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()
	testURL := server.URL + "/fps.html"

	conn, br, closeBrowser, err := browserfixt.SetUpWithURL(
		ctx, s.FixtValue().(chrome.HasChrome).Chrome(), s.Param().(browser.Type), testURL)
	if err != nil {
		s.Fatal("Failed to set up browser: ", err)
	}
	defer closeBrowser(ctx)
	defer conn.Close()

	tconn, err := br.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	infos, err := display.GetInfo(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get display info: ", err)
	}

	if err := conn.WaitForExpr(ctx, "document.readyState === 'complete'"); err != nil {
		s.Fatal("Waiting load failed: ", err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to initialize the keyboard writer: ", err)
	}
	defer kb.Close(ctx)

	// Iterate over each display.
	for _, info := range infos {
		// Displays may support many modes at the same refresh rates (e.g. different
		// resolutions). Instead of testing all modes, only test one mode per unique refresh
		// rate.
		rates := make(map[float64]*display.DisplayMode)
		for _, mode := range info.Modes {
			rates[mode.RefreshRate] = mode
		}

		// Iterate over each refresh rate for the current display.
		for _, mode := range rates {
			if err := display.SetDisplayProperties(ctx, tconn, info.ID,
				display.DisplayProperties{DisplayMode: mode}); err != nil {
				s.Fatal("Failed to set display properties: ", err)
			}

			// Wait for display configuration to update.
			// 5 seconds is chosen arbitrarily and may need adjustment.
			const configurationDelay = 5 * time.Second
			if err := testing.Sleep(ctx, configurationDelay); err != nil {
				s.Fatal("Cannot sleep: ", err)
			}

			// Acknowledge the configuration change or it will automatically revert
			// after some time.
			if err := kb.Accel(ctx, "Enter"); err != nil {
				s.Fatal("Failed to send keyboard action to confirm display configuration: ",
					err)
			}

			// Clear trace file.
			if err := graphics.ClearTraceBuffer(); err != nil {
				s.Fatal("Failed to clear trace buffer: ", err)
			}
			defer graphics.ClearTraceBuffer()

			outputPath := filepath.Join(s.OutDir(), "trace.txt")
			if err := graphics.CollectFPSTrace(ctx, collectTime, outputPath); err != nil {
				s.Fatal("Failed to collect fps trace: ", err)
			}

			crtcs, err := graphics.ModetestCrtcs(ctx)
			if err != nil {
				s.Fatal("Failed to read crtcs from modetest: ", err)
			}

			// Parse trace file and compute statistics.
			fullFpsData, err := graphics.ParseFPSTrace(outputPath)
			if err != nil {
				s.Fatal("Cannot parse trace: ", err)
			}

			// Check trace data for each crtc at its respective refresh rate.
			for index, crtc := range crtcs {
				if crtc.Mode == nil {
					continue
				}

				targetFPS := crtc.Mode.Refresh
				if targetFPS <= 0 {
					continue
				}
				s.Logf("Checking crtc=%d at %fHz", index, targetFPS)

				// If there are few samples due to i.e. PSR, skip this crtc.
				if len(fullFpsData) <= index || len(fullFpsData[index]) < minSamples {
					s.Logf("Not enough fps samples for crtc=%d", index)
					continue
				}

				// Log untrimmed stats.
				fpsStats := graphics.CalculateFPSStats(fullFpsData[index], 0)
				s.Logf("%d total samples, mean: %f, stddev: %f (min/max %f/%f)",
					fpsStats.NumSamples, fpsStats.Mean, fpsStats.Stddev, fpsStats.Min,
					fpsStats.Max)

				// Check results after trimming outliers.
				fpsStats = graphics.CalculateFPSStats(fullFpsData[index], trimPercent)
				s.Logf("%d trimmed samples, mean: %f, stddev: %f (min/max %f/%f)",
					fpsStats.NumSamples, fpsStats.Mean, fpsStats.Stddev, fpsStats.Min,
					fpsStats.Max)

				// Check results.
				if fpsStats.Mean > targetFPS+margin || fpsStats.Mean < targetFPS-margin {
					s.Fatalf("Mean FPS %f out of expected range %f +/- %f",
						fpsStats.Mean, targetFPS, margin)
				}

				// TODO(b/172225622): re-enable stddev check if we can find
				// meaningful bounds.
				if fpsStats.Stddev > maxStddev {
					s.Logf("FPS standard deviation %f too large (> %f)", fpsStats.Stddev,
						maxStddev)
				}
			}
		}

		// Move the window to the next display.
		if err := kb.Accel(ctx, "Search+Alt+M"); err != nil {
			s.Fatal("Failed to send keybord action to move the active window between displays: ",
				err)
		}
	}
}
