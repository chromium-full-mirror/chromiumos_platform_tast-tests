// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/power/suspend"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Suspend,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Simple, single-cycle Suspend and Resume",
		Contacts: []string{
			"chromeos-platform-power@google.com",
		},
		BugComponent: "b:1361410",
		Attr:         []string{"group:mainline", "informational"},
		Timeout:      4 * time.Minute,
	})
}

// Suspend suspends the DUT and wakes again. If the suspend fails, an
// error is returned. If the resume fails, the DUT may stay suspended
// indefinitely, causing the test infrastucture to mark the test as failed.
// TODO: add different kinds of suspend test, including stress test
func Suspend(ctx context.Context, s *testing.State) {
	_, err := suspend.ForDuration(ctx, 10*time.Second)
	if err != nil {
		s.Fatal("Failed to suspend: ", err)
	}

	if err := shill.WaitForOnlineAfterResume(ctx); err != nil {
		s.Fatal("Network failed to comeback after resuming: ", err)
	}
}
