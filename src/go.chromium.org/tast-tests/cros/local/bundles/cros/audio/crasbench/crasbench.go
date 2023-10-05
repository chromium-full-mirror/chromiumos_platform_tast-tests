// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package crasbench implements cras_bench and cras_bench_smoke tests.
package crasbench

import (
	"context"
	"encoding/json"
	"strings"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/dlc"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// result is the output of `cras_bench`.
//
// An output example (in JSON format):
//
//	{
//	  ...,
//	  "benchmarks": [
//	    {
//	      "name": "BM_Dsp/Eq2/256",
//	      "family_index": 0,
//	      "per_family_instance_index": 0,
//	      "run_name": "BM_Dsp/Eq2/256",
//	      "run_type": "iteration",
//	      "repetitions": 1,
//	      "repetition_index": 0,
//	      "threads": 1,
//	      "iterations": 323328,
//	      "real_time": 2.1650514616736227e+03,
//	      "cpu_time": 2.1631811411322246e+03,
//	      "time_unit": "ns",
//	      "frames_per_second": 1.1834422699617644e+08,
//	      "time_per_48k_frames": 4.0569433410672857e-04
//	    },
//	    ...
//	  ]
//	}
type result struct {
	Benchmarks []struct {
		Name             string  `json:"name"`
		CPUTime          float64 `json:"cpu_time"`
		FPS              float64 `json:"frames_per_second"`
		Time4096Frame    float64 `json:"time_per_4096_frames"`
		MaxTime4096Frame float64 `json:"max_time_per_4096_frames"`
	} `json:"benchmarks"`
}

// runCrasBench runs the cras_bench command and parse the result.
func runCrasBench(ctx context.Context, benchmarkFilter string) (*result, error) {
	out, err := testexec.CommandContext(ctx, "cras_bench", "--benchmark_format=json", "--benchmark_filter="+benchmarkFilter).Output(testexec.DumpLogOnError)
	if err != nil {
		return nil, errors.Errorf("cras_bench exec error: %s", err)
	}

	result := &result{}
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, errors.Errorf("failed to unmarshal test results: %s", err)
	}

	return result, nil
}

// Run the benchmark.
func Run(ctx context.Context, s *testing.State) {
	param := s.Param().(Parameters)

	if param.DLC != "" {
		if err := dlc.Install(ctx, param.DLC, ""); err != nil {
			s.Fatal("Failed to install dlc: ", err)
		}
	}

	result, err := runCrasBench(ctx, param.BenchmarkFilter)
	if err != nil {
		s.Fatal("Failed to run cras_bench: ", err)
	}

	p := perf.NewValues()
	for _, res := range result.Benchmarks {
		// Name field in perf.Metric accepts only "_", "." and "-".
		name := strings.ReplaceAll(res.Name, "/", "_")
		cpuTime := perf.Metric{Name: name, Variant: "cpu_time", Unit: "ns", Direction: perf.SmallerIsBetter}
		p.Set(cpuTime, res.CPUTime)
		if param.MetricFps {
			fps := perf.Metric{Name: name, Variant: "fps", Unit: "fps", Direction: perf.BiggerIsBetter}
			p.Set(fps, res.FPS)
		}
		if param.Time4096Frame {
			time4096 := perf.Metric{Name: name, Variant: "time_per_4096_frames", Unit: "ns", Direction: perf.SmallerIsBetter}
			p.Set(time4096, res.Time4096Frame)
		}
		if param.MaxTime4096Frame {
			maxTime4096 := perf.Metric{Name: name, Variant: "max_time_per_4096_frames", Unit: "ns", Direction: perf.SmallerIsBetter}
			p.Set(maxTime4096, res.MaxTime4096Frame)
		}
	}
	if err := p.Save(s.OutDir()); err != nil {
		s.Error("Failed saving perf data: ", err)
	}
}
