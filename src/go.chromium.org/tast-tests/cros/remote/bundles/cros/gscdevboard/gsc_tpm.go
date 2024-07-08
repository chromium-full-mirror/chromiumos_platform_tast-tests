// Copyright 2023 The ChromiumOS Authors
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
		Func:    GSCTPM,
		Desc:    "Test TPM functionality of ti50 in remote environment(Andreiboard connected to devboardsvc host)",
		Timeout: 30 * time.Second,
		Contacts: []string{
			"cros-hwsec@google.com", // CrOS GSC Developers
			"aluo@chromium.org",     // Test Author
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr: []string{"group:gsc",
			"gsc_dt_ab", "gsc_dt_shield", "gsc_h1_shield",
			"gsc_image_ti50",
			"gsc_nightly"},
		Fixture: fixture.GSCOpenCCD,
		Params: []testing.Param{{
			Name: "spi",
			Val:  ti50.TpmBusSpi,
		}, {
			Name:      "i2c",
			Val:       ti50.TpmBusI2c,
			ExtraAttr: []string{"gsc_ot_fpga_cw310", "gsc_ot_shield"},
		}},
	})
}

func GSCTPM(ctx context.Context, s *testing.State) {
	bus := s.Param().(ti50.TpmBus)
	b := utils.NewDevboardHelper(s)
	i := ti50.MustOpenCrOSImage(ctx, b, s)
	defer i.Close(ctx)

	tpmHandle := b.ResetAndTpmStartupForBus(ctx, i, bus, ti50.CcdSuzyQ, ti50.FfClamshell)

	// Read boot mode as a simple check of vendor command.
	bm, err := tpmHandle.TpmvGetBootMode()
	if err != nil {
		s.Error("boot mode error: ", err)
	}
	s.Logf("Read boot mode %d", bm)

	out, err := b.GSCToolCommandViaTPM(ctx, bus, "", "--fwver")
	if err != nil {
		s.Error("Could not get version via TPM: ", err)
	}
	s.Logf("GSC version: %s", out)
}
