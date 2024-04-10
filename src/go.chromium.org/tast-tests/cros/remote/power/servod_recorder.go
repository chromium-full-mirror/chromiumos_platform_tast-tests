// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	cp "go.chromium.org/tast-tests/cros/common/power"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast/core/errors"
)

// ServodRecorder is a utility to measure servod metrics during tests.
type ServodRecorder struct {
	interval    time.Duration
	isRecording bool
	// Note: t currently only holds ServodMetric.
	t          *perf.Timeline
	perfValues *perf.Values
}

// NewServodRecorder creates and returns a new ServodRecorder.
// Note: If also using the power recorder, it is recommended to use the same/similar interval.
func NewServodRecorder(ctx context.Context, interval time.Duration, svo *servo.Servo, cpd, useAccumulators bool, filters ...*regexp.Regexp) (*ServodRecorder, error) {
	sm, err := cp.NewServodMetrics(ctx, svo, cpd, useAccumulators, filters...)
	if err != nil {
		return nil, errors.Wrap(err, "setting up servod metrics")
	}
	t, err := perf.NewTimeline(ctx,
		[]perf.TimelineDatasource{sm},
		perf.Prefix(cp.ServodMetricType),
		perf.Interval(interval))
	if err != nil {
		return nil, errors.Wrap(err, "building metrics timeline")
	}

	return &ServodRecorder{
		interval:    interval,
		isRecording: false,
		t:           t,
		perfValues:  nil,
	}, nil
}

// Start collecting servod metrics.
func (r *ServodRecorder) Start(ctx context.Context) error {
	if r.isRecording {
		return errors.New("ServodRecorder has already started recording")
	}
	if err := r.t.Start(ctx); err != nil {
		return errors.Wrap(err, "starting servod timeline")
	}
	if err := r.t.StartRecording(ctx); err != nil {
		return errors.Wrap(err, "starting servod recorder")
	}
	r.isRecording = true
	return nil
}

// Stop recording servod metrics and return unprocessed data.
// This function does not upload metrics to power dashboard.
func (r *ServodRecorder) Stop(ctx context.Context) (*perf.Values, error) {
	if !r.isRecording {
		return nil, errors.New("ServodRecorder is not recording")
	}
	r.isRecording = false
	p, err := r.t.StopRecording(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "recording metrics or stopping ServodRecorder")
	}
	r.perfValues = p
	return p, nil
}

// ProcessResults trims servod values to the timeline of the results from a given subtest.
// It returns only the trimmed servod perf.Values.
func (r *ServodRecorder) ProcessResults(ctx context.Context, outDir, subtest string) (*perf.Values, error) {
	mS, mE, err := cp.FindMeasureStartAndEnd(ctx, outDir, subtest)
	if err != nil {
		return nil, errors.Wrap(err, "finding start and end of subtest")
	}
	trimmedVals, err := cp.TrimSubtestResults(mS, mE, r.perfValues)
	if err != nil {
		return nil, errors.Wrap(err, "trimming servod results")
	}
	return trimmedVals, nil
}
