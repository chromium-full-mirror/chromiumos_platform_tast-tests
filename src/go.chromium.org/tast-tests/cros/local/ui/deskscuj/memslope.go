// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package deskscuj

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/memory"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// memSettleTime is how long to wait before each memory snapshot, so that
// window creation and page loads settle.
const memSettleTime = 5 * time.Second

// Process categories of memory.GetHostMetrics.
const (
	chromeBrowserCategory  = "chrome_browser"
	chromeGPUCategory      = "chrome_gpu"
	chromeRendererCategory = "chrome_renderer"
	chromeOtherCategory    = "chrome_other"
)

// memSample is one memory snapshot, taken with a given number of open
// windows.
type memSample struct {
	windows    int
	systemMB   float64 // MemTotal - MemAvailable.
	chromeMB   float64 // Sum of PSS of all Chrome processes.
	gpuMB      float64 // PSS of the GPU process.
	rendererMB float64 // Sum of PSS of renderer processes.
	// The same as above, but PSS + SwapPss. Swapped-out pages (zram) still
	// count, so these don't drop when the kernel swaps under memory
	// pressure.
	chromeWithSwapMB   float64
	gpuWithSwapMB      float64
	rendererWithSwapMB float64
}

// memTracker takes memory snapshots as windows are opened, and reports the
// memory cost per window as the least-squares slope of memory over the
// number of open windows.
//
// Why a slope: some memory costs are per window. For example, every
// browser window owns its own omnibox popups, so their cost grows with the
// number of windows. Total memory at one point in time mixes that cost
// with memory that doesn't depend on the number of windows (the OS, the
// browser process, other processes), which differs between devices and
// images and is often larger. Modeling memory as
//
//	memory = baseline + perWindow * windows
//
// and fitting a line to snapshots taken at several window counts separates
// the two: the slope is perWindow, in MB per window. Fitting all snapshots
// (e.g. at 5, 9, 11 and 12 windows), rather than taking the difference
// between the first and the last, also averages out the noise of any one
// snapshot.
//
// Two more snapshots are reported separately, and aren't used for the
// slope, so that it only covers windows opened the same way:
//   - A snapshot after more windows were opened in a different way (e.g.
//     via the omnibox, so that their omnibox popups were shown), reported
//     as the memory per additional window (<prefix>.MemPerShownWindow.*).
//   - A final snapshot after the workload, reported as is
//     (<prefix>.Mem.Final.*) and as the growth since the last window was
//     opened (<prefix>.MemGrowth.*).
type memTracker struct {
	// prefix is prepended to the reported metric names, e.g. "Omnibox"
	// gives "Omnibox.MemSlope.SystemUsed".
	prefix string
	// samples are the snapshots used for the slope.
	samples []memSample
	// shown is the snapshot after windows were opened in a different way,
	// if any.
	shown *memSample
	// final is the snapshot taken after the workload, if any.
	final *memSample
}

// newMemTracker returns a memTracker that reports metrics with the given
// name prefix.
func newMemTracker(prefix string) *memTracker {
	return &memTracker{prefix: prefix}
}

// snapshot records a memory sample for the slope, with the given number of
// open windows. Take it right after the windows are opened. Failures are
// logged and don't fail the test.
func (t *memTracker) snapshot(ctx context.Context, windows int) {
	if sample, ok := captureAndLogMemSample(ctx, windows); ok {
		t.samples = append(t.samples, sample)
	}
}

// snapshotShown records a memory sample after more windows were opened in a
// different way than the slope windows, e.g. via the omnibox so that their
// popups were shown. It isn't used for the slope. Failures are logged and
// don't fail the test.
func (t *memTracker) snapshotShown(ctx context.Context, windows int) {
	if sample, ok := captureAndLogMemSample(ctx, windows); ok {
		t.shown = &sample
	}
}

// snapshotFinal records a memory sample after the workload. It isn't used
// for the slope. Failures are logged and don't fail the test.
func (t *memTracker) snapshotFinal(ctx context.Context, windows int) {
	if sample, ok := captureAndLogMemSample(ctx, windows); ok {
		t.final = &sample
	}
}

// captureAndLogMemSample captures and logs a memory sample. It returns
// false if the sample couldn't be captured.
func captureAndLogMemSample(ctx context.Context, windows int) (memSample, bool) {
	sample, err := captureMemSample(ctx, windows)
	if err != nil {
		testing.ContextLogf(ctx, "Failed to capture memory at %d windows: %v", windows, err)
		return memSample{}, false
	}
	testing.ContextLogf(ctx, "Memory at %d windows: system=%.1fMB chrome=%.1fMB(+swap %.1fMB) gpu=%.1fMB(+swap %.1fMB) renderer=%.1fMB(+swap %.1fMB)",
		windows, sample.systemMB, sample.chromeMB, sample.chromeWithSwapMB, sample.gpuMB, sample.gpuWithSwapMB, sample.rendererMB, sample.rendererWithSwapMB)
	return sample, true
}

