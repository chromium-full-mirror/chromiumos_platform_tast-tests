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
		Func:         HAL3Perf,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Measures camera HAL3 performance",
		Contacts:     []string{"chromeos-camera-eng@google.com", "hywu@chromium.org", "shik@chromium.org"},
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		SoftwareDeps: []string{"arc", "arc_camera3", "chrome", caps.BuiltinCamera},
		Pre:          chrome.LoggedIn(),
		Timeout:      4*time.Minute + hal3.AdditionalTimeout,
		BugComponent: "b:167281",
	})
}

func HAL3Perf(ctx context.Context, s *testing.State) {
	if err := hal3.RunTest(ctx, hal3.PerfTestConfig()); err != nil {
		s.Error("Test failed: ", err)
	}
}
