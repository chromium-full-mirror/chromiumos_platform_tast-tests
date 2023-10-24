// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/firmware/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ECHash,
		Desc: "Basic check for EC hash validation",
		Contacts: []string{
			"chromeos-faft@google.com",
			"js@semihalf.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		// TODO: When stable, change firmware_unstable to a different attr.
		Attr:         []string{"group:firmware", "firmware_unstable"},
		Fixture:      fixture.NormalMode,
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Timeout:      20 * time.Minute,
		ServiceDeps:  []string{"tast.cros.firmware.BiosService"},
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

// ECHash tries to invalidate the hash of EC firmware and then check
// if it gets recomputed back again on warm reboot to ensure that AP
// correctly measures EC validity
func ECHash(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}

	if err := h.RequireBiosServiceClient(ctx); err != nil {
		s.Fatal("Requiring BiosServiceClient: ", err)
	}

	cleanupContext := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 3*time.Minute)
	defer cancel()
	restore, err := utils.EnableSoftwareSync(ctx, h, false)
	if err != nil {
		if restore != nil {
			s.Log("Failed to clear disable software sync flag: ", err)
			restore(cleanupContext, s)
		} else {
			s.Fatal("Failed to clear disable software sync flag: ", err)
		}
	}
	defer restore(cleanupContext, s)

	ectool := firmware.NewECTool(s.DUT(), firmware.ECToolNameMain)

	s.Log("Reading current EC hash")
	initialECHash, err := ectool.Hash(ctx)
	if err != nil {
		s.Fatal("Failed to get initial ec hash: ", err)
	}
	s.Log("Initial EC hash is: ", initialECHash.Hash)

	// Below this line, the EC_RW contents should be considered as
	// modified and every failure should lead to immediate restore
	// of EC firmware!
	s.Log("Invalidating current EC hash")
	invalidatedECHash, err := ectool.Hash(ctx, "recalc", "0", "4")
	if err != nil {
		s.Fatal("Failed to invalidate current EC hash: ", err)
	}
	s.Log("Invalidated EC hash is: ", invalidatedECHash.Hash)

	if invalidatedECHash.Hash == initialECHash.Hash {
		s.Fatal("Invalidated EC hash is equal to initial EC hash")
	}

	s.Log("Warm rebooting DUT to recalculate EC hash with AP")
	if err := h.DUT.Reboot(ctx); err != nil {
		s.Fatal("Failed rebooting DUT: ", err)
	}
	if err := h.WaitConnect(ctx); err != nil {
		s.Fatal("Failed to wait for DUT to reconncet: ", err)
	}

	s.Log("Reading EC hash after reboot")
	newECHash, err := ectool.Hash(ctx)
	if err != nil {
		s.Fatal("Failed to retrieve current EC hash: ", err)
	}
	s.Log("After a reboot, current EC hash is: ", newECHash.Hash)

	if invalidatedECHash.Hash == newECHash.Hash {
		s.Fatal("New EC hash is equal to invalidated EC hash")
	}

	if initialECHash.Hash != newECHash.Hash {
		s.Fatal("New EC hash does not match initial EC hash")
	}

	s.Log("Current EC hash matches initial EC hash")
}