// captureMemSample captures system used memory and per-process-type PSS.
func captureMemSample(ctx context.Context, windows int) (memSample, error) {
	// GoBigSleepLint: Let window creation and page loads settle before
	// sampling memory.
	if err := testing.Sleep(ctx, memSettleTime); err != nil {
		return memSample{}, err
	}

	hostMetrics, err := memory.GetHostMetrics(ctx, "", "")
	if err != nil {
		return memSample{}, errors.Wrap(err, "failed to read memory metrics")
	}

	// The host metrics are in KiB.
	pssMB := func(category string) float64 {
		if metrics, ok := hostMetrics.CategoryMetrics[category]; ok {
			return float64(metrics.Pss) / memory.KiBInMiB
		}
		return 0
	}
	pssWithSwapMB := func(category string) float64 {
		if metrics, ok := hostMetrics.CategoryMetrics[category]; ok {
			return float64(metrics.Pss+metrics.PssSwap) / memory.KiBInMiB
		}
		return 0
	}
	sample := memSample{
		windows:            windows,
		systemMB:           float64(hostMetrics.LinuxUsed) / memory.KiBInMiB,
		gpuMB:              pssMB(chromeGPUCategory),
		rendererMB:         pssMB(chromeRendererCategory),
		gpuWithSwapMB:      pssWithSwapMB(chromeGPUCategory),
		rendererWithSwapMB: pssWithSwapMB(chromeRendererCategory),
	}
	sample.chromeMB = sample.gpuMB + sample.rendererMB +
		pssMB(chromeBrowserCategory) + pssMB(chromeOtherCategory)
	sample.chromeWithSwapMB = sample.gpuWithSwapMB + sample.rendererWithSwapMB +
		pssWithSwapMB(chromeBrowserCategory) + pssWithSwapMB(chromeOtherCategory)
	return sample, nil
}

// linearSlope returns the slope b of the least-squares line y = a + b*x
// through the points (x[i], y[i]), i.e. the b (and a) that minimize the
// sum of squared errors sum((y[i] - a - b*x[i])^2). The solution is
//
//	b = sum((x[i] - xMean) * (y[i] - yMean)) / sum((x[i] - xMean)^2)
//
// where xMean and yMean are the means of x and y. For memory snapshots, x
// is the number of open windows and y the memory in MB, so b is the
// memory per window in MB. The intercept a (the memory that doesn't
// depend on the number of windows) isn't needed and isn't computed.
func linearSlope(x, y []float64) (float64, error) {
	if len(x) != len(y) {
		return 0, errors.Errorf("mismatched lengths: %d vs %d", len(x), len(y))
	}
	n := len(x)
	if n < 2 {
		return 0, errors.Errorf("need at least 2 points, got %d", n)
	}
	var xMean, yMean float64
	for i := 0; i < n; i++ {
		xMean += x[i]
		yMean += y[i]
	}
	xMean /= float64(n)
	yMean /= float64(n)

	// numerator = sum((x[i] - xMean) * (y[i] - yMean))
	// denominator = sum((x[i] - xMean)^2)
	var numerator, denominator float64
	for i := 0; i < n; i++ {
		numerator += (x[i] - xMean) * (y[i] - yMean)
		denominator += (x[i] - xMean) * (x[i] - xMean)
	}
	if denominator == 0 {
		return 0, errors.New("all x values are equal")
	}
	return numerator / denominator, nil
}

// memMetrics are the reported memory metrics. Chrome, GPU and renderer
// memory is reported both as PSS and as PSS + SwapPss ("WithSwap"), since
// PSS alone drops when pages are swapped out under memory pressure.
var memMetrics = []struct {
	name string
	get  func(memSample) float64
}{
	{"SystemUsed", func(sample memSample) float64 { return sample.systemMB }},
	{"ChromePSS", func(sample memSample) float64 { return sample.chromeMB }},
	{"ChromePSSWithSwap", func(sample memSample) float64 { return sample.chromeWithSwapMB }},
	{"GPU", func(sample memSample) float64 { return sample.gpuMB }},
	{"GPUWithSwap", func(sample memSample) float64 { return sample.gpuWithSwapMB }},
	{"Renderer", func(sample memSample) float64 { return sample.rendererMB }},
	{"RendererWithSwap", func(sample memSample) float64 { return sample.rendererWithSwapMB }},
}

