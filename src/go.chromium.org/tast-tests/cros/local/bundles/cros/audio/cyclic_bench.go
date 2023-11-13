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

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/tracing"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
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
}

const (
	// crasPrioriy indicates the rt-priority of cras.
	crasPriority = 12
	// crasClientPriority indicates the rt-priority of cras client.
	crasClientPriority = 10
	// defaultStressPriority indicates the default rt-priority of stress threads.
	defaultStressPriority = 20
	// defaultInterval is the default interval used in cyclictest.
	defaultInterval = 10000 * time.Microsecond
	// defaultLoops is the default number of loops tested in cyclictest.
	defaultLoops = 6000
	// defaultMaxLatencyThreshold is the default max latency threshold allowed in cyclictest.
	defaultMaxLatencyThreshold = 3000 * time.Microsecond
	// defaultStressWorker is the number of workers spawned in the stress test per cpu thread.
	defaultStressWorker = 2
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CyclicBench,
		LacrosStatus: testing.LacrosVariantUnknown,
		Desc:         "Benchmarks for scheduling latency with cyclictest binary",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "eddyhsu@chromium.org", "paulhsia@chromium.org", "cychiang@chromium.org"},
		BugComponent: "b:776546",
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		Data:         []string{"perfetto_trace.txtpb"},
		SoftwareDeps: []string{"cras", "chrome"},
		Timeout:      5 * time.Minute,
		Params: []testing.Param{
			{
				Name: "rr12_1thread_10ms",
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
				},
			},
			{
				Name: "rr10_1thread_10ms",
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
				},
			},
			{
				Name: "rr10_1thread_10ms_hrtimer_off",
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
				},
				Fixture:           fixture.HighResTimerOff,
				ExtraHardwareDeps: hwdep.D(hwdep.HasDynamicHighResTimerControl()),
			},
			{
				Name: "rr12_4thread_10ms",
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
				},
			},
			{
				Name: "rr10_4thread_10ms",
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
				},
			},
			{
				Name: "rr12_1thread_10ms_stress_rr20_2workers_per_cpu",
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
					StressConfig: &schedConfig{
						Policy:   rrSched,
						Priority: defaultStressPriority,
					},
				},
			},
			{
				Name: "rr12_1thread_10ms_stress_nice_p0_2workers_per_cpu",
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
					StressConfig: &schedConfig{
						Policy:   otherSched,
						Priority: 0,
					},
					ShouldFail: true,
				},
			},
			{
				Name: "nice_p0_1thread_10ms",
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
				},
			},
			{
				Name: "nice_p0_1thread_10ms_hrtimer_off",
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
				},
				Fixture:           fixture.HighResTimerOff,
				ExtraHardwareDeps: hwdep.D(hwdep.HasDynamicHighResTimerControl()),
			},
			{
				Name: "nice_n20_1thread_10ms",
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
				},
			},
			{
				Name: "nice_n20_1thread_10ms_hrtimer_off",
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
				},
				Fixture:           fixture.HighResTimerOff,
				ExtraHardwareDeps: hwdep.D(hwdep.HasDynamicHighResTimerControl()),
			},
			{
				Name: "nice_p19_1thread_10ms",
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
				},
			},
			{
				Name: "nice_p0_1thread_10ms_stress_nice_p0_2workers_per_cpu",
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
					StressConfig: &schedConfig{
						Policy:   otherSched,
						Priority: 0,
					},
				},
			},
			{
				Name: "rr12_1thread_10ms_small_core",
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
				},
				ExtraSoftwareDeps: []string{"cpu_heterogeneous"},
			},
			{
				Name: "rr12_1thread_10ms_big_core",
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
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, chrome.ResetTimeout)
	defer cancel()

	chrome, err := chrome.New(
		ctx,
		chrome.GuestLogin(),
	)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer chrome.Close(cleanupCtx)

	param := s.Param().(cyclicTestParameters)

	cmdStr := []string{"cyclic_bench.py",
		"--policy=" + param.Config.Policy.String(),
		"--priority=" + strconv.Itoa(param.Config.Priority),
		"--interval=" + strconv.Itoa(int(param.Interval/time.Microsecond)),
		"--threads=" + strconv.Itoa(param.Threads),
		"--loops=" + strconv.Itoa(param.Loops),
		"--affinity=" + param.Affinity.String(),
		"--json",
	}
	if param.StressConfig != nil {
		cmdStr = append(cmdStr,
			"--stress_policy="+param.StressConfig.Policy.String(),
			"--stress_priority="+strconv.Itoa(param.StressConfig.Priority),
			"--workers_per_cpu="+strconv.Itoa(defaultStressWorker))
	}

	testing.ContextLog(ctx, "Start tracing")
	cleanupTraceCtx := ctx
	ctx, cancel2 := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel2()

	filepath := filepath.Join(
		s.OutDir(),
		fmt.Sprintf("perfetto-%d.pb", time.Now().Unix()))

	session, err := tracing.StartSession(ctx, s.DataPath("perfetto_trace.txtpb"),
		tracing.WithTraceDataPath(filepath),
		tracing.WithCompression(),
	)
	if err != nil {
		s.Error(err, "failed to start tracing")
	}
	defer session.Finalize(cleanupTraceCtx)

	testing.ContextLog(ctx, "Start to execute cyclic_bench.py")
	out, err := testexec.CommandContext(ctx, cmdStr[0], cmdStr[1:]...).Output(testexec.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed to execute cyclic_bench.py: ", err)
	}

	testing.ContextLog(ctx, "Stop tracing: "+filepath)
	if err := session.Stop(); err != nil {
		s.Error(err, "failed to stop tracing")
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

	err = json.Unmarshal(out, &stats)
	if err != nil {
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

		if stat.Max > float64(param.MaxLatencyThreshold/time.Microsecond) {
			if param.ShouldFail {
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
