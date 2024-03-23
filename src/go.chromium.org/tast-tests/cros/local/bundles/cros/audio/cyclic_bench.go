// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/crosconfig"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"golang.org/x/exp/slices"
)

type schedPolicy int

const (
	// rrSched uses rr as the scheduler.
	rrSched schedPolicy = iota
	// otherSched uses other(normal) as the scheduler.
	otherSched
)

type affinity int

const (
	// defaultAff will use all the processors in round-robin order.
	defaultAff affinity = iota
	// smallCore will run all the threads on a single small core.
	smallCore
	// bigCore will run all the threads on a single big core.
	bigCore
)

type schedConfig struct {
	Policy   schedPolicy // the schedule policy.
	Priority int         // Priority of the process. If `Policy` is real time, `Priority` is real time priority. If `Policy` is CFS, `Priority` specify the nice value.
}

// cyclicTestParameters contains all the data needed to run a single test iteration.
type cyclicTestParameters struct {
	Config              schedConfig   // The schedule config of the cyclictest.
	Threads             int           // Number of threads.
	Interval            time.Duration // Interval time.
	Loops               int           // Number of times.
	Affinity            affinity      // Run cyclictest threads on which sets of processors.
	MaxLatencyThreshold time.Duration // Max latency threshold.
	StressConfig        *schedConfig  // The schedule config of the stress process. if `StressConfig` is nil, no stress process will be run.
	ShouldFail          bool          // Whether the test should fail based on the threshold. This should only be true for tests that simulate actual CRAS specs to prevent noise.
	UI                  bool          // Test with UI running or not.
	Tracer              bool          // Test with ftrace running or not.
}

const (
	// crasPrioriy indicates the rt-priority of cras.
	crasPriority = 12
	// crasClientPriority indicates the rt-priority of cras client.
	crasClientPriority = 10
	// defaultStressPriority indicates the default rt-priority of stress threads.
	defaultStressPriority = 8
	// defaultInterval is the default interval used in cyclictest.
	defaultInterval = 10000 * time.Microsecond
	// defaultLoops is the default number of loops tested in cyclictest.
	defaultLoops = 60000
	// defaultMaxLatencyThreshold is the default max latency threshold allowed in cyclictest.
	defaultMaxLatencyThreshold = 3000 * time.Microsecond
	// defaultStressWorker is the number of workers spawned in the stress test per cpu thread.
	defaultStressWorker = 2
)

