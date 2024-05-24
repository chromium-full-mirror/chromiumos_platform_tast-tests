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
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    GSCFactoryUpdate,
		Desc:    "Verify the factory image can update to the image under test",
		Timeout: 10 * time.Minute,
		Contacts: []string{
			"gsc-sheriff@google.com",
			"mruthven@chromium.org",
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr:         []string{"group:gsc", "gsc_image_ti50", "gsc_nightly"},
		Fixture:      fixture.GSCUpdate,
		Params: []testing.Param{{
			Name:      "cr50_0_3_22",
			Val:       "0.3.22",
			ExtraAttr: []string{"gsc_h1_shield"},
		}, {
			Name:      "ti50_0_23_30",
			Val:       "0.23.30",
			ExtraAttr: []string{"gsc_dt_shield"},
		}, {
			Name:      "ti50_0_23_74",
			Val:       "0.23.74",
			ExtraAttr: []string{"gsc_dt_shield"},
		}},
	})
}

// GSCFactoryUpdate requires HW setup with SuzyQ cable from devboard to drone/workstation.
func GSCFactoryUpdate(ctx context.Context, s *testing.State) {
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	b := utils.NewDevboardHelper(s)
	i := ti50.MustOpenCrOSImage(ctx, b, s)
	defer i.Close(ctx)

	f := s.FixtValue().(*fixture.Value)
	factoryVersion := s.Param().(string)

	// Valid image under test path.
	if f.ImagePath == "" {
		s.Fatal("Supply buildurl")
	}
	if f.EfiImagePath == "" {
		s.Fatal("Failed to find efi image")
	}
	if f.DebugImagePath == "" {
		s.Fatal("Failed to find debug image")
	}
	imageUnderTest := f.ImagePath
	debugImage := f.DebugImagePath
	efiImage := f.EfiImagePath

	// Download the factory image image.
	fwName := fixture.FindFwName(f.TestbedProperties.TestbedType)
	factoryImageURL, err := fixture.LookupGSCReleaseTarball(ctx, factoryVersion, fwName)
	th.MustSucceed(err, "failed to find gsc factory image")

	factoryImage, err := fixture.DownloadToTempFile(ctx, "factory image", factoryImageURL)
	factoryImage, err = fixture.ExtractGSCQualImageFromTarball(ctx, factoryImage)
	th.MustSucceed(err, "failed to extract gsc factory image")

	_, factoryVer, _, _, err := b.GSCToolBinVersion(ctx, factoryImage)
	th.MustSucceed(err, "failed to parse factory version")
	_, releaseVer, _, _, err := b.GSCToolBinVersion(ctx, imageUnderTest)
	th.MustSucceed(err, "failed to parse image under test version")
	_, debugVer, _, _, err := b.GSCToolBinVersion(ctx, debugImage)
	th.MustSucceed(err, "failed to debug image version")
	_, efiVer, _, _, err := b.GSCToolBinVersion(ctx, efiImage)
	th.MustSucceed(err, "failed to efi image version")

	s.Logf("image under test %s: %s", releaseVer, imageUnderTest)
	s.Logf("factory %s: %s", factoryVer, factoryImage)
	s.Logf("dbg %s: %s", debugVer, debugImage)
	s.Logf("efi %s: %s", efiVer, efiImage)

	// Connect Suzyq, so the test can update over ccd.
	s.Log("Simulating insertion of SuzyQ and resetting")
	b.ResetWithStraps(ctx, ti50.CcdSuzyQ)
	th.MustSucceed(i.WaitUntilBooted(ctx), "Ti50 revives after reboot")
	b.WaitUntilCCDConnected(ctx)

	// Erase info1 and rollback to the factory image.
	err = b.RollbackAndRunEraseFlashInfoUpdate(ctx, i, factoryImage, efiImage, debugImage)
	th.MustSucceed(err, "failed to update to factory image")
	s.Log("Updated to the factory image")
	s.Log("Wait 61s to avoid update too soon error")

	// GoBigSleepLint sleeping for known required period of time.
	testing.Sleep(ctx, 61*time.Second)

	th.MustSucceed(b.GSCToolWaitUntilReady(ctx), "failed to enable ccd after factory update")

	// Verify gsc can update from the factory image to the image under test.
	if err = b.UpdateOnce(ctx, i, imageUnderTest, releaseVer); err != nil {
		s.Fatal("Failed to update to the image under test from the factory image: ", err)
	}
	s.Log("Successfully ran the update from the factory image to the image under test")
}
