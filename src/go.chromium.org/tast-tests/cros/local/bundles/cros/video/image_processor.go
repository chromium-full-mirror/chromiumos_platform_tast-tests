// Copyright 2022 The ChromiumOS Authors
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

const imageProcessorUnitTestBin = "image_processor_test"

func init() {
	testing.AddTest(&testing.Test{
		Func: ImageProcessor,
		Desc: "Runs ImageProcessor unit tests",
		Contacts: []string{
			"bchoobineh@google.com",
			"chromeos-gfx-video@google.com",
			"nhebert@chromium.org", // Test author
		},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video
		Attr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
		Fixture:      "graphicsNoChrome",
		Params: []testing.Param{
			{
				Name:    "image_processor_unit_test",
				Timeout: 5 * time.Minute,
				ExtraData: []string{
					"images/bear_192x320_270.nv12.yuv",
					"images/bear_192x320_270.nv12.yuv.json",
					"images/bear_192x320_90.nv12.yuv",
					"images/bear_192x320_90.nv12.yuv.json",
					"images/bear_320x192_180.nv12.yuv",
					"images/bear_320x192_180.nv12.yuv.json",
					"images/bear_320x192.bgra",
					"images/bear_320x192.bgra.json",
					"images/bear_320x192.i420.yuv",
					"images/bear_320x192.i420.yuv.json",
					"images/bear_320x192.i422.yuv",
					"images/bear_320x192.i422.yuv.json",
					"images/bear_320x192.nv12.yuv",
					"images/bear_320x192.nv12.yuv.json",
					"images/bear_320x192.rgba",
					"images/bear_320x192.rgba.json",
					"images/bear_320x192.yuyv.yuv",
					"images/bear_320x192.yuyv.yuv.json",
					"images/bear_320x192.yv12.yuv",
					"images/bear_320x192.yv12.yuv.json",
					"images/puppets-1280x720.nv12.yuv",
					"images/puppets-1280x720.nv12.yuv.json",
					"images/puppets-320x180.nv12.yuv",
					"images/puppets-320x180.nv12.yuv.json",
					"images/puppets-480x270.i420.yuv",
					"images/puppets-480x270.i420.yuv.json",
					"images/puppets-480x270.mm21.yuv",
					"images/puppets-480x270.mm21.yuv.json",
					"images/puppets-480x270.nv12.yuv",
					"images/puppets-480x270.nv12.yuv.json",
					"images/puppets-640x360.i420.yuv",
					"images/puppets-640x360.i420.yuv.json",
					"images/puppets-640x360.i422.yuv",
					"images/puppets-640x360.i422.yuv.json",
					"images/puppets-640x360_in_640x480.nv12.yuv",
					"images/puppets-640x360_in_640x480.nv12.yuv.json",
					"images/puppets-640x360.nv12.yuv",
					"images/puppets-640x360.nv12.yuv.json",
					"images/puppets-640x360.yuyv.yuv",
					"images/puppets-640x360.yuyv.yuv.json",
				},
				// Note: this regex tells gtest_filter to execute every test except for the Vulkan ones.
				Val: "*-*Vulkan*",
			},
			{
				Name:    "image_processor_unit_test_vulkan",
				Timeout: 5 * time.Minute,
				ExtraData: []string{
					"images/bear_320x192.rgba",
					"images/bear_320x192.rgba.json",
					"images/puppets-480x270.mm21.yuv",
					"images/puppets-480x270.mm21.yuv.json",
					"images/crowd_run_1080x512.mt2t",
					"images/crowd_run_1080x512.mt2t.json",
					"images/crowd_run_1080x512.p010",
					"images/crowd_run_1080x512.p010.json",
				},
				ExtraHardwareDeps: hwdep.D(hwdep.SkipGPUFamily("rogue")),
				Val:               "*Vulkan*-*MT2T*",
			},
			{
				Name:    "image_processor_unit_test_vulkan_mt2t",
				Timeout: 5 * time.Minute,
				ExtraData: []string{
					"images/bear_320x192.rgba",
					"images/bear_320x192.rgba.json",
					"images/puppets-480x270.mm21.yuv",
					"images/puppets-480x270.mm21.yuv.json",
					"images/crowd_run_1080x512.mt2t",
					"images/crowd_run_1080x512.mt2t.json",
					"images/crowd_run_1080x512.p010",
					"images/crowd_run_1080x512.p010.json",
				},
				ExtraHardwareDeps: hwdep.D(hwdep.Supports10BitOverlays(), hwdep.SkipGPUFamily("rogue")),
				Val:               "*Vulkan*MT2T*",
			},
		},
	})
}

// ImageProcessor runs image_processor_test binary and checks for errors.
// For some V4L2 platforms, the GPU is used. For others, libyuv is used.
// TODO(nhebert): Add platform specific ImageProcessor controls.
func ImageProcessor(ctx context.Context, s *testing.State) {
	vl, err := logging.NewVideoLogger()
	if err != nil {
		s.Fatal("Failed to create new video logger: ", err)
	}
	defer vl.Close()

	dataDirectory := filepath.Dir(s.DataPath("images/bear_320x192.rgba"))

	testName := s.Param().(string)

	s.Logf("Running %s", imageProcessorUnitTestBin)

	testArgs := []string{fmt.Sprintf("--source_directory=%s", dataDirectory),
		logging.ChromeVmoduleFlag()}

	gtestFilter := gtest.Filter(testName)

	exec := filepath.Join(chrome.BinTestDir, imageProcessorUnitTestBin)
	logfile := filepath.Join(s.OutDir(),
		fmt.Sprintf("output_%s_%d.txt", filepath.Base(exec), time.Now().Unix()))
	t := gtest.New(exec, gtest.Logfile(logfile),
		gtestFilter,
		gtest.ExtraArgs(testArgs...),
		gtest.UID(int(sysutil.ChronosUID)))

	command, err := t.Args()
	if err != nil {
		s.Fatal("Failed to get gtest args: ", err)
	}
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
