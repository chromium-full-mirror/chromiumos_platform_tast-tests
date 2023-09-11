// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/arc/multidisplay"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/arc/wm"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         MultiDisplayPhysical,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Mutli-display ARC window management tests",
		Contacts:     []string{"arc-framework+tast@google.com", "ruanc@chromium.org", "niwa@chromium.org", "brpol@google.com"},
		// ChromeOS > Software > ARC++ > Framework > Window Management
		BugComponent: "b:537272",
		Data:         []string{wm.WhiteWallpaperFileName},
		SoftwareDeps: []string{"arc", "chrome"},
		Timeout:      arc.BootTimeout + 2*time.Minute,
		Fixture:      arc.ArcBooted,
		Params: []testing.Param{
			{
				Val: multidisplay.StableTestSet,
			},
			{
				Name:              "android_vm_only",
				ExtraSoftwareDeps: []string{"android_vm"},
				Val:               multidisplay.AndroidVm,
			},
			{
				Name:              "physical_device_only",
				ExtraSoftwareDeps: []string{"android_vm"},
				Val:               multidisplay.PhysicalOnly,
			}},
	})
}

// MultiDisplayPhysical test requires two connected physical displays.
func MultiDisplayPhysical(ctx context.Context, s *testing.State) {
	multidisplay.SharedVirtualPhysical(ctx, s, nil)
}
