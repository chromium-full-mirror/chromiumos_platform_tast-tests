// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"time"

	"chromiumos/tast/common/perf"
	"chromiumos/tast/local/cpu"
	"go.chromium.org/tast/core/errors"
)

// Recorder is a utility to measure power metrics during tests.
type Recorder struct {
	metrics  *perf.Timeline
	outDir   string
	testName string
}

// Cooldown device before running test load.
// In:
// ctx: context for the test.
// Out:
// error: propagate back to the test.
func (r *Recorder) Cooldown(ctx context.Context) error {
	// Wait until CPU is cooled down and idle.
	if _, err := cpu.WaitUntilCoolDown(ctx, cpu.IdleCoolDownConfig()); err != nil {
		return errors.Wrap(err, "CPU failed to cool down")
	}
	if err := cpu.WaitUntilIdle(ctx); err != nil {
		return errors.Wrap(err, "CPU failed to idle")
	}

	// TODO(b/271799379): Need to add additional cool down time here.

	return nil
}

// Start collecting power metrics.
// In:
// ctx: context for the test.
// Out:
// error: propagate back to the test.
func (r *Recorder) Start(ctx context.Context) error {
	if err := r.metrics.Start(ctx); err != nil {
		return errors.Wrap(err, "failed to start metrics")
	}

	if err := r.metrics.StartRecording(ctx); err != nil {
		return errors.Wrap(err, "failed to start recording")
	}

	return nil
}

// Finish collecting power metrics and post-processing data.
// In:
// ctx: context for the test.
// Out:
// error: propagate back to the test.
func (r *Recorder) Finish(ctx context.Context) error {
	p, err := r.metrics.StopRecording(ctx)
	if err != nil {
		return errors.Wrap(err, "error while recording power metrics")
	}

	if err := GeneratePowerLogAndSaveToCrosbolt(ctx, r.outDir, r.testName, p); err != nil {
		return errors.Wrap(err, "failed to generate power_log.json and/or save perf data for crosbolt")
	}

	return nil
}

// NewRecorder creates and returns a new Recorder.
// In:
// ctx: context for the test.
// interval: time interval between two data points.
// outDir: directory to print test results.
// testName: name of the test.
// Out:
// Recorder: collect power metrics in the test.
// error: propagate back to the test.
func NewRecorder(ctx context.Context, interval time.Duration, outDir, testName string) (*Recorder, error) {
	metrics, err := perf.NewTimeline(ctx, TestMetrics(), perf.Interval(interval))
	if err != nil {
		return nil, errors.Wrap(err, "failed to build metrics timeline")
	}

	r := &Recorder{
		metrics:  metrics,
		outDir:   outDir,
		testName: testName,
	}

	return r, nil
}
