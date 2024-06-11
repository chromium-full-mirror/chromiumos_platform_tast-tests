// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package metrics

import (
	"bufio"
	"context"
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/shirou/gopsutil/v3/cpu"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/power"
	"go.chromium.org/tast/core/errors"
)

// CPUUsageSource is an implementation of perf.TimelineDataSource which reports
// the CPU usage.
type CPUUsageSource struct {
	// `name` is the prefix name for cpu metrics.
	name string
	// `intervalName` provides information about what interval the cpu metrics
	// with multiple values are taken at, e.g. `5s`.
	intervalName string
	// `prevStats` is previous `cpu.TimesStat` keyed by cpu name.
	prevStats map[string]cpu.TimesStat
	// `prevTimeInState` is previous cpu time in state keyed by cpu name.
	// CPU time in state is keyed by frequency and the value is time in
	// usertime units of time at that frequency. Usertime units of time
	// is 10mS.
	prevTimeInState map[string]map[int64]float64
	// `maxFreqReported` is whether the current "policy limits" (in kHz)
	// and the maximum operating frequency the processor can run at(in kHz)
	// are already recorded, keyed by cpu name.
	maxFreqReported map[string]bool
	// `cpuMaxFreqs` is the maximum operating frequency the processor
	// can run at (in kHz), keyed by cpu name.
	cpuMaxFreqs map[string]float64
	// `cpuCapacities` is the capacity that provides the scheduler
	// information about CPUs heterogeneity, keyed by cpu name.
	cpuCapacities map[string]float64
	// `totalCPUCapacity` is the sum of all CPUs' capacities.
	totalCPUCapacity float64
}

func cpuUtilization(prev, time cpu.TimesStat) float64 {
	// TimesStat (defined in gopsutil/cpu/cpu.go) contains the amounts of time the
	// CPU has spent performing different types of work. Time units are in seconds
	// and it is based on linux /proc/stat file.
	//
	// The types of work are: user, nice, system, idle, iowait, irq, softirq, steal,
	// guest, guest_nice. It is documented in linux proc.rst.
	//
	// Here we consider everything but idle and iowait as "busy".
	prevBusy := prev.User + prev.System + prev.Nice + prev.Irq + prev.Softirq + prev.Steal
	newBusy := time.User + time.System + time.Nice + time.Irq + time.Softirq + time.Steal
	prevTotal := prevBusy + prev.Idle + prev.Iowait
	newTotal := newBusy + time.Idle + time.Iowait
	if prevBusy > newBusy {
		return 0
	}
	if prevTotal >= newTotal {
		return 100
	}
	return math.Max(0, math.Min(100, 100*(newBusy-prevBusy)/(newTotal-prevTotal)))
}

// cpuFreq reads frequency data from cpufreq/cpuinfo_`type`_freq where `type` can be
// `max`, `min` or `cur`.
func cpuFreq(cpuName, freqType string) (float64, error) {
	data, err := ioutil.ReadFile(filepath.Join(
		"/sys/devices/system/cpu", cpuName, "cpufreq", fmt.Sprintf("cpuinfo_%s_freq", freqType)))
	if err != nil {
		return 0, err
	}

	freq, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, err
	}
	// frequency data is in kHz.
	return float64(freq), nil
}

// CPUScalingFreq reads frequency data from cpufreq/scaling_`type`_freq where `type` can be
// `max`, `min` or `cur`.
func CPUScalingFreq(cpuName, freqType string) (float64, error) {
	data, err := ioutil.ReadFile(filepath.Join(
		"/sys/devices/system/cpu", cpuName, "cpufreq", fmt.Sprintf("scaling_%s_freq", freqType)))
	if err != nil {
		return 0, err
	}

	freq, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, err
	}
	// frequency data is in kHz.
	return float64(freq), nil
}

func cpuCapacity(cpuName string) (float64, error) {
	data, err := ioutil.ReadFile(filepath.Join(
		"/sys/devices/system/cpu", cpuName, "cpu_capacity"))
	if err != nil {
		return 0, err
	}

	capacity, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, err
	}
	return float64(capacity), nil
}

