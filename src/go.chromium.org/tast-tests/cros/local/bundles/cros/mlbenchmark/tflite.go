// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package mlbenchmark contains benchmarks related to ML performance on ChromeOS.
package mlbenchmark

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/mlbenchmark"
	"go.chromium.org/tast-tests/cros/local/mlbenchmark/fixture"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: TFLite,
		Desc: "Benchmarks to measure raw TFLite processing performance",
		Contacts: []string{
			"chromeos-platform-ml@google.com",
			"jmpollock@google.com",
		},
		BugComponent: "b:1140118",
		Attr:         []string{"group:ml_benchmark", "ml_benchmark_nightly"},
		Timeout:      30 * time.Minute,
		Fixture:      fixture.MLBenchmark,
		Params: []testing.Param{
			{
				Name:      "mobilenet_v2_1_0_224",
				ExtraData: []string{"ml-test-assets.tar.xz"},
				Val: mlbenchmark.TFLiteBenchmarkParams{
					DataFilename:  "ml-test-assets.tar.xz",
					GraphFilename: "mobilenet_v2_1.0_224.tflite",
				},
			},
			{
				Name:      "mobilenet_v2_1_0_224_gpu_opencl",
				ExtraData: []string{"ml-test-assets.tar.xz"},
				Val: mlbenchmark.TFLiteBenchmarkParams{
					DataFilename:  "ml-test-assets.tar.xz",
					GraphFilename: "mobilenet_v2_1.0_224.tflite",
					Backend:       mlbenchmark.KGpuOpenCl,
				},
			},
			{
				Name:      "mobilenet_v2_1_0_224_quant",
				ExtraData: []string{"ml-test-assets.tar.xz"},
				Val: mlbenchmark.TFLiteBenchmarkParams{
					DataFilename:  "ml-test-assets.tar.xz",
					GraphFilename: "mobilenet_v2_1.0_224_quant.tflite",
				},
			},
		},
	})
}

func TFLite(ctx context.Context, s *testing.State) {
	params, ok := s.Param().(mlbenchmark.TFLiteBenchmarkParams)
	if !ok {
		s.Fatal("Failed to convert test params to TFLiteBenchmarkParams")
	}

	if err := mlbenchmark.RunTFLiteBenchmark(ctx, s.TestName(), s.DataPath(params.DataFilename), params.GraphFilename, params.Backend); err != nil {
		s.Fatal(err, "error running benchmark")
	}
}
