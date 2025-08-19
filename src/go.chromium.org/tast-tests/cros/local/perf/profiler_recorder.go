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

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// ProfilerRecorder records data collected via profilers.
type ProfilerRecorder struct {
	// Prefix for the metric names.
	prefix string

	// Collection intervals.
	interval time.Duration

	// Output dir.
	outDir string

	// Values at intervals from profiler.
	stat profiler.PerfStatValuesAtIntervalsOutput

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
		profiler.Perf(profiler.PerfStatCyclesAndInstsAtIntervalsOpts(&t.stat, t.interval.Milliseconds())),
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
	baseName := t.prefix + "Profiler"
	intervalName := baseName + ".t"

	timeMetric := perf.Metric{
		Name:     intervalName,
		Unit:     "s",
		Multiple: true,
	}

	c := cases.Title(language.English, cases.NoLower)

	var metricMap = make(map[string]perf.Metric)
	var totalMap = make(map[string]int64)
	for _, data := range t.stat.ValuesAtIntervals {
		pv.Append(timeMetric, data.Timestamp.Seconds())

		for _, valueWithCPU := range data.ValuesWithCPU {
			core := valueWithCPU.CoreType
			event := c.String(valueWithCPU.Event)
			metric := event + "." + core
			if _, ok := metricMap[metric]; !ok {
				metricMap[metric] = perf.Metric{
					Name:      baseName + "." + metric,
					Unit:      "count",
					Multiple:  true,
					Direction: perf.SmallerIsBetter,
					Interval:  intervalName,
				}
			}

			pv.Append(metricMap[metric], float64(valueWithCPU.Value))

			if total, ok := totalMap[metric]; ok {
				totalMap[metric] = total + valueWithCPU.Value
			} else {
				totalMap[metric] = valueWithCPU.Value
			}

			if total, ok := totalMap[event]; ok {
				totalMap[event] = total + valueWithCPU.Value
			} else {
				totalMap[event] = valueWithCPU.Value
			}
		}
	}

	for metric, total := range totalMap {
		pv.Set(perf.Metric{
			Name:      baseName + "." + metric + ".Total",
			Unit:      "count",
			Direction: perf.SmallerIsBetter,
		}, float64(total))
	}
}
