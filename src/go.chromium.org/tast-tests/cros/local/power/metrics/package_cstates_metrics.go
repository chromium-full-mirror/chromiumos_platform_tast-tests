// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package metrics

import (
	"context"
	"runtime"

	"go.chromium.org/tast-tests/cros/common/perf"
	cp "go.chromium.org/tast-tests/cros/common/power"
	"go.chromium.org/tast-tests/cros/local/cpu"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	c0C1Key             = "C0_C1"
	aggregateNonC0C1Key = "non-C0_C1"
)

// PackageCStatesMetrics records the package C-states of the DUT. This metric
// is only supported on Intel devices.
type PackageCStatesMetrics struct {
	pCStates       map[string]int64
	perPackageCPUs []int
	prefix         string
	lastStats      map[string]uint64
	metrics        map[string]perf.Metric
	intervalName   string
}

// Assert that PackageCStatesMetrics can be used in perf.Timeline.
var _ perf.TimelineDatasource = &PackageCStatesMetrics{}

// NewPackageCStatesMetrics creates a timeline metric to collect package
// C-state numbers.
func NewPackageCStatesMetrics() *PackageCStatesMetrics {
	return &PackageCStatesMetrics{nil, nil, "", nil, make(map[string]perf.Metric), ""}
}

// Setup determines what C-states are supported and which CPUs should be queried.
func (cs *PackageCStatesMetrics) Setup(ctx context.Context, prefix, intervalName string) error {
	// ARM is not supported
	if arch := runtime.GOARCH; arch == "arm" || arch == "arm64" {
		return nil
	}

	pCStates, err := cpu.FetchPackageStates()
	if err != nil {
		return errors.Wrap(err, "error finding package C-states")
	}
	if pCStates == nil {
		testing.ContextLog(ctx, "Failed to find package C-states")
		return nil
	}
	perPackageCPUs, err := cpu.FindCPUPerPackage(ctx)
	if err != nil {
		return errors.Wrap(err, "error finding per package cpus")
	}
	cs.pCStates = pCStates
	cs.perPackageCPUs = perPackageCPUs
	cs.prefix = prefix
	cs.intervalName = intervalName
	return nil
}

// Start collects initial package cstate numbers which we can use to
// compute the residency between now and the first Snapshot.
func (cs *PackageCStatesMetrics) Start(ctx context.Context) error {
	if cs.pCStates == nil {
		return nil // Not supported.
	}
	stats, err := cpu.ReadPackageCStates(cs.perPackageCPUs, cs.pCStates)
	if err != nil {
		return errors.Wrap(err, "failed to collect initial metrics")
	}
	for name := range stats {
		cs.metrics[name] = perf.Metric{
			Name:      cs.prefix + cp.PackageCstatesMetricType + "package-" + name,
			Unit:      cp.PackageCstatesMetricTypeUnit,
			Direction: perf.SmallerIsBetter,
			Multiple:  true,
			Interval:  cs.intervalName}
	}
	cs.lastStats = stats
	return nil
}

// Snapshot computes the package cstate residency between this and
// the previous snapshot, and reports them as metrics.
func (cs *PackageCStatesMetrics) Snapshot(ctx context.Context, values *perf.Values) error {
	if cs.pCStates == nil {
		return nil // Not supported
	}

	stats, err := cpu.ReadPackageCStates(cs.perPackageCPUs, cs.pCStates)
	if err != nil {
		return errors.Wrap(err, "failed to collect metrics")
	}

	diffs := make(map[string]uint64)
	for name, stat := range stats {
		diffs[name] = stat - cs.lastStats[name]
	}

	total := diffs[c0C1Key] + diffs[aggregateNonC0C1Key]
	for name, diff := range diffs {
		values.Append(cs.metrics[name], (float64(diff)/float64(total))*100)
	}
	cs.lastStats = stats
	return nil
}

// Stop does nothing.
func (cs *PackageCStatesMetrics) Stop(ctx context.Context, values *perf.Values) error {
	return nil
}
