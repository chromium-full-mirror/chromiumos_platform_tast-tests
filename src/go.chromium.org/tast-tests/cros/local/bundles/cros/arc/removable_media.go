// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/arc/removablemedia"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     RemovableMedia,
		Desc:     "Verifies ARC removable media integration is working",
		Contacts: []string{"arc-storage@google.com", "youkichihosoi@chromium.org", "momohatt@google.com"},
		// ChromeOS > Software > ARC++ > Storage
		BugComponent: "b:516669",
		SoftwareDeps: []string{"chrome"},
		Fixture:      "arcBooted",
		Attr:         []string{"group:mainline", "group:hw_agnostic"},
		Params: []testing.Param{
			{
				Name:              "vm",
				ExtraSoftwareDeps: []string{"android_vm"},
				ExtraHardwareDeps: arc.ArcAppHwDeps,
			}},
	})
}

func RemovableMedia(ctx context.Context, s *testing.State) {
	a := s.FixtValue().(*arc.PreData).ARC
	cr := s.FixtValue().(*arc.PreData).Chrome
	d := s.FixtValue().(*arc.PreData).UIDevice

	removablemedia.RunTest(ctx, s, a, cr, d, "storage.txt")
}
