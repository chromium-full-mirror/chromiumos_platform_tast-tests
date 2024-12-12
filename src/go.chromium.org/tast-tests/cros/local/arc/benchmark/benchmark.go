// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package benchmark contains utilities to gather performance benchmarks from
// ARC.
package benchmark

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/memory/metrics"
	"go.chromium.org/tast/core/errors"
)

// BenchmarkingSession stores data for capturing results at the end of the session.
type BenchmarkingSession struct {
	sfm     *SurfaceFlingerMetrics
	basemem *metrics.BaseMemoryStats
	tconn   *chrome.TestConn
	arc     *arc.ARC
	outDir  string
}

// appTracingResults stores results for appTracing calls.
type appTracingResults struct {
	FPS              float64 `json:"fps"`
	PerceivedFPS     float64 `json:"perceivedFps"`
	CommitDeviation  float64 `json:"commitDeviation"`
	PresentDeviation float64 `json:"presentDeviation"`
	RenderQuality    float64 `json:"renderQuality"`
	JanksPerMinute   float64 `json:"janksPerMinute"`
	JanksPercentage  float64 `json:"janksPercentage"`
}

// Results stores results for the calls to benchmarking.
type Results struct {
	// FPS is a metric that shows average FPS during the sampled period.
	FPS float64
	// PerceivedFPS is similar to FPS but does not include app commits which were not presented.
	PerceivedFPS float64
	// CommitDeviation is a metric that shows deviation from the ideal time of committing frames
	// during the sampled period, based on FPS.
	CommitDeviation float64
	// PresentDeviation is similar to CommitDeviation, but based on PerceivedFPS rather than FPS.
	PresentDeviation float64
	// RenderQuality is a metric in range 0%..100% that shows quality of the render during the
	// sampled period. 100% is ideal quality when frames are produced on time according to FPS.
	RenderQuality float64
	// JanksPerMinute is a metric that shows the number of janks per minute during the sampled period.
	JanksPerMinute float64
	// JanksPercentage is a metric that shows the percentage of present frames during the tracing
	// period which contain UI janks.
	JanksPercentage float64
	// SurfaceFlingerFPS is a metric that shows average FPS during the sampled
	// period, calculated via SurfaceFlinger.
	SurfaceFlingerFPS float64
	// SurfaceFlingerLatency is a metric that shows average latency during the
	// sampled period, calculated via SurfaceFlinger, by taking the average elapsed
	// time between all submission and draw timestamps.
	SurfaceFlingerLatency float64
	/// memoryPerfValues contains memory metrics capture at the start and end of a benchmarking session.
	memoryPerfValues *perf.Values
}

// StartBenchmarking begins the benchmarking process.
func StartBenchmarking(ctx context.Context, appPackageName string, tconn *chrome.TestConn, arc *arc.ARC, outDir string) (*BenchmarkingSession, error) {
	basemem, err := metrics.NewBaseMemoryStats(ctx, arc)
	if err != nil {
		return nil, errors.Wrap(err, "failed to retrieve base memory stats")
	}

	// Leave the mini-game running for while recording metrics.
	sfm := NewSurfaceFlingerMetrics(appPackageName, arc)
	if err := sfm.Start(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to start SurfaceFlinger")
	}
	if err := tconn.Call(ctx, nil, `tast.promisify(chrome.autotestPrivate.arcAppTracingStart)`); err != nil {
		return nil, errors.Wrap(err, "failed to start arcAppTracing benchmarking")
	}

	return &BenchmarkingSession{sfm, basemem, tconn, arc, outDir}, nil
}

// Stop stops the benchmarking process and returns the parsed results.
func (session *BenchmarkingSession) Stop(ctx context.Context) (results Results, err error) {
	var r Results
	if session.sfm != nil {
		r.SurfaceFlingerFPS, r.SurfaceFlingerLatency, err = session.sfm.Stop(ctx)
		if err != nil {
			return r, errors.Wrap(err, "failed to stop SurfaceFlinger")
		}
	}
	var a appTracingResults
	if err := session.tconn.Call(ctx, &a, `tast.promisify(chrome.autotestPrivate.arcAppTracingStopAndAnalyze)`); err != nil {
		return r, errors.Wrap(err, "failed to stop arcAppTracing benchmarking")
	}

	memPerfValues := perf.NewValues()
	if err := metrics.LogMemoryStats(ctx, session.basemem, session.arc, memPerfValues, session.outDir, "_final"); err != nil {
		return r, errors.Wrap(err, "failed to collect memory metrics")
	}

	r.FPS = a.FPS
	r.PerceivedFPS = a.PerceivedFPS
	r.CommitDeviation = a.CommitDeviation
	r.PresentDeviation = a.PresentDeviation
	r.RenderQuality = a.RenderQuality
	r.JanksPerMinute = a.JanksPerMinute
	r.JanksPercentage = a.JanksPercentage

	// Explicitly check for zero FPS (http://b/330397308).
	if r.FPS == 0 {
		return r, errors.New("invalid zero FPS value returned")
	}

	r.memoryPerfValues = memPerfValues
	return r, nil
}

// LogMemoryStats logs the memory stats with a suffix.
func (session *BenchmarkingSession) LogMemoryStats(ctx context.Context, p *perf.Values, suffix string) (err error) {
	if err := metrics.LogMemoryStats(ctx, session.basemem, session.arc, p, session.outDir, suffix); err != nil {
		return errors.Wrap(err, "failed to collect memory metrics")
	}
	return nil
}

