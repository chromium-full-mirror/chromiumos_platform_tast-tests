// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package graphics

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"chromiumos/tast/common/testexec"
	"chromiumos/tast/local/graphics/swapbench"
	"chromiumos/tast/testing"
	"chromiumos/tast/testing/hwdep"
)

const (
	// Benchmark name
	benchmarkName = "SwapBench"
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
		SoftwareDeps: []string{"no_qemu", "borealis_host"},
		Fixture:      "graphicsNoChrome",
		Timeout:      5 * time.Minute,
	})
}

func SwapBench(ctx context.Context, s *testing.State) {
	targetRefreshRate, refreshMs, err := swapbench.GetRefreshRateArgs(ctx)
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
	err = swapbench.OutputResultsToJSON(ctx, benchmarkName, s.OutDir(), file)
	if err != nil {
		s.Error("Failed to calculate results-chart.json metrics: ", err)
	}
}