// record saves:
//   - the slope snapshots as <prefix>.Mem.*, and the per-window memory
//     slopes over them as <prefix>.MemSlope.*;
//   - if a shown snapshot was taken, the memory per window opened since
//     the last slope snapshot as <prefix>.MemPerShownWindow.*;
//   - if a final snapshot was taken, the final values as
//     <prefix>.Mem.Final.*, and the growth since the last window was
//     opened (the shown snapshot, or else the last slope snapshot) as
//     <prefix>.MemGrowth.*.
func (t *memTracker) record(ctx context.Context, pv *perf.Values) {
	t.recordSlopes(ctx, pv)
	t.recordShown(ctx, pv)
	t.recordFinal(ctx, pv)
}

// lastSample returns the last slope snapshot, or nil if there is none.
func (t *memTracker) lastSample() *memSample {
	if len(t.samples) == 0 {
		return nil
	}
	return &t.samples[len(t.samples)-1]
}

// shownSample returns the shown snapshot if it was taken with more windows
// than the last slope snapshot, or nil.
func (t *memTracker) shownSample() *memSample {
	last := t.lastSample()
	if t.shown == nil || last == nil || t.shown.windows <= last.windows {
		return nil
	}
	return t.shown
}

// recordSlopes saves the slope snapshots and the per-window memory slopes
// over them.
func (t *memTracker) recordSlopes(ctx context.Context, pv *perf.Values) {
	windows := make([]float64, len(t.samples))
	for i, sample := range t.samples {
		windows[i] = float64(sample.windows)
	}
	pv.Set(perf.Metric{
		Name:      t.prefix + ".Mem.Windows",
		Unit:      "count",
		Direction: perf.SmallerIsBetter,
		Multiple:  true,
	}, windows...)
	for _, metric := range memMetrics {
		values := make([]float64, len(t.samples))
		for i, sample := range t.samples {
			values[i] = metric.get(sample)
		}
		pv.Set(perf.Metric{
			Name:      t.prefix + ".Mem." + metric.name,
			Unit:      "MB",
			Direction: perf.SmallerIsBetter,
			Multiple:  true,
		}, values...)
		slope, err := linearSlope(windows, values)
		if err != nil {
			testing.ContextLogf(ctx, "Failed to compute the %s memory slope: %v", metric.name, err)
			continue
		}
		testing.ContextLogf(ctx, "Memory slope %s: %.2f MB/window", metric.name, slope)
		pv.Set(perf.Metric{
			Name:      t.prefix + ".MemSlope." + metric.name,
			Unit:      "MB_per_window",
			Direction: perf.SmallerIsBetter,
		}, slope)
	}
}

// recordShown saves the memory per window opened between the last slope
// snapshot and the shown snapshot, if one was taken.
func (t *memTracker) recordShown(ctx context.Context, pv *perf.Values) {
	shown := t.shownSample()
	if shown == nil {
		return
	}
	last := t.lastSample()
	for _, metric := range memMetrics {
		perWindow := (metric.get(*shown) - metric.get(*last)) / float64(shown.windows-last.windows)
		testing.ContextLogf(ctx, "Memory per shown window %s: %.2f MB/window", metric.name, perWindow)
		pv.Set(perf.Metric{
			Name:      t.prefix + ".MemPerShownWindow." + metric.name,
			Unit:      "MB_per_window",
			Direction: perf.SmallerIsBetter,
		}, perWindow)
	}
}

// recordFinal saves the final snapshot, if one was taken, and the growth
// since the last window was opened.
func (t *memTracker) recordFinal(ctx context.Context, pv *perf.Values) {
	if t.final == nil {
		return
	}
	last := t.shownSample()
	if last == nil {
		last = t.lastSample()
	}
	for _, metric := range memMetrics {
		final := metric.get(*t.final)
		pv.Set(perf.Metric{
			Name:      t.prefix + ".Mem.Final." + metric.name,
			Unit:      "MB",
			Direction: perf.SmallerIsBetter,
		}, final)
		if last == nil {
			continue
		}
		growth := final - metric.get(*last)
		testing.ContextLogf(ctx, "Memory growth %s after the last window was opened: %.1f MB", metric.name, growth)
		pv.Set(perf.Metric{
			Name:      t.prefix + ".MemGrowth." + metric.name,
			Unit:      "MB",
			Direction: perf.SmallerIsBetter,
		}, growth)
	}
}