// TODO(b/297967956): Remove devices when latencies are stable
var cyclicBenchUnstableModels = []string{
	// asurada
	"hayato", "spherion",
	// volteer
	"chronicler", "collis", "copano", "delbin", "drobit", "elemi", "lillipup", "lindar", "volet", "volta",
	// strongbad
	"coachz", "homestar", "quackingstick", "wormdingler",
	// trogdor
	"kingoftown", "lazor", "limozeen", "pazquel", "pazquel360", "pompom",
	// staryu
	"starmie",
	// elm
	"elm",
	// jacuzzi
	"burnet", "willow",
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         CyclicBench,
		LacrosStatus: testing.LacrosVariantUnknown,
		Desc:         "Benchmarks for scheduling latency with cyclictest binary",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "eddyhsu@chromium.org", "paulhsia@chromium.org", "cychiang@chromium.org"},
		BugComponent: "b:776546",
		Data:         []string{"perfetto_trace.txtpb"},
		SoftwareDeps: []string{"cras", "chrome"},
		Params: []testing.Param{
			{
				Name:      "rr12_1thread_10ms",
				ExtraAttr: []string{"group:crosbolt", "crosbolt_perbuild"},
				Timeout:   15 * time.Minute,
				Val: cyclicTestParameters{
					Config: schedConfig{
						Policy:   rrSched,
						Priority: crasPriority,
					},
					Threads:             1,
					Interval:            defaultInterval,
					Loops:               defaultLoops,
					Affinity:            defaultAff,
					MaxLatencyThreshold: defaultMaxLatencyThreshold,
					StressConfig:        nil,
					ShouldFail:          true,
					UI:                  true,
				},
			},
			{
				Name:      "rr12_1thread_10ms_with_tracer",
				ExtraAttr: []string{"group:crosbolt", "crosbolt_perbuild"},
				Timeout:   15 * time.Minute,
				Val: cyclicTestParameters{
					Config: schedConfig{
						Policy:   rrSched,
						Priority: crasPriority,
					},
					Threads:             1,
					Interval:            defaultInterval,
					Loops:               defaultLoops,
					Affinity:            defaultAff,
					MaxLatencyThreshold: defaultMaxLatencyThreshold,
					StressConfig:        nil,
					ShouldFail:          false,
					UI:                  true,
					Tracer:              true,
				},
			},
			{
				Name:      "rr12_1thread_10ms_ui_stopped",
				Fixture:   "uiStopped",
				ExtraAttr: []string{"group:crosbolt", "crosbolt_perbuild"},
				Timeout:   15 * time.Minute,
				Val: cyclicTestParameters{
					Config: schedConfig{
						Policy:   rrSched,
						Priority: crasPriority,
					},
					Threads:             1,
					Interval:            defaultInterval,
					Loops:               defaultLoops,
					Affinity:            defaultAff,
					MaxLatencyThreshold: defaultMaxLatencyThreshold,
					StressConfig:        nil,
					ShouldFail:          true,
					UI:                  false,
				},
			},
			{
				Name:      "rr12_1thread_10ms_ui_stopped_with_tracer",
				Fixture:   "uiStopped",
				ExtraAttr: []string{"group:crosbolt", "crosbolt_perbuild"},
				Timeout:   15 * time.Minute,
				Val: cyclicTestParameters{
					Config: schedConfig{
						Policy:   rrSched,
						Priority: crasPriority,
					},
					Threads:             1,
					Interval:            defaultInterval,
					Loops:               defaultLoops,
					Affinity:            defaultAff,
					MaxLatencyThreshold: defaultMaxLatencyThreshold,
					StressConfig:        nil,
					ShouldFail:          false,
					UI:                  false,
					Tracer:              true,
				},
			},
			{
				Name:    "rr12_1thread_10ms_double_loop_count",
				Timeout: 150 * time.Minute,
				Val: cyclicTestParameters{
					Config: schedConfig{
						Policy:   rrSched,
						Priority: crasPriority,
					},
					Threads:             1,
					Interval:            defaultInterval,
					Loops:               defaultLoops * 10, // The pass rate is 0.6, we need to run 10X to make the pass rate < 0.01
					Affinity:            defaultAff,
					MaxLatencyThreshold: defaultMaxLatencyThreshold,
					StressConfig:        nil,
					ShouldFail:          true,
					UI:                  true,
					Tracer:              true,
				},
			},
			{
				Name:      "rr10_1thread_10ms",
				ExtraAttr: []string{"group:crosbolt", "crosbolt_perbuild"},
				Timeout:   15 * time.Minute,
				Val: cyclicTestParameters{
					Config: schedConfig{
						Policy:   rrSched,
						Priority: crasClientPriority,
					},
					Threads:             1,
					Interval:            defaultInterval,
					Loops:               defaultLoops,
					Affinity:            defaultAff,
					MaxLatencyThreshold: defaultMaxLatencyThreshold,
					StressConfig:        nil,
					UI:                  true,
				},
			},
			{
				Name:      "rr12_4thread_10ms",
				ExtraAttr: []string{"group:crosbolt", "crosbolt_perbuild"},
				Timeout:   15 * time.Minute,
				Val: cyclicTestParameters{
					Config: schedConfig{
						Policy:   rrSched,
						Priority: crasPriority,
					},
					Threads:             4,
					Interval:            defaultInterval,
					Loops:               defaultLoops,
					Affinity:            defaultAff,
					MaxLatencyThreshold: defaultMaxLatencyThreshold,
					StressConfig:        nil,
					UI:                  true,
				},
			},
			{
				Name:      "rr10_4thread_10ms",
				ExtraAttr: []string{"group:crosbolt", "crosbolt_perbuild"},
				Timeout:   15 * time.Minute,
				Val: cyclicTestParameters{
					Config: schedConfig{
						Policy:   rrSched,
						Priority: crasClientPriority,
					},
					Threads:             4,
					Interval:            defaultInterval,
					Loops:               defaultLoops,
					Affinity:            defaultAff,
					MaxLatencyThreshold: defaultMaxLatencyThreshold,
					StressConfig:        nil,
					UI:                  true,
				},
			},
			{
				Name:      "rr12_1thread_10ms_stress_rr8_2workers_per_cpu",
				ExtraAttr: []string{"group:crosbolt", "crosbolt_perbuild"},
				Timeout:   15 * time.Minute,
				Val: cyclicTestParameters{
					Config: schedConfig{
						Policy:   rrSched,
						Priority: crasPriority,
					},
					Threads:  1,
					Interval: defaultInterval,
					// Use 6000 loops to prevent crash. See b/314889993
					Loops:               6000,
					Affinity:            defaultAff,
					MaxLatencyThreshold: defaultMaxLatencyThreshold,
					StressConfig: &schedConfig{
						Policy:   rrSched,
						Priority: defaultStressPriority,
					},
					UI: true,
				},
			},
			{
				Name:      "rr12_1thread_10ms_stress_nice_p0_2workers_per_cpu",
				ExtraAttr: []string{"group:crosbolt", "crosbolt_perbuild"},
				Timeout:   15 * time.Minute,
				Val: cyclicTestParameters{
					Config: schedConfig{
						Policy:   rrSched,
						Priority: crasPriority,
					},
					Threads:  1,
					Interval: defaultInterval,
					// Use 6000 loops to prevent crash. See b/322308493
					Loops:               6000,
					Affinity:            defaultAff,
					MaxLatencyThreshold: defaultMaxLatencyThreshold,
					StressConfig: &schedConfig{
						Policy:   otherSched,
						Priority: 0,
					},
					UI: true,
				},
			},
			{
				Name:      "nice_p0_1thread_10ms",
				ExtraAttr: []string{"group:crosbolt", "crosbolt_perbuild"},
				Timeout:   15 * time.Minute,
				Val: cyclicTestParameters{
					Config: schedConfig{
						Policy:   otherSched,
						Priority: 0,
					},
					Threads:             1,
					Interval:            defaultInterval,
					Loops:               defaultLoops,
					Affinity:            defaultAff,
					MaxLatencyThreshold: 10000 * time.Microsecond,
					StressConfig:        nil,
					UI:                  true,
				},
			},
			{
				Name:      "nice_n20_1thread_10ms",
				ExtraAttr: []string{"group:crosbolt", "crosbolt_perbuild"},
				Timeout:   15 * time.Minute,
				Val: cyclicTestParameters{
					Config: schedConfig{
						Policy:   otherSched,
						Priority: -20,
					},
					Threads:             1,
					Interval:            defaultInterval,
					Loops:               defaultLoops,
					Affinity:            defaultAff,
					MaxLatencyThreshold: 5000 * time.Microsecond,
					StressConfig:        nil,
					UI:                  true,
				},
			},
			{
				Name:      "nice_p19_1thread_10ms",
				ExtraAttr: []string{"group:crosbolt", "crosbolt_perbuild"},
				Timeout:   15 * time.Minute,
				Val: cyclicTestParameters{
					Config: schedConfig{
						Policy:   otherSched,
						Priority: 19,
					},
					Threads:             1,
					Interval:            defaultInterval,
					Loops:               defaultLoops,
					Affinity:            defaultAff,
					MaxLatencyThreshold: 20000 * time.Microsecond,
					StressConfig:        nil,
					UI:                  true,
				},
			},
			{
				Name:      "nice_p0_1thread_10ms_stress_nice_p0_2workers_per_cpu",
				ExtraAttr: []string{"group:crosbolt", "crosbolt_perbuild"},
				Timeout:   15 * time.Minute,
				Val: cyclicTestParameters{
					Config: schedConfig{
						Policy:   otherSched,
						Priority: 0,
					},
					Threads:  1,
					Interval: defaultInterval,
					// Use 6000 loops to prevent crash. See b/322308493
					Loops:               6000,
					Affinity:            defaultAff,
					MaxLatencyThreshold: 10000 * time.Microsecond,
					StressConfig: &schedConfig{
						Policy:   otherSched,
						Priority: 0,
					},
					UI: true,
				},
			},
			{
				Name:      "rr12_1thread_10ms_small_core",
				ExtraAttr: []string{"group:crosbolt", "crosbolt_perbuild"},
				Timeout:   15 * time.Minute,
				Val: cyclicTestParameters{
					Config: schedConfig{
						Policy:   rrSched,
						Priority: crasPriority,
					},
					Threads:             1,
					Interval:            defaultInterval,
					Loops:               defaultLoops,
					Affinity:            smallCore,
					MaxLatencyThreshold: defaultMaxLatencyThreshold,
					StressConfig:        nil,
					UI:                  true,
				},
				ExtraSoftwareDeps: []string{"cpu_heterogeneous"},
			},
			{
				Name:      "rr12_1thread_10ms_big_core",
				ExtraAttr: []string{"group:crosbolt", "crosbolt_perbuild"},
				Timeout:   15 * time.Minute,
				Val: cyclicTestParameters{
					Config: schedConfig{
						Policy:   rrSched,
						Priority: crasPriority,
					},
					Threads:             1,
					Interval:            defaultInterval,
					Loops:               defaultLoops,
					Affinity:            bigCore,
					MaxLatencyThreshold: defaultMaxLatencyThreshold,
					StressConfig:        nil,
					UI:                  true,
				},
				ExtraSoftwareDeps: []string{"cpu_heterogeneous"},
			},
		},
	})
}

