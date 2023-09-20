// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/cpu"
	"go.chromium.org/tast-tests/cros/local/gtest"
	mediacpu "go.chromium.org/tast-tests/cros/local/media/cpu"
	"go.chromium.org/tast-tests/cros/local/sysutil"
	"go.chromium.org/tast-tests/cros/local/upstart"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DecodeAccelJPEGPerf,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Measures jpeg_decode_accelerator_unittest performance",
		Contacts:     []string{"chromeos-camera-eng@google.com", "kamesan@chromium.org"},
		BugComponent: "b:167281", // ChromeOS > Platform > Technologies > Camera
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		SoftwareDeps: []string{"chrome", caps.HWDecodeJPEG},
		Data: []string{"peach_pi-1280x720.jpg", "pink-nature-1920x1080.jpg",
			"red-squirrel-2560x1920.jpg", "bonsai-tree-3840x2160.jpg"},
		// TODO(b/287584650): Modify the gtest to finish in a given time so we can control the timeout.
		Timeout: 60 * time.Minute,
	})
}

type decodeAccelJpegPerfResolution struct {
	TestFile       string
	JPEGResolution string
}

var decodeAccelJpegPerfResolutions = []decodeAccelJpegPerfResolution{
	{
		TestFile:       "peach_pi-1280x720.jpg",
		JPEGResolution: "1280x720",
	},
	{
		TestFile:       "pink-nature-1920x1080.jpg",
		JPEGResolution: "1920x1080",
	},
	{
		TestFile:       "red-squirrel-2560x1920.jpg",
		JPEGResolution: "2560x1920",
	},
	{
		TestFile:       "bonsai-tree-3840x2160.jpg",
		JPEGResolution: "3840x2160",
	},
}

// DecodeAccelJPEGPerf runs for specific resolutions to measure the performance
// of SW/HW JPEG decoders and generates performance metrics for each resolution.
func DecodeAccelJPEGPerf(ctx context.Context, s *testing.State) {
	p := perf.NewValues()
	for _, item := range decodeAccelJpegPerfResolutions {
		decodeAccelJPEGPerfForResolution(ctx, s, item, p)
	}
	p.Save(s.OutDir())
}

// decodeAccelJPEGPerfForResolution measures SW/HW jpeg decode performance of a resolution
// by running the PerfSW and PerfJDA tests in the jpeg_decode_accelerator_unittest.
// TODO(dstaessens@) Currently the performance tests decode JPEGs as fast as
// possible. But this means a performant HW decoder might actually increase
// CPU usage, as the CPU becomes the bottleneck.
func decodeAccelJPEGPerfForResolution(ctx context.Context, s *testing.State,
	item decodeAccelJpegPerfResolution, p *perf.Values) {
	const (
		// Duration of the interval during which CPU usage will be measured.
		measureDuration = 10 * time.Second
		// GTest filter used to run SW JPEG decode tests.
		swFilter = "MjpegDecodeAcceleratorTest.PerfSW"
		// GTest filter used to run HW JPEG decode tests.
		hwFilter = "All/MjpegDecodeAcceleratorTest.PerfJDA/DMABUF"
		// Number of JPEG decodes, needs to be high enough to run for measurement duration.
		perfJPEGDecodeTimes = 10000
		// time reserved for cleanup.
		cleanupTime = 5 * time.Second
	)

	testDir := filepath.Dir(s.DataPath(item.TestFile))

	// Stop the UI job. While this isn't required to run the test binary, it's
	// possible a previous tests left tabs open or an animation is playing,
	// influencing our performance results.
	if err := upstart.StopJob(ctx, "ui"); err != nil {
		s.Fatal("Failed to stop ui: ", err)
	}
	defer upstart.EnsureJobRunning(ctx, "ui")

	cleanUpBenchmark, err := mediacpu.SetUpBenchmark(ctx)
	if err != nil {
		s.Fatal("Failed to set up benchmark mode: ", err)
	}
	defer cleanUpBenchmark(ctx)

	// Reserve time for cleanup and restarting the ui job at the end of the test.
	ctx, cancel := ctxutil.Shorten(ctx, cleanupTime)
	defer cancel()

	if err := cpu.WaitUntilIdle(ctx); err != nil {
		s.Fatal("Failed to wait for CPU to become idle: ", err)
	}

	s.Log("Measuring SW JPEG decode performance")
	cpuUsageSW, metricsSW := runJPEGPerfBenchmark(ctx, s, testDir,
		measureDuration, perfJPEGDecodeTimes, swFilter, "sw", item)
	s.Log("Measuring HW JPEG decode performance")
	cpuUsageHW, metricsHW := runJPEGPerfBenchmark(ctx, s, testDir,
		measureDuration, perfJPEGDecodeTimes, hwFilter, "hw", item)

	p.Set(perf.Metric{
		Name:      "sw_jpeg_decode_cpu_" + item.JPEGResolution,
		Unit:      "percent",
		Direction: perf.SmallerIsBetter,
	}, cpuUsageSW)
	p.Set(perf.Metric{
		Name:      "hw_jpeg_decode_cpu_" + item.JPEGResolution,
		Unit:      "percent",
		Direction: perf.SmallerIsBetter,
	}, cpuUsageHW)
	for name, value := range metricsSW {
		p.Set(perf.Metric{
			Name:      name + "_" + item.JPEGResolution,
			Unit:      "milliseconds",
			Direction: perf.SmallerIsBetter,
		}, value)
	}
	for name, value := range metricsHW {
		p.Set(perf.Metric{
			Name:      name + "_" + item.JPEGResolution,
			Unit:      "milliseconds",
			Direction: perf.SmallerIsBetter,
		}, value)
	}
}

