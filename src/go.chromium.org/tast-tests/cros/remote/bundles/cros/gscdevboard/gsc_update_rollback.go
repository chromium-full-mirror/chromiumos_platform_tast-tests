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
		Func:    GSCUpdateRollback,
		Desc:    "Verify that GSC will rollback image new image crashes multiple times before AP starts up",
		Timeout: 4 * time.Minute,
		Contacts: []string{
			"cros-hwsec@google.com",
			"jettrink@chromium.org",
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr:         []string{"group:gsc", "gsc_dt_shield", "gsc_h1_shield", "gsc_image_ti50", "gsc_nightly"},
		Fixture:      fixture.GSCUpdate,
	})
}

// GSCUpdateRollback verifies that GSC will rollback to previous image.
func GSCUpdateRollback(ctx context.Context, s *testing.State) {
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	b := utils.NewDevboardHelper(s)
	i := ti50.MustOpenCrOSImage(ctx, b, s)
	defer i.Close(ctx)

	f := s.FixtValue().(*fixture.Value)

	// Validate image under test path.
	if f.ImagePath == "" {
		s.Fatal("Supply BUILDURL")
	}

	// Validate debug image is available
	if f.DebugImagePath == "" {
		s.Fatal("DUT must have DBG image")
	}

	currentImage := f.ImagePath
	debugImage := f.DebugImagePath

	_, currentVer, _, _, err := b.GSCToolBinVersion(ctx, currentImage)
	th.MustSucceed(err, "Unable to get version from current image "+currentImage)

	_, debugVer, _, _, err := b.GSCToolBinVersion(ctx, debugImage)
	th.MustSucceed(err, "Unable to get version from "+debugImage)

	if debugVer.Less(currentVer) || debugVer == currentVer {
		s.Fatal("DBG version must be greater than current version")
	}

	s.Logf("Image under test %s: %s", currentVer, currentImage)

	s.Log("Simulating insertion of SuzyQ and resetting")
	tpmBus := b.GscProperties().PreferredTPMBus()
	b.ResetAndTpmStartupForBus(ctx, i, tpmBus, ti50.CcdSuzyQ, ti50.FfClamshell)
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")

	// AP turns on so TPM bus will be active
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, true)
	b.GSCToolCommandViaTPM(ctx, tpmBus, debugImage)

	// Turn AP off to simulate crashing Ti50 FW image
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, false)

	numCrashesForRollback := 0
	for attempt := 0; attempt < 10; attempt++ {
		// Give the device a little more time than normal to reboot since we are
		// using the watchdog reset.
		err := i.CommandImage.WaitUntilBooted(ctx, 8*time.Second)
		th.MustSucceed(err, "GSC revives after attempt %d", attempt)

		versionInfo, err := i.VersionInfo(ctx)
		th.MustSucceed(err, "get version info")
		s.Logf("Version info on attempt %d: %+v", attempt, versionInfo)

		if versionInfo.ActiveRw().Version != debugVer.String() {
			numCrashesForRollback = attempt
			break
		}

		th.MustSucceed(i.SendDBGConsoleCrashCmd(ctx), "calling crash cmd")
		immediateCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		// We need to find and remove the fatal message from the UART output
		// otherwise other console matches will return an error when they detect
		// the crash output.
		_, _, err = i.ReadSerialSubmatch(immediateCtx, ti50.FatalMsg)
		th.MustSucceed(err, "No fatal reset found in UART")
	}

	if numCrashesForRollback < 5 || numCrashesForRollback > 8 {
		s.Error("Number of crash for rollback out of range: ", numCrashesForRollback)
	} else {
		s.Logf("Rolled back after %d crashes", numCrashesForRollback)
	}

	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after crash")

	versionInfo, err := i.VersionInfo(ctx)
	th.MustSucceed(err, "get version info")
	s.Logf("Version info final crash: %+v", versionInfo)

	if versionInfo.ActiveRw().Version != currentVer.String() {
		s.Error("Not running original image")
	}
}
