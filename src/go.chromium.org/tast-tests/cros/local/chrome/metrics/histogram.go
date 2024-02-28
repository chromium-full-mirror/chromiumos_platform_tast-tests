// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package metrics

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"go.chromium.org/tast-tests/cros/common/chrome/histogram"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// histogramTransferFile is the file that ChromeOS services write out Histogram
	// data to. At intervals, Chrome's chromeos::ExternalMetrics comes through and
	// adds events from this file into Chrome's internal list of Histograms. This
	// can be a hidden, asynchronous source of diffs to Histograms that are not
	// purely Chrome-based.
	histogramTransferFile = "/var/lib/metrics/uma-events"
)

// Histogram type alias for backward compatibility in tast-test-private
type Histogram = histogram.Histogram

// GetHistogram returns the current state of a Chrome histogram (e.g. "Tabs.TabCountActiveWindow").
// If no samples have been reported for the histogram since Chrome was started, the zero value for
// Histogram is returned.
func GetHistogram(ctx context.Context, tconn *chrome.TestConn, name string) (*histogram.Histogram, error) {
	h := histogram.Histogram{Name: name}
	if err := tconn.Call(ctx, &h, `tast.promisify(chrome.metricsPrivate.getHistogram)`, name); err != nil {
		if strings.Contains(err.Error(), fmt.Sprintf("Histogram %s not found", name)) {
			return &histogram.Histogram{Name: name}, nil
		}
		return nil, err
	}
	if err := h.Validate(); err != nil {
		return nil, errors.Wrapf(err, "bad histogram %v", h)
	}
	return &h, nil
}

// WaitForHistogram is a convenience function that calls GetHistogram until the requested histogram is available,
// ctx's deadline is reached, or timeout (if positive) has elapsed.
func WaitForHistogram(ctx context.Context, tconn *chrome.TestConn, name string, timeout time.Duration) (*histogram.Histogram, error) {
	var h *histogram.Histogram
	err := testing.Poll(ctx, func(ctx context.Context) error {
		var err error
		h, err = GetHistogram(ctx, tconn, name)
		if err != nil {
			return err
		}
		if len(h.Buckets) == 0 {
			return errors.Errorf("histogram %s not found", name)
		}
		return nil
	}, &testing.PollOptions{Timeout: timeout})
	return h, err
}

// WaitForHistogramUpdate is a convenience function that calls GetHistogram until the requested histogram contains
// at least one sample not present in old, an earlier snapshot of the same histogram.
// A histogram containing the new samples is returned; see Histogram.Diff for details.
func WaitForHistogramUpdate(ctx context.Context, tconn *chrome.TestConn, name string,
	old *histogram.Histogram, timeout time.Duration) (*histogram.Histogram, error) {
	var h *histogram.Histogram
	err := testing.Poll(ctx, func(ctx context.Context) error {
		var err error
		if h, err = GetHistogram(ctx, tconn, name); err != nil {
			return err
		}
		if reflect.DeepEqual(h, old) {
			return errors.New("histogram unchanged")
		}
		return nil
	}, &testing.PollOptions{Timeout: timeout})

	if err != nil {
		return nil, err
	}
	return h.Diff(old)
}

// GetHistograms is a convenience function to get multiple histograms.
func GetHistograms(ctx context.Context, tconn *chrome.TestConn, histogramNames []string) ([]*histogram.Histogram, error) {
	var result []*histogram.Histogram
	for _, name := range histogramNames {
		histogram, err := GetHistogram(ctx, tconn, name)
		if err != nil {
			return nil, err
		}
		result = append(result, histogram)
	}
	return result, nil
}

// Recorder tracks a snapshot to calculate diffs since start or last wait call.
type Recorder struct {
	snapshot []*histogram.Histogram
}

// names returns names of the histograms tracked by the recorder.
func (r *Recorder) names() []string {
	var names []string
	for _, h := range r.snapshot {
		names = append(names, h.Name)
	}
	return names
}

// StartRecorder captures a snapshot to calculate histograms diffs later.
func StartRecorder(ctx context.Context, tconn *chrome.TestConn, names ...string) (*Recorder, error) {
	s, err := GetHistograms(ctx, tconn, names)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get snapshot")
	}

	return &Recorder{snapshot: s}, nil
}

// Histogram returns the histogram diffs since the recorder is started.
func (r *Recorder) Histogram(ctx context.Context, tconn *chrome.TestConn) ([]*histogram.Histogram, error) {
	names := r.names()

	s, err := GetHistograms(ctx, tconn, names)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get snapshot")
	}

	return histogram.DiffHistograms(r.snapshot, s)
}

// waitMode defines ways to wait.
type waitMode int

const (
	waitAny waitMode = iota
	waitAll
)

// wait implements the wait logic for WaitAny and WaitAll.
func (r *Recorder) wait(ctx context.Context, tconn *chrome.TestConn, mode waitMode, timeout time.Duration) ([]*histogram.Histogram, error) {
	names := r.names()
	if len(names) == 0 {
		return nil, errors.New("no histograms to wait")
	}

	var diffs []*histogram.Histogram
	err := testing.Poll(ctx, func(ctx context.Context) error {
		allNames := make(map[string]bool, len(names))
		for _, name := range names {
			allNames[name] = true
		}
		s, err := GetHistograms(ctx, tconn, names)
		if err != nil {
			return err
		}

		diffs, err = histogram.DiffHistograms(r.snapshot, s)
		if err != nil {
			return err
		}

		cnt := 0
		for _, diff := range diffs {
			if len(diff.Buckets) != 0 {
				cnt++
				delete(allNames, diff.Name)
			}
		}

		if mode == waitAll {
			if cnt != len(s) {
				missingNames := make([]string, 0, len(allNames))
				for name := range allNames {
					missingNames = append(missingNames, name)
				}
				return errors.Errorf("not all histogram changed, missing %v", missingNames)
			}
		} else {
			if cnt == 0 {
				return errors.New("histograms unchanged")
			}
		}

		return nil
	}, &testing.PollOptions{Timeout: timeout})

	if err != nil {
		return nil, errors.Wrap(err, "failed to wait")
	}

	return diffs, nil
}

