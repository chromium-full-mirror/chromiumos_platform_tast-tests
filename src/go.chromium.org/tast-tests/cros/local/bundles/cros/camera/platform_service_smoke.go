// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"fmt"
	"path/filepath"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/camera/testutil"
	"go.chromium.org/tast-tests/cros/local/gtest"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type testParam struct {
	Count int
}

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
				// TODO(b/346995892): IPU6 driver in upstream doesn't work with the HAL. Remove once supported.
				ExtraSoftwareDeps: []string{"no_kernel_upstream"},
				ExtraHardwareDeps: hwdep.D(hwdep.CameraEnumerated(), hwdep.SkipOnModel(testutil.FlakyModel...), hwdep.SkipOnCameraUSBModule(testutil.FlakyUSBCamera...)),
				Val:               testParam{Count: 1},
			}, {
				Name:              "twice",
				ExtraHardwareDeps: hwdep.D(hwdep.CameraEnumerated(), hwdep.SkipOnModel(testutil.FlakyModel...), hwdep.SkipOnCameraUSBModule(testutil.FlakyUSBCamera...)),
				Val:               testParam{Count: 2},
				ExtraAttr:         []string{"informational"},
			}, {
				Name:              "flaky_model",
				ExtraHardwareDeps: hwdep.D(hwdep.Model(testutil.FlakyModel...)),
				Val:               testParam{Count: 1},
				ExtraAttr:         []string{"informational"},
			}, {
				Name:              "flaky_camera",
				ExtraHardwareDeps: hwdep.D(hwdep.CameraUSBModule(testutil.FlakyUSBCamera...)),
				Val:               testParam{Count: 1},
				ExtraAttr:         []string{"informational"},
			},
		},
	})
}

func PlatformServiceSmoke(ctx context.Context, s *testing.State) {
	const exec = "cros_camera_connector_test"

	params, ok := s.Param().(testParam)
	if !ok {
		s.Fatal("Failed to parse test param")
	}

	for i := 1; i <= params.Count; i++ {
		logfile := fmt.Sprintf("gtest-%v.log", i)
		t := gtest.New(exec,
			gtest.Logfile(filepath.Join(s.OutDir(), logfile)),
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
}
