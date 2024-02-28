// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package metrics

import (
	"context"
	"fmt"

	"go.chromium.org/tast-tests/cros/common/chrome/histogram"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/errors"
)

// HistogramMetricWriter are functions that log perf metrics from Histograms.
type HistogramMetricWriter func(pv *perf.Values, histogram *histogram.Histogram) error

// HistogramMetrics is a helper to log performance metrics from the diff
// between two samples of a histogram.
type HistogramMetrics struct {
	name    string
	writers []HistogramMetricWriter
	prev    *histogram.Histogram
}

// NewHistogramMetrics creates a heler to log performance metrics from the
// difference in histogram values between this call and HistogramMetrics.Write.
func NewHistogramMetrics(name string, writers ...HistogramMetricWriter) *HistogramMetrics {
	return &HistogramMetrics{name, writers, nil}
}

// Start gets the histogram value used as a baseline for the first Write call.
func (hm *HistogramMetrics) Start(ctx context.Context, tconn *chrome.TestConn) error {
	prev, err := GetHistogram(ctx, tconn, hm.name)
	if err != nil {
		return errors.Wrapf(err, "failed to get histogram %q", hm.name)
	}
	hm.prev = prev
	return nil
}

// Write the metrics calculated from the change in histogram values since the
// call to NewHistogramMetrics.
func (hm *HistogramMetrics) Write(ctx context.Context, tconn *chrome.TestConn, pv *perf.Values) error {
	if hm.prev == nil {
		return errors.New("you need to call Start before Write")
	}
	curr, err := GetHistogram(ctx, tconn, hm.name)
	if err != nil {
		return errors.Wrapf(err, "failed to get histogram %q for logging metrics", hm.name)
	}
	diff, err := curr.Diff(hm.prev)
	if err != nil {
		return errors.Wrapf(err, "failed to diff histogram %q for logging metrics", hm.name)
	}
	if diff.TotalCount() == 0 {
		// Histogram has no data, so don't run any writers.
		return nil
	}
	for _, writer := range hm.writers {
		if err := writer(pv, diff); err != nil {
			return errors.Wrapf(err, "failed to write metric for histrogram %q", hm.name)
		}
	}
	hm.prev = curr
	return nil
}

// StartHistogramMetrics is a helper function that calls Start on a slice of
// HistogramMetrics.
func StartHistogramMetrics(ctx context.Context, tconn *chrome.TestConn, metrics []*HistogramMetrics) error {
	for _, metric := range metrics {
		if err := metric.Start(ctx, tconn); err != nil {
			return err
		}
	}
	return nil
}

// WriteHistogramMetrics is a helper function that calls Write on a slice of
// HistogramMetrics.
func WriteHistogramMetrics(ctx context.Context, tconn *chrome.TestConn, pv *perf.Values, metrics []*HistogramMetrics) error {
	for _, metric := range metrics {
		if err := metric.Write(ctx, tconn, pv); err != nil {
			return err
		}
	}
	return nil
}

// SetMeanHistogramMetricWriter creates a HistogramMetricWriter that calls
// perf.Values.Set with the average value of the Histogram.
//
// The perf.Metric has the following properties:
//
//	Name: the histogram name with the suffix param appended
//	Unit: the unit param
//	Direction: the direction param
func SetMeanHistogramMetricWriter(suffix, unit string, direction perf.Direction) HistogramMetricWriter {
	return func(pv *perf.Values, histogram *histogram.Histogram) error {
		mean, err := histogram.Mean()
		if err != nil {
			return err
		}
		name := fmt.Sprintf("%s%s", histogram.Name, suffix)
		pv.Set(perf.Metric{
			Name:      name,
			Unit:      unit,
			Direction: direction,
			Multiple:  false,
		}, mean)
		return nil
	}
}

// AppendMeanHistogramMetricWriter creates a HistogramMetricWriter that calls
// perf.Values.Append with the average value of the Histogram.
//
// The perf.Metric has the following properties:
//
//	Name: the histogram name with the suffix param appended
//	Unit: the unit param
//	Direction: the direction param
func AppendMeanHistogramMetricWriter(suffix, unit string, direction perf.Direction) HistogramMetricWriter {
	return func(pv *perf.Values, histogram *histogram.Histogram) error {
		mean, err := histogram.Mean()
		if err != nil {
			return err
		}
		name := fmt.Sprintf("%s%s", histogram.Name, suffix)
		pv.Append(perf.Metric{
			Name:      name,
			Unit:      unit,
			Direction: direction,
			Multiple:  true,
		}, mean)
		return nil
	}
}

// SetPercentileHistogramMetricWriter creates a HistogramMetricWriter that calls
// perf.Values.Set with the value of the midpoint of the bucket containing the
// given percentile param.
//
// The perf.Metric has the following properties:
//
//	Name: the histogram name with the suffix param appended
//	Unit: the unit param
//	Direction: the direction param
func SetPercentileHistogramMetricWriter(percentile int, suffix, unit string, direction perf.Direction) HistogramMetricWriter {
	return func(pv *perf.Values, histogram *histogram.Histogram) error {
		value, err := histogram.Percentile(percentile)
		if err != nil {
			return err
		}
		name := fmt.Sprintf("%s%s", histogram.Name, suffix)
		pv.Set(perf.Metric{
			Name:      name,
			Unit:      unit,
			Direction: direction,
			Multiple:  false,
		}, value)
		return nil
	}
}

// AppendPercentileHistogramMetricWriter creates a HistogramMetricWriter that
// calls perf.Values.Append with the value of the midpoint of the bucket
// containing the given percentile param.
//
// The perf.Metric has the following properties:
//
//	Name: the histogram name with the suffix param appended
//	Unit: the unit param
//	Direction: the direction param
func AppendPercentileHistogramMetricWriter(percentile int, suffix, unit string, direction perf.Direction) HistogramMetricWriter {
	return func(pv *perf.Values, histogram *histogram.Histogram) error {
		value, err := histogram.Percentile(percentile)
		if err != nil {
			return err
		}
		name := fmt.Sprintf("%s%s", histogram.Name, suffix)
		pv.Append(perf.Metric{
			Name:      name,
			Unit:      unit,
			Direction: direction,
			Multiple:  true,
		}, value)
		return nil
	}
}
