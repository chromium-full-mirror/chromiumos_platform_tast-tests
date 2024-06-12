// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"path/filepath"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/camera/testutil"
	"go.chromium.org/tast-tests/cros/local/gtest"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PlatformServiceSmoke,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Smoke test for the Platform Camera Service",
		Contacts:     []string{"chromeos-camera-eng@google.com", "hidenorik@chromium.org"},
		BugComponent: "b:167281", // ChromeOS > Platform > Technologies > Camera
		Attr:         []string{"group:mainline", "group:camera-libcamera", "group:cq-medium", "group:camera-stability", "group:camera-kernelnext"},
		SoftwareDeps: []string{"arc_camera3", "chrome", caps.BuiltinCamera},
		Fixture:      fixture.CameraConnectorReady,
		Params: []testing.Param{
			{
				ExtraHardwareDeps: hwdep.D(hwdep.CameraEnumerated(), hwdep.SkipOnModel(testutil.FlakyMIPIModel...), hwdep.SkipOnCameraUSBModule(testutil.FlakyUSBCamera...)),
			}, {
				Name:              "flaky_model",
				ExtraHardwareDeps: hwdep.D(hwdep.Model(testutil.FlakyMIPIModel...)),
				ExtraAttr:         []string{"informational"},
			}, {
				Name:              "flaky_camera",
				ExtraHardwareDeps: hwdep.D(hwdep.CameraUSBModule(testutil.FlakyUSBCamera...)),
				ExtraAttr:         []string{"informational"},
			},
		},
	})
}

func PlatformServiceSmoke(ctx context.Context, s *testing.State) {
	const exec = "cros_camera_connector_test"

	t := gtest.New(exec,
		gtest.Logfile(filepath.Join(s.OutDir(), "gtest.log")),
		gtest.Filter("ConnectorTest/CaptureTest.OneFrame/NV12_640x480_30fps"))

	if report, err := t.Run(ctx); err != nil {
		if report != nil {
			for _, name := range report.FailedTestNames() {
				s.Error(name, " failed")
			}
		}
		s.Errorf("Failed to run %v: %v", exec, err)
	}
}
