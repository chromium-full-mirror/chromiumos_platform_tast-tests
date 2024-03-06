// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package getdisplaymedia provides common code for WebRTC's getDisplayMedia
// tests; this API is used for screen, window and tab capture, see
// https://developer.mozilla.org/en-US/docs/Web/API/MediaDevices/getDisplayMedia
// and https://w3c.github.io/mediacapture-screen-share/.
package getdisplaymedia

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/audio/crastestclient"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/cpu"
	"go.chromium.org/tast-tests/cros/local/graphics"
	"go.chromium.org/tast-tests/cros/local/power/metrics"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// htmlFile is the file containing the HTML+JS code exercising getDisplayMedia().
	htmlFile            = "getdisplaymedia.html"
	measurementDuration = 60 * time.Second
)

// RunGetDisplayMediaPerf drives the code verifying the getDisplayMedia functionality and collects performance data.
func RunGetDisplayMediaPerf(ctx context.Context, fileSystem http.FileSystem, cs ash.ConnSource, tconn, bTconn *chrome.TestConn, surfaceType string) error {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 3*time.Second)
	defer cancel()

	// Rotate the display to landscape-primary.
	if _, err := display.GetInternalInfo(ctx, tconn); err == nil {
		if err = graphics.RotateDisplayToLandscapePrimary(ctx, tconn); err != nil {
			return errors.Wrap(err, "failed to set display to landscape-primary orientation")
		}
	}
	// Set shelf to auto-hide.
	dispInfo, err := display.GetPrimaryInfo(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to get primary display info")
	}
	origShelfBehavior, err := ash.GetShelfBehavior(ctx, tconn, dispInfo.ID)
	if err != nil {
		return errors.Wrap(err, "failed to get shelf behavior")
	}
	if err := ash.SetShelfBehavior(ctx, tconn, dispInfo.ID, ash.ShelfBehaviorAlwaysAutoHide); err != nil {
		return errors.Wrap(err, "failed to set shelf behavior to Auto Hide")
	}
	defer ash.SetShelfBehavior(cleanupCtx, tconn, dispInfo.ID, origShelfBehavior)

	if err := crastestclient.Mute(ctx); err != nil {
		return errors.Wrap(err, "failed to mute device")
	}
	defer crastestclient.Unmute(ctx)

	server := httptest.NewServer(http.FileServer(fileSystem))
	defer server.Close()

	conn, err := cs.NewConn(ctx, server.URL+"/"+htmlFile)
	if err != nil {
		return errors.Wrapf(err, "failed to open %v", htmlFile)
	}
	defer conn.Close()
	defer conn.CloseTarget(cleanupCtx)

	// Maximize the playback window.
	w, err := ash.WaitForAnyWindowWithTitle(ctx, tconn, "GetDisplayMedia test")
	if err != nil {
		return errors.Wrap(err, "failed to find the window with the title GetDisplayMedia test")
	}
	if err := ash.SetWindowStateAndWait(ctx, tconn, w.ID, ash.WindowStateMaximized); err != nil {
		return errors.Wrap(err, "failed to maximize the window with the title GetDisplayMedia test")
	}

	if err := cpu.Cooldown(ctx); err != nil {
		return errors.Wrap(err, "failed waiting for CPU to cool down")
	}
	testing.ContextLog(ctx, "Starting GetDisplayMedia()")
	if err := conn.Call(ctx, nil, "start", surfaceType); err != nil {
		return errors.Wrap(err, "failed to run getDisplayMedia()")
	}

	p := graphics.NewThreadSafePerfValues()
	// Save the perf result to OutDir even if something went wrong while measuring the performance.
	defer func() {
		outDir, ok := testing.ContextOutDir(ctx)
		if !ok {
			return
		}
		p.GetUnderlyingValues().Save(outDir)
	}()

	var gpuPerf *perf.Values
	var gpuMetricsErr, gpuErr, i915IRQErr, cStateErr, cpuErr, batErr, wakeupErr error
	var wg sync.WaitGroup
	wg.Add(7)
	go func() {
		defer wg.Done()
		gpuPerf, gpuMetricsErr = measureGPUMetrics(ctx)
	}()
	go func() {
		defer wg.Done()
		gpuErr = graphics.MeasureGPUCounters(ctx, measurementDuration, p)
	}()
	go func() {
		defer wg.Done()
		i915IRQErr = graphics.MeasureI915IRQs(ctx, measurementDuration, p)
	}()
	go func() {
		defer wg.Done()
		cStateErr = graphics.MeasurePackageCStateCounters(ctx, measurementDuration, p)
	}()
	go func() {
		defer wg.Done()
		cpuErr = graphics.MeasureCPUUsageAndPower(ctx, 0 /*stabilizationDuration*/, measurementDuration, p)
	}()
	go func() {
		defer wg.Done()
		batErr = graphics.MeasureSystemPowerConsumption(ctx, tconn, measurementDuration, p)
	}()
	go func() {
		defer wg.Done()
		wakeupErr = graphics.MeasureThreadPoolUnnecessaryWakeups(ctx, bTconn, measurementDuration, p)
	}()

	wg.Wait()
	if gpuMetricsErr != nil {
		return errors.Wrap(gpuErr, "failed to measure GPU counters")
	}
	if gpuErr != nil {
		return errors.Wrap(gpuErr, "failed to measure GPU counters")
	}
	if i915IRQErr != nil {
		return errors.Wrap(i915IRQErr, "failed to measure i915 IRQs/s")
	}
	if cStateErr != nil {
		return errors.Wrap(cStateErr, "failed to measure Package C-State residency")
	}
	if cpuErr != nil {
		return errors.Wrap(cpuErr, "failed to measure CPU/Package power")
	}
	if batErr != nil {
		return errors.Wrap(batErr, "failed to measure system power consumption")
	}
	if wakeupErr != nil {
		return errors.Wrap(wakeupErr, "failed to measure unnecessary wakeups of ThreadPool")
	}
	p.GetUnderlyingValues().Merge(gpuPerf)
	return nil
}

// DataFiles returns a list of files required to run the tests in this package.
func DataFiles() []string {
	return []string{
		htmlFile,
		"canvas_animation.js",
		"third_party/blackframe.js",
	}
}

// measureGPUMetrics measures GPU frequency and GPU usage, and returns them as perf.Values.
// TODO(b/293221069): Remove this function if the power library enables to
// configure collected metrics.
func measureGPUMetrics(ctx context.Context) (*perf.Values, error) {
	const powerMeasurementInterval = 5 * time.Second

	metrics, err := perf.NewTimeline(ctx, []perf.TimelineDatasource{
		metrics.NewGPUFreqMetrics(),
		metrics.NewGPUUsageDataSource(),
	}, perf.Interval(powerMeasurementInterval))
	if err != nil {
		return nil, errors.Wrap(err, "failed to build metrics timeline")
	}
	if err := metrics.Start(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to start metrics")
	}
	if err := metrics.StartRecording(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to start recording")
	}
	// GoBigSleepLint: Sleep to measure the performance metrics.
	if err := testing.Sleep(ctx, measurementDuration); err != nil {
		return nil, errors.Wrapf(err, "failed to sleep for %v", measurementDuration)
	}

	gpuPerf, err := metrics.StopRecording(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to stop recording")
	}
	return gpuPerf, nil
}
