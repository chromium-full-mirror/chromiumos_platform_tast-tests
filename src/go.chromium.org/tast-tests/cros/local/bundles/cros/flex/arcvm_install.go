// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package flex

import (
	"context"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ARCVMInstall,
		Desc: "Tests that the ARCVM DLC is automatically installed and activated",
		Contacts: []string{
			"chromeos-flex-eng+oncall@google.com",
			"josephsussman@google.com", // Test author
		},
		BugComponent: "b:998633", // ChromeOS > Platform > Enablement > ChromeOS Flex
		Fixture:      fixture.ChromeEnrolledLoggedInARCFlex,
		HardwareDeps: hwdep.D(hwdep.Model("reven")),
		SoftwareDeps: []string{"chrome"},
		Timeout:      15 * time.Minute,
	})
}

func ARCVMInstall(ctx context.Context, s *testing.State) {
	s.Log("Waiting for Android system image to appear")
	testing.Poll(ctx, func(ctx context.Context) error {
		if _, err := os.Stat("/opt/google/vms/android/system.raw.img"); err == nil {
			return nil
		} else if os.IsNotExist(err) {
			return errors.New("system image is not present")
		} else {
			return errors.Errorf("failed to check system image: %q", err)
		}
	}, &testing.PollOptions{Timeout: 10 * time.Minute})
}
