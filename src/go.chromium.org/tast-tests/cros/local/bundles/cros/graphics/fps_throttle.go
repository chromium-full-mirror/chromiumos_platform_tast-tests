// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package graphics

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/graphics"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         FPSThrottle,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check that device FPS throttles and recovers",
		Contacts:     []string{"chromeos-gfx-display@google.com", "ddavenport@google.com"},
		BugComponent: "b:188154", // ChromeOS > Platform > Graphics > Display
		Attr:         []string{"group:graphics", "graphics_nightly"},
		SoftwareDeps: []string{"chrome", "no_chrome_dcheck", "no_qemu"},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay(), hwdep.SeamlessRefreshRate()),
		Fixture:      "chromeGraphics",
		Data:         []string{"fps.html"},
		Timeout:      5 * time.Minute,
	})
}

func measureRefreshRate(ctx context.Context, outputPath string) (float64, error) {
	const (
		// Collect statistics for 5 seconds.
		collectTime = 5 * time.Second

		// Trim 10% outliers of measurements.
		trimPercent = 10

		// Minimum number of vblank samples.
		minimumSamples = 30
	)

	if err := graphics.ClearTraceBuffer(); err != nil {
		return 0.0, errors.Wrap(err, "failed to clear trace buffer")
	}
	defer graphics.ClearTraceBuffer()

	if err := graphics.CollectVblankTrace(ctx, collectTime, outputPath); err != nil {
		return 0.0, errors.Wrap(err, "failed to collect vblank trace")
	}

	// Parse trace file and compute statistics.
	traceData, err := graphics.ParseVblankTrace(outputPath)
	if err != nil {
		return 0.0, errors.Wrap(err, "cannot parse trace")
	}

	// Assume that crtc 0 is the internal display.
	internalDisplayIndex := 0
	if len(traceData) == 0 || len(traceData[internalDisplayIndex]) < minimumSamples {
		return 0.0, errors.New("Few or no vblank samples")
	}

	stats := graphics.CalculateVblankStats(traceData[internalDisplayIndex], trimPercent)
	return graphics.Hz(stats.Mean), nil
}

func FPSThrottle(ctx context.Context, s *testing.State) {
	const (
		// Time to wait after sending D-Bus signal indicating a change in AC state.
		powerStatePropagationDelay = 2 * time.Second

		// Minimum acceptable threshold for unpluggedRefreshRate/pluggedInRefreshRate.
		rateDifferenceThreshold = 0.75

		// Two refresh rates within this threshold are close enough to be considered the same.
		sameRateThreshold = 0.9

		// Battery percent at or under which the refresh rate is expected to be throttled when on battery power.
		lowBatteryLevel = 10
	)
	// Open web page with constantly changing content to defeat PSR.
	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()
	testURL := server.URL + "/fps.html"

	cr := s.FixtValue().(*chrome.Chrome)
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	infos, err := display.GetInfo(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get display info: ", err)
	}

	conn, err := cr.NewConn(ctx, testURL)
	if err != nil {
		s.Fatalf("Failed to open %s: %v", testURL, err)
	}
	defer conn.Close()

	if err := conn.WaitForExpr(ctx, "document.readyState === 'complete'"); err != nil {
		s.Fatal("Waiting load failed: ", err)
	}

	// Stop powerd so we can simulate AC state.
	if err := upstart.StopJob(ctx, "powerd"); err != nil {
		s.Fatal("Failed to stop powerd job: ", err)
	}

	// Iterate over each display.
	for _, info := range infos {
		// Only check internal display.
		if !info.IsInternal {
			continue
		}

		// Get the refresh rate for the plugged-in state.
		outputPath := filepath.Join(s.OutDir(), "pluggedInTrace.txt")
		pluggedInRefreshRate, err := measureRefreshRate(ctx, outputPath)
		if err != nil {
			s.Fatal("Failed to detect refresh rate: ", err)
		}
		s.Logf("Plugged-in refresh rate: %f", pluggedInRefreshRate)

		// Send D-Bus signal indicating AC is unplugged, and wait for downstream clients to react to it.
		if err := testexec.CommandContext(ctx, "sudo", "-u", "power", "send_debug_power_status", "--external_power=2", fmt.Sprintf("--battery_percent=%d", lowBatteryLevel)).Run(); err != nil {
			s.Fatal("Failed to send power status: ", err)
		}
		// GoBigSleepLint: Wait for power state propagation.
		if err := testing.Sleep(ctx, powerStatePropagationDelay); err != nil {
			s.Fatal("Cannot sleep: ", err)
		}

		// Get the refresh rate for the unplugged state.
		outputPath = filepath.Join(s.OutDir(), "unPluggedTrace.txt")
		unpluggedRefreshRate, err := measureRefreshRate(ctx, outputPath)
		if err != nil {
			s.Fatal("Failed to detect refresh rate: ", err)
		}
		s.Logf("Unplugged refresh rate: %f", unpluggedRefreshRate)

		// Verify that the unplugged refresh rate is lower than the plugged in refresh rate by some threshold
		if unpluggedRefreshRate/pluggedInRefreshRate > rateDifferenceThreshold {
			s.Fatalf("Refresh rate did not decrease when unplugged: %f -> %f", pluggedInRefreshRate, unpluggedRefreshRate)
		}

		// Re-plug and verify the transition back to high framerate.
		if err := testexec.CommandContext(ctx, "sudo", "-u", "power", "send_debug_power_status", "--external_power=0", "--battery_percent=100").Run(); err != nil {
			s.Fatal("Failed to send power status: ", err)
		}
		// GoBigSleepLint: Wait for power state propagation.
		if err := testing.Sleep(ctx, powerStatePropagationDelay); err != nil {
			s.Fatal("Cannot sleep: ", err)
		}

		// Get the refresh rate for the plugged-in state again.
		outputPath = filepath.Join(s.OutDir(), "rePluggedInTrace.txt")
		rePluggedRefreshRate, err := measureRefreshRate(ctx, outputPath)
		if err != nil {
			s.Fatal("Failed to detect refresh rate: ", err)
		}
		s.Logf("Re-plugged refresh rate: %f", rePluggedRefreshRate)

		// Check that it's the same as before unplugging.
		if math.Min(pluggedInRefreshRate, rePluggedRefreshRate)/math.Max(pluggedInRefreshRate, rePluggedRefreshRate) < sameRateThreshold {
			s.Fatalf("Refresh rate did not recover when re-plugged: %f != %f", pluggedInRefreshRate, rePluggedRefreshRate)
		}

		pv := perf.NewValues()
		defer func() {
			if err := pv.Save(s.OutDir()); err != nil {
				s.Error("Failed to save perf data: ", err)
			}
		}()
		pv.Set(perf.Metric{
			Name:      "plugged_in",
			Unit:      "fps",
			Direction: perf.BiggerIsBetter,
		}, float64(pluggedInRefreshRate))
		pv.Set(perf.Metric{
			Name:      "unplugged",
			Unit:      "fps",
			Direction: perf.BiggerIsBetter,
		}, float64(unpluggedRefreshRate))
	}
}
