// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package graphics

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/cpu"
	"go.chromium.org/tast-tests/cros/local/graphics"
	"go.chromium.org/tast-tests/cros/local/screenshot"
	"go.chromium.org/tast/core/testing"
)

const (
	// Time to allow fishes to run before recording metrics.
	runFishesFor = 30 * time.Second
	// The time to wait just after stating to play the aquarium so that CPU usage gets stable.
	stabilizationDuration = 5 * time.Second
)

var (
	commandsMap = map[string]string{
		"avg_fps":             "g_crosFpsCounter.getAvgFps()",
		"avg_interframe_time": "g_crosFpsCounter.getAvgInterFrameTime()",
		"avg_render_time_":    "g_crosFpsCounter.getAvgRenderTime()",
		"std_interframe_time": "g_crosFpsCounter.getStdInterFrameTime()",
	}
	fishSettings = map[int][]string{
		50:   {"'setSetting2'", "2"},
		1000: {"'setSetting6'", "6"},
	}
)

type aquariumParamData struct {
	fishCount int
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         WebGLAquarium,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Runs WebGL aquarium demo from a local build and reports metrics",
		Contacts: []string{
			"chromeos-gfx@google.com",
			"syedfaaiz@google.com",
		},
		// ChromeOS > Platform > Graphics > GPU
		BugComponent: "b:995569",
		Attr:         []string{"graphics_perbuild", "group:graphics", "group:mainline", "informational"},
		Timeout:      7 * time.Minute,
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{{
			Name:    "50_fishes",
			Fixture: "chromeGraphicsWebContent.webglaquarium_gl",
			Val: aquariumParamData{
				fishCount: 50,
			},
		}, {
			Name:      "1000_fishes",
			Fixture:   "chromeGraphicsWebContent.webglaquarium_gl",
			ExtraAttr: []string{"group:crosbolt", "crosbolt_fsi_check"},
			Val: aquariumParamData{
				fishCount: 1000,
			},
		}, {
			Name:    "50_fishes_vulkan",
			Fixture: "chromeGraphicsWebContent.webglaquarium_vulkan",
			Val: aquariumParamData{
				fishCount: 50,
			},
		}, {
			Name:      "1000_fishes_vulkan",
			Fixture:   "chromeGraphicsWebContent.webglaquarium_vulkan",
			ExtraAttr: []string{"group:crosbolt", "crosbolt_fsi_check"},
			Val: aquariumParamData{
				fishCount: 1000,
			},
		}},
	})
}

func savePerfVal(number float64, name, unit string, pv *perf.Values) {
	direction := perf.SmallerIsBetter
	if name == "avg_fps" {
		direction = perf.BiggerIsBetter
		unit = "fps"
	}
	pv.Set(perf.Metric{
		Name:      name,
		Unit:      unit,
		Direction: direction,
	}, float64(number))
}

func WebGLAquarium(ctx context.Context, s *testing.State) {
	if _, err := cpu.WaitUntilCoolDown(ctx, cpu.DefaultCoolDownConfig(cpu.CoolDownPreserveUI)); err != nil {
		s.Log("WARNING: Failed to wait until CPU is cooled down: ", err)
	}
	numFish := s.Param().(aquariumParamData).fishCount

	br := s.FixtValue().(chrome.HasChrome).Chrome()
	conn, err := br.NewConnForTarget(ctx, func(t *chrome.Target) bool {
		return strings.HasSuffix(t.URL, "aquarium.html")
	})
	if err != nil {
		s.Fatal("Failed to find aquarium.html: ", err)
	}
	// Dump debug files and take a screenshot if test fails.
	defer func() {
		if !s.HasError() {
			return
		}
		path := filepath.Join(s.OutDir(), "screenshot.png")
		screenshot.CaptureChrome(ctx, br, path)
		graphics.DumpGraphicsDebugFiles(ctx, s.OutDir())
	}()
	if err = conn.WaitForExpr(ctx, "gl !== null"); err != nil {
		s.Fatal("Failed to create gl context: ", err)
	}
	elemID := strings.Replace("document.getElementsById(*)", "*", fishSettings[numFish][0], 1)
	if err = conn.Call(ctx, nil, "setSetting", elemID, fishSettings[numFish][1]); err != nil {
		s.Fatal("Could not get the intrinsic fish set id: ", err)
	}
	if err = conn.Call(ctx, nil, "g_crosFpsCounter.reset"); err != nil {
		s.Fatal("Could not reset the FPS counter: ", err)
	}

	pv := perf.NewValues()
	var gpuErr, cStateErr, cpuErr error
	var wg sync.WaitGroup
	wg.Add(4)
	go func() {
		defer wg.Done()
		// GoBigSleepLint: This is the measurement time.
		testing.Sleep(ctx, runFishesFor)
	}()
	go func() {
		defer wg.Done()
		gpuErr = graphics.MeasureGPUCounters(ctx, runFishesFor, pv)
	}()
	go func() {
		defer wg.Done()
		cStateErr = graphics.MeasurePackageCStateCounters(ctx, runFishesFor, pv)
	}()
	go func() {
		defer wg.Done()
		cpuErr = graphics.MeasureCPUUsageAndPower(ctx, stabilizationDuration, runFishesFor, pv)
	}()
	wg.Wait()

	if gpuErr != nil {
		s.Fatal("Failed to measure GPU counters", gpuErr)
	}
	if cStateErr != nil {
		s.Fatal("Failed to measure Package C-State residency", cStateErr)
	}
	if cpuErr != nil {
		s.Fatal("Failed to measure CPU/Package power", cpuErr)
	}
	defer func() {
		if err := pv.Save(s.OutDir()); err != nil {
			s.Error("Failed to save perf data: ", err)
		}
	}()
	for metricName, evalCommand := range commandsMap {
		var output float64
		if err = conn.Eval(ctx, evalCommand, &output); err != nil {
			s.Fatalf("Failed while fetching values for metric %s : %s", evalCommand, err)
		}
		savePerfVal(output, metricName, "ms", pv)
	}
}
