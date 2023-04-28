// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"time"

	"chromiumos/tast/common/media/caps"
	"chromiumos/tast/local/bundles/cros/camera/hal3"
	"chromiumos/tast/local/chrome"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         HAL3Preview,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies camera preview function with HAL3 interface",
		Contacts:     []string{"chromeos-camera-eng@google.com", "hywu@chromium.org", "shik@chromium.org"},
		Attr:         []string{"group:mainline", "informational", "group:camera-libcamera"},
		SoftwareDeps: []string{"arc", "arc_camera3", "chrome", caps.BuiltinCamera},
		Pre:          chrome.LoggedIn(),
		Timeout:      4 * time.Minute,
		BugComponent: "b:167281",
	})
}

func HAL3Preview(ctx context.Context, s *testing.State) {
	if err := hal3.RunTest(ctx, hal3.PreviewTestConfig()); err != nil {
		s.Error("Test failed: ", err)
	}
}
