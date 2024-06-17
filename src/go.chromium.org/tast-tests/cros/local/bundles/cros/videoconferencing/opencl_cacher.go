// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package videoconferencing

import (
	"context"
	"os"
	"time"

	upstartcommon "go.chromium.org/tast-tests/cros/common/upstart"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast-tests/cros/local/videoconferencing/effects"
	"go.chromium.org/tast-tests/cros/local/videoconferencing/fixture"

	"go.chromium.org/tast/core/testing"
)

const dlcCLCacheDir = "/run/imageloader/ml-core-internal/package/root/cl_cache"

func init() {
	testing.AddTest(&testing.Test{
		Func:    OpenCLCacher,
		Desc:    "Validates that the OpenCL Cacher service runs without error",
		Timeout: 5 * time.Minute,
		Contacts: []string{
			"chromeos-platform-ml-accelerators@google.com",
			"jmpollock@google.com",
			"zhaon@google.com",
		},
		BugComponent: "b:1212695",
		Attr: []string{
			"group:mainline",
			"group:video_conference",
			"group:cbx", "cbx_feature_enabled", "cbx_unstable",
			"informational",
		},
		Fixture:      fixture.LoggedInWithFakeHALAndEffectsEnabled,
		SoftwareDeps: []string{"camera_feature_effects"},
	})
}

func OpenCLCacher(ctx context.Context, s *testing.State) {
	s.Log("Clearing cache in: " + effects.OpenCLCacheDir)
	if err := effects.ClearOpenCLCache(); err != nil {
		s.Fatal("Failed to clear opencl_cache: ", err)
	}

	if !(upstart.JobExists(ctx, "opencl-cacher")) {
		s.Fatal("opencl-cacher job does not exist")
	}

	s.Log("Starting opencl-cacher service")
	if err := upstart.EnsureJobRunning(ctx, "opencl-cacher"); err != nil {
		s.Fatal("Failed to start opencl-cacher: ", err)
	}

	if err := upstart.WaitForJobStatus(ctx, "opencl-cacher", upstartcommon.StopGoal, upstartcommon.WaitingState, upstart.TolerateWrongGoal, 30*time.Second); err != nil {
		s.Fatal("Failed: opencl-cacher did not exit cleanly: ", err)
	}
	s.Log("Completed opencl-cacher service")

	// ReadDir sorts directory entries by filename.
	libFiles, err := os.ReadDir(effects.OpenCLCacheDir)
	if err != nil {
		s.Fatal("Failed to read files in "+effects.OpenCLCacheDir+": ", err)
	}

	dlcFiles, err := os.ReadDir(dlcCLCacheDir)
	if err != nil {
		s.Fatal("Failed to read files in "+dlcCLCacheDir+": ", err)
	}

	// Check length
	l, d := len(libFiles), len(dlcFiles)
	if l < d {
		s.Fatal("Mismatched number of entries in opencl_cache and DLC: got ", l, "; want ", d)
	}

	for i, file := range libFiles {
		if dlcFiles[i].Name() != file.Name() {
			s.Fatal("Failed to find DLC file ", dlcFiles[i].Name(), " in opencl_cache")
		}
	}
}
