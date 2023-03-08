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

	"chromiumos/tast/common/perf"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/graphics"
	"chromiumos/tast/testing"
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

	f.Seek(0, io.SeekStart)
	fileScanner := bufio.NewScanner(f)
	fileScanner.Split(bufio.ScanLines)

	for fileScanner.Scan() {
		line := fileScanner.Text()
		if strings.Contains(line, "frame") && strings.Contains(line, "gpu_elapsed_time") {
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
	}
	lenTimes := len(gpuTimes)
	if lenTimes == 0 {
		return nil
	}
	if lenTimes < 1000 {
		testing.ContextLogf(ctx, "Warning: compositortest only recorded %d frames", lenTimes)
	} else {
		testing.ContextLogf(ctx, "Scanned %d frames from compositortest", lenTimes)
	}
	avg := gpuTotalTime / float64(lenTimes)
	sort.Float64s(gpuTimes)

	perfValues := perf.NewValues()
	perfValues.Set(perf.Metric{
		Name:      "Benchmark." + version + ".GPU_frame_time_average",
		Unit:      "ms",
		Direction: perf.BiggerIsBetter,
	}, avg)

	if lenTimes >= 1000 {
		nineNinetyNineIndex := int(math.Floor(float64(lenTimes) * float64(0.999)))
		nineNinetyNineValue := gpuTimes[nineNinetyNineIndex]
		perfValues.Set(perf.Metric{
			Name:      "Benchmark." + version + ".GPU_frame_time_999",
			Unit:      "ms",
			Direction: perf.BiggerIsBetter,
		}, nineNinetyNineValue)

		oneIndex := int(math.Floor(float64(lenTimes) * float64(0.001)))
		oneValue := gpuTimes[oneIndex]
		perfValues.Set(perf.Metric{
			Name:      "Benchmark." + version + ".GPU_frame_time_001",
			Unit:      "ms",
			Direction: perf.BiggerIsBetter,
		}, oneValue)
	}

	if lenTimes >= 100 {
		nineNinetyIndex := int(math.Floor(float64(lenTimes) * float64(0.990)))
		nineNinetyValue := gpuTimes[nineNinetyIndex]
		perfValues.Set(perf.Metric{
			Name:      "Benchmark." + version + ".GPU_frame_time_990",
			Unit:      "ms",
			Direction: perf.BiggerIsBetter,
		}, nineNinetyValue)

		tenIndex := int(math.Floor(float64(lenTimes) * float64(0.010)))
		tenValue := gpuTimes[tenIndex]
		perfValues.Set(perf.Metric{
			Name:      "Benchmark." + version + ".GPU_frame_time_010",
			Unit:      "ms",
			Direction: perf.BiggerIsBetter,
		}, tenValue)
	}

	if lenTimes >= 20 {
		ninetyFiveIndex := int(math.Floor(float64(lenTimes) * float64(0.950)))
		ninetyFiveValue := gpuTimes[ninetyFiveIndex]
		perfValues.Set(perf.Metric{
			Name:      "Benchmark." + version + ".GPU_frame_time_950",
			Unit:      "ms",
			Direction: perf.BiggerIsBetter,
		}, ninetyFiveValue)

		fiveHundredIndex := int(math.Floor(float64(lenTimes) * float64(0.500)))
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

// GetRefreshRateArgs queries modetest for the devices panel refresh rate,
// and returns
func GetRefreshRateArgs(ctx context.Context) (string, string, error) {
	refreshRate := 60.0
	connectors, err := graphics.ModetestConnectors(ctx)
	if err != nil {
		return targetRefreshRate, refreshMs, errors.Wrap(err, "failed to get connectors")
	}
	// switch manually input refresh rate instead of targeted/automatic one
	targetRefreshRate = "--no-target-refresh-rate"
	for _, connector := range connectors {
		refreshRate = connector.Modes[0].Refresh
		break
	}

	testing.ContextLogf(ctx, "Display has refresh rate of %f Hz", refreshRate)
	refreshMs := strconv.FormatFloat(1000/refreshRate, 'f', 2, 64)

	return targetRefreshRate, refreshMs, nil
}
