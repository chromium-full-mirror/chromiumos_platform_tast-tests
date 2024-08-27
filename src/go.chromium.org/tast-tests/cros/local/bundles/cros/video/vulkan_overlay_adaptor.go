// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package video

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/gtest"
	"go.chromium.org/tast-tests/cros/local/media/logging"
	"go.chromium.org/tast-tests/cros/local/sysutil"
	"go.chromium.org/tast/core/shutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const unitTestBin = "vulkan_overlay_adaptor_test"

func init() {
	testing.AddTest(&testing.Test{
		Func: VulkanOverlayAdaptor,
		Desc: "Runs Vulkan Overlay Adaptor unit tests",
		Contacts: []string{
			"chromeos-gfx-video@google.com",
			"greenjustin@google.com",
		},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video
		Attr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
		Fixture:      "graphicsNoChrome",
		Timeout:      5 * time.Minute,
		Data: []string{
			"images/puppets-480x270.mm21.yuv",
			"images/puppets-480x270.mm21.yuv.json",
			"images/crowd_run_1080x512.mt2t",
			"images/crowd_run_1080x512.mt2t.json",
		},
		HardwareDeps: hwdep.D(hwdep.CPUSocFamily("mediatek"), hwdep.SkipGPUFamily("rogue"), hwdep.Supports10BitOverlays()),
	})
}

func VulkanOverlayAdaptor(ctx context.Context, s *testing.State) {
	vl, err := logging.NewVideoLogger()
	if err != nil {
		s.Fatal("Failed to create new video logger: ", err)
	}
	defer vl.Close()

	dataDirectory := filepath.Dir(s.DataPath("images/puppets-480x270.mm21.yuv"))

	testArgs := []string{fmt.Sprintf("--source_directory=%s", dataDirectory),
		logging.ChromeVmoduleFlag()}

	// Filter for correctness tests instead of perf tests.
	gtestFilter := gtest.Filter("*Correctness*")

	exec := filepath.Join(chrome.BinTestDir, unitTestBin)
	logfile := filepath.Join(s.OutDir(),
		fmt.Sprintf("output_%s_%d.txt", filepath.Base(exec), time.Now().Unix()))
	t := gtest.New(exec, gtest.Logfile(logfile),
		gtestFilter,
		gtest.ExtraArgs(testArgs...),
		gtest.UID(int(sysutil.ChronosUID)))

	command, _ := t.Args()
	testing.ContextLogf(ctx, "Running %s", shutil.EscapeSlice(command))
	if report, err := t.Run(ctx); err != nil {
		if report != nil {
			for _, name := range report.FailedTestNames() {
				s.Error(name, " failed")
			}
		}
		s.Fatalf("Failed to run %v: %v", exec, err)
	}
}
