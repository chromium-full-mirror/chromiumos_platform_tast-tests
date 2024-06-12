// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/camera/hal3"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         HAL3RotateCrop,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies Rotate and Crop stream manipulator with HAL3 interface",
		Contacts:     []string{"chromeos-camera-eng@google.com", "yerlandinata@chromium.org", "kamesan@chromium.org"},
		BugComponent: "b:167281", // ChromeOS > Platform > Technologies > Camera
		Attr:         []string{"group:mainline", "informational", "group:camera-libcamera"},
		SoftwareDeps: []string{"arc", "arc_camera3", "chrome", "no_android_p", "no_android_r", caps.BuiltinCamera},
		Fixture:      "chromeLoggedIn",
		// Some devices support very long list of resolutions.
		Timeout: 5*time.Minute + hal3.AdditionalTimeout,
	})
}

func HAL3RotateCrop(ctx context.Context, s *testing.State) {
	if err := hal3.RunTest(ctx, hal3.RotateCropTestConfig()); err != nil {
		s.Error("Test failed: ", err)
	}
}
