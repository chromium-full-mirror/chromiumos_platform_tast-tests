// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/utils"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    GSCUpdateConcurrent,
		Desc:    "Verify GSC update is not interrupted by CCD gsctool command",
		Timeout: 7 * time.Minute,
		Contacts: []string{
			"cros-hwsec@google.com",
			"jettrink@chromium.org",
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr: []string{"group:gsc",
			"gsc_dt_shield", "gsc_ot_shield", "gsc_ot_fpga_cw310",
			"gsc_image_ti50",
			"gsc_nightly"},
		Fixture: fixture.SystemDevboard,
		Params: []testing.Param{{
			Name: "i2c",
			Val:  ti50.TpmBusI2c,
		}, {
			Name:      "spi",
			Val:       ti50.TpmBusSpi,
			ExtraAttr: []string{"gsc_h1_shield"},
		}},
	})
}

// GSCUpdateConcurrent updates GSC firmware over TPM bus then asynchronously
// sends gsctool version request commands over CCD connection.
func GSCUpdateConcurrent(ctx context.Context, s *testing.State) {
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	b := utils.NewDevboardHelper(s, s.FixtValue().(*fixture.Value))
	i := ti50.MustOpenCrOSImage(ctx, b, s, b.TestbedType)
	defer i.Close(ctx)

	f := s.FixtValue().(*fixture.Value)
	testBus := s.Param().(ti50.TpmBus)

	// Inform fixture that this test may replace the firmware image in flash.
	th.MustSucceed(f.ImageMayBeUpdatedByTest(), "image may be updated")

	currentImage := f.ImagePath
	debugImage, err := f.DebugImagePath(ctx)
	th.MustSucceed(err, "Failed to find debug image")

	_, currentVer, _, _, err := b.GSCToolBinVersion(ctx, currentImage)
	th.MustSucceed(err, "Unable to get version from current image "+currentImage)

	_, debugVer, _, _, err := b.GSCToolBinVersion(ctx, debugImage)
	th.MustSucceed(err, "Unable to get version from "+debugImage)

	if debugVer.Less(currentVer) || debugVer == currentVer {
		s.Fatalf("DBG version (%s) must be greater than current version (%s)", debugVer, currentVer)
	}

	s.Logf("Image under test %s: %s", currentVer, currentImage)

	// Connect Suzyq, so the test can update over ccd.
	s.Log("Enabling CCD mode and resetting")
	b.ResetAndTpmStartupForBus(ctx, i, testBus, ti50.CCDModeOn, ti50.FfClamshell)
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")

	go func() {
		for i := 0; i < 10; i++ {
			out, err := b.GSCToolCommand(ctx, "", "-f")
			if err != nil {
				s.Error("Failed to get version from CCD: ", err)
				return
			}
			s.Logf("CCD version output: %s", out)
			// GoBigSleepLint: Rate limit the number of time we call version command
			testing.Sleep(ctx, time.Second)
		}

	}()
	out, _ := b.GSCToolCommandViaTPM(ctx, testBus, debugImage)
	s.Logf("TPM update output: %s", out)

	err = i.WaitUntilBooted(ctx)
	if err != nil {
		s.Error("GSC did not revive from reboot: ", err)
	}

	version, err := i.VersionInfo(ctx)
	th.MustSucceed(err, "get version info")
	s.Logf("RW_A: %+v", version.RwA)
	s.Logf("RW_B: %+v", version.RwB)

	if version.ActiveRw().Version != debugVer.String() {
		s.Errorf("GSC did not update. wanted %s, got %s", debugVer.String(), version.ActiveRw().Version)
	}
}
