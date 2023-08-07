// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package video

import (
	"context"
	"path/filepath"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/gtest"
	"go.chromium.org/tast-tests/cros/local/sysutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: V4L2Unittest,
		Desc: "Runs V4l2 unit tests",
		Contacts: []string{
			"chromeos-gfx-video@google.com",
			"bchoobineh@google.com",
		},
		SoftwareDeps: []string{"v4l2_codec"},
		HardwareDeps: hwdep.D(hwdep.SkipGPUFamily("rogue")), // MTK8173
		BugComponent: "b:168352",
		Attr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
		Fixture:      "graphicsNoChrome",
	})
}

// V4L2Unittest runs a set of tests which compares MiniGBM allocated
// buffers and decode device allocated buffers.
func V4L2Unittest(ctx context.Context, s *testing.State) {
	const exec = "v4l2_unittest"
	if report, err := gtest.New(
		filepath.Join(chrome.BinTestDir, exec),
		gtest.Logfile(filepath.Join(s.OutDir(), exec+".log")),
		gtest.UID(int(sysutil.ChronosUID)),
	).Run(ctx); err != nil {
		s.Errorf("Failed to run %v: %v", exec, err)
		if report != nil {
			for _, name := range report.FailedTestNames() {
				s.Error(name, " failed")
			}
		} else {
			s.Error("No additional information is available for this failure")
		}
	}
}