func cpuTimeInState(cpuName string) (map[int64]float64, error) {
	timeInState := make(map[int64]float64)
	// time_in_state gives the amount of time spent in each of the frequencies supported by
	// this CPU. The cat output will have "<frequency> <time>" pair in each line, which
	// will mean this CPU spent <time> usertime units of time at <frequency>. Output
	// will have one line for each of the supported frequencies. usertime units here
	// is 10mS (similar to other time exported in /proc).
	//
	// e.g. /sys/devices/system/cpu/cpu0/cpufreq/stats
	// 3600000 2089
	// 3400000 136
	// 3200000 34
	// 3000000 67
	// 2800000 172488
	file, err := os.Open(filepath.Join(
		"/sys/devices/system/cpu", cpuName, "cpufreq/stats/time_in_state"))
	if err != nil {
		return timeInState, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		s := strings.Split(scanner.Text(), " ")
		if len(s) != 2 {
			continue
		}
		freq, err := strconv.ParseInt(s[0], 10, 64)
		if err != nil {
			continue
		}
		time, err := strconv.ParseFloat(s[1], 64)
		if err != nil {
			continue
		}
		timeInState[freq] = time
	}
	return timeInState, nil
}

func diffTimeInState(cur, prev map[int64]float64) (map[int64]float64, error) {
	diff := map[int64]float64{}
	for freq, curTime := range cur {
		prevTime, ok := prev[freq]
		if !ok {
			return diff, errors.Errorf("new frequency %d does not exist in previous snapshot", freq)
		}
		diff[freq] = curTime - prevTime
	}
	return diff, nil
}

// NewCPUUsageSource creates a new instance of CPUUsageSource for the given
// metric name.
func NewCPUUsageSource(name string) *CPUUsageSource {
	if name == "" {
		name = "CPUUsage"
	}
	return &CPUUsageSource{
		name:             name,
		prevStats:        map[string]cpu.TimesStat{},
		prevTimeInState:  map[string]map[int64]float64{},
		maxFreqReported:  map[string]bool{},
		cpuMaxFreqs:      map[string]float64{},
		cpuCapacities:    map[string]float64{},
		totalCPUCapacity: 0,
	}
}

// Setup implements perf.TimelineDatasource.Setup.
func (s *CPUUsageSource) Setup(ctx context.Context, prefix, intervalName string) error {
	s.name = prefix + s.name
	s.intervalName = intervalName
	return nil
}

// Start implements perf.TimelineDatasource.Start.
func (s *CPUUsageSource) Start(ctx context.Context) error {
	times, err := cpu.TimesWithContext(ctx, true /*perCPU*/)
	if err != nil {
		return err
	}
	for _, time := range times {
		s.prevStats[time.CPU] = time
		if capacity, err := cpuCapacity(time.CPU); err == nil {
			s.cpuCapacities[time.CPU] = capacity
			s.totalCPUCapacity += capacity
		}
		if maxFreq, err := cpuFreq(time.CPU, "max"); err == nil {
			s.cpuMaxFreqs[time.CPU] = maxFreq
		}
		if state, err := cpuTimeInState(time.CPU); err == nil {
			s.prevTimeInState[time.CPU] = state
		}
	}

	return nil
}

