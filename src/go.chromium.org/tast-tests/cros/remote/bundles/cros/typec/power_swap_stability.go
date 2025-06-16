// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package typec

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/typecutils"
	"go.chromium.org/tast-tests/cros/common/usbutils/unigraf"
	"go.chromium.org/tast-tests/cros/remote/typec/typecunigraf"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: PowerSwapStability,
		Desc: "Check power swap stability on a typec port",
		// ChromeOS > Platform > Technologies > USB
		BugComponent: "b:958036",
		Fixture:      "typecUnigraf",
		Contacts:     []string{"chromeos-usb-champs@google.com", "danielgeorgem@google.com"},
		Attr:         []string{"group:typec", "typec_unigraf274", "typec_informational"},
	})
}

func PowerSwapStability(ctx context.Context, s *testing.State) {
	numIterations := 30
	dutTestPortID := 1

	// Get Unigraf controller from fixture.
	fixtData, ok := s.FixtValue().(*typecunigraf.FixtureData)
	if !ok {
		s.Fatal("Failed to get Unigraf controller from fixture")
	}
	unigrafctl := fixtData.Unigraf

	if err := unigrafctl.SetTestPort(ctx, 0); err != nil {
		s.Fatal("Failed to set testing port: ", err)
	}
	s.Log("Unigraf testing port was set to port 0")

	// We want to do the toggle operation a bunch of time.
	for i := 0; i < numIterations; i++ {
		prevRole, err := unigrafctl.PowerRole(ctx)
		if err != nil {
			s.Fatal("Failed to get power role: ", err)
		}

		newRole := unigraf.PowerRoleSnk
		if prevRole == unigraf.PowerRoleSnk {
			newRole = unigraf.PowerRoleSrc
		}
		s.Logf("Unigraf power role is %s, switch to %s", prevRole.String(), newRole.String())

		if err := unigrafctl.SetPowerRole(ctx, newRole); err != nil {
			s.Fatalf("Failed to set power role to %s, err=%v", newRole, err)
		}

		// Verify the new power state on the dut.
		pollParams := testing.PollOptions{Interval: time.Second, Timeout: 10 * time.Second}
		pollFunc := func(ctx context.Context) error {
			// We set the unigraf to newRole, so after the swap the DUT should have
			// the role of the unigraf before the swap.
			return typecutils.CheckPowerRole(ctx, s.DUT(), prevRole.String(), dutTestPortID)
		}
		if err := testing.Poll(ctx, pollFunc, &pollParams); err != nil {
			s.Fatalf("Power role on the DUT is not %s", newRole.String())
		}

		s.Logf("Iter (%d/%d) OK", i+1, numIterations)

		// GoBigSleepLint: if we try to switch again too fast the unigraf might fail.
		testing.Sleep(ctx, time.Second)
	}
}
