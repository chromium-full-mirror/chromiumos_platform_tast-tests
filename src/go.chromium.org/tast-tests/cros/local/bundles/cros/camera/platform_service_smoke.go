// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"path/filepath"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/gtest"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

// TODO(b/331445568): Skip until we solve the flakiness. Remove when resolved.
var flakyModel = []string{"homestar"}

// TODO(b/243048705): skip the test on faulty flash. Remove when resolved.
var flakyCamera1 = []string{"0408:3028", "0408:4021", "05c8:03f4"}

// TODO(b/340123520): skip the test on flaky camera. Remove when resolved.
var flakyCamera2 = []string{"13d3:56ec"}

var flakyCamera = append(flakyCamera1, flakyCamera2...)

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
				ExtraHardwareDeps: hwdep.D(hwdep.CameraEnumerated(), hwdep.SkipOnModel(flakyModel...), hwdep.SkipOnCameraUSBModule(flakyCamera...)),
			}, {
				Name:              "flaky_model",
				ExtraHardwareDeps: hwdep.D(hwdep.Model(flakyModel...)),
				ExtraAttr:         []string{"informational"},
			}, {
				Name:              "flaky_camera",
				ExtraHardwareDeps: hwdep.D(hwdep.CameraUSBModule(flakyCamera...)),
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