// runJPEGPerfBenchmark runs the JPEG decode accelerator unittest binary, and
// returns the measured CPU usage percentage and decode latency.
func runJPEGPerfBenchmark(ctx context.Context, s *testing.State, testDir string,
	measureDuration time.Duration, perfJPEGDecodeTimes int, filter, id string,
	item decodeAccelJpegPerfResolution) (float64, map[string]float64) {
	// Measures CPU usage while running the unittest, and waits for the unittest
	// process to finish for the complete logs.
	const exec = "jpeg_decode_accelerator_unittest"
	logPath := fmt.Sprintf("%s/%s.%s.%s.log", s.OutDir(), exec, item.JPEGResolution, id)
	outPath := fmt.Sprintf("%s/perf_output.%s.%s.json", s.OutDir(), item.JPEGResolution, id)
	startTime := time.Now()
	measurements, err := mediacpu.MeasureProcessUsage(ctx, measureDuration, mediacpu.WaitProcess,
		gtest.New(
			filepath.Join(chrome.BinTestDir, exec),
			gtest.Logfile(logPath),
			gtest.Filter(filter),
			gtest.ExtraArgs(
				"--perf_decode_times="+strconv.Itoa(perfJPEGDecodeTimes),
				"--perf_output_path="+outPath,
				"--test_data_path="+testDir+"/",
				"--jpeg_filenames="+item.TestFile),
			gtest.UID(int(sysutil.ChronosUID)),
		))
	if err != nil {
		s.Fatalf("Failed to measure CPU usage %v: %v", exec, err)
	}
	cpuUsage := measurements["cpu"]
	duration := time.Since(startTime)

	// Check the total decoding time is longer than the measure duration. If not,
	// the measured CPU usage is inaccurate and we should fail this test.
	if duration < measureDuration {
		s.Fatal("Decoder did not run long enough for measuring CPU usage")
	}

	// Parse the log file for the decode latency measured by the unittest.
	out, err := ioutil.ReadFile(outPath)
	if err != nil {
		s.Fatal("Failed to read output file: ", err)
	}
	var metrics map[string]float64
	if err := json.Unmarshal(out, &metrics); err != nil {
		s.Fatal("Failed to parse output file: ", err)
	}

	return cpuUsage, metrics
}
