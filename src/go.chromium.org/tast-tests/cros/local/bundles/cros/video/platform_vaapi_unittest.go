// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package video

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/gtest"
	"go.chromium.org/tast-tests/cros/local/sysutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: PlatformVAAPIUnittest,
		Desc: "Runs test_va_api, a shallow libva API test",
		Contacts: []string{
			"chromeos-gfx-video@google.com",
			"stevecho@chromium.org",
		},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video
		Attr:         []string{"group:graphics", "graphics_video", "graphics_weekly"},
		SoftwareDeps: []string{"vaapi"},
		Fixture:      "gpuWatchHangs",
		Timeout:      120 * time.Minute,
		// TODO(b/191801955): Reenable on grunt when it stops hanging forever.
		HardwareDeps: hwdep.D(hwdep.SkipGPUFamily("stoney")),
	})
}

// PlatformVAAPIUnittest runs the "test_va_api" GTtest binary
// from the libva-test package,
// see https://github.com/intel/libva-utils.
func PlatformVAAPIUnittest(ctx context.Context, s *testing.State) {
	const exec = "test_va_api"
	if report, err := gtest.New(exec,
		gtest.Logfile(filepath.Join(s.OutDir(), exec+".log")),
		gtest.UID(int(sysutil.ChronosUID)),
	).Run(ctx); err != nil {
		s.Errorf("%v failed: %v", exec, err)
		if report != nil {
			for _, name := range report.FailedTestNames() {
				s.Error(name, " failed")
			}
		} else {
			s.Error("No additional information is available for this failure")
		}
	}
}
