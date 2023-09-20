// Copyright 2020 The ChromiumOS Authors
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
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Connector,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies the camera service connector library works",
		Contacts:     []string{"chromeos-camera-eng@google.com", "shik@chromium.org"},
		BugComponent: "b:167281", // ChromeOS > Platform > Technologies > Camera
		Attr:         []string{"group:mainline", "informational", "group:camera-libcamera"},
		SoftwareDeps: []string{"arc_camera3", "chrome", caps.BuiltinOrVividCamera},
	})
}

func Connector(ctx context.Context, s *testing.State) {
	const exec = "cros_camera_connector_test"

	// TODO(b/151270948): Temporarily disable ARC.
	// The cros-camera service would kill itself when running the test if
	// arc_setup.cc is triggered at that time, which will fail the test.
	cr, err := chrome.New(ctx, chrome.ARCDisabled(), chrome.NoLogin())
	if err != nil {
		s.Fatal("Failed to start chrome: ", err)
	}
	defer cr.Close(ctx)

	// Leave some time for cr.Close.
	ctx, cancel := ctxutil.Shorten(ctx, time.Second)
	defer cancel()

	err = testutil.WaitForCameraSocket(ctx)
	if err != nil {
		s.Fatal("Failed to wait for Camera Socket: ", err)
	}

	t := gtest.New(exec, gtest.Logfile(filepath.Join(s.OutDir(), "gtest.log")))

	if report, err := t.Run(ctx); err != nil {
		if report != nil {
			for _, name := range report.FailedTestNames() {
				s.Error(name, " failed")
			}
		}
		s.Errorf("Failed to run %v: %v", exec, err)
	}
}
