// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/camera/testutil"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/gtest"
	"go.chromium.org/tast-tests/cros/local/power/suspend"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

// TODO(b/324533891): This model is skipped in power.Suspend.
// Remove it when the bug is closed.
var unstableModel = []string{"sona"}

func init() {
	testing.AddTest(&testing.Test{
		Func:         Suspend,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies the camera stack works after a suspend",
		Contacts: []string{
			"chromeos-camera-eng@google.com",
			"ribalda@chromium.org",
		},
		BugComponent: "b:167281", // ChromeOS > Platform > Technologies > Camera
		Attr:         []string{"group:mainline", "informational", "group:camera-libcamera", "group:cq-medium", "group:camera-stability"},
		Timeout:      4 * time.Minute,
		SoftwareDeps: []string{"arc_camera3", "chrome", caps.BuiltinCamera},
		HardwareDeps: hwdep.D(hwdep.CameraEnumerated(), hwdep.SkipOnModel(unstableModel...)),
	})
}

func Suspend(ctx context.Context, s *testing.State) {
	// TODO(b/151270948): Temporarily disable ARC.
	// The cros-camera service would kill itself when running the test if
	// arc_setup.cc is triggered at that time, which will fail the test.
	cr, err := chrome.New(ctx, chrome.ARCDisabled(), chrome.NoLogin())
	if err != nil {
		s.Fatal("Failed to start chrome: ", err)
	}
	defer cr.Close(ctx)

	// Leave some time for cr.Close.
	ctx, cancel := ctxutil.Shorten(ctx, chrome.ResetTimeout)
	defer cancel()

	if err := upstart.EnsureJobRunning(ctx, "powerd"); err != nil {
		s.Fatal("Failed to make powerd running: ", err)
	}

	if err := testutil.WaitForCameraServiceBinding(ctx); err != nil {
		s.Fatal("Failed to wait for Camera Service Binding before suspend: ", err)
	}

	t := gtest.New("cros_camera_connector_test",
		gtest.Logfile(filepath.Join(s.OutDir(), "gtest-before.log")),
		gtest.Filter("ConnectorTest/CaptureTest.OneFrame/NV12_640x480_30fps"))
	if _, err := t.Run(ctx); err != nil {
		s.Fatal("Failed to use camera before suspend: ", err)
	}

	if _, err := suspend.ForDurationWithKernelFreezeTimeout(ctx, 10*time.Second, 8*time.Second); err != nil {
		s.Fatal("Failed to suspend: ", err)
	}

	if err := testutil.WaitForCameraServiceBinding(ctx); err != nil {
		s.Fatal("Failed to wait for Camera Service Binding after suspend: ", err)
	}

	t = gtest.New("cros_camera_connector_test",
		gtest.Logfile(filepath.Join(s.OutDir(), "gtest-after.log")),
		gtest.Filter("ConnectorTest/CaptureTest.OneFrame/NV12_640x480_30fps"))
	if _, err := t.Run(ctx); err != nil {
		s.Fatal("Failed to use camera after suspend: ", err)
	}
}
