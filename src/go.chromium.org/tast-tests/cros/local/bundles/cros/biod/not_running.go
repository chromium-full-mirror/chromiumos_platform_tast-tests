// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package biod

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: NotRunning,
		Desc: "Checks that biod is not running on devices without fingerprint sensor",
		Contacts: []string{
			"chromeos-fingerprint@google.com",
			"fsammoura@google.com",
		},
		// ChromeOS > Platform > Services > Fingerprint
		BugComponent: "b:782045",
		Attr:         []string{"group:mainline", "informational", "group:fingerprint-cq"},
		HardwareDeps: hwdep.D(hwdep.NoFingerprint()),
	})
}

// NotRunning checks the biod job and fails if it is running or has a process
// in the zombie state.
func NotRunning(ctx context.Context, s *testing.State) {
	if err := upstart.CheckJob(ctx, "biod"); err == nil {
		s.Fatal("Test failed: ", err)
	}
}