func (s schedPolicy) String() string {
	return []string{"rr", "other"}[s]
}

func (a affinity) String() string {
	return []string{"default", "small_core", "big_core"}[a]
}

func CyclicBench(ctx context.Context, s *testing.State) {
	const tracingOverhead = 100 * time.Microsecond
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, chrome.ResetTimeout)
	defer cancel()

	param := s.Param().(cyclicTestParameters)
	if param.UI {
		chrome, err := chrome.New(
			ctx,
			chrome.GuestLogin(),
		)
		if err != nil {
			s.Fatal("Failed to start Chrome: ", err)
		}
		defer chrome.Close(cleanupCtx)
	}

	cmdStr := []string{"cyclic_bench.py",
		"--policy=" + param.Config.Policy.String(),
		"--priority=" + strconv.Itoa(param.Config.Priority),
		"--interval=" + strconv.Itoa(int(param.Interval/time.Microsecond)),
		"--threads=" + strconv.Itoa(param.Threads),
		"--loops=" + strconv.Itoa(param.Loops),
		"--affinity=" + param.Affinity.String(),
		"--json",
	}
	if param.Tracer {
		cmdStr = append(cmdStr,
			fmt.Sprintf("--breaktrace=%d", int((param.MaxLatencyThreshold+tracingOverhead)/time.Microsecond)),
			"--tracemark")
	}
	if param.StressConfig != nil {
		cmdStr = append(cmdStr,
			"--stress_policy="+param.StressConfig.Policy.String(),
			"--stress_priority="+strconv.Itoa(param.StressConfig.Priority),
			"--workers_per_cpu="+strconv.Itoa(defaultStressWorker))
	}

	if param.Tracer {
		testing.ContextLog(ctx, "Start trace-cmd")

		if err := testexec.CommandContext(ctx, "trace-cmd", "start", "-e", "sched", "-e", "timer", "-e", "irq", "-e", "irq_vectors", "-e", "irq_matrix", "-e", "softirq_raise", "-R", "stacktrace", "-b", "1500", "-C", "local").Run(testexec.DumpLogOnError); err != nil {
			s.Error("Cannot run trace-cmd start: ", err)
		}
	}

	testing.ContextLog(ctx, "Start to execute cyclic_bench.py")
	out, err := testexec.CommandContext(ctx, cmdStr[0], cmdStr[1:]...).Output(testexec.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed to execute cyclic_bench.py: ", err)
	}

	if param.Tracer {
		filepath := filepath.Join(s.OutDir(), fmt.Sprintf("trace-cyclic-%d.dat", time.Now().Unix()))
		testing.ContextLog(ctx, "Start trace-cmd extract")
		traceExtractCmd := testexec.CommandContext(ctx, "trace-cmd", "extract", "-o", filepath)
		traceExtractCmd.Dir = "/tmp"
		if err := traceExtractCmd.Run(testexec.DumpLogOnError); err != nil {
			s.Fatal("Failed to execute trace-cmd extract: ", err)
		}
		testing.ContextLog(ctx, "Stop trace-cmd")
		if err := testexec.CommandContext(ctx, "trace-cmd", "stop").Run(testexec.DumpLogOnError); err != nil {
			s.Error("Cannot run trace-cmd stop: ", err)
		}
	}

	stats := struct {
		CyclicTestStat []struct {
			ThreadID float64 `json:"thread_id"`
			Min      float64 `json:"min"`
			Median   float64 `json:"median"`
			P99      float64 `json:"p99"`
			Max      float64 `json:"max"`
		} `json:"stats"`
	}{}

	if err := json.Unmarshal(out, &stats); err != nil {
		s.Error("Failed to parse result file: ", err)
	}

	p := perf.NewValues()
	for _, stat := range stats.CyclicTestStat {
		threadID := int(stat.ThreadID)
		name := "Thread_" + strconv.Itoa(threadID)
		minLatency := perf.Metric{
			Name:      name,
			Variant:   "min_latency",
			Unit:      "us",
			Direction: perf.SmallerIsBetter}
		p.Set(minLatency, stat.Min)
		medianLatency := perf.Metric{
			Name:      name,
			Variant:   "p50_latency",
			Unit:      "us",
			Direction: perf.SmallerIsBetter}
		p.Set(medianLatency, stat.Median)
		p99Latency := perf.Metric{
			Name:      name,
			Variant:   "p99_latency",
			Unit:      "us",
			Direction: perf.SmallerIsBetter}
		p.Set(p99Latency, stat.P99)
		maxLatency := perf.Metric{
			Name:      name,
			Variant:   "max_latency",
			Unit:      "us",
			Direction: perf.SmallerIsBetter}
		p.Set(maxLatency, stat.Max)

		model, err := crosconfig.Get(ctx, "/", "name")
		if err != nil {
			s.Error("Failed to read model from cros_config: ", err)
		}

		if stat.Max > float64(param.MaxLatencyThreshold/time.Microsecond) {
			if param.ShouldFail && !slices.Contains(cyclicBenchUnstableModels, model) {
				s.Error("Max latency exceeds threshold: ", stat.Max,
					"us > ", param.MaxLatencyThreshold)
			} else {
				s.Log("Max latency exceeds threshold: ", stat.Max,
					"us > ", param.MaxLatencyThreshold)
			}
		}
	}
	if err := p.Save(s.OutDir()); err != nil {
		s.Error("Failed saving perf data: ", err)
	}

}
