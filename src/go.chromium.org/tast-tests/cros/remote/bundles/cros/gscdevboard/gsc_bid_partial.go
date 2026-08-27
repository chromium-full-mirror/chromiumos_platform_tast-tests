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

const (
	testType = 0x5a5a5a5a
	// These flags should work with all test image flags (0x10, 0x10000, and 0x20000)
	testFlags = 0x37f7f
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    GSCBIDPartial,
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
	})
}

// GSCBIDPartial verifies the BID flags can be set without setting the type.
func GSCBIDPartial(ctx context.Context, s *testing.State) {
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	b := utils.NewDevboardHelper(s, s.FixtValue().(*fixture.Value))
	i := ti50.MustOpenCrOSImage(ctx, b, s, b.TestbedType)
	defer i.Close(ctx)

	tpm := b.ResetAndTpmStartup(ctx, i, ti50.FfClamshell)
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC failed to boot")

	bid, err := i.ChipBID(ctx)
	th.MustSucceed(err, "failed to get board id")
	s.Log("Got board id")
	if !bid.IsErased {
		s.Fatal("Board ID is set")
	}
	s.Logf("BID: %+v", bid)

	err = tpm.TpmvSetBoardID(ti50.UnsetBID, testFlags)
	th.MustSucceed(err, "failed to set partial board id")

	bid, err = i.ChipBID(ctx)
	th.MustSucceed(err, "failed to get board id")
	s.Logf("Partial BID: %+v", bid)
	if !bid.TypeIsErased {
		s.Fatal("Board ID Type is set")
	}
	if bid.FlagsAreErased || bid.Flags != testFlags {
		s.Fatal("Flags were not set correctly")
	}
	err = tpm.TpmvSetBoardID(testType, testFlags)
	th.MustSucceed(err, "failed to set board id type")

	bid, err = i.ChipBID(ctx)
	th.MustSucceed(err, "failed to get board id")
	s.Logf("Full BID: %+v", bid)
	if bid.TypeIsErased || bid.Type != testType {
		s.Fatal("Unable to set board id type")
	}
	if bid.FlagsAreErased || bid.Flags != testFlags {
		s.Fatal("Flags were not set correctly")
	}
}
