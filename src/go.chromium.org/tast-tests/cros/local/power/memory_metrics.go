// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/power/util"
	"go.chromium.org/tast/core/errors"
)

// MemoryMetrics holds the memory metrics read from procfs.
type MemoryMetrics struct {
	memBuffers    perf.Metric
	memCached     perf.Metric
	memUsed       perf.Metric
	memPageTables perf.Metric
	swapUsed      perf.Metric
	memSize       int64
	swapSize      int64
}

// Assert that MemoryMetrics can be used in perf.Timeline.
var _ perf.TimelineDatasource = &MemoryMetrics{}

// NewMemoryMetrics creates a struct to capture memory metrics with procfs.
func NewMemoryMetrics() *MemoryMetrics {
	return &MemoryMetrics{}
}

// Setup creates the memory metrics.
func (m *MemoryMetrics) Setup(ctx context.Context, prefix, intervalName string) error {
	totalSizes := []string{util.MemInfoMemTotal, util.MemInfoSwapTotal}
	metrics := util.GetMemInfoMetrics(ctx, totalSizes)
	if len(metrics) != len(totalSizes) {
		return errors.New("Unable to determine total RAM and swap sizes")
	}
	m.memSize = metrics[util.MemInfoMemTotal]
	m.swapSize = metrics[util.MemInfoSwapTotal]
	// Temporary storage buffers for block devices.
	m.memBuffers = perf.Metric{
		Name:      prefix + memoryMetricType + "buffers",
		Unit:      memoryMetricTypeUnit,
		Direction: perf.SmallerIsBetter,
		Multiple:  true,
		Interval:  intervalName}
	// In-memory caches for disk reads including Slab-reclaimable memory.
	m.memCached = perf.Metric{
		Name:      prefix + memoryMetricType + "cached",
		Unit:      memoryMetricTypeUnit,
		Direction: perf.SmallerIsBetter,
		Multiple:  true,
		Interval:  intervalName}
	// Total - (Free + Buffers + Cached + SReclaimable)
	m.memUsed = perf.Metric{
		Name:      prefix + memoryMetricType + "used",
		Unit:      memoryMetricTypeUnit,
		Direction: perf.SmallerIsBetter,
		Multiple:  true,
		Interval:  intervalName}
	// Memory used for the lowest-level page tables.
	m.memPageTables = perf.Metric{
		Name:      prefix + memoryMetricType + "page_tables",
		Unit:      memoryMetricTypeUnit,
		Direction: perf.SmallerIsBetter,
		Multiple:  true,
		Interval:  intervalName}
	// Amount of used swap memory.
	m.swapUsed = perf.Metric{
		Name:      prefix + memoryMetricType + "swap",
		Unit:      memoryMetricTypeUnit,
		Direction: perf.SmallerIsBetter,
		Multiple:  true,
		Interval:  intervalName}
	return nil
}

// Start takes the first snapshot of memory metrics.
func (m *MemoryMetrics) Start(ctx context.Context) error {
	// Not required.
	return nil
}

var requiredMetrics = []string{
	// Data for RAM metrics.
	util.MemInfoMemFree, util.MemInfoBuffers, util.MemInfoCached,
	util.MemInfoSReclaimable, util.MemInfoPageTables,
	// Data for swap metrics.
	util.MemInfoSwapFree}

// Snapshot takes a snapshot of memory metrics.
func (m *MemoryMetrics) Snapshot(ctx context.Context, values *perf.Values) error {
	metrics := util.GetMemInfoMetrics(ctx, requiredMetrics)
	if metrics == nil {
		return nil
	}
	// In case gathering any of the metrics failed, map will return a zero-value
	// for int64, which makes calculations simpler.
	free := metrics[util.MemInfoMemFree]
	buffers := metrics[util.MemInfoBuffers]
	cached := metrics[util.MemInfoCached] + metrics[util.MemInfoSReclaimable]
	pageTables := metrics[util.MemInfoPageTables]
	swapFree := metrics[util.MemInfoSwapFree]
	swapUsed := m.swapSize - swapFree

	values.Append(m.memBuffers, float64(buffers))
	values.Append(m.memCached, float64(cached))
	values.Append(m.memUsed, float64(m.memSize-free-cached-buffers))
	values.Append(m.memPageTables, float64(pageTables))
	values.Append(m.swapUsed, float64(swapUsed))

	return nil
}

// Stop does nothing.
func (m *MemoryMetrics) Stop(ctx context.Context, values *perf.Values) error {
	return nil
}
