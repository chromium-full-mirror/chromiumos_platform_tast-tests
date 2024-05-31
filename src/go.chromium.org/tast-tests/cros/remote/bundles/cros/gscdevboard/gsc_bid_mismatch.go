// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast/core/testing"
)

const (
	// ZZZZ - any BID Type should work. Images are not locked to the board
	// id type anymore.
	testBIDType = 0x5a5a5a5a
	// All test images should be locked to flags. If the chip board id flags
	// are set to 0, it should trigger GSC invalid flag handling
	invalidBIDFlags = 0
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    GSCBIDMismatch,
		Desc:    "Verify GSC handles an invalid board id correctly",
		Timeout: 7 * time.Minute,
		Contacts: []string{
			"gsc-sheriff@google.com",
			"mruthven@chromium.org",
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr: []string{"group:gsc",
			"gsc_h1_shield",
			"gsc_image_ti50",
			"gsc_nightly"},
		Fixture: fixture.GSCInitialFactory,
	})
}

// GSCBIDMismatch verifies GSC handles a board id mismatch correctly.
func GSCBIDMismatch(ctx context.Context, s *testing.State) {
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	b := utils.NewDevboardHelper(s)
	i := ti50.MustOpenCrOSImage(ctx, b, s)
	defer i.Close(ctx)

	f := s.FixtValue().(*fixture.Value)

	// Valid image under test path.
	if f.ImagePath == "" {
		s.Fatal("Supply buildurl")
	}
	if f.DebugImagePath == "" {
		s.Fatal("Failed to find debug image")
	}
	debugImage := f.DebugImagePath

	_, debugVer, _, _, err := b.GSCToolBinVersion(ctx, debugImage)
	th.MustSucceed(err, "Unable to get version from "+debugImage)

	s.Log("Simulating insertion of SuzyQ and resetting")
	b.ResetWithStraps(ctx, ti50.CcdSuzyQ)
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")
	b.WaitUntilCCDConnected(ctx)

	version, err := i.VersionInfo(ctx)
	th.MustSucceed(err, "failed to get version")

	if version.BID.Flags == invalidBIDFlags {
		s.Log("Nothing to do. All board ids are valid")
		return
	}

	bid, err := i.ChipBID(ctx)
	th.MustSucceed(err, "failed to get board id")
	s.Log("Got board id")
	if !bid.IsErased {
		s.Fatal("Board ID is set")
	}
	s.Logf("BID: %+v", bid)

	th.MustSucceed(b.UpdateOnce(ctx, i, debugImage, debugVer), "update to DBG image")

	tpm := b.ResetAndTpmStartup(ctx, i, ti50.FfClamshell)
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC failed to boot")

	err = tpm.TpmvSetBoardID(testBIDType, invalidBIDFlags)
	th.MustSucceed(err, "failed to set board id")

	bid, err = i.ChipBID(ctx)
	th.MustSucceed(err, "failed to get board id")
	s.Logf("BID: %+v", bid)
	if bid.Type != testBIDType || bid.Flags != invalidBIDFlags {
		s.Fatalf("board id not set correctly: expected %x:%x got %+v", testBIDType, invalidBIDFlags, bid)
	}

	version, err = i.VersionInfo(ctx)
	th.MustSucceed(err, "failed to get version")
	s.Logf("Version: %+v", version)

	i.Rollback(ctx)

	runningDBG, err := i.CheckRunningVersion(ctx, debugVer.String(), true, false)
	th.MustSucceed(err, "failed to get version after rollback")
	// TODO: check RO output to verify it shows there's a BID mismatch.
	if f.TestbedProperties.TestbedType != ti50.GscH1Shield {
		if !runningDBG {
			s.Fatal("Ti50 rolled back to image with board id mismatch")
		}
		return
	}
	if runningDBG {
		s.Fatal("Cr50 rejected rollback")
	}
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")
	b.WaitUntilCCDConnected(ctx)

	out, err := i.Command(ctx, "ccd open")
	th.MustSucceed(err, "unable to get ccd output")
	s.Logf("ccd open output: %s", out)
	if !strings.Contains(out, "BoardID mismatch") || !strings.Contains(out, "error") {
		s.Errorf("CCD open worked with board id mismatch: %s", out)
	} else {
		s.Log("Board ID mismatch blocked ccd open")
	}
	s.Log("Wait for update")
	// GoBigSleepLint sleeping for known required period of time.
	testing.Sleep(ctx, 61*time.Second)

	err = b.UpdateOnce(ctx, i, debugImage, debugVer)
	th.MustSucceed(err, "failed to run update with BID mismatch")
	s.Log("Updated from board id mismatch")
}
