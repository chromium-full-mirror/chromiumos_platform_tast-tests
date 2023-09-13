// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package perfutil

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/metrics"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// DefaultRuns provides the default number of iteration for a perftest conducts.
const DefaultRuns = 10

// ScenarioFunc is the function to conduct the test operation and returns the
// metric value.
type ScenarioFunc func(context.Context, string) ([]*metrics.Histogram, error)

// RunAndWaitAll is a utility function to create ScenarioFunc which conducts
// f with metrics.RunAndWaitAll.
func RunAndWaitAll(tconn *chrome.TestConn, f func(ctx context.Context) error, names ...string) ScenarioFunc {
	return func(ctx context.Context, name string) ([]*metrics.Histogram, error) {
		return metrics.RunAndWaitAll(ctx, tconn, time.Minute, f, names...)
	}
}

// RunAndWaitAny is a utility function to create ScenarioFunc which conducts
// f with metrics.RunAndWaitAny.
func RunAndWaitAny(tconn *chrome.TestConn, f func(ctx context.Context) error, names ...string) ScenarioFunc {
	return func(ctx context.Context, name string) ([]*metrics.Histogram, error) {
		return metrics.RunAndWaitAny(ctx, tconn, time.Minute, f, names...)
	}
}

// StoreFunc is a function to be used for RunMultiple.
type StoreFunc func(ctx context.Context, pv *Values, hists []*metrics.Histogram) error

// StoreAllWithHeuristics is a utility function to store all metrics. It
// determines the direction of perf (bigger is better or smaller is better)
// and unit through heuristics from the name of metrics.
func StoreAllWithHeuristics(suffix string) StoreFunc {
	return func(ctx context.Context, pv *Values, hists []*metrics.Histogram) error {
		for _, hist := range hists {
			mean, err := hist.Mean()
			if err != nil {
				return errors.Wrapf(err, "failed to get mean for histogram %s", hist.Name)
			}
			name := hist.Name
			if suffix != "" {
				name = name + "." + suffix
			}
			testing.ContextLog(ctx, name, " = ", mean)
			direction, unit := estimateMetricPresenattionType(ctx, name)
			pv.Append(perf.Metric{
				Name:      name,
				Unit:      unit,
				Direction: direction,
			}, mean)
		}
		return nil
	}
}

// StoreAll is a function to store all histograms into values.
func StoreAll(direction perf.Direction, unit, suffix string) StoreFunc {
	return func(ctx context.Context, pv *Values, hists []*metrics.Histogram) error {
		for _, hist := range hists {
			mean, err := hist.Mean()
			if err != nil {
				return errors.Wrapf(err, "failed to get mean for histogram %s", hist.Name)
			}
			name := hist.Name
			if suffix != "" {
				name = name + "." + suffix
			}
			testing.ContextLog(ctx, name, " = ", mean)

			pv.Append(perf.Metric{
				Name:      name,
				Unit:      unit,
				Direction: direction,
			}, mean)
		}
		return nil
	}
}

// StoreSmoothness is a utility function to store animation smoothness metrics.
func StoreSmoothness(ctx context.Context, pv *Values, hists []*metrics.Histogram) error {
	return StoreAll(perf.BiggerIsBetter, "percent", "")(ctx, pv, hists)
}

// StoreLatency is a utility function to store input-latency metrics.
func StoreLatency(ctx context.Context, pv *Values, hists []*metrics.Histogram) error {
	return StoreAll(perf.SmallerIsBetter, "ms", "")(ctx, pv, hists)
}

// RunnerOptions is a structure to help name NewRunner() parameters.
type RunnerOptions struct {
	// In many tests the first run will actually initialize resources and
	// thus will be very slow. If `IgnoreFirstRun` flag is true the metrics
	// of the first run will be ignored.
	IgnoreFirstRun bool

	// We may want to drop minimum and maximum metric values to reduce
	// flakiness. If `DropMinMaxValues` flag is true, the min and max
	// metric values are dropped before reporting.
	DropMinMaxValues bool
}

// Runner is an entity to manage multiple runs of the test scenario.
// Runner will run test in a loop until `minSuccessfulRuns` number of successful
// test runs reached. If this number was not reached until `maxRuns` runner
// will be raised.
type Runner struct {
	br                *browser.Browser
	pv                *Values
	maxRuns           int
	minSuccessfulRuns int
	RunTracing        bool

	options RunnerOptions
}

// NewRunner creates a new instance of Runner.
func NewRunner(br *browser.Browser, options RunnerOptions) *Runner {
	return &Runner{
		br:                br,
		pv:                NewValues(options.DropMinMaxValues),
		maxRuns:           DefaultRuns,
		minSuccessfulRuns: DefaultRuns,
		RunTracing:        (br != nil),
		options:           options,
	}
}

// RunnerCyclesOptions is a structure to help name SetRunsNumber() parameters.
type RunnerCyclesOptions struct {
	// This sets the maximum number of test runs for the RunMultiple().
	MaxRuns int

	// This sets the maximum number of required successful runs for the
	// RunMultiple(). If MinSuccessfulRuns < MaxRuns the RunMultiple will
	// stop as soon as MinSuccessfulRuns is reached.
	MinSuccessfulRuns int
}