// WaitAny waits for update from any of histograms being recorded and returns
// the diffs since the recorder is started.
func (r *Recorder) WaitAny(ctx context.Context, tconn *chrome.TestConn, timeout time.Duration) ([]*histogram.Histogram, error) {
	return r.wait(ctx, tconn, waitAny, timeout)
}

// WaitAll waits for update from all of histograms being recorded and returns
// the diffs since the recorder is started.
func (r *Recorder) WaitAll(ctx context.Context, tconn *chrome.TestConn, timeout time.Duration) ([]*histogram.Histogram, error) {
	return r.wait(ctx, tconn, waitAll, timeout)
}

// Run is a helper to calculate histogram diffs before and after running a given
// function.
func Run(ctx context.Context, tconn *chrome.TestConn, f func(ctx context.Context) error, names ...string) ([]*histogram.Histogram, error) {
	r, err := StartRecorder(ctx, tconn, names...)
	if err != nil {
		return nil, err
	}

	if err := f(ctx); err != nil {
		return nil, err
	}

	return r.Histogram(ctx, tconn)
}

// runAndWait is a helper to calculate histogram diffs before and after
// running a given function and waits for histograms change based on wait mode.
func runAndWait(ctx context.Context, tconn *chrome.TestConn,
	mode waitMode,
	timeout time.Duration,
	f func(ctx context.Context) error, names ...string) ([]*histogram.Histogram, error) {
	r, err := StartRecorder(ctx, tconn, names...)
	if err != nil {
		return nil, err
	}

	if err := f(ctx); err != nil {
		return nil, err
	}

	// No need to wait if there is no histograms to capture.
	if len(r.names()) == 0 {
		return r.Histogram(ctx, tconn)
	}

	if mode == waitAll {
		return r.WaitAll(ctx, tconn, timeout)
	}

	return r.WaitAny(ctx, tconn, timeout)
}

// RunAndWaitAll is a helper to calculate histogram diffs before and after
// running a given function. It waits for all the histograms before return if
// there are histograms to wait. Otherwise, it returns immediately like Run.
func RunAndWaitAll(ctx context.Context, tconn *chrome.TestConn,
	timeout time.Duration,
	f func(ctx context.Context) error, names ...string) ([]*histogram.Histogram, error) {
	return runAndWait(ctx, tconn, waitAll, timeout, f, names...)
}

// RunAndWaitAny is a helper to calculate histogram diffs before and after
// running a given function. It waits for any one of the histograms before
// return if there are histograms to wait. Otherwise, it returns immediately
// like Run.
func RunAndWaitAny(ctx context.Context, tconn *chrome.TestConn,
	timeout time.Duration,
	f func(ctx context.Context) error, names ...string) ([]*histogram.Histogram, error) {
	return runAndWait(ctx, tconn, waitAny, timeout, f, names...)
}

// ClearHistogramTransferFile clears the histogramTransferFile. The
// histogramTransferFile is how Histograms get from ChromeOS services into Chrome --
// ChromeOS services write their updates to the file, and periodicially,
// Chrome's chromeos::ExternalMetrics services scrapes the file and adds the
// Histograms to its own internal list which this package queries.
//
// While this system works well in production, it can be a source of unexpected
// asynchronous changes in Histograms during tests. Tests intending to watch a
// Histogram which originates in ChromeOS should clear this file before
// establishing a "before" Histogram. This will avoid having events from previous
// tests show up as unexpected diffs in the current test.
func ClearHistogramTransferFile() error {
	return clearHistogramTransferFileByName(histogramTransferFile)
}

// clearHistogramTransferFileByName is the heart of ClearHistogramTransferFile.
// It is broken up into a separate function for ease of testing.
func clearHistogramTransferFileByName(fileName string) error {
	file, err := os.OpenFile(fileName, os.O_RDWR, 0666)
	if os.IsNotExist(err) {
		// File doesn't exist, so it's already truncated.
		return nil
	}
	if err != nil {
		return errors.Wrapf(err, "unable to open %s", fileName)
	}
	defer file.Close()

	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX); err != nil {
		return errors.Wrapf(err, "unable to lock %s", fileName)
	}
	defer unix.Flock(int(file.Fd()), unix.LOCK_UN)

	if err := unix.Ftruncate(int(file.Fd()), 0); err != nil {
		return errors.Wrapf(err, "unable to truncate %s", fileName)
	}

	return nil
}

// SaveHistogramsMeanValue saves the mean of each histogram as the performance metrics value. Note that this function does not save data in file.
func SaveHistogramsMeanValue(ctx context.Context, pv *perf.Values, histograms []*histogram.Histogram, metric perf.Metric) error {
	for _, h := range histograms {
		mean, err := h.Mean()
		if err != nil {
			return errors.Wrapf(err, "failed to get mean for histogram %s", h.Name)
		}

		metric.Name = fmt.Sprintf("%s", h.Name)
		pv.Set(metric, mean)
	}

	return nil
}
