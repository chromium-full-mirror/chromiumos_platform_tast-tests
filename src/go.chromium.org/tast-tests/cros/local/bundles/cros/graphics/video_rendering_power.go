// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package graphics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path"
	"sync"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/cpu"
	"go.chromium.org/tast-tests/cros/local/graphics"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/power/setup"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const (
	// time between gathering power metrics
	checkInterval      = 1 * time.Second
	cleanTime          = 10 * time.Second
	dischargeThreshold = 55.0
	h264Video          = "720_h264.mp4"
	vp8Video           = "720_vp8.webm"
	vp9Video           = "720_vp9.webm"
	playbackTime       = 1 * time.Minute
	stabilizeTime      = 10 * time.Second
	// We need to record twice, once for RAPL and once for system_power.
	testTime = 10 * time.Minute
)

var (
	battDischarge    = setup.NewBatteryDischarge(true, false, dischargeThreshold)
	powerTestOptions = setup.PowerTestOptions{
		// The default for the following options is to disable these setting.
		NightLight: setup.DisableNightLight,
		Powerd:     setup.DoNotChangePowerd,
		DPTF:       setup.DisableDPTF,
		Audio:      setup.Mute,
		Bluetooth:  setup.DisableBluetoothInterfaces,
	}
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         VideoRenderingPower,
		LacrosStatus: testing.LacrosVariantUnknown,
		Desc:         "Reports power consumption metrics while rendering different classes of videos",
		Contacts: []string{
			"chromeos-gfx@google.com",
			"syedfaaiz@google.com",
		},
		// ChromeOS > Platform > Graphics > GPU
		BugComponent: "b:995569",
		Attr:         []string{"graphics_perbuild", "group:graphics"},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Fixture:      "chromeGraphics",
		Timeout:      testTime,
		Params: []testing.Param{{
			Name:      "h264",
			ExtraData: []string{h264Video},
			Val:       h264Video,
		}, {
			Name:      "vp8",
			ExtraData: []string{vp8Video},
			Val:       vp8Video,
		}, {
			Name:      "vp9",
			ExtraData: []string{vp9Video},
			Val:       vp9Video,
		}},
	})
}

func VideoRenderingPower(ctx context.Context, s *testing.State) {
	// Reserve some time at the end for cleaning up.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, cleanTime)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	video := s.Param().(string)

	// setup.New configures a DUT for a test, and cleans up after.
	sup, cleanup := setup.New(video)
	defer func() {
		if err := cleanup(cleanupCtx); err != nil {
			s.Fatal("Cleanup failed: ", err)
		}
	}()

	// Add the default power test configuration.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}
	sup.Add(setup.PowerTest(ctx, tconn, powerTestOptions, battDischarge))
	if err := sup.Check(ctx); err != nil {
		s.Fatal("Setup failed: ", err)
	}

	// Get a browser instance ready to start playing the video.
	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()
	if err := cpu.WaitUntilIdle(ctx); err != nil {
		s.Error("Failed to wait for CPU to become idle: ", err)
	}
	url := path.Join(server.URL, video)
	conn, err := cr.NewConn(ctx, url)
	if err != nil {
		s.Fatal("Cannot create new tab for video playback: ", err)
	}
	defer conn.Close()
	defer conn.CloseTarget(cleanupCtx)

	// Wait for document to be completely loaded before recording metrics.
	if err = conn.WaitForExpr(ctx, "document.readyState === 'complete'"); err != nil {
		s.Fatal("Page failed to load: ", err)
	}
	// GoBigSleepLint: stabilize the machine before recording perf.
	if err = testing.Sleep(ctx, stabilizeTime); err != nil {
		s.Fatalf("Failed to sleep before starting to record metrics: %s", err)
	}

	pv := perf.NewValues()
	defer func() {
		if err := pv.Save(s.OutDir()); err != nil {
			s.Error("Failed saving perf data: ", err)
		}
	}()

	// Setup up the metrics for recording.
	metrics, err := perf.NewTimeline(
		ctx,
		power.TestMetrics(),
		perf.Interval(checkInterval),
	)
	if err != nil {
		s.Fatal("Failed to build metrics: ", err)
	}
	if err := metrics.Start(ctx); err != nil {
		s.Fatal("Failed to start metrics: ", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	var raplErr, systemPowerErr error
	go func() {
		defer wg.Done()
		if err := metrics.StartRecording(ctx); err != nil {
			raplErr = errors.Wrap(err, "failed to start metric")
			return
		}
		// GoBigSleepLint: Playback time for perf measurement.
		// Let the video play as we record metrics.
		if err := testing.Sleep(ctx, playbackTime); err != nil {
			raplErr = errors.Wrap(err, "failed to sleep for playback measurement")
			return
		}

		// Stop the recording for metrics.
		p, err := metrics.StopRecording(ctx)
		if err != nil {
			raplErr = errors.Wrap(err, "failed to snapshot metric")
			return
		}
		pv.Merge(p)
	}()
	go func() {
		defer wg.Done()
		// Record the system power consumptiom metrics.
		systemPowerErr = graphics.MeasureSystemPowerConsumption(ctx, tconn, playbackTime, pv)
	}()
	wg.Wait()
	if raplErr != nil {
		s.Fatal("Failed to record power metrics: ", raplErr)
	}
	if systemPowerErr != nil {
		s.Fatal("Error measuring system power consumption: ", systemPowerErr)
	}
}
