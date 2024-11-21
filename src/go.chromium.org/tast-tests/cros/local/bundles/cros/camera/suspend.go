// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/camera/testutil"
	"go.chromium.org/tast-tests/cros/local/gtest"
	"go.chromium.org/tast-tests/cros/local/power/suspend"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

// TODO(b/324533891): "sona" is skipped in power.Suspend. Remove when the bug is closed.
var unstableSuspendModel = []string{"sona"}
var flakyModel = append(unstableSuspendModel, testutil.FlakyModel...)

func init() {
	testing.AddTest(&testing.Test{
		Func: Suspend,
		Desc: "Verifies the camera stack works after a suspend",
		Contacts: []string{
			"chromeos-camera-eng@google.com",
			"ribalda@chromium.org",
		},
		BugComponent: "b:167281", // ChromeOS > Platform > Technologies > Camera
		Attr:         []string{"group:mainline", "group:camera-libcamera", "group:cq-medium", "group:camera-stability", "group:camera", "camera_service", "camera_functional"},
		Timeout:      4 * time.Minute,
		SoftwareDeps: []string{"arc_camera3", "chrome", caps.BuiltinCamera},
		Fixture:      fixture.CameraConnectorRestarted,
		Params: []testing.Param{
			{
				ExtraHardwareDeps: hwdep.D(hwdep.CameraEnumerated(), hwdep.SkipOnModel(flakyModel...), hwdep.SkipOnCameraUSBModule(testutil.FlakyUSBCamera...)),
				ExtraAttr:         []string{"informational"},
			}, {
				Name:              "flaky_model",
				ExtraHardwareDeps: hwdep.D(hwdep.Model(flakyModel...)),
				ExtraAttr:         []string{"informational"},
			}, {
				Name:              "flaky_camera",
				ExtraHardwareDeps: hwdep.D(hwdep.CameraUSBModule(testutil.FlakyUSBCamera...)),
				ExtraAttr:         []string{"informational"},
			},
		},
	})
}

func Suspend(ctx context.Context, s *testing.State) {
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
