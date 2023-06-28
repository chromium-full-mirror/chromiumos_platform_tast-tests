// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	arcmultidisplay "go.chromium.org/tast-tests/cros/local/bundles/cros/arc/multidisplay"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/arc/wm"
	"go.chromium.org/tast-tests/cros/local/chrome"
	virtualmultidisplay "go.chromium.org/tast-tests/cros/local/virtualmultidisplay"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         MultiDisplayVirtual,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Mutli-display ARC window management tests",
		Contacts:     []string{"arc-framework+tast@google.com", "ruanc@chromium.org", "niwa@chromium.org", "brpol@google.com"},
		// ChromeOS > Software > ARC++ > Framework > Window Management
		BugComponent: "b:537272",
		SoftwareDeps: []string{"chrome", "virtual_multidisplay", "android_vm"},
		Timeout:      arc.BootTimeout + 2*time.Minute,
		Data:         []string{wm.WhiteWallpaperFileName},
		Fixture:      arc.ArcBootedMultiDisplay,
		Params: []testing.Param{
			{
				Val:       arcmultidisplay.StableTestSet,
				ExtraAttr: []string{"group:mainline", "informational", "group:hw_agnostic"},
			},
			{
				Name:      "android_vm_only",
				Val:       arcmultidisplay.AndroidVM,
				ExtraAttr: []string{"group:mainline", "informational", "group:hw_agnostic"},
			},
		},
	})

}

// MultiDisplayVirtual test uses a virtual display driver to emulate displays for running on betty devices.
func MultiDisplayVirtual(ctx context.Context, s *testing.State) {
	displayController := s.FixtValue().(chrome.HasParentState).ParentState().(virtualmultidisplay.HasVirtualDisplayController).VirtualDisplayController()
	arcmultidisplay.SharedVirtualPhysical(ctx, s, displayController)
}
