// Copyright 2019 The ChromiumOS Authors
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
		Func:         HAL3JDA,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies JPEG decode accelerator works in USB HALv3",
		Contacts:     []string{"chromeos-camera-eng@google.com", "beckerh@chromium.org", "shik@chromium.org", "xinggu@chromium.org"},
		BugComponent: "b:167281", // ChromeOS > Platform > Technologies > Camera
		Attr:         []string{"group:mainline", "informational", "group:camera-postsubmit", "group:camera-libcamera", "group:camera_dependent"},
		SoftwareDeps: []string{"arc", "arc_camera3", "chrome", caps.HWDecodeJPEG, caps.BuiltinUSBCamera},
		Fixture:      "chromeLoggedIn",
		Timeout:      4*time.Minute + hal3.AdditionalTimeout,
	})
}

func HAL3JDA(ctx context.Context, s *testing.State) {
	if err := hal3.RunTest(ctx, hal3.JDATestConfig()); err != nil {
		s.Error("Test failed: ", err)
	}
}
