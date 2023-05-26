// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package perf

import (
	"context"
	"fmt"

	"chromiumos/tast/local/arc"
	"chromiumos/tast/local/memory"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// psiDataSource is a perf.TimelineDatasource reporting the PSI
// stats at a given point.
type psiDataSource struct {
	arc          *arc.ARC
	prefix       string
	intervalName string
}

// NewPSIDataSource creates a new instance of psiDataSource.
func NewPSIDataSource(a *arc.ARC) *psiDataSource {
	return &psiDataSource{arc: a}
}

// Setup implements perf.TimelineDatasource.Setup.
func (s *psiDataSource) Setup(ctx context.Context, prefix, intervalName string) error {
	s.prefix = prefix
	s.intervalName = intervalName
	return nil
}

// Start implements perf.TimelineDatasource.Start.
func (s *psiDataSource) Start(ctx context.Context) error {
	return nil
}

// Snapshot implements perf.TimelineDatasource.Snapshot.
func (s *psiDataSource) Snapshot(ctx context.Context, p *perf.Values) error {
	stats, err := memory.NewPSIStats(ctx, s.arc)
	if err != nil {
		return errors.Wrap(err, "failed to get PSI stats")
	}
	if stats == nil {
		testing.ContextLog(ctx, "Found empty PSI stats")
		return nil
	}

	generateMetrics(s.prefix, "", s.intervalName, stats.Host, p)
	if s.arc != nil && stats.Arc != nil {
		generateMetrics(s.prefix, "arc_", s.intervalName, stats.Arc, p)
	}

	return nil
}

// Stop does nothing.
func (s *psiDataSource) Stop(_ context.Context, values *perf.Values) error {
	return nil
}

// generateMetrics creates a metric for each memory pressure stat, in
// the form:
// <prefix><sysname>psi_<tag>_avg<bucket size>
//
// Examples:
// 1. Memory.psi_some_avg10
// 2. psi_full_avg60
// 3. arc_psi_full_avg10
func generateMetrics(prefix, sysname, intervalName string, stats *memory.PSIOneSystemStats, p *perf.Values) {
	for _, stat := range []struct {
		tag    string
		detail memory.PSIDetail
	}{
		{tag: "some", detail: stats.Some},
		{tag: "full", detail: stats.Full},
	} {
		p.Append(
			perf.Metric{
				Name:      fmt.Sprintf("%s%spsi_%s_avg10", prefix, sysname, stat.tag),
				Unit:      "percent",
				Direction: perf.SmallerIsBetter,
				Multiple:  true,
				Interval:  intervalName,
			},
			stat.detail.Avg10,
		)
		p.Append(
			perf.Metric{
				Name:      fmt.Sprintf("%s%spsi_%s_avg60", prefix, sysname, stat.tag),
				Unit:      "percent",
				Direction: perf.SmallerIsBetter,
				Multiple:  true,
				Interval:  intervalName,
			},
			stat.detail.Avg60,
		)
		p.Append(
			perf.Metric{
				Name:      fmt.Sprintf("%s%spsi_%s_avg300", prefix, sysname, stat.tag),
				Unit:      "percent",
				Direction: perf.SmallerIsBetter,
				Multiple:  true,
				Interval:  intervalName,
			},
			stat.detail.Avg300,
		)
	}
}
