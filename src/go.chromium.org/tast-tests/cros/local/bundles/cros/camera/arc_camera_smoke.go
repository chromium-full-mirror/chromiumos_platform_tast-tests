// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ARCCameraSmoke,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks if camera can be probed in ARC",
		Contacts:     []string{"chromeos-camera-eng@google.com", "seannli@google.com"},
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome", caps.BuiltinOrVividCamera},
		Fixture:      "arcWithWorkingCamera",
		BugComponent: "b:978428",
	})
}

func ARCCameraSmoke(ctx context.Context, s *testing.State) {}
