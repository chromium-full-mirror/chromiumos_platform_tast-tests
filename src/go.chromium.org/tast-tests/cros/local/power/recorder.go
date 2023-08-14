// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/cpu"

	"go.chromium.org/tast-tests/cros/local/screenshot"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// RecorderTempCooldownTimeout is the max amount of time needed for device
	// temperature to cooldown.
	RecorderTempCooldownTimeout = 5 * time.Minute
	// RecorderIdleStateCooldownTimeout is the max amount of time needed for cpu
	// idle state activity to drop.
	RecorderIdleStateCooldownTimeout = 2 * time.Minute
	// RecorderPkgStateCooldownTimeout is the max amount of time needed for cpu
	// pkg state activity to drop.
	RecorderPkgStateCooldownTimeout = 3 * time.Minute

	// RecorderCooldownTimeout is the max amount of time that Recorder allows
	// for all combined cooldown items.
	RecorderCooldownTimeout = RecorderTempCooldownTimeout + RecorderIdleStateCooldownTimeout +
		RecorderPkgStateCooldownTimeout

	// RecorderTimeout is the max amount of time that Recorder is expected to
	// take. It includes recorder & metrics construction, recorder cooldown,
	// recorder destruction and data post-processing.
	RecorderTimeout = RecorderCooldownTimeout + 3*time.Minute
)

// Recorder is a utility to measure power metrics during tests.
type Recorder struct {
	// Fields passed in from NewRecorder().
	interval     time.Duration // The metrics collect interval.
	outDir       string
	testName     string
	optionalArgs []OptionalRecorderArg

	// Fields used internally by the recorder.
	dataSources []perf.TimelineDatasource
	metrics     *perf.Timeline
	isRecording bool
}

// OptionalRecorderArg is used for denoting optional args for recorder.
// For example, pdash_note.
type OptionalRecorderArg struct {
	argName  string
	argValue interface{}
}

// AddOptionalRecorderArg adds optional args for recorder, for example pdash_note.
func (r *Recorder) AddOptionalRecorderArg(key string, val interface{}) {
	r.optionalArgs = append(r.optionalArgs, OptionalRecorderArg{key, val})
}

// Cooldown device before running test load.
// In:
// ctx: context for the test.
// Out:
// error: propagate back to the test.
func (r *Recorder) Cooldown(ctx context.Context) error {
	return cpu.Cooldown(ctx)
}

// Start collecting power metrics.
// In:
// ctx: context for the test.
// Out:
// error: propagate back to the test.
func (r *Recorder) Start(ctx context.Context) error {
	if r.isRecording {
		return errors.New("recorder has already started recording")
	}

	metrics, err := perf.NewTimeline(ctx, r.dataSources, perf.Interval(r.interval))
	if err != nil {
		return errors.Wrap(err, "failed to build metrics timeline")
	}

	if err := metrics.Start(ctx); err != nil {
		return errors.Wrap(err, "failed to start metrics")
	}

	if err := metrics.StartRecording(ctx); err != nil {
		return errors.Wrap(err, "failed to start recording")
	}
	r.isRecording = true
	r.metrics = metrics

	return nil
}

// Finish collecting power metrics and post-processing data.
// In:
// ctx: context for the test.
// vs: additional custom perf values to publish.
// Out:
// error: propagate back to the test.
func (r *Recorder) Finish(ctx context.Context, vs ...*perf.Values) error {
	if !r.isRecording {
		return errors.New("recorder is not recording")
	}
	r.isRecording = false
	p, err := r.metrics.StopRecording(ctx)
	if err != nil {
		return errors.Wrap(err, "error while recording power metrics")
	}

	if len(strings.TrimSpace(pdashNoteVar.Value())) != 0 {
		r.AddOptionalRecorderArg("pdash_note", strings.TrimSpace(pdashNoteVar.Value()))
	}

	p.Merge(vs...)

	if err := GeneratePowerLogAndSaveToCrosbolt(ctx, r.outDir, r.testName, p, r.optionalArgs...); err != nil {
		return errors.Wrap(err, "failed to generate power_log.json and/or save perf data for crosbolt")
	}

	return nil
}

// Record does the setup, execution, and result collection for the power test
// logic as defined in the given function f.
// It calls Cooldown() and Start() before the test, and does Finish() after the
// test.
// In:
// ctx: context for the test.
// f: the function containing main test logic.
// Out:
// error: propagate back to the test.
func (r *Recorder) Record(ctx context.Context, f func(context.Context) error) error {
	if err := r.Cooldown(ctx); err != nil {
		return errors.Wrap(err, "failed to cool down")
	}
	if err := r.Start(ctx); err != nil {
		return errors.Wrap(err, "failed to start the recorder")
	}
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// Execute the main test logic.
	err := f(ctx)

	// Record the performance result even if the main test func returns error.
	finishErr := r.Finish(cleanupCtx)
	if finishErr != nil {
		if err == nil {
			// Wrap finishErr as the return error.
			err = errors.Wrap(finishErr, "failed to finish recording")
		} else {
			// Just log the Finish error.
			testing.ContextLog(ctx, "Failed to finish recording: ", finishErr)
		}
	}
	return err
}

// Close cleans up the recorder resources.
// In:
// ctx: context for the test.
// Out:
// error: propagate back to the test.
func (r *Recorder) Close(ctx context.Context) error {
	if !r.isRecording {
		return nil
	}
	r.isRecording = false
	if _, err := r.metrics.StopRecording(ctx); err != nil {
		return errors.Wrap(err, "failed to stop metrics recording")
	}
	return nil
}

// RegisterMetrics registers test specific metrics to the recorder.
// In:
// metrics: timeline data sources to be registered.
// Out:
// None.
func (r *Recorder) RegisterMetrics(metrics ...perf.TimelineDatasource) {
	r.dataSources = append(r.dataSources, metrics...)
}

// NewRecorder creates and returns a new Recorder.
// In:
// ctx: context for the test.
// interval: time interval between two data points.
// outDir: directory to print test results.
// testName: name of the test.
// args: optional recorder args for recorder.
// Out:
// Recorder: collect power metrics in the test.
func NewRecorder(ctx context.Context, interval time.Duration, outDir, testName string, args ...OptionalRecorderArg) *Recorder {
	return &Recorder{
		interval:     interval,
		outDir:       outDir,
		testName:     testName,
		optionalArgs: args,

		dataSources: TestMetrics(),
		isRecording: false,
	}
}

// SaveScreenshot takes a screenshot and saves to test output.
func SaveScreenshot(ctx context.Context, cr *chrome.Chrome) error {
	dir, ok := testing.ContextOutDir(ctx)
	if !ok || dir == "" {
		return errors.New("failed to get name of output directory")
	}
	path := filepath.Join(dir, "screenshot_power_test.png")
	if err := screenshot.CaptureChrome(ctx, cr, path); err != nil {
		return errors.Wrap(err, "couldn't take a screenshot")
	}
	return nil
}
