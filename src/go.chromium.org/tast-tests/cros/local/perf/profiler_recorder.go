// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package perf

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/profiler"
	"go.chromium.org/tast/core/errors"
)

// ProfilerRecorder records data collected via profilers.
type ProfilerRecorder struct {
	// Prefix for the metric names.
	prefix string

	// Collection intervals.
	interval time.Duration

	// Output dir.
	outDir string

	// Instructions at intervals from profiler.
	statInst profiler.PerfStatInstructionsAtIntervalsOutput

	p *profiler.RunningProf
}

// NewProfilerRecorder creates a new instance of ProfilerRecorder.
func NewProfilerRecorder(ctx context.Context, prefix string, interval time.Duration, outDir string) (*ProfilerRecorder, error) {
	return &ProfilerRecorder{
		prefix:   prefix,
		interval: interval,
		outDir:   outDir,
	}, nil
}

// Start starts profiling.
func (t *ProfilerRecorder) Start(ctx context.Context) error {
	p, err := profiler.Start(ctx, t.outDir,
		profiler.Perf(profiler.PerfStatInstructionsAtIntervalsOpts(&t.statInst, t.interval.Milliseconds())),
	)
	if err != nil {
		return errors.Wrap(err, "failed to start profiler")
	}

	t.p = p
	return nil
}

// Stop stops profiling.
func (t *ProfilerRecorder) Stop(ctx context.Context) error {
	if t.p == nil {
		return nil
	}

	if err := t.p.End(ctx); err != nil {
		return errors.Wrap(err, "failed to stop profiler")
	}

	t.p = nil
	return nil
}

// Record records the collected data.
func (t *ProfilerRecorder) Record(pv *perf.Values) {
	baseName := t.prefix + "Profiler.Instructions"
	intervalName := baseName + ".t"

	timeMetric := perf.Metric{
		Name:     intervalName,
		Unit:     "s",
		Multiple: true,
	}

	var metricMap = make(map[string]perf.Metric)
	var totalMap = make(map[string]int64)
	for _, data := range t.statInst.InstructionsAtIntervals {
		pv.Append(timeMetric, data.Timestamp.Seconds())

		for _, instPerCPU := range data.InstructionsPerCPU {
			core := instPerCPU.CoreType
			if _, ok := metricMap[core]; !ok {
				metricMap[core] = perf.Metric{
					Name:      baseName + "." + core,
					Unit:      "count",
					Multiple:  true,
					Direction: perf.SmallerIsBetter,
					Interval:  intervalName,
				}
			}

			pv.Append(metricMap[core], float64(instPerCPU.Value))

			if total, ok := totalMap[core]; ok {
				totalMap[core] = total + instPerCPU.Value
			} else {
				totalMap[core] = instPerCPU.Value
			}
		}
	}

	var grandTotal int64
	for core, total := range totalMap {
		pv.Set(perf.Metric{
			Name:      baseName + "." + core + ".Total",
			Unit:      "count",
			Direction: perf.SmallerIsBetter,
		}, float64(total))
		grandTotal += total
	}

	pv.Set(perf.Metric{
		Name:      baseName + ".Total",
		Unit:      "count",
		Direction: perf.SmallerIsBetter,
	}, float64(grandTotal))

}
