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

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/graphics"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

var ms = graphics.MsF
var hz = graphics.Hz
var fromHz = graphics.FromHz

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
			Val:               browser.TypeLacros,
			Fixture:           "chromeGraphicsLacros",
		}},
	})
}

func FPS(ctx context.Context, s *testing.State) {
	const (
		// Collect statistics for 5 seconds.
		collectTime = 5 * time.Second
		// Trim 10% outliers of measurements.
		trimPercent = 10
		// Minimum number of vblank samples required.
		minSamples = 30
	)

	// Accept a margin of error equivalent to 0.2 fps at 60Hz.
	margin := fromHz(60.0) - fromHz(60.2)
	// Accept a standard deviation equivalent to 0.2 fps at 60Hz.
	maxStddev := fromHz(60.0) - fromHz(60.2)

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

			// 5 seconds is chosen arbitrarily and may need adjustment.
			const configurationDelay = 5 * time.Second
			// GoBigSleepLint: Wait for display configuration to update.
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
			if err := graphics.CollectVblankTrace(ctx, collectTime, outputPath); err != nil {
				s.Fatal("Failed to collect vblank trace: ", err)
			}

			crtcs, err := graphics.ModetestCrtcs(ctx)
			if err != nil {
				s.Fatal("Failed to read crtcs from modetest: ", err)
			}

			// Parse trace file and compute statistics.
			traceData, err := graphics.ParseVblankTrace(outputPath)
			if err != nil {
				s.Fatal("Cannot parse trace: ", err)
			}

			// Check trace data for each crtc at its respective refresh rate.
			for index, crtc := range crtcs {
				if crtc.Mode == nil {
					continue
				}

				if crtc.Mode.Refresh <= 0 {
					continue
				}
				nominalInterval := fromHz(crtc.Mode.Refresh)
				s.Logf("Checking CRTC #%d at %fms (%fHz)", index, ms(nominalInterval), hz(nominalInterval))

				// If there are few samples due to i.e. PSR, skip this crtc.
				if len(traceData) <= index || len(traceData[index]) < minSamples {
					s.Logf("Not enough vblank samples for CRTC #%d", index)
					continue
				}

				// Log untrimmed stats.
				stats := graphics.CalculateVblankStats(traceData[index], 0)
				s.Logf("%d total samples, mean: %fms (%fHz), stddev: %fms (min/max %f/%f)",
					stats.NumSamples, ms(stats.Mean), hz(stats.Mean), ms(stats.Stddev),
					ms(stats.Min), ms(stats.Max))

				// Check results after trimming outliers.
				stats = graphics.CalculateVblankStats(traceData[index], trimPercent)
				s.Logf("%d trimmed samples, mean: %fms (%fHz), stddev: %fms (min/max %f/%f)",
					stats.NumSamples, ms(stats.Mean), hz(stats.Mean), ms(stats.Stddev),
					ms(stats.Min), ms(stats.Max))

				// Check results.
				if time.Duration.Abs(stats.Mean-nominalInterval) > margin {
					s.Fatalf("Mean interval of %fms (%fHz) out of expected range %fms (%fHz) +/- %fms",
						ms(stats.Mean), hz(stats.Mean), ms(nominalInterval), hz(nominalInterval),
						ms(margin))
				}

				// TODO(b/172225622): re-enable stddev check if we can find
				// meaningful bounds.
				if stats.Stddev > maxStddev {
					s.Logf("Standard deviation %fms too large (> %f)", ms(stats.Stddev),
						ms(maxStddev))
				}
			}
		}

		// Move the window to the next display.
		if err := kb.Accel(ctx, "Search+Alt+M"); err != nil {
			s.Fatal("Failed to send keyboard action to move the active window between displays: ",
				err)
		}
	}
}
