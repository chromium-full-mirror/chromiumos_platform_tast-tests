// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/utils"
	"go.chromium.org/tast/core/testing"
)

const (
	// BID value for TEST
	testBIDRollbackType = 0x54455354
	// Test images have some flags. Set the CHIP flags to 0 to ensure BID mismatch.
	testBIDRollbackFlags = 0
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    GSCBIDMismatchRollback,
		Desc:    "Verify GSC RW triggers a rollback or RO rejects the image when there's a BID mismatch",
		Timeout: 7 * time.Minute,
		Contacts: []string{
			"cros-hwsec@google.com",
			"mruthven@chromium.org",
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr: []string{"group:gsc",
			"gsc_image_ti50",
			"gsc_nightly"},
		Params: []testing.Param{{
			Name:      "rw",
			Val:       true,
			ExtraAttr: []string{"gsc_h1_shield"},
		}, {
			Name:      "ro",
			Val:       false,
			ExtraAttr: []string{"gsc_dt_shield"},
		}},
		Fixture: fixture.GSCInitialFactory,
	})
}

// GSCBIDMismatchRollback verifies GSC RW triggers a rollback or RO rejects the
// image when there's a BID mismatch
// Flash a DBG image that won't enforce BID checks. Then flash an image with a
// board id mismatch. Verify RW triggers a rollback or RO rejects the image.
func GSCBIDMismatchRollback(ctx context.Context, s *testing.State) {
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	b := utils.NewDevboardHelper(s, s.FixtValue().(*fixture.Value))
	i := ti50.MustOpenCrOSImage(ctx, b, s, b.TestbedType)
	defer i.Close(ctx)

	f := s.FixtValue().(*fixture.Value)

	rwEnforcement := s.Param().(bool)
	s.Logf("RW enforcement %t", rwEnforcement)
	bidDesc := fmt.Sprintf("%x:%x", testBIDRollbackType, testBIDRollbackFlags)

	debugLowVerImage, err := f.DebugLowVerImagePath(ctx)
	if err != nil {
		s.Fatal("Failed to find debug low version image")
	}
	efiImage, err := f.EfiImagePath(ctx)
	if err != nil {
		s.Fatal("Failed to find EFI image")
	}
	debugImage, err := f.DebugImagePath(ctx)
	if err != nil {
		s.Fatal("Failed to find debug image")
	}
	imageUnderTest := f.ImagePath

	_, lowDebugVer, _, _, err := b.GSCToolBinVersion(ctx, debugLowVerImage)
	th.MustSucceed(err, "failed to parse low debug image version")

	// Connect Suzyq, so the test can update over ccd.
	s.Log("Enabling CCD mode and resetting")
	b.ResetWithStraps(ctx, ti50.CCDModeOn)
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")
	version, err := i.VersionInfo(ctx)
	th.MustSucceed(err, "failed to get version")
	s.Logf("RW_A: %+v", version.RwA)
	s.Logf("RW_B: %+v", version.RwB)
	s.Logf("Image BID: %+v", version.BID)
	if version.BID.Flags == 0 {
		s.Fatal("Image Under Test image is not board id locked")
	}

	b.WaitUntilCCDConnected(ctx)

	err = b.RollbackUpdate(ctx, i, debugLowVerImage, debugImage, true)
	th.MustSucceed(err, "failed to rollback to the low version debug image")

	version, err = i.VersionInfo(ctx)
	th.MustSucceed(err, "failed to get version")
	s.Logf("RW_A: %+v", version.RwA)
	s.Logf("RW_B: %+v", version.RwB)
	startVersion := version.ActiveRw().Version
	if lowDebugVer.String() != startVersion {
		s.Fatal("Did not update to DBG image with the low version")
	}

	tpm := b.ResetAndTpmStartup(ctx, i, ti50.FfClamshell, ti50.CCDModeOn)
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC failed to boot")
	b.WaitUntilCCDConnected(ctx)

	bid, err := i.ChipBID(ctx)
	th.MustSucceed(err, "failed to get board id")
	s.Log("Got board id")
	if !bid.IsErased {
		s.Fatal("Board ID is set")
	}
	s.Logf("BID: %+v", bid)

	// Make sure to erase the Chip BID
	defer b.RollbackAndRunEraseFlashInfoUpdate(ctx, i, imageUnderTest, efiImage, debugImage)

	err = tpm.TpmvSetBoardID(testBIDRollbackType, testBIDRollbackFlags)
	th.MustSucceed(err, "failed to set board id to %s", bidDesc)

	bid, err = i.ChipBID(ctx)
	th.MustSucceed(err, "failed to get board id")
	s.Logf("BID: %+v", bid)
	if bid.Type != testBIDRollbackType || bid.Flags != testBIDRollbackFlags {
		s.Fatalf("Incorrect BID expected %s got %+v", bidDesc, bid)
	}

	// GSC should accept the update, but it won't jump to the image. The
	// board id locked locked images are not signed with valid keys.
	err = b.GSCToolUpdateSkipBidCheck(ctx, i, imageUnderTest)
	th.MustSucceed(err, "failed to run image under test update")

	// TODO check the GSC console output for a BID mismatch error
	testing.Sleep(ctx, 15*time.Second) // GoBigSleepLint: wait for GSC to pickup the update
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")

	version, err = i.VersionInfo(ctx)
	if err != nil {
		s.Error("Failed to get version")
	} else {
		s.Logf("RW_A: %+v", version.RwA)
		s.Logf("RW_B: %+v", version.RwB)
		if startVersion != version.ActiveRw().Version {
			s.Error("Did not jump back to DBG image")
		}
	}
	sysinfo, err := i.Sysinfo(ctx)
	if err != nil {
		s.Error("Failed to get sysinfo")
	} else {
		if rwEnforcement {
			if !sysinfo.RollbackDetected {
				s.Error("RW did not trigger rollback")
			}
		} else if sysinfo.RollbackDetected {
			// RO should just chose the image without a BID mismatch
			// It shouldn't trigger rollback
			s.Error("Unexpected with RO enforcement: Reset count triggered rollback")
		}
	}
}
