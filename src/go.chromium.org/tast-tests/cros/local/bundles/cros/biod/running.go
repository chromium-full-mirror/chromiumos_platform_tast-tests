// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package biod

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: Running,
		Desc: "Checks that biod is running on devices with fingerprint sensor",
		Contacts: []string{
			"chromeos-fingerprint@google.com",
			"tomhughes@chromium.org",
		},
		// ChromeOS > Platform > Services > Fingerprint
		BugComponent: "b:782045",
		Attr:         []string{"group:fingerprint-cq"},
		SoftwareDeps: []string{"biometrics_daemon"},
		HardwareDeps: hwdep.D(hwdep.Fingerprint()),
		TestBedDeps:  []string{tbdep.Fingerprint},
	})
}

// Running checks the biod job and fails if it isn't running or has a process
// in the zombie state.
func Running(ctx context.Context, s *testing.State) {
	if err := upstart.CheckJob(ctx, "biod"); err != nil {
		s.Fatal("Test failed: ", err)
	}
}
