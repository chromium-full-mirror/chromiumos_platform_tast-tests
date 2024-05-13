// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package vm

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/vm/audioutils"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/vm/dlc"
	"go.chromium.org/tast/core/testing"
)

const runCyclicTest string = "run-cyclic-test.sh"

func init() {
	testing.AddTest(&testing.Test{
		Func:         AudioCyclicBench,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Benchmarks for scheduling latency with cyclictest binary",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "eddyhsu@chromium.org", "paulhsia@chromium.org", "cychiang@chromium.org"},
		BugComponent: "b:1332660",
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild", "group:audio"},
		Data:         []string{runCyclicTest},
		SoftwareDeps: []string{"cras", "vm_host", "chrome", "dlc"},
		Timeout:      6 * time.Minute,
		Fixture:      "vmDLC",
		Params: []testing.Param{
			{
				Name: "rr12_1thread_10ms",
				Val: audio.CyclicTestParameters{
					Config: audio.SchedConfig{
						Policy:   audio.RrSched,
						Priority: audio.CrasPriority,
					},
					Threads:             1,
					Interval:            audio.DefaultInterval,
					Loops:               audio.DefaultLoops,
					Affinity:            audio.AllCores,
					P99Threshold:        audio.DefaultP99Threshold,
					StressConfig:        nil,
					StressOutOfVMConfig: nil,
				},
			},
			{
				Name: "rr10_1thread_10ms",
				Val: audio.CyclicTestParameters{
					Config: audio.SchedConfig{
						Policy:   audio.RrSched,
						Priority: audio.CrasClientPriority,
					},
					Threads:             1,
					Interval:            audio.DefaultInterval,
					Loops:               audio.DefaultLoops,
					Affinity:            audio.AllCores,
					P99Threshold:        audio.DefaultP99Threshold,
					StressConfig:        nil,
					StressOutOfVMConfig: nil,
				},
			},
			{
				Name: "rr12_1thread_10ms_stress_nice_p0_2workers_per_cpu",
				Val: audio.CyclicTestParameters{
					Config: audio.SchedConfig{
						Policy:   audio.RrSched,
						Priority: audio.CrasPriority,
					},
					Threads:      1,
					Interval:     audio.DefaultInterval,
					Loops:        audio.DefaultLoops,
					Affinity:     audio.AllCores,
					P99Threshold: audio.DefaultP99Threshold,
					StressConfig: &audio.SchedConfig{
						Policy:   audio.OtherSched,
						Priority: 0,
					},
					StressOutOfVMConfig: nil,
				},
			},
			{
				Name: "nice_p0_1thread_10ms",
				Val: audio.CyclicTestParameters{
					Config: audio.SchedConfig{
						Policy:   audio.OtherSched,
						Priority: 0,
					},
					Threads:             1,
					Interval:            audio.DefaultInterval,
					Loops:               audio.DefaultLoops,
					Affinity:            audio.AllCores,
					P99Threshold:        1800 * time.Microsecond,
					StressConfig:        nil,
					StressOutOfVMConfig: nil,
				},
			},
			{
				Name: "nice_n20_1thread_10ms",
				Val: audio.CyclicTestParameters{
					Config: audio.SchedConfig{
						Policy:   audio.OtherSched,
						Priority: -20,
					},
					Threads:             1,
					Interval:            audio.DefaultInterval,
					Loops:               audio.DefaultLoops,
					Affinity:            audio.AllCores,
					P99Threshold:        1800 * time.Microsecond,
					StressConfig:        nil,
					StressOutOfVMConfig: nil,
				},
			},
			{
				Name: "nice_p19_1thread_10ms",
				Val: audio.CyclicTestParameters{
					Config: audio.SchedConfig{
						Policy:   audio.OtherSched,
						Priority: 19,
					},
					Threads:             1,
					Interval:            audio.DefaultInterval,
					Loops:               audio.DefaultLoops,
					Affinity:            audio.AllCores,
					P99Threshold:        5000 * time.Microsecond,
					StressConfig:        nil,
					StressOutOfVMConfig: nil,
				},
			},
			{
				Name: "nice_p0_1thread_10ms_stress_nice_p0_2workers_per_cpu",
				Val: audio.CyclicTestParameters{
					Config: audio.SchedConfig{
						Policy:   audio.OtherSched,
						Priority: 0,
					},
					Threads:      1,
					Interval:     audio.DefaultInterval,
					Loops:        audio.DefaultLoops,
					Affinity:     audio.AllCores,
					P99Threshold: 30000 * time.Microsecond,
					StressConfig: &audio.SchedConfig{
						Policy:   audio.OtherSched,
						Priority: 0,
					},
					StressOutOfVMConfig: nil,
				},
			},
			{
				Name: "rr12_1thread_10ms_stress_out_of_vm_nice_p0_2workers_per_cpu",
				Val: audio.CyclicTestParameters{
					Config: audio.SchedConfig{
						Policy:   audio.RrSched,
						Priority: audio.CrasPriority,
					},
					Threads:      1,
					Interval:     audio.DefaultInterval,
					Loops:        audio.DefaultLoops,
					Affinity:     audio.AllCores,
					P99Threshold: audio.DefaultP99Threshold,
					StressConfig: nil,
					StressOutOfVMConfig: &audio.SchedConfig{
						Policy:   audio.OtherSched,
						Priority: 0,
					},
				},
			},
		},
	})
}

// AudioCyclicBench is used as one of the standard SW gates (go/pe-sw-gates).
// Please ask to crosvm-core@ if you want to modify or delete this test.
func AudioCyclicBench(ctx context.Context, s *testing.State) {
	param := s.Param().(audio.CyclicTestParameters)
	data := s.FixtValue().(dlc.FixtData)

	kernelLogPath := filepath.Join(s.OutDir(), "kernel.log")
	outputFilePath := filepath.Join(s.OutDir(), "output.log")

	cyclictestArgs := audio.GetCyclicTestArgs(param, outputFilePath)

	kernelArgs := []string{
		fmt.Sprintf("init=%s", s.DataPath(runCyclicTest)),
		"--",
	}
	kernelArgs = append(kernelArgs, cyclictestArgs...)

	stressOutOfVM, err := audio.GetStressCommandContext(ctx, param)
	if err != nil {
		s.Error("Failed to get stress(out of vm) command context: ", err)
	}

	if stressOutOfVM != nil {
		// Working directory of `stress-ng` must be readable and writeable
		stressOutOfVM.Dir = "/tmp"
		if err := stressOutOfVM.Start(); err != nil {
			s.Fatal("Failed to start stress workload out of vm: ", err)
		}
	}

	s.Log("Running Cyclic test")
	if err := audioutils.RunCrosvm(ctx, data.Kernel, kernelLogPath, kernelArgs, audioutils.Config{}); err != nil {
		s.Fatal("Failed to run crosvm: ", err)
	}

	result, err := audio.ReadCyclicTestResult(outputFilePath)
	if err != nil {
		s.Fatal("Failed to read the result: ", err)
	}
	for _, stat := range result.CyclicTestStats {
		if stat.P99 > float64(param.P99Threshold/time.Microsecond) {
			s.Log("p99 latency exceeds threshold: ", stat.P99,
				" > ", param.P99Threshold)
		}
	}

	if stressOutOfVM != nil {
		if err := stressOutOfVM.Wait(); err != nil {
			s.Error("stress-ng failed to finish: ", err)
		}
	}

	p := perf.NewValues()
	audio.UpdatePerfFromCyclicTestResult(p, result)
	if err := p.Save(s.OutDir()); err != nil {
		s.Error("Failed saving perf data: ", err)
	}
}
