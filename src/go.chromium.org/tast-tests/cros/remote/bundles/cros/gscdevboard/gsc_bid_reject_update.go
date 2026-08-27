// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast/core/testing"
)

const (
	// BID value for TEST
	okChipType = 0x54455354
	// BID value for TESU. The test images are signed with TEST.
	// Any value other than TEST should get blocked.
	badChipType = 0x54455355
	// Test images are signed with flags 0xfffff
	okChipFlags = 0xfffff
	// All bits set in the image must be set in the Chip BID
	badChipFlags = 0xffffe
	// The chip can have extra bits set in the flags.
	okLargerChipFlags = 0x7fffffff
	// Test chip flags that work for all images
	universalTestImageFlags = 0x30010
	imageTemplate           = fixture.DevGSCImageBucket + "test/%s.dbg.no_devid.bin.unsigned.%s"
)

type testBIDConfig struct {
	imageBID  string
	testFlags ti50.BIDField
	testType  ti50.BIDField
	blocked   bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:    GSCBIDRejectUpdate,
		Desc:    "Verify GSC can set the board id flags without setting the type",
		Timeout: 7 * time.Minute,
		Contacts: []string{
			"cros-hwsec@google.com",
			"mruthven@chromium.org",
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr: []string{"group:gsc",
			"gsc_h1_shield", "gsc_dt_shield", "gsc_ot_shield", "gsc_ot_fpga_cw310",
			"gsc_image_ti50",
			"gsc_nightly"},
		Fixture: fixture.GSCInitialFactory,
		Params: []testing.Param{{
			Name: "ok_type",
			Val: testBIDConfig{
				imageBID:  "TEST_ffffffff_00000000",
				testType:  okChipType,
				testFlags: universalTestImageFlags,
				blocked:   false,
			},
		}, {
			Name: "bad_type",
			Val: testBIDConfig{
				imageBID:  "TEST_ffffffff_00000000",
				testType:  badChipType,
				testFlags: okLargerChipFlags,
				blocked:   true,
			},
		}, {
			Name: "ok_flags1",
			Val: testBIDConfig{
				imageBID:  "TEST_00000000_000fffff",
				testType:  badChipType,
				testFlags: okChipFlags,
				blocked:   false,
			},
		}, {
			Name: "ok_flags2",
			Val: testBIDConfig{
				imageBID:  "TEST_00000000_000fffff",
				testType:  badChipType,
				testFlags: okLargerChipFlags,
				blocked:   false,
			},
		}, {
			Name: "bad_flags",
			Val: testBIDConfig{
				imageBID:  "TEST_00000000_000fffff",
				testType:  okChipType,
				testFlags: badChipFlags,
				blocked:   true,
			},
		}},
	})
}

// GSCBIDRejectUpdate verifies that GSC rejects images signed with the wrong board id.
func GSCBIDRejectUpdate(ctx context.Context, s *testing.State) {
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	b := utils.NewDevboardHelper(s, s.FixtValue().(*fixture.Value))
	i := ti50.MustOpenCrOSImage(ctx, b, s, b.TestbedType)
	defer i.Close(ctx)

	f := s.FixtValue().(*fixture.Value)
	fwName := fixture.FindFwName(f.TestbedProperties.TestbedType)
	imagePrefix := fixture.GetFwAndDevicePrefix(f.TestbedProperties.TestbedType)

	testConfig := s.Param().(testBIDConfig)
	testFlags := testConfig.testFlags
	testType := testConfig.testType
	bidDesc := fmt.Sprintf("%x:%x", testType, testFlags)

	gsURL := fmt.Sprintf(imageTemplate, fwName, imagePrefix, testConfig.imageBID)
	bidImagePath, err := fixture.DownloadToTempFile(ctx, "testBID", gsURL)
	th.MustSucceed(err, "Failed to download %s", gsURL)

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

	err = tpm.TpmvSetBoardID(testType, testFlags)
	th.MustSucceed(err, "failed to set board id to %s", bidDesc)

	bid, err = i.ChipBID(ctx)
	th.MustSucceed(err, "failed to get board id")
	s.Logf("BID: %+v", bid)
	if bid.Type != testType || bid.Flags != testFlags {
		s.Fatalf("Incorrect BID expected %s got %+v", bidDesc, bid)
	}

	// GSC should accept the update, but it won't jump to the image. The
	// board id locked locked images are not signed with valid keys.
	err = b.GSCToolUpdateSkipBidCheck(ctx, i, bidImagePath)
	if testConfig.blocked {
		if err == nil {
			s.Fatalf("BID %s did not block update to image with %s BID", bidDesc, testConfig.imageBID)
		}
	} else if err != nil {
		s.Fatalf("Unable to update to %s with BID %s: %s", testConfig.imageBID, bidDesc, err)
	}
}
