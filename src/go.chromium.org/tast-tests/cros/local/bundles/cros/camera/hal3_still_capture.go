// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/camera/hal3"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         HAL3StillCapture,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies camera still capture function with HAL3 interface",
		Contacts:     []string{"chromeos-camera-eng@google.com", "hywu@chromium.org", "shik@chromium.org"},
		Attr:         []string{"group:mainline", "informational", "group:camera-libcamera"},
		SoftwareDeps: []string{"arc", "arc_camera3", "chrome", caps.BuiltinCamera},
		Pre:          chrome.LoggedIn(),
		// Krane needs 4 minutes and 30 seconds for whole dark environment(covering the camera lens).
		// We also need rooms for preparation time.
		Timeout:      6*time.Minute + hal3.AdditionalTimeout,
		BugComponent: "b:167281",
	})
}

func HAL3StillCapture(ctx context.Context, s *testing.State) {
	if err := hal3.RunTest(ctx, hal3.StillCaptureTestConfig()); err != nil {
		s.Error("Test failed: ", err)
	}
}
