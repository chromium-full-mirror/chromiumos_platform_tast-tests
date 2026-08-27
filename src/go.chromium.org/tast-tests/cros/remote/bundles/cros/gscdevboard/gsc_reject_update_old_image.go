// Copyright 2024 The ChromiumOS Authors
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

const (
	// Old GSC versions for each chip type.
	oldH1ImageVersion = "0.3.22"
	oldDTImageVersion = "0.21.1"
	oldOTImageVersion = "0.33.170"
	// Error 8 means the image is older than the one running.
	errOldImage = "(Error: status 0x8|Error 8)"
	// Upstart updates skip the update with "nothing to do"
	errNothingToDo = "nothing to do"
)

type configRejectOldImageUpdate struct {
	bus ti50.TpmBus
	cmd string
	ccd bool
	err string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:    GSCRejectUpdateOldImage,
		Desc:    "Verify GSC rejects updates to old images",
		Timeout: 7 * time.Minute,
		Contacts: []string{
			"cros-hwsec@google.com",
			"mruthven@chromium.org",
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr: []string{"group:gsc",
			"gsc_dt_shield", "gsc_ot_shield", "gsc_ot_fpga_cw310",
			"gsc_image_ti50",
			"gsc_nightly"},
		Fixture: fixture.SystemDevboard,
		Params: []testing.Param{{
			Name: "ccd",
			Val: configRejectOldImageUpdate{
				// -V adds verbose gsctool output. It doesn't change the gsc behavior. It's just
				// an arg, so the test GSCTool command handling works.
				cmd: "-V",
				ccd: true,
				err: errOldImage,
			},
			ExtraAttr: []string{"gsc_h1_shield"},
		}, {
			Name: "ccd_post_reset",
			Val: configRejectOldImageUpdate{
				cmd: "-p",
				ccd: true,
				err: errOldImage,
			},
			ExtraAttr: []string{"gsc_h1_shield"},
		}, {
			Name: "ccd_upstart",
			Val: configRejectOldImageUpdate{
				cmd: "-u",
				ccd: true,
				err: errNothingToDo,
			},
			ExtraAttr: []string{"gsc_h1_shield"},
		}, {
			Name: "tpm_i2c",
			Val: configRejectOldImageUpdate{
				bus: ti50.TpmBusI2c,
				// -V adds verbose gsctool output. It doesn't change the gsc behavior. It's just
				// an arg, so the test GSCTool command handling works.
				cmd: "-V",
				err: errOldImage,
			},
		}, {
			Name: "tpm_i2c_post_reset",
			Val: configRejectOldImageUpdate{
				bus: ti50.TpmBusI2c,
				cmd: "-p",
				err: errOldImage,
			},
		}, {
			Name: "tpm_i2c_upstart",
			Val: configRejectOldImageUpdate{
				bus: ti50.TpmBusI2c,
				cmd: "-u",
				err: errNothingToDo,
			},
		}, {
			Name: "tpm_spi",
			Val: configRejectOldImageUpdate{
				bus: ti50.TpmBusSpi,
				// -V adds verbose gsctool output. It doesn't change the gsc behavior. It's just
				// an arg, so the test GSCTool command handling works.
				cmd: "-V",
				err: errOldImage,
			},
			ExtraAttr: []string{"gsc_h1_shield"},
		}, {
			Name: "tpm_spi_post_reset",
			Val: configRejectOldImageUpdate{
				bus: ti50.TpmBusSpi,
				cmd: "-p",
				err: errOldImage,
			},
			ExtraAttr: []string{"gsc_h1_shield"},
		}, {
			Name: "tpm_spi_upstart",
			Val: configRejectOldImageUpdate{
				bus: ti50.TpmBusSpi,
				cmd: "-u",
				err: errNothingToDo,
			},
			ExtraAttr: []string{"gsc_h1_shield"},
		}},
	})
}

// GSCRejectUpdateOldImage verifies that GSC rejects updates to old iamges
func GSCRejectUpdateOldImage(ctx context.Context, s *testing.State) {
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	b := utils.NewDevboardHelper(s, s.FixtValue().(*fixture.Value))
	i := ti50.MustOpenCrOSImage(ctx, b, s, b.TestbedType)
	defer i.Close(ctx)

	f := s.FixtValue().(*fixture.Value)
	fwName := fixture.FindFwName(f.TestbedProperties.TestbedType)

	config := s.Param().(configRejectOldImageUpdate)

	// Inform fixture that this test may replace the firmware image in flash.
	th.MustSucceed(f.ImageMayBeUpdatedByTest(), "image may be updated")

	var fwVersion string

	switch b.GscProperties().ChipType() {
	case ti50.GscH1:
		fwVersion = oldH1ImageVersion
	case ti50.GscDT:
		fwVersion = oldDTImageVersion
	case ti50.GscOT:
		fwVersion = oldOTImageVersion
	default:
		s.Fatal("Unsupported testbed type")
	}

	bus := config.bus
	if config.ccd {
		bus = b.GscProperties().PreferredTPMBus()
	}

	gsURL, err := fixture.LookupGSCReleaseTarball(ctx, fwVersion, fwName)
	th.MustSucceed(err, "Failed to lookup %s image", fwVersion)
	oldTarball, err := fixture.DownloadToTempFile(ctx, "testOldUpdate", gsURL)
	th.MustSucceed(err, "Failed to download %s", gsURL)
	oldImage, err := fixture.ExtractGSCQualImageFromTarball(ctx, oldTarball)
	th.MustSucceed(err, "failed to extract old gsc image")

	b.ResetAndTpmStartupForBus(ctx, i, bus, ti50.FfClamshell, ti50.CCDModeOn)
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC failed to boot")
	b.WaitUntilCCDConnected(ctx)

	out, err := b.GSCToolCommand(ctx, "", "-f")
	th.MustSucceed(err, "failed to get running version")
	s.Logf("running version: %s", out)
	// GSC should reject the update to old images.
	if config.ccd {
		out, _ = b.GSCToolCommand(ctx, oldImage, config.cmd)
	} else {
		out, _ = b.GSCToolCommandViaTPM(ctx, bus, oldImage, config.cmd)
	}
	s.Logf("gsctool %s update output: %s", config.cmd, out)
	errorRE := regexp.MustCompile(config.err)
	match := errorRE.FindStringSubmatch(string(out))
	if match == nil {
		s.Fatalf("Did not find %s in update output", config.err)
	}
	s.Logf("Found %s in update output", match[0])
}