// Snapshot implements perf.TimelineDatasource.Snapshot.
func (s *CPUUsageSource) Snapshot(ctx context.Context, values *perf.Values) error {
	times, err := cpu.TimesWithContext(ctx, true /*perCPU*/)
	if err != nil {
		return err
	}

	busyPercentages := map[string]float64{}
	for _, time := range times {
		busyPercentages[time.CPU] = cpuUtilization(s.prevStats[time.CPU], time) / 100
	}
	armCPUUsage := 0.0

	var totalPercent float64
	for _, time := range times {
		var percent float64
		var prevTime cpu.TimesStat
		if pt, ok := s.prevStats[time.CPU]; ok {
			prevTime = pt
		} else {
			prevTime = cpu.TimesStat{}
		}
		percent = cpuUtilization(prevTime, time)
		values.Append(perf.Metric{
			Name:      s.name + "." + time.CPU,
			Variant:   "usage",
			Multiple:  true,
			Unit:      power.CPUUsageMetricTypeUnit,
			Direction: perf.SmallerIsBetter,
			Interval:  s.intervalName,
		}, percent)
		totalPercent += percent
		s.prevStats[time.CPU] = time
		freq, err := cpuFreq(time.CPU, "cur")
		if err != nil {
			// `cpuinfo_cur_freq` is expected to be the frequency the hardware actually
			// runs at. If that frequency cannot be determined, this attribute should
			// not be present. In this case we use scaling_cur_freq instead.
			if freq, err = CPUScalingFreq(time.CPU, "cur"); err != nil {
				return err
			}
		}
		values.Append(perf.Metric{
			Name:      s.name + "." + time.CPU + ".Frequency",
			Multiple:  true,
			Unit:      power.CPUFreqMetricTypeUnit,
			Direction: perf.BiggerIsBetter,
			Interval:  s.intervalName,
		}, freq/1000)
		if !s.maxFreqReported[time.CPU] {
			maxFreq := s.cpuMaxFreqs[time.CPU]
			values.Set(perf.Metric{
				Name:      s.name + "." + time.CPU + ".MaxFrequency",
				Unit:      power.CPUFreqMetricTypeUnit,
				Direction: perf.BiggerIsBetter,
			}, maxFreq/1000)
			maxScalingFreq, err := CPUScalingFreq(time.CPU, "max")
			if err != nil {
				return err
			}
			values.Set(perf.Metric{
				Name:      s.name + "." + time.CPU + ".MaxScalingFrequency",
				Unit:      power.CPUFreqMetricTypeUnit,
				Direction: perf.BiggerIsBetter,
			}, maxScalingFreq/1000)
			s.maxFreqReported[time.CPU] = true
		}

		// For non-ARM devices, there might not be `cpu_capacity` files
		// thus `totalCPUCapacity` would be 0 in that case.
		if s.totalCPUCapacity != 0 {
			curTimeInState, err := cpuTimeInState(time.CPU)
			if err != nil {
				return err
			}
			prevTimeInState := s.prevTimeInState[time.CPU]

			diffTimeInState, err := diffTimeInState(curTimeInState, prevTimeInState)
			if err != nil {
				return errors.Wrapf(err, "time_in_state for %s has been changed", time.CPU)
			}
			s.prevTimeInState[time.CPU] = curTimeInState

			totalTime := 0.0
			for _, time := range diffTimeInState {
				totalTime += time
			}

			averageFreq := 0.0
			for freq, time := range diffTimeInState {
				if time != 0 {
					averageFreq += float64(freq) * time / totalTime
				}
			}

			maxFreq := s.cpuMaxFreqs[time.CPU]
			// `maxFreq` is in kHz (from cpufreq/cpuinfo_max_freq) and `averageFreq`
			// from `time_in_state` is also in kHz.
			freqPct := averageFreq / maxFreq

			armCPUUsage += freqPct * busyPercentages[time.CPU] * s.cpuCapacities[time.CPU]
		}
	}

	// Only record ARM CPU usage on ARM devices.
	if s.totalCPUCapacity != 0 && armCPUUsage != 0.0 {
		values.Append(perf.Metric{
			Name:      s.name + ".ARM",
			Multiple:  true,
			Unit:      power.CPUUsageMetricTypeUnit,
			Direction: perf.SmallerIsBetter,
			Interval:  s.intervalName,
		}, 100*armCPUUsage/s.totalCPUCapacity)
	}

	values.Append(perf.Metric{
		Name:      s.name,
		Multiple:  true,
		Unit:      power.CPUUsageMetricTypeUnit,
		Direction: perf.SmallerIsBetter,
		Interval:  s.intervalName,
	}, totalPercent/float64(len(times)))

	return nil
}

// Stop does nothing.
func (s *CPUUsageSource) Stop(_ context.Context, values *perf.Values) error {
	return nil
}
