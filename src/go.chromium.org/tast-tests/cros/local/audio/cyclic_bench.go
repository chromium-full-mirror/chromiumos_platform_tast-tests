// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"encoding/json"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
)

// SchedPolicy is the enum for the Scheduling Policy
type SchedPolicy int

const (
	// RrSched uses rr (Round-Robin) as the scheduler.
	RrSched SchedPolicy = iota
	// OtherSched uses other(normal) as the scheduler.
	OtherSched
)

// Affinity is the affinity of the processor where the test runs
type Affinity int

const (
	// AllCores will use all the cores in round-robin order.
	AllCores Affinity = iota
	// SmallCore will run all the threads on a single small core.
	SmallCore
	// BigCore will run all the threads on a single big core.
	BigCore
)

// SchedConfig is the Scheduling Config
type SchedConfig struct {
	Policy   SchedPolicy // the schedule policy.
	Priority int         // Priority of the process. If `Policy` is real time, `Priority` is real time priority. If `Policy` is CFS, `Priority` specify the nice value.
}

// CyclicTestParameters contains all the data needed to run a single test iteration.
type CyclicTestParameters struct {
	Config              SchedConfig   // The schedule config of the cyclictest.
	Threads             int           // Number of threads.
	Interval            time.Duration // Interval time.
	Loops               int           // Number of times.
	Affinity            Affinity      // Run cyclictest threads on which sets of processors.
	P99Threshold        time.Duration // P99 latency threshold.
	StressConfig        *SchedConfig  // The schedule config of the stress process. if `StressConfig` is nil, no stress process will be run.
	StressOutOfVMConfig *SchedConfig  // The schedule config of the stress process out of VM. If `StressOutOfVMConfig` is nil, no stress out of VM will be run.
}

// CyclicTestResult is the structure of the output from cyclic_bench.py
type CyclicTestResult struct {
	CyclicTestStats []struct {
		ThreadID float64 `json:"thread_id"`
		Min      float64 `json:"min"`
		Median   float64 `json:"median"`
		P99      float64 `json:"p99"`
		Max      float64 `json:"max"`
	} `json:"stats"`
}

const (
	// CrasPriority indicates the rt-priority of cras.
	CrasPriority = 12
	// CrasClientPriority indicates the rt-priority of cras client.
	CrasClientPriority = 10
	// DefaultStressPriority indicates the default rt-priority of stress threads.
	DefaultStressPriority = 20
	// DefaultInterval is the default interval used in cyclictest.
	DefaultInterval = 10000 * time.Microsecond
	// DefaultLoops is the default number of loops tested in cyclictest.
	DefaultLoops = 6000
	// DefaultP99Threshold is the default p99 latency threshold allowed in cyclictest.
	DefaultP99Threshold = 1200 * time.Microsecond
	// DefaultStressWorker is the number of workers spawned in the stress test per cpu thread.
	DefaultStressWorker = 2
)

func (s SchedPolicy) String() string {
	return []string{"rr", "other"}[s]
}

func (a Affinity) String() string {
	// Affinity for using `AllCores` parameter is "default"
	return []string{"default", "small_core", "big_core"}[a]
}

func getNumberOfCPU(ctx context.Context) (int, error) {
	lscpu := testexec.CommandContext(ctx, "lscpu")
	out, err := lscpu.Output()
	if err != nil {
		return -1, errors.Wrap(err, "lscpu failed")
	}
	cpuRe := regexp.MustCompile(`^CPU\(s\):\s*(.*)$`)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if !cpuRe.MatchString(line) {
			continue
		}
		cpus := cpuRe.FindStringSubmatch(line)
		ret, err := strconv.Atoi(cpus[1])
		if err != nil {
			return -1, errors.Wrap(err, "parsing number of cpus failed")
		}
		return ret, nil
	}
	return -1, errors.New("can't find CPU(s) info in lscpu")
}

// GetStressCommandContext returns the command to give stress to the host.
func GetStressCommandContext(ctx context.Context, param CyclicTestParameters) (*testexec.Cmd, error) {
	if param.StressOutOfVMConfig == nil {
		return nil, nil
	}

	config := param.StressOutOfVMConfig

	// Set the timeout of stress to be 10% more of the expected time
	// of cyclic test in case the stress-ng failed to be killed.
	timeout := param.Loops * (int(param.Interval / time.Microsecond)) / 1000000 * 11 / 10
	if param.StressConfig != nil {
		// Set the timeout twice if there's stress inside the vm
		// to avoid stress out of vm finishing too early.
		timeout = timeout * 2
	}

	cpus, err := getNumberOfCPU(ctx)
	if err != nil {
		return nil, err
	}
	totalWorkers := DefaultStressWorker * cpus

	commands := []string{
		"stress-ng",
		"--cpu=" + strconv.Itoa(totalWorkers),
		"--timeout=" + strconv.Itoa(timeout) + "s",
		"--sched=" + config.Policy.String(),
	}

	switch config.Policy {
	case RrSched:
		commands = append(commands, "--sched-prio="+strconv.Itoa(config.Priority))
	case OtherSched:
		prependNice := []string{"nice", "-n", strconv.Itoa(config.Priority)}
		commands = append(prependNice, commands...)
	default:
		return nil, errors.New("unsupported stress scheduling policy")
	}

	return testexec.CommandContext(ctx, commands[0], commands[1:]...), nil
}

// GetCyclicTestArgs returns args for the cyclic_bench.py script.
func GetCyclicTestArgs(param CyclicTestParameters, outputFilePath string) []string {
	testArgs := []string{
		"--policy=" + param.Config.Policy.String(),
		"--priority=" + strconv.Itoa(param.Config.Priority),
		"--interval=" + strconv.Itoa(int(param.Interval/time.Microsecond)),
		"--threads=" + strconv.Itoa(param.Threads),
		"--loops=" + strconv.Itoa(param.Loops),
		"--affinity=" + param.Affinity.String(),
		"--output_file=" + outputFilePath,
		"--json"}
	if param.StressConfig != nil {
		testArgs = append(testArgs,
			"--stress_policy="+param.StressConfig.Policy.String(),
			"--stress_priority="+strconv.Itoa(param.StressConfig.Priority),
			"--workers_per_cpu="+strconv.Itoa(DefaultStressWorker),
		)
	}
	return testArgs
}

// ReadCyclicTestResult parses the json file from the cyclic_bench output.
func ReadCyclicTestResult(outputFilePath string) (*CyclicTestResult, error) {
	buf, err := os.ReadFile(outputFilePath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read output file")
	}

	result := &CyclicTestResult{}
	err = json.Unmarshal(buf, result)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse result file")
	}
	return result, nil
}

// UpdatePerfFromCyclicTestResult modifies the perf.value to record the test result.
func UpdatePerfFromCyclicTestResult(p *perf.Values, result *CyclicTestResult) {
	for _, stat := range result.CyclicTestStats {
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
	}
}
