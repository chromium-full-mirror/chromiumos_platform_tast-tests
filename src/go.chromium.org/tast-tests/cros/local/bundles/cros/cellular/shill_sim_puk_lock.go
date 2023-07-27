// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ShillSimPukLock,
		Desc:         "Verifies that cellular device SIM PUK lock",
		Contacts:     []string{"chromeos-cellular-team@google.com", "srikanthkumar@google.com"},
		BugComponent: "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_sim_pinlock"},
		Fixture:      "cellularSIMLockCleared",
		Timeout:      5 * time.Minute,
	})
}

// ShillSimPukLock tests successfully enabling sim lock and locking the sim with puk-lock.
func ShillSimPukLock(ctx context.Context, s *testing.State) {
	helper := s.FixtValue().(*cellular.FixtData).Helper

	iccid, err := helper.GetCurrentICCID(ctx)
	if err != nil {
		s.Fatal("Could not get current ICCID: ", err)
	}
	currentPin, currentPuk, err := helper.GetPINAndPUKForICCID(ctx, iccid)
	if err != nil {
		s.Fatal("Could not get Pin and Puk : ", err)
	}
	if currentPuk == "" {
		s.Fatal("Unable to find PUK code for ICCID : ", iccid)
	}

	s.Log("Attempting to enable sim pin lock and set in puk lock state")
	if err = helper.PukLockSim(ctx, currentPin); err != nil {
		// Unlock and disable pin lock if failed after locking pin.
		if errNew := helper.ClearSIMLock(ctx, currentPin, currentPuk); errNew != nil {
			s.Log("Failed to clear default pin lock in puklocksim: ", errNew)
		}
		s.Fatal("Failed to enable puk lock: ", err)
	}

	// Shorten deadline to leave time for cleanup
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	defer func(ctx context.Context) {
		// Unlock and disable pin lock.
		if err = helper.ClearSIMLock(ctx, currentPin, currentPuk); err != nil {
			s.Fatal("Failed to clear default pin lock: ", err)
		}
	}(cleanupCtx)

	if retriesLeft, err := helper.GetRetriesLeft(ctx); err != nil {
		s.Fatal("Could not get pin retries left: ", err)
	} else if retriesLeft <= 1 {
		s.Fatal("No retries left to try error state after puk locked")
	}

	// Get default puk, pin from initialize and unlock using puk.
	if err = helper.Device.UnblockPin(ctx, currentPuk, currentPin); err != nil {
		s.Fatal("Could not unlock puk: ", err)
	}

	locked := helper.IsSimPukLocked(ctx)
	if locked {
		s.Fatal("Failed to do puk unlock-manual repair needed on dut: ", err)
	}
	pinLocked := helper.IsSimPinLocked(ctx)
	if pinLocked {
		s.Log("Pin-lock got locked while unlocking the puk-lock: ", err)
	}
	enabled := helper.IsSimLockEnabled(ctx)
	if !enabled {
		s.Fatal("SIM lock got disabled when attemping to unlock a puk-locked sim: ", err)
	}
}
