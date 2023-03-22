// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"

	"chromiumos/tast/local/arc"
	"chromiumos/tast/local/bundles/cros/arc/containermount"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ContainerMount,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies mount points' shared flags for ARC",
		Contacts:     []string{"arc-storage@google.com", "youkichihosoi@chromium.org"},
		// ChromeOS > Software > ARC++ > Storage
		BugComponent: "b:516669",
		// TODO(yusukes,ricardoq): ARCVM does not need the test. Remove this once we retire ARC container.
		SoftwareDeps: []string{
			"android_p",
			"chrome",
		},
		Attr:    []string{"group:mainline", "group:hw_agnostic"},
		Fixture: "arcBooted",
	})
}

func ContainerMount(ctx context.Context, s *testing.State) {
	containermount.RunTest(ctx, s, s.FixtValue().(*arc.PreData).ARC)
}
