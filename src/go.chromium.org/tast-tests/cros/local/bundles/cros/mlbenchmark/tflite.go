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
		Data:         []string{"ml-test-assets.tar.xz"},
		Timeout:      30 * time.Minute,
		Fixture:      fixture.MLBenchmark,
		Params: []testing.Param{
			{
				Name: "mobilenet_v2_1_0_224",
				Val: mlbenchmark.TFLiteBenchmarkParams{
					DataFilename:  "ml-test-assets.tar.xz",
					GraphFilename: "mobilenet_v2_1.0_224.tflite",
				},
			},
			{
				Name:              "mobilenet_v2_1_0_224_gpu_opencl",
				ExtraSoftwareDeps: []string{"vulkan"},
				Val: mlbenchmark.TFLiteBenchmarkParams{
					DataFilename:  "ml-test-assets.tar.xz",
					GraphFilename: "mobilenet_v2_1.0_224.tflite",
					Backend:       mlbenchmark.KGpuOpenCl,
				},
			},
			{
				Name: "mobilenet_v2_1_0_224_quant",
				Val: mlbenchmark.TFLiteBenchmarkParams{
					DataFilename:  "ml-test-assets.tar.xz",
					GraphFilename: "mobilenet_v2_1.0_224_quant.tflite",
				},
			},
			{
				Name:              "mobilenet_v2_1_0_224_quant_gpu_opencl",
				ExtraSoftwareDeps: []string{"vulkan"},
				Val: mlbenchmark.TFLiteBenchmarkParams{
					DataFilename:  "ml-test-assets.tar.xz",
					GraphFilename: "mobilenet_v2_1.0_224_quant.tflite",
					Backend:       mlbenchmark.KGpuOpenCl,
				},
			},
			{
				Name: "convolution_benchmark_1",
				Val: mlbenchmark.TFLiteBenchmarkParams{
					DataFilename:  "ml-test-assets.tar.xz",
					GraphFilename: "convolution_benchmark_1.tflite",
				},
			},
			{
				Name:              "convolution_benchmark_1_gpu_opencl",
				ExtraSoftwareDeps: []string{"vulkan"},
				Val: mlbenchmark.TFLiteBenchmarkParams{
					DataFilename:  "ml-test-assets.tar.xz",
					GraphFilename: "convolution_benchmark_1.tflite",
					Backend:       mlbenchmark.KGpuOpenCl,
				},
			},
			{
				Name: "segmentation_256x256",
				Val: mlbenchmark.TFLiteBenchmarkParams{
					DataFilename:  "ml-test-assets.tar.xz",
					GraphFilename: "selfie_segmentation_landscape_256x256.f16.tflite",
				},
			},
			{
				Name:              "segmentation_256x256_gpu_opencl",
				ExtraSoftwareDeps: []string{"vulkan"},
				Val: mlbenchmark.TFLiteBenchmarkParams{
					DataFilename:  "ml-test-assets.tar.xz",
					GraphFilename: "selfie_segmentation_landscape_256x256.f16.tflite",
					Backend:       mlbenchmark.KGpuOpenCl,
				},
			},
			{
				Name: "lstm_benchmark_1",
				Val: mlbenchmark.TFLiteBenchmarkParams{
					DataFilename:  "ml-test-assets.tar.xz",
					GraphFilename: "lstm.tflite",
				},
			},
			{
				Name: "seanet_stft",
				Val: mlbenchmark.TFLiteBenchmarkParams{
					DataFilename:  "ml-test-assets.tar.xz",
					GraphFilename: "seanet_stft.tflite",
				},
			},
			{
				Name: "seanet_wave",
				Val: mlbenchmark.TFLiteBenchmarkParams{
					DataFilename:  "ml-test-assets.tar.xz",
					GraphFilename: "seanet_wave.tflite",
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
