// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package video

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/gtest"
	mediacpu "go.chromium.org/tast-tests/cros/local/media/cpu"
	"go.chromium.org/tast-tests/cros/local/sysutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ImageProcessorPerf,
		Desc: "Runs ImageProcessorPerf tests",
		Contacts: []string{
			"chromeos-gfx-video@google.com",
			"bchoobineh@google.com",
		},
		SoftwareDeps: []string{"v4l2_codec"},
		HardwareDeps: hwdep.D(hwdep.CPUSocFamily([]string{"mediatek"})),
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video
		Attr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
		Fixture:      "graphicsNoChrome",
	})
}

func ImageProcessorPerf(ctx context.Context, s *testing.State) {
	const exec = "image_processor_perf_test"

	if report, err := gtest.New(
		filepath.Join(chrome.BinTestDir, exec),
		gtest.Logfile(filepath.Join(s.OutDir(), exec+".log")),
		gtest.UID(int(sysutil.ChronosUID)),
	).Run(ctx); err != nil {
		s.Errorf("Failed to run %v: %v", exec, err)
		if report != nil {
			s.Error("Following gTests failed to pass: ", report.FailedTestNames())
		}
	}

	// Run the same test case on repeat for a while and collect CPU and power
	// usage.
	const measureDuration = 5 * time.Second
	const cleanupTime = 5 * time.Second
	p := perf.NewValues()
	measurements, err := mediacpu.MeasureProcessUsage(ctx, measureDuration, mediacpu.KillProcess, gtest.New(
		filepath.Join(chrome.BinTestDir, exec),
		gtest.Logfile(filepath.Join(s.OutDir(), exec+".log")),
	))
	if err != nil {
		s.Error("No additional information is available for this failure")
	}
	p.Set(perf.Metric{
		Name:      "ImageProcPerf" + ".cpu_usage",
		Unit:      "percent",
		Direction: perf.SmallerIsBetter,
	}, measurements["cpu"])

	// Power measurements are not supported on all platforms.
	if power, ok := measurements["power"]; ok {
		p.Set(perf.Metric{
			Name:      "ImageProcPerf" + ".power_consumption",
			Unit:      "watt",
			Direction: perf.SmallerIsBetter,
		}, power)
	}

	if err := p.Save(s.OutDir()); err != nil {
		s.Error("Failed to save CPU Performance metrics")
	}

}
