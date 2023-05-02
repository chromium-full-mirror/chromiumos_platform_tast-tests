// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"path/filepath"

	"chromiumos/tast/common/media/caps"
	"chromiumos/tast/local/camera/testutil"
	"chromiumos/tast/local/gtest"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Smoke,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Video Pipeline Smoke Test",
		Contacts:     []string{"chromeos-camera-eng@google.com", "hidenorik@chromium.org"},
		Attr:         []string{"group:mainline", "informational", "group:camera-libcamera", "group:criticalstaging", "group:cq-medium"},
		SoftwareDeps: []string{"arc_camera3", caps.BuiltinCamera},
		BugComponent: "b:167281",
	})
}

func Smoke(ctx context.Context, s *testing.State) {
	const exec = "cros_camera_connector_test"

	if err := testutil.WaitForCameraSocket(ctx); err != nil {
		s.Fatal("Failed to wait for Camera Socket: ", err)
	}

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
