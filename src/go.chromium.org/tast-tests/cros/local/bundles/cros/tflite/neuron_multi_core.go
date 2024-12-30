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

func init() {
	testing.AddTest(&testing.Test{
		Func:         NeuronMultiCore,
		Desc:         "Runs the test to use multiple NPU cores in MediaTek Neuron Delegate",
		Contacts:     []string{"cros-odml-foundations-eng@google.com", "ototot@chromium.org"},
		BugComponent: "b:1445284", // ChromeOS > Platform > Technologies > Machine Learning > On-Device ML
		Attr:         []string{"group:mainline", "informational"},
		// Only rauru family has multiple NPU cores for now.
		HardwareDeps: hwdep.D(hwdep.Model("rauru", "navi", "hylia")),
		SoftwareDeps: []string{"ml_service", "tflite_mtk_neuron"},
		Params: []testing.Param{{
			Name: "mdla_4",
			Val: testingParams{
				MaxMdla: 4,
				MaxMvpu: 1,
				Model:   "conv2dx4.tflite",
			},
			ExtraData: []string{"conv2dx4.tflite"},
		}, {
			Name: "mvpu_2",
			Val: testingParams{
				MaxMdla: 1,
				MaxMvpu: 2,
				Model:   "castx2.tflite",
			},
			ExtraData: []string{"castx2.tflite"},
		}},
	})
}

type testingParams struct {
	MaxMdla int
	MaxMvpu int
	Model   string
}

func NeuronMultiCore(ctx context.Context, s *testing.State) {
	param := s.Param().(testingParams)

	if param.MaxMdla > 1 && param.MaxMvpu > 1 {
		s.Fatal("Either MaxMdla or MaxMvpu must be one: ", param)
	}

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
		"--graph":                         s.DataPath(param.Model),
	}

	var inferenceLatencies []float64
	for numMdla := 1; numMdla <= param.MaxMdla; numMdla++ {
		for numMvpu := 1; numMvpu <= param.MaxMvpu; numMvpu++ {
			s.Logf("Testing %d mdla %d mvpu", numMdla, numMvpu)

			var neuronSettings = neuronBaseSettings
			neuronSettings.MtkNeuronSettings.CompileOptions = []string{"--num-mdla", strconv.Itoa(numMdla), "--num-mvpu", strconv.Itoa(numMvpu)}
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
}