// SetRunsNumber allows to adjust the number of test runs.
func (r *Runner) SetRunsNumber(options RunnerCyclesOptions) {
	if (options.MinSuccessfulRuns < 1) || (options.MaxRuns < options.MinSuccessfulRuns) {
		panic("Incorrect RunnerCyclesOptions")
	}
	r.maxRuns = options.MaxRuns
	r.minSuccessfulRuns = options.MinSuccessfulRuns
}

// Values returns the values in the runner.
func (r *Runner) Values() *Values {
	return r.pv
}

// RunMultiple runs a test scenario multiple times and stores the results using
// the store function. It runs the `scenario` and `store` functions until the
// minimum number of successful runs (`r.minSuccessfulRuns`) is reached, up to
// a maximum of `r.maxRuns`. After that, it runs the scenario again with
// tracing enabled. If any of the runs fail, their errors are added to the
// resulting errors array. If `r.minSuccessfulRuns` cannot be met (i.e. the
// remaining number of allowed runs is not enough to reach the required number
// of successful runs), it quits immediately and reports an error. The test
// function is executed within the scenario function and must be implemented by
// the caller. The name parameter is used as the prefix for subtest names when
// calling the scenario/store function and as the prefix for the trace data
// file. The name can be empty, in which case the runner uses default prefix
// values.
func (r *Runner) RunMultiple(ctx context.Context, name string, scenario ScenarioFunc, store StoreFunc) ([]string, error) {
	runPrefix := name
	if name == "" {
		runPrefix = "run"
	}
	successCount := 0
	var runErrors []string

	appendRunError := func(index int, e error) error {
		if e == nil {
			return nil
		}
		runErrors = append(runErrors, fmt.Sprintf("\nRun %d of %d failed with: %+v", index, r.maxRuns, e))
		if r.maxRuns-(index+1) < r.minSuccessfulRuns-successCount {
			return errors.Wrapf(e,
				"failed to get the required number of "+
					"succesful runs (%d) in test run %d of %d; got %d "+
					"successful runs, with %d errors: %v",
				r.minSuccessfulRuns, index, r.maxRuns, successCount,
				len(runErrors), runErrors)
		}
		return nil
	}

	for i := 0; i < r.maxRuns; i++ {
		hists, runError := scenario(ctx, fmt.Sprintf("%s-%d", runPrefix, i))
		if err := appendRunError(i, runError); err != nil {
			return runErrors, err
		}
		storage := r.pv
		if r.options.IgnoreFirstRun && i == 0 {
			// store() may have side-effects and must always be called.
			storage = NewValues(false)
		}
		storeError := store(ctx, storage, hists)
		if err := appendRunError(i, storeError); err != nil {
			return runErrors, errors.Wrap(err, "failed to store the histogram data")
		}
		if runError == nil && storeError == nil {
			successCount++
		}
		if successCount >= r.minSuccessfulRuns {
			break
		}
	}
	if !r.RunTracing {
		return runErrors, nil
	}

	const traceCleanupDuration = 2 * time.Second
	if deadline, ok := ctx.Deadline(); ok && deadline.Sub(time.Now()) < traceCleanupDuration {
		testing.ContextLog(ctx, "There are no time to conduct a tracing run. Skipping")
		return runErrors, nil
	}

	defer r.br.StopTracing(ctx)
	sctx, cancel := ctxutil.Shorten(ctx, traceCleanupDuration)
	defer cancel()
	// At this time, systrace causes kernel crash on dedede devices. Because of
	// that and data points from systrace isn't actually helpful to most of
	// UI tests, disable systraces for the time being.
	// TODO(https://crbug.com/1162385, b/177636800): enable it.
	if err := r.br.StartTracing(sctx, []string{"benchmark", "cc", "gpu", "input", "toplevel", "ui", "views", "viz"}, browser.DisableSystrace()); err != nil {
		return runErrors, errors.Wrap(err, "failed to start tracing")
	}

	_, err := scenario(sctx, fmt.Sprintf("%s-tracing", runPrefix))
	if err != nil {
		return runErrors, errors.Wrap(err, "failed to run the test scenario")
	}

	tr, err := r.br.StopTracing(ctx)
	if err != nil {
		return runErrors, errors.Wrap(err, "failed to stop tracing")
	}
	if tr == nil || len(tr.Packet) == 0 {
		return runErrors, errors.Wrap(err, "no trace data is collected")
	}
	filename := "trace.data.gz"
	if name != "" {
		filename = fmt.Sprintf("%s-%s", name, filename)
	}

	outdir, ok := testing.ContextOutDir(ctx)
	if !ok {
		return runErrors, errors.Wrap(err, "failed to get name of the output directory")
	}

	if err := chrome.SaveTraceToFile(ctx, tr, filepath.Join(outdir, filename)); err != nil {
		return runErrors, errors.Wrap(err, "failed to save trace to file")
	}

	return runErrors, nil
}

// RunMultipleAndSave is a utility to create a new runner, conduct runs multiple times,
// and save the recorded values.
func RunMultipleAndSave(ctx context.Context, outDir string, br *browser.Browser, scenario ScenarioFunc, store StoreFunc, options RunnerOptions) error {
	r := NewRunner(br, options)
	if _, err := r.RunMultiple(ctx, "", scenario, store); err != nil {
		return err
	}
	if err := r.Values().Save(ctx, outDir); err != nil {
		return errors.Wrap(err, "failed to save results")
	}
	return nil
}
