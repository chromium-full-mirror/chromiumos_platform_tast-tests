// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/utils"
	"go.chromium.org/tast/core/testing"
)

var consoleUpdateTooSoonRegexp = regexp.MustCompile("(Attempted update too soon|chunk_came_too_soon)")
var gsctoolUpdateTooSoonRegexp = regexp.MustCompile(`Error: status 0x9`)

func init() {
	testing.AddTest(&testing.Test{
		Func:    GSCCCDUpdate,
		Desc:    "GSC firmware update over CCD using gsctool",
		Timeout: 5 * time.Minute,
		Contacts: []string{
			"cros-hwsec@google.com", // CrOS GSC Developers
			"ecgh@chromium.org",
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr: []string{"group:gsc",
			"gsc_dt_ab", "gsc_dt_shield", "gsc_h1_shield", "gsc_ot_shield",
			"gsc_image_ti50",
			"gsc_nightly"},
		Fixture: fixture.SystemDevboard,
	})
}

// GSCCCDUpdate requires HW setup with SuzyQ cable from Andreib to drone/workstation.
func GSCCCDUpdate(ctx context.Context, s *testing.State) {
	f := s.FixtValue().(*fixture.Value)
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	b := utils.NewDevboardHelper(s, s.FixtValue().(*fixture.Value))
	i := ti50.MustOpenCrOSImage(ctx, b, s, b.TestbedType)
	defer i.Close(ctx)

	// Inform fixture that this test may replace the firmware image in flash.
	th.MustSucceed(f.ImageMayBeUpdatedByTest(), "image may be updated")

	debugImage, err := f.DebugImagePath(ctx)
	if err != nil {
		s.Fatal("DUT must have DBG image")
	}

	_, currentVer, _, _, err := b.GSCToolBinVersion(ctx, f.ImagePath)
	th.MustSucceed(err, "Unable to get version from current image "+f.ImagePath)

	_, debugVer, _, _, err := b.GSCToolBinVersion(ctx, debugImage)
	th.MustSucceed(err, "Unable to get version from "+debugImage)

	if debugVer.Less(currentVer) {
		s.Fatal("DBG version must be greater than current version")
	}

	s.Log("Enabling CCD mode and resetting")
	tpm := b.ResetAndTpmStartup(ctx, i, ti50.CCDModeOn, ti50.FfClamshell)
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")

	// Perform a GSC reset to ensure that the rate limiting is engaged
	th.MustSucceed(tpm.TpmvReboot(500), "Reboot GSC through TPMV")
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")
	b.WaitUntilCCDConnected(ctx)

	s.Log("Sleep for 30 seconds to ensure ensure it is still rate limited")
	testing.Sleep(ctx, 30*time.Second) // GoBigSleepLint: Needed for rate limit testing

	out, _ := b.GSCToolCommand(ctx, debugImage)
	if !gsctoolUpdateTooSoonRegexp.Match(out) {
		s.Fatalf("Wrong gsctool output for update too soon after 1st update: %s", out)
	}

	if _, err = i.WaitUntilMatch(ctx, consoleUpdateTooSoonRegexp, time.Second*3); err != nil {
		s.Fatal("Wrong console message for update too soon after 1st update: ", err)
	}

	s.Log("Sleep for 40 seconds to ensure no longer rate limited")
	testing.Sleep(ctx, 40*time.Second) // GoBigSleepLint: Needed for rate limit testing

	th.MustSucceed(b.UpdateOnce(ctx, i, debugImage, debugVer), "update to DBG image")
}
