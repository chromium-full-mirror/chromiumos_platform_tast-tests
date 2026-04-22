// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package metrics

import (
	"context"
	"strings"

	"go.chromium.org/tast-tests/cros/common/chrome/histogram"
	"go.chromium.org/tast-tests/cros/common/perf"
	cp "go.chromium.org/tast-tests/cros/common/power"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/metrics"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// HistogramMetrics records the histogram stat.
// The snapshot interval of the perf.Timeline used for HistogramMetrics must
// be larger than the sample generation interval of all the histograms. In
// other words, histogram sample generation must be more frequent than data
// collection. Otherwise the Snapshot function will return errors due to lack
// of samples.
type HistogramMetrics struct {
	tconn     *chrome.TestConn
	names     []string // Histogram names.
	metrics   map[string]perf.Metric
	lastHists []*histogram.Histogram
	// label is used to identify the HistogramMetrics instance in log messages,
	// when it is embedded in other structs.
	label string
}

// Assert that HistogramMetrics can be used in perf.Timeline.
var _ perf.TimelineDatasource = &HistogramMetrics{}

type histogramMetadata struct {
	unit      string
	direction perf.Direction
}

// OverlayStrategyMetric is the metric name of the video overlay strategy.
const OverlayStrategyMetric = "Viz.DisplayCompositor.OverlayStrategy"

// A map of supported histogram name to {unit, direction}
var validHistogramsMap = map[string]histogramMetadata{
	"EventLatency.KeyPressed.TotalLatency":   {cp.HistogramLatencyMetricTypeUnit, perf.SmallerIsBetter},
	"EventLatency.MousePressed.TotalLatency": {cp.HistogramLatencyMetricTypeUnit, perf.SmallerIsBetter},
	OverlayStrategyMetric:                    {cp.HistogramOverlayStrategyMetricTypeUnit, perf.SmallerIsBetter},
}

// OverlayStrategyVariants maps the enum values of Viz.DisplayCompositor.OverlayStrategy
// to their string representations.
var OverlayStrategyVariants = map[int64]string{
	0: "Unknown",
	1: "NoStrategyUsed",
	2: "Fullscreen",
	3: "SingleOnTop",
	4: "Underlay",
	5: "UnderlayCast",
	6: "NoStrategyAllFail",
	7: "NoStrategyFailMin",
}

// NewHistogramMetrics creates the struct to store Chrome histogram metrics.
func NewHistogramMetrics(tconn *chrome.TestConn, names []string) *HistogramMetrics {
	return newHistogramMetricsWithLabel(tconn, names, "histogram metrics")
}

// newHistogramMetricsWithLabel creates the struct to store Chrome histogram
// metrics, and name the instance with the given label.
func newHistogramMetricsWithLabel(tconn *chrome.TestConn, names []string, label string) *HistogramMetrics {
	return &HistogramMetrics{
		tconn:   tconn,
		names:   names,
		metrics: make(map[string]perf.Metric),
		label:   label,
	}
}

// Setup creates metrics.
func (v *HistogramMetrics) Setup(ctx context.Context, prefix, intervalName string) error {
	metrics.ClearHistogramTransferFile()
	for _, name := range v.names {
		metric := perf.Metric{
			Multiple: true,
			Interval: intervalName,
		}
		if err := setUpMetric(prefix, name, &metric); err != nil {
			return err
		}
		// Example of the timeline based metrics in results-chart.json:
		// "histogram.EventLatencyKeyPressedTotalLatency": {
		//   "summary": {
		//     "units": "us",
		//     "improvement_direction": "down",
		//     "type": "list_of_scalar_values",
		//     "interval": "t",
		//     "values": [
		//       49558.72222222222,
		//       48571.11111111111,
		//       50601.5,
		//       53118.681818181816,
		//       49765.13636363636,
		//       54704.63157894737
		//     ]
		//   }
		// }
		v.metrics[name] = metric
	}
	return nil
}

// Start logs the start of histogram stats tracker.
func (v *HistogramMetrics) Start(ctx context.Context) error {
	testing.ContextLogf(ctx, "Start tracking %s stats", v.label)
	histograms, err := metrics.GetHistograms(ctx, v.tconn, v.names)
	if err != nil {
		return errors.Wrapf(err, "failed to get histograms %v", v.names)
	}
	v.lastHists = histograms
	return nil
}

// Snapshot takes one snapshot of histogram stats.
func (v *HistogramMetrics) Snapshot(ctx context.Context, values *perf.Values) error {
	return v.snapshot(ctx, values)
}

// Stop logs the stop of histogram stats tracker.
// This function is required by perf.Timeline. It does not need to make another snapshot.
func (v *HistogramMetrics) Stop(ctx context.Context, values *perf.Values) error {
	testing.ContextLogf(ctx, "Stop tracking %s stats", v.label)
	return nil
}

func (v *HistogramMetrics) snapshot(ctx context.Context, values *perf.Values) error {
	newHists, err := metrics.GetHistograms(ctx, v.tconn, v.names)
	if err != nil {
		return errors.Wrapf(err, "failed to get histograms %v", v.names)
	}
	diffs, err := histogram.DiffHistograms(v.lastHists, newHists)
	if err != nil {
		return errors.Wrapf(err, "failed to diff histograms, old had length %d; new had length %d", len(v.lastHists), len(newHists))
	}
	for _, h := range diffs {
		mean, err := h.Mean()
		if err != nil {
			return errors.Wrapf(err, "failed to get mean for histogram %s", h.Name)
		}
		m, ok := v.metrics[h.Name]
		if !ok {
			return errors.Errorf("failed to get metric with name %s", h.Name)
		}
		if m.Multiple {
			values.Append(m, mean)
		} else {
			values.Set(m, mean)
		}
	}
	v.lastHists = newHists
	return nil
}

// HistogramAverageMetrics is a timeline data source implementation used to
// record the average value of all samples of each metric. It reuses some
// HistogramMetrics implemenation by embedding it.
// Unlike HistogramMetrics, the perf.Timeline for HistogramAverageMetrics can
// have a shorter snapshot interval than the histogram sample generation
// interval without causing Snapshot function to fail. In other words,
// histogram sample generation can be infrequent.
type HistogramAverageMetrics struct {
	*HistogramMetrics
}

// Assert that HistogramAverageMetrics can be used in perf.Timeline.
var _ perf.TimelineDatasource = &HistogramAverageMetrics{}

// NewHistogramAverageMetrics creates the struct to store average values of
// Chrome histogram metrics.
func NewHistogramAverageMetrics(tconn *chrome.TestConn, names []string) *HistogramAverageMetrics {
	return &HistogramAverageMetrics{
		HistogramMetrics: newHistogramMetricsWithLabel(tconn, names, "average histogram metrics"),
	}
}

// Setup creates metrics.
func (v *HistogramAverageMetrics) Setup(ctx context.Context, prefix, intervalName string) error {
	metrics.ClearHistogramTransferFile()
	for _, name := range v.names {
		metric := perf.Metric{
			Variant:  "average",
			Multiple: false,
		}
		if err := setUpMetric(prefix, name, &metric); err != nil {
			return err
		}
		// Example of the average metrics in results-chart.json:
		// "histogram.EventLatencyMousePressedTotalLatency": {
		//   "average": {
		//     "units": "us",
		//     "improvement_direction": "down",
		//     "type": "scalar",
		//     "value": 71725.53846153847
		//   }
		// }
		v.metrics[name] = metric
	}
	return nil
}

// Snapshot does nothing because there's no need to do intermediate snapshots
// to get the average values.
func (v *HistogramAverageMetrics) Snapshot(ctx context.Context, values *perf.Values) error {
	return nil
}

// Stop logs the stop of histogram stats tracker and makes another snapshot
// in order to get the average values.
func (v *HistogramAverageMetrics) Stop(ctx context.Context, values *perf.Values) error {
	if err := v.HistogramMetrics.Stop(ctx, values); err != nil {
		return err
	}
	return v.snapshot(ctx, values)
}

// HistogramEnumMetric is a timeline data source implementation used to record
// histogram stats for histograms with enum values. It reuses some
// HistogramMetrics implementation by embedding it.
// This struct only supports one histogram per instance.
type HistogramEnumMetric struct {
	*HistogramMetrics
	// variants maps the enum value to the variants.
	variants map[int64]string
	// isTimelineMetric determines how the metric will be collected.
	// When true, the metric will be collected every interval.
	// When false, the metric will be collected once at the end of the measurement.
	isTimelineMetric bool
}

// Assert that HistogramEnumMetric can be used in perf.Timeline.
var _ perf.TimelineDatasource = &HistogramEnumMetric{}

// NewHistogramEnumMetric creates a timeline data source that records a Chrome
// histogram with enum values.
func NewHistogramEnumMetric(tconn *chrome.TestConn, name string, variants map[int64]string, isTimelineMetric bool) *HistogramEnumMetric {
	return newHistogramEnumMetricWithLabel(tconn, name, variants, "histogram metrics", isTimelineMetric)
}

// newHistogramEnumMetricWithLabel creates the struct to store Chrome histogram
// metrics with enum values, and name the instance with the given label.
func newHistogramEnumMetricWithLabel(tconn *chrome.TestConn, name string, variants map[int64]string, label string, isTimelineMetric bool) *HistogramEnumMetric {
	return &HistogramEnumMetric{
		HistogramMetrics: newHistogramMetricsWithLabel(tconn, []string{name}, label),
		variants:         variants,
		isTimelineMetric: isTimelineMetric,
	}
}

// Setup initializes metrics for each enum variant.
func (m *HistogramEnumMetric) Setup(ctx context.Context, prefix, intervalName string) error {
	if err := metrics.ClearHistogramTransferFile(); err != nil {
		testing.ContextLog(ctx, "Failed to clear histogram transfer file: ", err)
	}
	for _, variant := range m.variants {
		metric := perf.Metric{
			Multiple: m.isTimelineMetric,
		}
		if err := setUpMetricWithVariant(prefix, m.names[0], variant, &metric); err != nil {
			return err
		}
		m.metrics[variant] = metric
	}
	return nil
}

// Start logs the start of histogram stats tracker.
func (m *HistogramEnumMetric) Start(ctx context.Context) error {
	return m.HistogramMetrics.Start(ctx)
}

// Snapshot takes one snapshot of histogram stats for each enum variant if it
// is a timeline metric.
func (m *HistogramEnumMetric) Snapshot(ctx context.Context, values *perf.Values) error {
	if m.isTimelineMetric {
		return m.snapshot(ctx, values)
	}
	return nil
}

// Stop logs the stop of histogram stats tracker and makes a snapshot
// if it is not a timeline metric.
func (m *HistogramEnumMetric) Stop(ctx context.Context, values *perf.Values) error {
	testing.ContextLogf(ctx, "Stop tracking %s stats", m.label)
	// No need to make an extra snapshot for timeline metrics.
	if m.isTimelineMetric {
		return nil
	}
	return m.snapshot(ctx, values)
}

func (m *HistogramEnumMetric) snapshot(ctx context.Context, values *perf.Values) error {
	const expectedHistogramNum = 1
	newHists, err := metrics.GetHistograms(ctx, m.tconn, m.names)
	if err != nil {
		return errors.Wrapf(err, "failed to get histograms %v", m.names)
	}
	diffs, err := histogram.DiffHistograms(m.lastHists, newHists)
	if err != nil {
		return errors.Wrap(err, "failed to diff histogram")
	} else if len(diffs) != expectedHistogramNum {
		return errors.Errorf("unexpected histogram diff length, got %d, want %d", len(diffs), expectedHistogramNum)
	}

	for _, bucket := range diffs[0].Buckets {
		enum := bucket.Min
		variant, ok := m.variants[enum]
		if !ok {
			return errors.Errorf("failed to get variant name with enum %d", enum)
		}
		metric, ok := m.metrics[variant]
		if !ok {
			return errors.Errorf("failed to get metric with variant %s", variant)
		}
		if metric.Multiple {
			values.Append(metric, float64(bucket.Count))
		} else {
			values.Set(metric, float64(bucket.Count))
		}
	}
	m.lastHists = newHists
	return nil
}

// setUpMetric does histogram validation and sets up the metric name, unit and
// direction.
func setUpMetric(prefix, name string, metric *perf.Metric) error {
	return setUpMetricWithVariant(prefix, name, "", metric)
}

func setUpMetricWithVariant(prefix, name, variant string, metric *perf.Metric) error {
	if _, ok := validHistogramsMap[name]; !ok {
		return errors.Errorf("unexpected histogram name %s", name)
	}
	// Remove "." from the name because power_dashboard would confuse that with metric type.
	nameInChart := strings.ReplaceAll(name, ".", "")

	metric.Name = prefix + cp.HistogramMetricType + nameInChart + variant
	metric.Unit = validHistogramsMap[name].unit
	metric.Direction = validHistogramsMap[name].direction
	return nil
}
