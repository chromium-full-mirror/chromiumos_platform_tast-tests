// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"time"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/remote/firmware"
	"chromiumos/tast/remote/firmware/fixture"
	pb "chromiumos/tast/services/cros/firmware"
	"chromiumos/tast/ssh"
	"chromiumos/tast/testing"
	"chromiumos/tast/testing/hwdep"
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
		Attr:         []string{"group:firmware", "firmware_unstable"},
		Fixture:      fixture.NormalMode,
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Timeout:      20 * time.Minute,
		ServiceDeps:  []string{"tast.cros.firmware.BiosService"},
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

	s.Log("Backing up current EC_RW region for safety")
	ecPath, err := h.BiosServiceClient.BackupImageSection(ctx, &pb.FWSectionInfo{
		Programmer: pb.Programmer_ECProgrammer,
		Section:    pb.ImageSection_ECRWImageSection,
	})
	if err != nil {
		s.Fatal("Failed to backup current EC_RW region: ", err)
	}
	s.Log("EC_RW region backup is stored at: ", ecPath.Path)

	cleanupContext := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Minute)
	defer cancel()
	defer func(ctx context.Context) {
		h.DisconnectDUT(ctx)
		s.Log("Wait for DUT to reconnect")
		if err := h.EnsureDUTBooted(ctx); err != nil {
			s.Fatal("Failed to ensure the DUT is booted")
		}

		s.Log("Reconnecting to BiosService on DUT")
		if err := h.RequireBiosServiceClient(ctx); err != nil {
			s.Fatal("Failed to reconnect to BiosServiceClient on DUT: ", err)
		}

		s.Log("Restoring EC image")
		if _, err := h.BiosServiceClient.RestoreImageSection(ctx, ecPath); err != nil {
			s.Error("Failed to restore EC image: ", err)
		}
		s.Log("Removing EC image backup from DUT")
		if _, err := h.DUT.Conn().CommandContext(ctx, "rm", ecPath.Path).Output(ssh.DumpLogOnError); err != nil {
			s.Fatal("Failed to delete EC image from DUT: ", err)
		}
	}(cleanupContext)

	flg := pb.GBBFlagsState{Clear: []pb.GBBFlag{pb.GBBFlag_DISABLE_EC_SOFTWARE_SYNC}}
	if _, err := h.BiosServiceClient.ClearAndSetGBBFlags(ctx, &flg); err != nil {
		s.Fatal("Failed clearing DISABLE_EC_SOFTWARE_SYNC GBB flag")
	}

	ectool := firmware.NewECTool(s.DUT(), firmware.ECToolNameMain)

	s.Log("Reading current EC hash")
	initialECHash, err := ectool.Hash(ctx)
	if err != nil {
		s.Fatal("Failed to get initial ec hash: ", err)
	}
	s.Log("Current EC hash is: ", initialECHash.Hash)
	defer func(ctx context.Context) {
		s.Log("Reset EC hash offset and size to initial values and recalculate")
		resetECHash, err := ectool.Hash(ctx, "recalc", initialECHash.Offset, initialECHash.Size)
		if err != nil {
			s.Fatalf("Failed to reset EC hash to initial value of %v, %v", *resetECHash, err)
		}
	}(cleanupContext)

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
	h.DisconnectDUT(ctx)
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
