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
	"go.chromium.org/tast-tests/cros/local/power/metrics"
	"go.chromium.org/tast-tests/cros/local/power/util"

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

	// RecorderOverheadTimeout is the max amount of time needed for recorder &
	// metrics construction, recorder destruction and data post-processing.
	RecorderOverheadTimeout = 13 * time.Minute

	// RecorderTimeout is the max amount of time that Recorder is expected to
	// take.
	RecorderTimeout = RecorderCooldownTimeout + RecorderOverheadTimeout

	// OptionalRecorderArgCustomPerfKey is the key used to get optional custom
	// perf values saved only to results-chart.json.
	// Be very careful when you add optional custom perf values with a timeline,
	// it may overwrite the timeline in power recorder metrics.
	OptionalRecorderArgCustomPerfKey = "custom_perf_results_chart"

	// OptionalRecorderArgPowerLogCustomPerfKey is the key used to get optional
	// custom perf values saved to both power logs and results-chart.json.
	// Be very careful when you add custom perf values with a timeline,
	// it may overwrite the timeline in power recorder metrics.
	OptionalRecorderArgPowerLogCustomPerfKey = "custom_perf_power_log"

	// OptionalRecorderArgDischargeWatchdogKey is the arg name to enable
	// discharge watchdog in recorder.
	OptionalRecorderArgDischargeWatchdogKey = "enable_watchdog"
)

// Recorder is a utility to measure power metrics during tests.
type Recorder struct {
	// Fields passed in from NewRecorder().
	interval     time.Duration // The metrics collect interval.
	outDir       string
	testName     string
	optionalArgs []OptionalRecorderArg

	// Fields used internally by the recorder.
	dataSources             []perf.TimelineDatasource
	metrics                 *perf.Timeline
	checkpoints             *perf.Checkpoints
	enableDischargeWatchdog bool
	isRecording             bool
	perfValues              *perf.Values
}

// OptionalRecorderArg is used for denoting optional args for recorder.
// For example, pdash_note.
type OptionalRecorderArg struct {
	argName  string
	argValue interface{}
}

// AddOptionalRecorderArg adds optional args for recorder. Examples:
// 1. r.AddOptionalRecorderArg(OptionalRecorderArgPowerLogCustomPerfKey, yourCustomPerfValues).
// 2. r.AddOptionalRecorderArg("pdash_note", strings.TrimSpace(pdashNoteVar.Value()))
func (r *Recorder) AddOptionalRecorderArg(key string, val interface{}) {
	r.optionalArgs = append(r.optionalArgs, OptionalRecorderArg{key, val})
}

// DischargeWatchdogOption returns a new instance of OptionalRecorderArg that
// controls if recorder enables discharge watchdog.
func DischargeWatchdogOption(discharge bool) OptionalRecorderArg {
	return OptionalRecorderArg{OptionalRecorderArgDischargeWatchdogKey, discharge}
}

// Cooldown device before running test load.
// In:
// ctx: context for the test.
// Out:
// error: propagate back to the test.
func Cooldown(ctx context.Context) error {
	if err := cpu.Cooldown(ctx); err != nil {
		return err
	}

	if err := util.WaitForIOCooldown(ctx); err != nil {
		return err
	}
	return nil
}

