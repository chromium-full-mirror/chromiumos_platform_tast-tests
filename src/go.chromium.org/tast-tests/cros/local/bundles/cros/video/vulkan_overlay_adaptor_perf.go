// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package video

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/graphics"
	"go.chromium.org/tast-tests/cros/local/gtest"
	mediacpu "go.chromium.org/tast-tests/cros/local/media/cpu"
	"go.chromium.org/tast-tests/cros/local/sysutil"
	"go.chromium.org/tast/core/shutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const testBin = "vulkan_overlay_adaptor_test"

func init() {
	testing.AddTest(&testing.Test{
		Func: VulkanOverlayAdaptorPerf,
		Desc: "Runs Vulkan Overlay Adaptor performance tests",
		Contacts: []string{
			"chromeos-gfx-video@google.com",
			"greenjustin@google.com",
		},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video
		Attr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
		Fixture:      "graphicsNoChrome",
		Timeout:      5 * time.Minute,
		HardwareDeps: hwdep.D(hwdep.CPUSocFamily("mediatek"), hwdep.SkipGPUFamily("rogue"), hwdep.Supports10BitOverlays()),
	})
}

func VulkanOverlayAdaptorPerf(ctx context.Context, s *testing.State) {
	exec := filepath.Join(chrome.BinTestDir, testBin)
	logfile := filepath.Join(s.OutDir(),
		fmt.Sprintf("output_%s_%d.txt", filepath.Base(exec), time.Now().Unix()))

	// Filter for perf tests instead of correctness tests.
	gtestFilter := gtest.Filter("*Performance*")
	t := gtest.New(exec, gtest.Logfile(logfile),
		gtestFilter,
		gtest.ExtraArgs("--output_directory="+s.OutDir()),
		gtest.UID(int(sysutil.ChronosUID)))

	command, err := t.Args()
	testing.ContextLogf(ctx, "Running %s", shutil.EscapeSlice(command))
	if report, err := t.Run(ctx); err != nil {
		if report != nil {
			s.Error("Following gTests failed to pass: ", report.FailedTestNames())
		}
	}

	// Run the same test case on repeat for a while and collect CPU and power
	// usage.
	const measureDuration = 5 * time.Second
	p := perf.NewValues()

	// Measure GPU Performance
	// Don't need to manually wait for GPU Counters to finish since
	// media cpu logic below will take twice as long to complete
	// and is blocking.
	go graphics.MeasureGPUCounters(ctx, measureDuration, p)

	measurements, err := mediacpu.MeasureProcessUsage(ctx, measureDuration, mediacpu.WaitProcess, gtest.New(
		exec,
		gtest.Logfile(filepath.Join(s.OutDir(), exec+".log")),
		gtestFilter,
		gtest.ExtraArgs("--output_directory="+s.OutDir()),
	))
	if err != nil {
		s.Error("No additional information is available for this failure")
	}
	p.Set(perf.Metric{
		Name:      "cpu_usage",
		Unit:      "percent",
		Direction: perf.SmallerIsBetter,
	}, measurements["cpu"])

	// Power measurements are not supported on all platforms.
	if power, ok := measurements["power"]; ok {
		p.Set(perf.Metric{
			Name:      "power_consumption",
			Unit:      "watt",
			Direction: perf.SmallerIsBetter,
		}, power)
	}

	logPath := s.OutDir() + "/VulkanOverlayAdaptorTest/Performance/"
	files, err := ioutil.ReadDir(logPath)
	if err != nil {
		s.Error("Failed to read VulkanOverlayAdaptor test result directory: ", err)
	}

	for _, f := range files {
		jsonFile, err := os.Open(logPath + f.Name())
		if err != nil {
			s.Error("Failed to open test json: ", err)
		}

		val, _ := ioutil.ReadAll(jsonFile)
		var result interface{}
		json.Unmarshal([]byte(val), &result)
		data := result.(map[string]interface{})

		jsonName := strings.TrimSuffix(f.Name(), ".json")
		if frames, ok := data["FramesDecoded"]; ok {
			p.Set(perf.Metric{
				Name:      jsonName + "_FramesDecoded",
				Unit:      "frames",
				Direction: perf.BiggerIsBetter,
			}, frames.(float64))
		}

		if fps, ok := data["FramesPerSecond"]; ok {
			p.Set(perf.Metric{
				Name:      jsonName + "_FramesPerSecond",
				Unit:      "frames",
				Direction: perf.BiggerIsBetter,
			}, fps.(float64))
		}

		if duration, ok := data["TotalDurationMs"]; ok {
			p.Set(perf.Metric{
				Name:      jsonName + "_TotalDurationMs",
				Unit:      "ms",
				Direction: perf.SmallerIsBetter,
			}, duration.(float64))
		}
		s.Log(jsonName)
		defer jsonFile.Close()
	}

	if err := p.Save(s.OutDir()); err != nil {
		s.Error("Failed to save CPU Performance metrics")
	}
}
