// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tflite

import (
	"context"
	"path/filepath"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/local/cpu"
	"go.chromium.org/tast-tests/cros/local/mlbenchmark"
	"go.chromium.org/tast-tests/cros/local/tflite"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const testModel = "conv2dx4.tflite"

func init() {
	testing.AddTest(&testing.Test{
		Func: NeuronMultiCore,
		// TODO(b/381326127): Add multi-MVPU test.
		Desc:         "Runs the test to use multi NPU core in MediaTek Neuron Delegate",
		Contacts:     []string{"cros-odml-foundations-eng@google.com", "ototot@chromium.org"},
		BugComponent: "b:1445284", // ChromeOS > Platform > Technologies > Machine Learning > On-Device ML
		Attr:         []string{"group:mainline", "informational"},
		HardwareDeps: hwdep.D(hwdep.Model("rauru")), // Only rauru has multiple MDLA for now.
		SoftwareDeps: []string{"ml_service", "tflite_mtk_neuron"},
		Data:         []string{testModel},
	})
}

func NeuronMultiCore(ctx context.Context, s *testing.State) {
	// Stop the UI job. While this isn't required to run the test binary, it's
	// possible a previous tests left tabs open or an animation is playing,
	// making the performance number unstable.
	if err := upstart.StopJob(ctx, "ui"); err != nil {
		s.Fatal("Failed to stop ui: ", err)
	}
	defer upstart.EnsureJobRunning(ctx, "ui")

	// Reserve time for restarting the ui job at the end of the test.
	const cleanupTime = 10 * time.Second
	ctx, cancel := ctxutil.Shorten(ctx, cleanupTime)
	defer cancel()

	settingsPath := filepath.Join(s.OutDir(), "settings.json")
	neuronBaseSettings := tflite.StableDelegateSettings{
		StableDelegateLoaderSettings: tflite.MtkNeuronDelegateLoaderSettings,
		MtkNeuronSettings: &tflite.MtkNeuronSettings{
			OperationCheckMode:        tflite.PreOperationCheck,
			AllowFp16PrecisionForFp32: true,
		},
	}
	benchmarkArgs := map[string]string{
		"--stable_delegate_settings_file": settingsPath,
		"--graph":                         s.DataPath(testModel),
	}

	var inferenceLatencies []float64
	for numMdla := 1; numMdla <= 4; numMdla++ {
		s.Log("Testing --num-mdla ", numMdla)

		var neuronSettings = neuronBaseSettings
		neuronSettings.MtkNeuronSettings.CompileOptions = []string{"--num-mdla", strconv.Itoa(numMdla)}
		neuronSettings.WriteTo(settingsPath)

		// Wait until the CPU idle so we can get more stable numbers.
		if err := cpu.WaitUntilIdle(ctx); err != nil {
			s.Fatal("Failed to wait until CPU idle: ", err)
		}

		results, err := mlbenchmark.ExecuteBenchmark(ctx, benchmarkArgs)
		if err != nil {
			s.Fatal("Benchmark failed: ", err)
		}
		inferenceLatencies = append(inferenceLatencies, results.AvgLatency)
	}

	for i, length := 1, len(inferenceLatencies); i < length; i++ {
		if inferenceLatencies[i] >= inferenceLatencies[i-1] {
			s.Fatal("Inference latencies should be decreasing")
		}
	}
}