// Cooldown device before running test load.
// In:
// ctx: context for the test.
// Out:
// error: propagate back to the test.
// TODO(b/321173687):Since Cooldown does not interact with Recorder, it should
// be able to run without a Recorder. Recommend using Cooldown() without a
// struct. Recorder.Cooldown() should be removed gradually.
func (r *Recorder) Cooldown(ctx context.Context) error {
	if err := cpu.Cooldown(ctx); err != nil {
		return err
	}

	if err := util.WaitForIOCooldown(ctx); err != nil {
		return err
	}
	return nil
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

	if r.enableDischargeWatchdog {
		r.StartDischargeWatchdog(ctx)
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

	r.metrics = metrics
	r.checkpoints = perf.NewCheckpoints()
	r.isRecording = true
	r.perfValues = nil

	return nil
}

// Stop recording power metrics and returns unprocessed data. This function does
// not upload metrics to power dashboard.
// In:
// ctx: context for the test.
// Out:
// *perf.Values: unprocessed power metrics.
// error: propagate back to the test.
func (r *Recorder) Stop(ctx context.Context) (*perf.Values, error) {
	if !r.isRecording {
		return nil, errors.New("recorder is not recording")
	}
	r.isRecording = false
	// Stop watching for battery discharge either way.
	r.enableDischargeWatchdog = false
	p, err := r.metrics.StopRecording(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed while recording metrics or stopping recorder")
	}
	r.perfValues = p
	return p, nil
}

// Finish collecting power metrics and post-processing data.
// In:
// ctx: context for the test.
// vs: additional custom perf values to publish; they will be added to results-chart.json.
// Out:
// error: propagate back to the test.
// TODO: b/332979331
// Best practice to use Finish():
// 1. If you wish to just add all your custom perf values to results-chart.json only,
// simply call Finish(ctx, customPerfValues1, customPerfValues2...customPerfValuesN).
// 2. If you wish to add some of your custom perf values to both, some only to results-chart.json,
// in your tests you could add the following, then call Finish(ctx):
// `r.AddOptionalRecorderArg(power.OptionalRecorderArgPowerLogCustomPerfKey, customPerfValues1)`
// `r.AddOptionalRecorderArg(power.OptionalRecorderArgPowerLogCustomPerfKey, customPerfValues2)`
// `r.AddOptionalRecorderArg(power.OptionalRecorderArgCustomPerfKey, customPerfValues3)`
// In this case, customPerfValues1 and customPerfValues2 will be added to both power logs
// and results-chart.json while customPerfValues3 only to results-chart.json.
// Calling r.AddOptionalRecorderArg(power.OptionalRecorderArgPowerLogCustomPerfKey, customPerfValues)
// and passing customPerfValues to Finish() do the same operation. Only call one of the two.
// Please also make sure your custom perf value metrics follow power metric conventions
// when adding them to power logs.
func (r *Recorder) Finish(ctx context.Context, vs ...*perf.Values) error {
	if r.perfValues == nil {
		if _, err := r.Stop(ctx); err != nil {
			return errors.Wrap(err, "failed to finish recording metrics")
		}
	}

	r.checkpoints.Save(r.outDir)

	if len(strings.TrimSpace(pdashNoteVar.Value())) != 0 {
		r.AddOptionalRecorderArg("pdash_note", strings.TrimSpace(pdashNoteVar.Value()))
	}

	if vs != nil {
		for _, customValue := range vs {
			r.AddOptionalRecorderArg(OptionalRecorderArgCustomPerfKey, customValue)
		}
	}

	if err := GeneratePowerLogAndSaveToCrosbolt(ctx, r.outDir, r.testName, r.perfValues, r.checkpoints, r.optionalArgs...); err != nil {
		return errors.Wrap(err, "failed to generate power_log.json and/or save perf data for crosbolt")
	}

	r.perfValues = nil
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

// StartCheckpoint starts tracking a Checkpoint under a name.
// In:
// name: name of the Checkpoint.
// Out:
// perf.Section: a Checkpoint Section to be ended later.
func (r *Recorder) StartCheckpoint(name string) *perf.Section {
	return r.checkpoints.NewSection(name)
}

// EndCheckpoint ends tracking a Checkpoint Section.
// In:
// perf.Section: an ongoing Checkpoint Section.
// Out:
// None.
func (r *Recorder) EndCheckpoint(section *perf.Section) {
	r.checkpoints.EndSection(section)
}

// StartDischargeWatchdog starts a goroutine that makes sure the device is on
// discharge to keep guard of chargeoverride reset out of AC charger connection
// flakiness.
func (r *Recorder) StartDischargeWatchdog(ctx context.Context) {
	var checkInterval = 10 * time.Second
	go func() {
		for r.enableDischargeWatchdog {
			status, err := GetStatus(ctx)
			if err != nil {
				testing.ContextLog(ctx, "Discharge watchdog failed to read power status: ", err)
				return
			}
			if IsLinePowerConnected(status) {
				testing.ContextLog(ctx, "Discharge watchdog detected power supply on, force discharge again")
				err = util.SimpleForceDischarge(ctx)
				if err != nil {
					testing.ContextLogf(ctx, "Discharge watchdog unable to set discharge: %s", err)
					return
				}
			}
			// GoBigSleepLint: Sleep this thread that aims to do periodic check.
			testing.Sleep(ctx, checkInterval)
		}
		return
	}()
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
	// Enable discharge watchdog if force discharge succeeds.
	discharge := false
	for _, optionalRecorderArg := range args {
		if optionalRecorderArg.argName == OptionalRecorderArgDischargeWatchdogKey {
			discharge = optionalRecorderArg.argValue.(bool)
		}
	}
	return &Recorder{
		interval:     interval,
		outDir:       outDir,
		testName:     testName,
		optionalArgs: args,

		dataSources:             metrics.TestMetrics(),
		enableDischargeWatchdog: discharge,
		isRecording:             false,
	}
}

// UseMetrics will rewrite recorder datasources
// to use only metrics specified by each metric class flag.
func (r *Recorder) UseMetrics(ctx context.Context, classes ...metrics.MetricClass) error {
	if len(classes) == 0 {
		return errors.New("no metric class flags were passed")
	}
	r.dataSources = metrics.TestMetrics(classes...)
	return nil
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
