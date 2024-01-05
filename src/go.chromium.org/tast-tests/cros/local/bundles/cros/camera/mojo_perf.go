// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/benchmark"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         MojoPerf,
		Desc:         "Measure Mojo Performance",
		Contacts:     []string{"chromeos-camera-eng@google.com", "ototot@chromium.org"},
		BugComponent: "b:167281", // ChromeOS > Platform > Technologies > Camera
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		Timeout:      5 * time.Minute,
	})
}

func MojoPerf(ctx context.Context, s *testing.State) {
	const exec = "mojo_perf_test"
	result, err := benchmark.New(exec).Run(ctx)

	if err != nil {
		s.Fatal("Failed to run Mojo performance test: ", err)
	}

	perfValues := perf.NewValues()
	for _, r := range result.Benchmarks {
		// Using RealTime in google benchmark
		// automatically adds a "/real_time" suffix to the benchmark name.
		// Besides, '/' is invalid in `perf.Metric.Name`.
		// Therefore, to make the name valid and readable,
		// we convert raw benchmark names like: "BM_MojoCallWithBuffer/256/real_time"
		// to a more readable perf metric name: "MojoCallWithBuffer_256".
		name := strings.TrimPrefix(r.Name, "BM_")
		name = strings.TrimSuffix(name, "/real_time")
		name = strings.ReplaceAll(name, "/", "_")
		perfValues.Set(perf.Metric{
			Name:      name,
			Variant:   "CpuTime",
			Unit:      r.TimeUnit,
			Direction: perf.SmallerIsBetter,
		}, r.CPUTime)
		perfValues.Set(perf.Metric{
			Name:      name,
			Variant:   "RealTime",
			Unit:      r.TimeUnit,
			Direction: perf.SmallerIsBetter,
		}, r.RealTime)
	}

	if err := perfValues.Save(s.OutDir()); err != nil {
		s.Fatal("Failed to save perf metrics: ", err)
	}
}
