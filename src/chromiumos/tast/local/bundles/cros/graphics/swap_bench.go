// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package graphics

import (
	"bufio"
	"context"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"chromiumos/tast/common/perf"
	"chromiumos/tast/common/testexec"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/graphics"
	"chromiumos/tast/testing"
	"chromiumos/tast/testing/hwdep"
)

var (
	// regexp to match string with format `time: 16.1234`
	timeMsPattern = regexp.MustCompile(`(?:time: )\d+.\d+`)
	// default refresh rate in ms
	refreshMs = "16.66"
	// target refresh rate command line option
	targetRefreshRate = "--target-refresh-rate"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: SwapBench,
		Desc: "Test overhead for rendering frames",
		Contacts: []string{
			"chromeos-gaming-core@google.com",
			"mrfemi@google.com",
		},
		BugComponent: "b:961455", // ChromeOS > Platform > Graphics > Gaming > Steam > Core Gfx
		Attr:         []string{"group:graphics", "graphics_perbuild"},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		SoftwareDeps: []string{"no_qemu"},
		Fixture:      "graphicsNoChrome",
		Timeout:      5 * time.Minute,
	})
}

// outputResultsToJSON reads the compositortest.log output and compute metrics
// the average and percentiles. Save these statistics to a results-chart JSON
// file that will get picked up by Crosbolt.
func outputResultsToJSON(ctx context.Context, outDir string, f *os.File) error {
	var times []float64
	var totalTime float64

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
				times = append(times, gpuElapsedTime)
				totalTime = totalTime + gpuElapsedTime
			}
		}
	}
	lenTimes := len(times)
	if lenTimes == 0 {
		return nil
	}
	if lenTimes < 1000 {
		testing.ContextLogf(ctx, "Warning: compositortest only recorded %d frames", lenTimes)
	} else {
		testing.ContextLogf(ctx, "Scanned %d frames from compositortest", lenTimes)
	}
	avg := totalTime / float64(lenTimes)
	sort.Float64s(times)

	perfValues := perf.NewValues()
	perfValues.Set(perf.Metric{
		Name:      "Benchmark.SwapBench.GPU_frame_time_average",
		Unit:      "ms",
		Direction: perf.SmallerIsBetter,
	}, avg)

	if lenTimes >= 1000 {
		nineNinetyNineIndex := int(math.Floor(float64(lenTimes) * float64(0.999)))
		nineNinetyNineValue := times[nineNinetyNineIndex]
		perfValues.Set(perf.Metric{
			Name:      "Benchmark.SwapBench.GPU_frame_time_999",
			Unit:      "ms",
			Direction: perf.SmallerIsBetter,
		}, nineNinetyNineValue)

		oneIndex := int(math.Floor(float64(lenTimes) * float64(0.001)))
		oneValue := times[oneIndex]
		perfValues.Set(perf.Metric{
			Name:      "Benchmark.SwapBench.GPU_frame_time_001",
			Unit:      "ms",
			Direction: perf.SmallerIsBetter,
		}, oneValue)
	}

	if lenTimes >= 100 {
		nineNinetyIndex := int(math.Floor(float64(lenTimes) * float64(0.990)))
		nineNinetyValue := times[nineNinetyIndex]
		perfValues.Set(perf.Metric{
			Name:      "Benchmark.SwapBench.GPU_frame_time_990",
			Unit:      "ms",
			Direction: perf.SmallerIsBetter,
		}, nineNinetyValue)

		tenIndex := int(math.Floor(float64(lenTimes) * float64(0.010)))
		tenValue := times[tenIndex]
		perfValues.Set(perf.Metric{
			Name:      "Benchmark.SwapBench.GPU_frame_time_010",
			Unit:      "ms",
			Direction: perf.SmallerIsBetter,
		}, tenValue)
	}

	if lenTimes >= 20 {
		ninetyFiveIndex := int(math.Floor(float64(lenTimes) * float64(0.950)))
		ninetyFiveValue := times[ninetyFiveIndex]
		perfValues.Set(perf.Metric{
			Name:      "Benchmark.SwapBench.GPU_frame_time_950",
			Unit:      "ms",
			Direction: perf.SmallerIsBetter,
		}, ninetyFiveValue)

		fiveHundredIndex := int(math.Floor(float64(lenTimes) * float64(0.500)))
		fifeHundredValue := times[fiveHundredIndex]
		perfValues.Set(perf.Metric{
			Name:      "Benchmark.SwapBench.GPU_frame_time_500",
			Unit:      "ms",
			Direction: perf.SmallerIsBetter,
		}, fifeHundredValue)
	}

	if err := perfValues.Save(outDir); err != nil {
		return errors.Wrap(err, "failed to save perf data")
	}
	return nil
}

func SwapBench(ctx context.Context, s *testing.State) {
	refreshRate := 60.0
	connectors, err := graphics.ModetestConnectors(ctx)
	if err != nil {
		s.Error("Failed to get connectors: ", err)
	} else {
		// switch manually input refresh rate instead of targeted/automatic one
		targetRefreshRate = "--no-target-refresh-rate"
		for _, connector := range connectors {
			refreshRate = connector.Modes[0].Refresh
			break
		}
	}

	s.Logf("Display has refresh rate of %f Hz", refreshRate)
	refreshMs = strconv.FormatFloat(1000/refreshRate, 'f', 2, 64)

	args := []string{
		targetRefreshRate,
		"--gpu-workload-ms", refreshMs,
	}

	out, err := testexec.CommandContext(ctx, "/usr/local/glbench/bin/compositortest", args...).Output(testexec.DumpLogOnError)
	if err != nil {
		s.Error("Failed to run compositortest: ", err)
	}
	file, err := os.Create(filepath.Join(s.OutDir(), "compositortest.log"))
	if err != nil {
		s.Fatal("Failed to create compositortest log output file: ", err)
	}
	defer file.Close()

	_, err = file.Write(out)
	if err != nil {
		s.Error("Failed to write compositortest log file: ", err)
	}
	err = outputResultsToJSON(ctx, s.OutDir(), file)
	if err != nil {
		s.Error("Failed to calculate results-chart.json metrics: ", err)
	}
}
