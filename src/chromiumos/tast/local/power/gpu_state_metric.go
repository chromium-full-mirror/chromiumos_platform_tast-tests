// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"math"
	"time"

	"chromiumos/tast/common/perf"
	"chromiumos/tast/errors"
	"chromiumos/tast/testing"
)

// GPUStateMetrics records the states residency of GPU.
type GPUStateMetrics struct {
	rc6Enabled   bool
	metrics      map[string]perf.Metric
	lastTime     time.Time
	lastRC6      int64
	cpuUarch     string
	intervalName string
}

// Assert that GPUStateMetrics can be used in perf.Timeline.
var _ perf.TimelineDatasource = &GPUStateMetrics{}

// maxCounter contains the max value that RC6 residency counter could record in ms.
// Old RC6 residency counter is a 32 bit register that could overflow.
// For small core processors, the tick frequency is once per ~ 833.33ns (1ms/1200)
// For big core processors, the tick frequency is once per ~1.28us (1ms/780)
// The max of counter is calculated as 2^32 * tick frequency
//
// For details, see https://patchwork.freedesktop.org/patch/79932/
//
// There are some roundings, but they're at a scale that is trivial to the final
// calculation of RC6 residency. So the numbers below are chosen to align with
// autotest: src/third_party/autotest/files/client/cros/power/power_status.py;l=3031
var maxCounter = map[string]int64{
	// Small core
	"Airmont":  3579125,
	"Goldmont": 3579125,
	// Big core
	"Broadwell": 5497558,
	"Haswell":   5497558,
	"Kaby Lake": 5497558,
	"Skylake":   5497558,
}

// hasRC6Support helps examine if RC6 is enabled with residency counter.
func hasRC6Support(ctx context.Context) bool {
	const enablepath = "/sys/class/drm/card0/power/rc6_enable"
	readResult, err := readInt64(ctx, enablepath)
	if err != nil {
		return false
	}
	return (readResult & 0x1) == 0x1
}

// readRCStateResidency reads RC6 residency info for Intel based systems.
func readRCStateResidency(ctx context.Context) (int64, error) {
	const residencyPath = "/sys/class/drm/card0/power/rc6_residency_ms"
	return readInt64(ctx, residencyPath)
}

// NewGPUStateMetrics creates the struct to store GPU state metrics.
func NewGPUStateMetrics() *GPUStateMetrics {
	newMetrics := &GPUStateMetrics{
		rc6Enabled:   false,
		metrics:      make(map[string]perf.Metric),
		lastTime:     time.Time{},
		lastRC6:      0,
		cpuUarch:     "",
		intervalName: "",
	}
	return newMetrics
}

// Setup creates the metric depending on devices' support on RC states.
func (g *GPUStateMetrics) Setup(ctx context.Context, prefix, intervalName string) error {
	if hasRC6Support(ctx) {
		g.rc6Enabled = true
		g.metrics["rc6"] = perf.Metric{
			Name:      prefix + gpuStateMetricType + "gpu_rc6",
			Unit:      gpuStateMetricTypeUnit,
			Direction: perf.BiggerIsBetter,
			Multiple:  true}
		g.metrics["rc0"] = perf.Metric{
			Name:      prefix + gpuStateMetricType + "gpu_rc0",
			Unit:      gpuStateMetricTypeUnit,
			Direction: perf.SmallerIsBetter,
			Multiple:  true}
		readCPUUarch, err := fetchIntelCPUUarch()
		if err != nil {
			return errors.Wrap(err, "unknown architecture: update power.fetchPackageStates()")
		}
		g.cpuUarch = readCPUUarch
	}
	return nil
}

// Start logs the start of GPU state metrics tracker.
// This function is required by perf.Timeline.
func (g *GPUStateMetrics) Start(ctx context.Context) error {
	if !g.rc6Enabled {
		testing.ContextLog(ctx, "Device does not enable RC6 residency counter")
		return nil
	}
	testing.ContextLog(ctx, "Start tracking GPU state metrics")
	g.lastTime = time.Now()
	readRC6, err := readRCStateResidency(ctx)
	if err != nil {
		return err
	}
	g.lastRC6 = readRC6
	return nil
}

// Snapshot logs one snapshot of all available GPU state stats once.
func (g *GPUStateMetrics) Snapshot(ctx context.Context, values *perf.Values) error {
	if !g.rc6Enabled {
		return nil
	}
	currentTime := time.Now()
	currentRC6, err := readRCStateResidency(ctx)
	if err != nil {
		return err
	}
	actualRC6 := currentRC6
	// Check for RC6 residency counter overflow.
	// Different CPU has different max value, stored in maxCounter.
	if currentRC6 < g.lastRC6 {
		if maxValue, ok := maxCounter[g.cpuUarch]; ok {
			actualRC6 += maxValue
		} else {
			testing.ContextLog(ctx, "GPU: Detect RC6 residency wraparound")
		}
	}
	rc6Ratio := float64(actualRC6-g.lastRC6) / float64(currentTime.Sub(g.lastTime).Milliseconds())

	// Since the metric reflects the ratio of a state residency, only numbers between 0 and 1
	// make sense. This number theoretically could be out of scope [0,1], at a very tiny chance,
	// as a result of roundings in max residency time and other unforeseen problems. But it
	// would be out of scope by very trivial amount such that clamping here will not distort
	// what it is meant to reflect.
	rc6Ratio = math.Min(1, rc6Ratio)
	rc6Ratio = math.Max(0, rc6Ratio)
	values.Append(g.metrics["rc6"], rc6Ratio)
	values.Append(g.metrics["rc0"], 1-rc6Ratio)
	g.lastTime = currentTime
	g.lastRC6 = currentRC6
	return nil
}

// Stop logs the stop of GPU state metrics tracker.
// This function is required by perf.Timeline. It does not need to make another snapshot.
func (g *GPUStateMetrics) Stop(ctx context.Context, values *perf.Values) error {
	if g.rc6Enabled {
		testing.ContextLog(ctx, "Stop tracking GPU metrics")
	}
	return nil
}
