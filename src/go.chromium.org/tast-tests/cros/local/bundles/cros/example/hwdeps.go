// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package example

import (
	"context"

	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         HWDeps,
		Desc:         "Validity check and demonstration of hardware deps feature",
		Contacts:     []string{"tast-core@google.com", "hidehiko@chromium.org"},
		BugComponent: "b:1034522", // ChromeOS > Test > Harness > Tast > Examples
		Attr:         []string{"group:mainline"},
		Params: []testing.Param{{
			ExtraHardwareDeps: hwdep.D(hwdep.Model("eve")),
		}, {
			Name:              "fingerprint",
			ExtraHardwareDeps: hwdep.D(hwdep.Fingerprint()),
		}},
	})
}

func HWDeps(ctx context.Context, s *testing.State) {
	// No errors means the test passed.
	// This test should run only on eve models.
}
