// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package swapbench provides functions for passing commands into
// and parsing output from compositortest (platform/glbench/src/) binary.
package swapbench

import (
	"bufio"
	"context"
	"io"
	"math"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/graphics/modetest"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

var (
	// regexp to match string with format `time: 16.1234`
	timeMsPattern = regexp.MustCompile(`(?:time: )\d+.\d+`)
	// default refresh rate in ms
	refreshMs = "16.66"
	// target refresh rate command line option
	targetRefreshRate = "--target-refresh-rate"
)

// OutputResultsToJSON reads the compositortest.log output and compute metrics
// the average and percentiles. Save these statistics to a results-chart JSON
// file that will get picked up by Crosbolt.
func OutputResultsToJSON(ctx context.Context, version, outDir string, f *os.File) error {
	var gpuTimes []float64
	var gpuTotalTime float64

	var cpuTimes []float64
	var cpuTotalTime float64

	f.Seek(0, io.SeekStart)
	fileScanner := bufio.NewScanner(f)
	fileScanner.Split(bufio.ScanLines)

	for fileScanner.Scan() {
		line := fileScanner.Text()
		if !strings.Contains(line, "frame") {
			continue
		}
		if strings.Contains(line, "gpu_elapsed_time") {
			match := timeMsPattern.FindString(line)
			if match != "" {
				// We ignore the `time: ` portion of match and parse the float
				timeString := strings.Split(match, " ")[1]
				gpuElapsedTime, err := strconv.ParseFloat(timeString, 64)
				if err != nil {
					return errors.Wrap(err, "error parsing test results")
				}
				gpuTimes = append(gpuTimes, gpuElapsedTime)
				gpuTotalTime = gpuTotalTime + gpuElapsedTime
			}
		}
		if strings.Contains(line, "cpu_elapsed_time") {
			match := timeMsPattern.FindString(line)
			if match != "" {
				// We ignore the `time: ` portion of match and parse the float
				timeString := strings.Split(match, " ")[1]
				cpuElapsedTime, err := strconv.ParseFloat(timeString, 64)
				if err != nil {
					return errors.Wrap(err, "error parsing test results")
				}
				cpuTimes = append(cpuTimes, cpuElapsedTime)
				cpuTotalTime = cpuTotalTime + cpuElapsedTime
			}
		}
	}

	averageGap := 0.0
	numGaps := len(cpuTimes)
	lenGpuTimes := len(gpuTimes)
	if lenGpuTimes < numGaps {
		numGaps = lenGpuTimes
	}
	if numGaps == 0 {
		return errors.New("no frame times were recorded to compositortest.log")
	}
	for i := 0; i < numGaps; i++ {
		if cpuTimes[i] < gpuTimes[i] {
			// This is a weird and unlikely scenario, so we log it.
			testing.ContextLogf(ctx, "Frame %d spent more time in GPU (%f ms) than CPU (%f ms)", i, gpuTimes[i], cpuTimes[i])
		} else {
			averageGap = averageGap + (cpuTimes[i] - gpuTimes[i])
		}
	}
	averageGap = averageGap / float64(numGaps)

	if lenGpuTimes < 1000 {
		testing.ContextLogf(ctx, "Warning: compositortest only recorded %d frames", lenGpuTimes)
	} else {
		testing.ContextLogf(ctx, "Scanned %d frames from compositortest", lenGpuTimes)
	}
	avg := gpuTotalTime / float64(lenGpuTimes)
	sort.Float64s(gpuTimes)

	perfValues := perf.NewValues()
	perfValues.Set(perf.Metric{
		Name:      "Benchmark." + version + ".GPU_frame_time_average",
		Unit:      "ms",
		Direction: perf.BiggerIsBetter,
	}, avg)

	perfValues.Set(perf.Metric{
		Name:      "Benchmark." + version + ".CPU_GPU_average_gap",
		Unit:      "ms",
		Direction: perf.SmallerIsBetter,
	}, averageGap)

	if lenGpuTimes >= 1000 {
		nineNinetyNineIndex := int(math.Floor(float64(lenGpuTimes) * float64(0.999)))
		nineNinetyNineValue := gpuTimes[nineNinetyNineIndex]
		perfValues.Set(perf.Metric{
			Name:      "Benchmark." + version + ".GPU_frame_time_999",
			Unit:      "ms",
			Direction: perf.BiggerIsBetter,
		}, nineNinetyNineValue)

		oneIndex := int(math.Floor(float64(lenGpuTimes) * float64(0.001)))
		oneValue := gpuTimes[oneIndex]
		perfValues.Set(perf.Metric{
			Name:      "Benchmark." + version + ".GPU_frame_time_001",
			Unit:      "ms",
			Direction: perf.BiggerIsBetter,
		}, oneValue)
	}

	if lenGpuTimes >= 100 {
		nineNinetyIndex := int(math.Floor(float64(lenGpuTimes) * float64(0.990)))
		nineNinetyValue := gpuTimes[nineNinetyIndex]
		perfValues.Set(perf.Metric{
			Name:      "Benchmark." + version + ".GPU_frame_time_990",
			Unit:      "ms",
			Direction: perf.BiggerIsBetter,
		}, nineNinetyValue)

		tenIndex := int(math.Floor(float64(lenGpuTimes) * float64(0.010)))
		tenValue := gpuTimes[tenIndex]
		perfValues.Set(perf.Metric{
			Name:      "Benchmark." + version + ".GPU_frame_time_010",
			Unit:      "ms",
			Direction: perf.BiggerIsBetter,
		}, tenValue)
	}

	if lenGpuTimes >= 20 {
		nineFiftyIndex := int(math.Floor(float64(lenGpuTimes) * float64(0.950)))
		nineFiftyValue := gpuTimes[nineFiftyIndex]
		perfValues.Set(perf.Metric{
			Name:      "Benchmark." + version + ".GPU_frame_time_950",
			Unit:      "ms",
			Direction: perf.BiggerIsBetter,
		}, nineFiftyValue)

		fiveHundredIndex := int(math.Floor(float64(lenGpuTimes) * float64(0.500)))
		fiveHundredValue := gpuTimes[fiveHundredIndex]
		perfValues.Set(perf.Metric{
			Name:      "Benchmark." + version + ".GPU_frame_time_500",
			Unit:      "ms",
			Direction: perf.BiggerIsBetter,
		}, fiveHundredValue)
	}

	if err := perfValues.Save(outDir); err != nil {
		return errors.Wrap(err, "failed to save perf data")
	}
	return nil
}

// GetRefreshRateArgs queries modetest for the devices panel refresh rate
// and resolution, returning an error on failure. It also returns either
// --target-refresh-rate or --no-target-refresh-rate depending on whether or not
// it was able to acquire the refresh rate of the device's internal display.
func GetRefreshRateArgs(ctx context.Context) (string, string, string, string, error) {
	refreshRate := 60.0
	width := "1280"
	height := "720"
	connectors, err := modetest.Connectors(ctx)
	if err != nil {
		return targetRefreshRate, refreshMs, width, height, errors.Wrap(err, "failed to get connectors")
	}
	// switch manually input refresh rate instead of targeted/automatic one
	targetRefreshRate = "--no-target-refresh-rate"
	for _, connector := range connectors {
		refreshRate = connector.Modes[0].Refresh
		width = strconv.FormatUint(uint64(connector.Modes[0].HDisplay), 10)
		height = strconv.FormatUint(uint64(connector.Modes[0].VDisplay), 10)
		break
	}

	testing.ContextLogf(ctx, "Display has refresh rate of %f Hz", refreshRate)
	refreshMs := strconv.FormatFloat(1000/refreshRate, 'f', 2, 64)

	return targetRefreshRate, refreshMs, width, height, nil
}
