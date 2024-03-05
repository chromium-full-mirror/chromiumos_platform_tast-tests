// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package graphics contains graphics-related utility functions for local tests.
package graphics

import (
	"sync"

	"go.chromium.org/tast-tests/cros/common/perf"
)

// ThreadSafePerfValues is a thread-safe wrapper around perf.Values. It calls
// perf.Values() functions while holding a lock.
type ThreadSafePerfValues struct {
	p  *perf.Values
	mu sync.Mutex
}

// NewThreadSafePerfValues returns a new ThreadSafeValues.
func NewThreadSafePerfValues() *ThreadSafePerfValues {
	return &ThreadSafePerfValues{p: perf.NewValues(), mu: sync.Mutex{}}
}

// GetUnderlyingValues exposes the underlying perf.Values.
// The returned perf.Values may be read or modified if and only if it can be
// guaranteed that no other threads can concurrently access the perf.Values.
func (tsv *ThreadSafePerfValues) GetUnderlyingValues() *perf.Values {
	return tsv.p
}

// Append is a thread-safe wrapper around perf.Values.Append().
func (tsv *ThreadSafePerfValues) Append(s perf.Metric, vs ...float64) {
	tsv.mu.Lock()
	defer tsv.mu.Unlock()
	tsv.p.Append(s, vs...)
}

// Set is a thread-safe wrapper around perf.Values.Set().
func (tsv *ThreadSafePerfValues) Set(s perf.Metric, vs ...float64) {
	tsv.mu.Lock()
	defer tsv.mu.Unlock()
	tsv.p.Set(s, vs...)
}