// SaveRuntimePerfResults sets and saves the runtime performance metric results.
func SaveRuntimePerfResults(p *perf.Values, r *Results, outDir string) error {
	p.Set(FpsPerfMetric(), r.FPS)
	p.Set(PerceivedFpsPerfMetric(), r.PerceivedFPS)
	p.Set(CommitDeviationPerfMetric(), r.CommitDeviation)
	p.Set(PresentDeviationPerfMetric(), r.PresentDeviation)
	p.Set(RenderQualityPerfMetric(), r.RenderQuality*100.0)
	p.Set(JanksPerMinutePerfMetric(), r.JanksPerMinute)
	p.Set(JanksPercentagePerfMetric(), r.JanksPercentage)
	p.Set(SurfaceFlingerFpsPerfMetric(), r.SurfaceFlingerFPS)
	p.Set(SurfaceFlingerLatencyPerfMetric(), r.SurfaceFlingerLatency)
	p.Merge(r.memoryPerfValues)
	return p.Save(outDir)
}

// SaveRuntimePerfResultsWithPrefix sets and saves the runtime performance metric results with the given prefix.
func SaveRuntimePerfResultsWithPrefix(p *perf.Values, r *Results, prefix, outDir string) error {
	setMetricWithPrefix := func(metric perf.Metric, value float64) {
		metric.Name = prefix + metric.Name
		p.Set(metric, value)
	}
	setMetricWithPrefix(FpsPerfMetric(), r.FPS)
	setMetricWithPrefix(PerceivedFpsPerfMetric(), r.PerceivedFPS)
	setMetricWithPrefix(CommitDeviationPerfMetric(), r.CommitDeviation)
	setMetricWithPrefix(PresentDeviationPerfMetric(), r.PresentDeviation)
	setMetricWithPrefix(RenderQualityPerfMetric(), r.RenderQuality*100.0)
	setMetricWithPrefix(JanksPerMinutePerfMetric(), r.JanksPerMinute)
	setMetricWithPrefix(JanksPercentagePerfMetric(), r.JanksPercentage)
	setMetricWithPrefix(SurfaceFlingerFpsPerfMetric(), r.SurfaceFlingerFPS)
	setMetricWithPrefix(SurfaceFlingerLatencyPerfMetric(), r.SurfaceFlingerLatency)
	p.MergeWithPrefix(prefix, r.memoryPerfValues)
	return p.Save(outDir)
}

// LaunchTimePerfMetric returns a standard metric that launch time can be saved in.
func LaunchTimePerfMetric() perf.Metric {
	return perf.Metric{
		Name:      "launchTime",
		Unit:      "seconds",
		Direction: perf.SmallerIsBetter,
	}
}

// LoginTimePerfMetric returns a standard metric that login time can be saved in.
func LoginTimePerfMetric() perf.Metric {
	return perf.Metric{
		Name:      "loginTime",
		Unit:      "seconds",
		Direction: perf.SmallerIsBetter,
	}
}

// TestTimePerfMetric returns a standard metric that test time can be saved in.
func TestTimePerfMetric() perf.Metric {
	return perf.Metric{
		Name:      "testTime",
		Unit:      "seconds",
		Direction: perf.SmallerIsBetter,
	}
}

// FpsPerfMetric returns a standard metric that measured FPS can be saved in.
func FpsPerfMetric() perf.Metric {
	return perf.Metric{
		Name:      "fps",
		Unit:      "fps",
		Direction: perf.BiggerIsBetter,
	}
}

// PerceivedFpsPerfMetric returns a standard metric that measured FPS can be saved in.
func PerceivedFpsPerfMetric() perf.Metric {
	return perf.Metric{
		Name:      "perceivedFps",
		Unit:      "fps",
		Direction: perf.BiggerIsBetter,
	}
}

// CommitDeviationPerfMetric returns a standard metric that commit deviation can be saved in.
func CommitDeviationPerfMetric() perf.Metric {
	return perf.Metric{
		Name:      "commitDeviation",
		Unit:      "ms",
		Direction: perf.SmallerIsBetter,
	}
}

// PresentDeviationPerfMetric returns a standard metric that present deviation can be saved in.
func PresentDeviationPerfMetric() perf.Metric {
	return perf.Metric{
		Name:      "presentDeviation",
		Unit:      "ms",
		Direction: perf.SmallerIsBetter,
	}
}

// RenderQualityPerfMetric returns a standard metric that render quality can be saved in.
func RenderQualityPerfMetric() perf.Metric {
	return perf.Metric{
		Name:      "renderQuality",
		Unit:      "percents",
		Direction: perf.BiggerIsBetter,
	}
}

// JanksPerMinutePerfMetric returns a standard metric that measured janks per minute can be saved in.
func JanksPerMinutePerfMetric() perf.Metric {
	return perf.Metric{
		Name:      "janksPerMinute",
		Unit:      "count",
		Direction: perf.SmallerIsBetter,
	}
}

// JanksPercentagePerfMetric returns a standard metric that measured janks percentage can be saved in.
func JanksPercentagePerfMetric() perf.Metric {
	return perf.Metric{
		Name:      "janksPercentage",
		Unit:      "percents",
		Direction: perf.SmallerIsBetter,
	}
}

// SurfaceFlingerFpsPerfMetric returns a standard metric that FPS measured through SurfaceFlinger can be saved in.
func SurfaceFlingerFpsPerfMetric() perf.Metric {
	return perf.Metric{
		Name:      "surfaceFlingerFps",
		Unit:      "fps",
		Direction: perf.BiggerIsBetter,
	}
}

// SurfaceFlingerLatencyPerfMetric returns a standard metric that latency measured through SurfaceFlinger can be saved in.
func SurfaceFlingerLatencyPerfMetric() perf.Metric {
	return perf.Metric{
		Name:      "surfaceFlingerLatency",
		Unit:      "seconds",
		Direction: perf.SmallerIsBetter,
	}
}
