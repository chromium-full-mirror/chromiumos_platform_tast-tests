// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/arc/minicontainer"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         MiniContainerState,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies ARC mini container starts right after ChromeOS shows the login screen",
		Contacts:     []string{"arc-core@google.com"},
		// ChromeOS > Software > ARC++ > Core
		BugComponent: "b:488493",
		Attr:         []string{"group:mainline", "informational"},
		// Container-R doesn't support mini-container.
		SoftwareDeps: []string{"android_p", "chrome"},
		Timeout:      chrome.LoginTimeout + arc.BootTimeout + 30*time.Second,
	})
}

func MiniContainerState(ctx context.Context, s *testing.State) {
	minicontainer.RunTest(ctx, s)
}
