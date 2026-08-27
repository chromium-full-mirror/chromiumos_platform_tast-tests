// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	oldDTVersion string = "0.23.30"
	oldH1Version string = "0.5.230"

	testInvalidateRW string = "invalidate RW"
	testSecondUpdate string = "second update"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    GSCRollbackBits,
		Desc:    "Verify rollback bits are blown correctly and lockout old images",
		Timeout: 10 * time.Minute,
		Contacts: []string{
			"cros-hwsec@google.com",
			"mruthven@chromium.org",
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr:         []string{"group:gsc", "gsc_image_ti50", "gsc_nightly", "gsc_h1_shield", "gsc_dt_shield"},
		Fixture:      fixture.SystemDevboard,
		Params: []testing.Param{{
			Name: "second_update",
			Val:  testSecondUpdate,
		}, {
			Name: "invalidate_rw",
			Val:  testInvalidateRW,
		}},
	})
}

// GSCRollbackBits requires HW setup with SuzyQ cable from devboard to drone/workstation.
func GSCRollbackBits(ctx context.Context, s *testing.State) {
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	b := utils.NewDevboardHelper(s, s.FixtValue().(*fixture.Value))
	i := ti50.MustOpenCrOSImage(ctx, b, s, b.TestbedType)
	defer i.Close(ctx)

	f := s.FixtValue().(*fixture.Value)
	testType := s.Param().(string)

	var oldVersion string

	switch f.TestbedProperties.TestbedType {
	case ti50.GscH1Shield:
		oldVersion = oldH1Version
	case ti50.GscDTShield:
		oldVersion = oldDTVersion
	default:
		s.Fatalf("Unsupported testbed type %s", f.TestbedProperties.TestbedType)
	}

	// Inform fixture that this test may replace the firmware image in flash.
	th.MustSucceed(f.ImageMayBeUpdatedByTest(), "image may be updated")

	efiImage, err := f.EfiImagePath(ctx)
	if err != nil {
		s.Fatal("Failed to find efi image")
	}
	debugImage, err := f.DebugImagePath(ctx)
	if err != nil {
		s.Fatal("Failed to find debug image")
	}
	imageUnderTest := f.ImagePath

	// Download the old image.
	fwName := fixture.FindFwName(f.TestbedProperties.TestbedType)
	oldImageURL, err := fixture.LookupGSCReleaseTarball(ctx, oldVersion, fwName)
	th.MustSucceed(err, "failed to find old gsc image")

	oldImage, err := fixture.DownloadToTempFile(ctx, "old image", oldImageURL)
	th.MustSucceed(err, "failed to download old gsc image")
	oldImage, err = fixture.ExtractGSCQualImageFromTarball(ctx, oldImage)
	th.MustSucceed(err, "failed to extract old gsc image")

	_, oldVer, _, _, err := b.GSCToolBinVersion(ctx, oldImage)
	th.MustSucceed(err, "failed to parse old image version")
	_, releaseVer, _, _, err := b.GSCToolBinVersion(ctx, imageUnderTest)
	th.MustSucceed(err, "failed to parse image under test version")
	_, debugVer, _, _, err := b.GSCToolBinVersion(ctx, debugImage)
	th.MustSucceed(err, "failed to debug image version")
	_, efiVer, _, _, err := b.GSCToolBinVersion(ctx, efiImage)
	th.MustSucceed(err, "failed to efi image version")

	s.Logf("image under test %s: %s", releaseVer, imageUnderTest)
	s.Logf("old image %s: %s", oldVer, oldImage)
	s.Logf("dbg %s: %s", debugVer, debugImage)
	s.Logf("efi %s: %s", efiVer, efiImage)

	// Connect Suzyq, so the test can update over ccd.
	s.Log("Enabling CCD mode and resetting")
	b.ResetWithStraps(ctx, ti50.CCDModeOn)
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")
	b.WaitUntilCCDConnected(ctx)

	// Erase info1 and rollback to the old image.
	err = b.RollbackAndRunEraseFlashInfoUpdate(ctx, i, oldImage, efiImage, debugImage)
	th.MustSucceed(err, "failed to update to old image")
	s.Log("Updated to the old image")

	version, err := i.VersionInfo(ctx)
	th.MustSucceed(err, "failed to get version")
	s.Log("Version after flashing old image")
	s.Logf("RW_A: %+v", version.RwA)
	s.Logf("RW_B: %+v", version.RwB)

	sysinfo, err := i.Sysinfo(ctx)
	th.MustSucceed(err, "failed to get sysinfo")
	s.Logf("Rollback info running old image %+v", sysinfo.RWRollbackBits)
	oldRollbackBits, err := activeImageRollbackBits(sysinfo.RWRollbackBits, version.RwA.Active)
	th.MustSucceed(err, "failed to get old image rollback bits")

	// Run power-on reset to clear update rate limit
	b.GpioSet(ctx, ti50.GpioTi50ResetL, false)
	b.GpioSet(ctx, ti50.GpioTi50ResetL, true)
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")
	th.MustSucceed(b.GSCToolWaitUntilReady(ctx), "failed to enable ccd after reset")

	// Update from the old image to the image under test
	if err = b.UpdateOnce(ctx, i, imageUnderTest, releaseVer); err != nil {
		s.Fatal("Failed to update to the image under test from the old image: ", err)
	}
	s.Logf("Successfully ran the update from %s to %s", oldVer, releaseVer)

	version, err = i.VersionInfo(ctx)
	th.MustSucceed(err, "failed to get version")
	s.Log("Version after update to release image")
	s.Logf("RW_A: %+v", version.RwA)
	s.Logf("RW_B: %+v", version.RwB)

	sysinfo, err = i.Sysinfo(ctx)
	th.MustSucceed(err, "failed to get sysinfo")
	s.Logf("Rollback info running release image %+v", sysinfo.RWRollbackBits)

	releaseRollbackBits, err := activeImageRollbackBits(sysinfo.RWRollbackBits, version.RwA.Active)
	th.MustSucceed(err, "failed to get release image rollback bits")

	if releaseRollbackBits == oldRollbackBits {
		s.Fatal("The release and old image have the same number of bits blown")
	}

	// For Ti50, only prod signed images should update the rollback bits.
	if sysinfo.ProdKeyladder || f.TestbedProperties.TestbedType == ti50.GscH1Shield {
		if oldRollbackBits != sysinfo.RWRollbackBits.Flash.Bits {
			s.Errorf("Did not erase rollback bits to match previous release. wanted: %d, got: %d", oldRollbackBits, sysinfo.RWRollbackBits.Flash.Bits)
		}
		s.Log("Rollback bits erased to match inactive images correctly")
	} else {
		s.Log("Skipped check for rollback image for non prod images")
	}

	previousFlashRollbackBits := sysinfo.RWRollbackBits.Flash.Bits

	if testType == testSecondUpdate {
		// Run power-on reset to clear update rate limit
		b.GpioSet(ctx, ti50.GpioTi50ResetL, false)
		b.GpioSet(ctx, ti50.GpioTi50ResetL, true)
		th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")
		th.MustSucceed(b.GSCToolWaitUntilReady(ctx), "failed to enable ccd after reset")

		// Update to the release
		if err = b.UpdateOnce(ctx, i, imageUnderTest, releaseVer); err != nil {
			s.Fatal("Failed to update to the image under test from the old image: ", err)
		}
		th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")
		s.Log("Successfully ran the second update")
	} else if testType == testInvalidateRW {
		tpm := b.ResetAndTpmStartup(ctx, i, ti50.CCDModeOn, ti50.FfClamshell)
		err = tpm.TpmvInvalidateInactiveRW()
		th.MustSucceed(err, "failed to send invalidate RW")
	}
	s.Log("Checking rollback bits were blown")

	sysinfo, err = i.Sysinfo(ctx)
	th.MustSucceed(err, "failed to get sysinfo")
	s.Logf("Rollback bits %+v", sysinfo.RWRollbackBits)

	// For Ti50, only prod signed images should update the rollback bits. If this
	// is a dev image, verify that rollback bits were not updated, then return as
	// we can't test that old image won't run since rollback bits were not updated
	// as expected.
	if !sysinfo.ProdKeyladder && f.TestbedProperties.TestbedType != ti50.GscH1Shield {
		if sysinfo.RWRollbackBits.Flash.Bits != previousFlashRollbackBits {
			s.Fatalf("Rollback bits were updated in flash for dev image. wanted: %d, got: %d", previousFlashRollbackBits, sysinfo.RWRollbackBits.Flash.Bits)
		}
		s.Logf("Rollback bits were correctly unmodified with dev image after %s", testType)
		return
	}

	// For prod key ladder, ensure that rollback bits were updated correctly
	if sysinfo.RWRollbackBits.Flash.Bits != releaseRollbackBits {
		s.Fatalf("Rollback bits were not updated in flash for prod image. wanted: %d, got: %d", releaseRollbackBits, sysinfo.RWRollbackBits.Flash.Bits)
	}
	s.Logf("Rollback bits successfully updated after %s", testType)

	// Verify GSC can no longer run the old image.

	// Run power-on reset to clear update rate limit
	b.GpioSet(ctx, ti50.GpioTi50ResetL, false)
	b.GpioSet(ctx, ti50.GpioTi50ResetL, true)
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")
	th.MustSucceed(b.GSCToolWaitUntilReady(ctx), "failed to enable ccd after reset")

	// Update to the DBG image
	if err = b.UpdateOnce(ctx, i, debugImage, debugVer); err != nil {
		s.Fatal("Failed to update to the debug image: ", err)
	}

	// Flash the old image in the inactive slot.
	if err = b.UpdateOnce(ctx, i, oldImage, debugVer); err != nil {
		s.Fatal("Failed to flash old image: ", err)
	}

	version, err = i.VersionInfo(ctx)
	th.MustSucceed(err, "failed to get version")
	s.Logf("RW_A: %+v", version.RwA)
	s.Logf("RW_B: %+v", version.RwB)

	sysinfo, err = i.Sysinfo(ctx)
	th.MustSucceed(err, "failed to get sysinfo")
	s.Logf("Rollback info running old image %+v", sysinfo.RWRollbackBits)

	var inactiveRollback ti50.SysinfoRollbackSlot
	var inactiveName string

	if version.RwA.Active {
		inactiveRollback = sysinfo.RWRollbackBits.SlotB
		inactiveName = "B"
	} else {
		inactiveRollback = sysinfo.RWRollbackBits.SlotA
		inactiveName = "A"
	}
	if !inactiveRollback.Valid {
		s.Error("Could not read old image rollback bits")
	} else if inactiveRollback.Bits != oldRollbackBits {
		s.Errorf("Slot %s rollback bits do not match the old image: expected %d got %+v", inactiveName, oldRollbackBits, sysinfo.RWRollbackBits)
	}
	if sysinfo.RWRollbackBits.Flash.Bits != releaseRollbackBits {
		s.Error("The DBG image updated the number of erased rollback bits")
	}
	// Verify GSC will not run the old image, because it has too few rollback
	// bits erased.
	i.Rollback(ctx)
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")

	version, err = i.VersionInfo(ctx)
	th.MustSucceed(err, "failed to get version")
	s.Log("Version after rollback")
	s.Logf("RW_A: %+v", version.RwA)
	s.Logf("RW_B: %+v", version.RwB)

	sysinfo, err = i.Sysinfo(ctx)
	th.MustSucceed(err, "failed to get sysinfo")
	s.Logf("Rollback info %+v", sysinfo.RWRollbackBits)

	if version.ActiveRw().Version == oldVersion {
		s.Error("GSC rolledback to the old image")
	}
}

func activeImageRollbackBits(rollback ti50.SysinfoRollbackBits, slotAActive bool) (uint8, error) {
	var activeRollback ti50.SysinfoRollbackSlot
	if slotAActive {
		activeRollback = rollback.SlotA
	} else {
		activeRollback = rollback.SlotB
	}
	if !activeRollback.Valid {
		return 0, errors.New("active image rollback information is invalid")
	}
	return activeRollback.Bits, nil
}
