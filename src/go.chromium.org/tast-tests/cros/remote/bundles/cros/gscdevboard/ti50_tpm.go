// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"bytes"
	"context"
	"regexp"
	"time"

	"github.com/google/go-tpm/tpm2"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    Ti50Tpm,
		Desc:    "Test TPM functionality of ti50 in remote environment(Andreiboard connected to devboardsvc host)",
		Timeout: 30 * time.Second,
		Contacts: []string{
			"chromeos-faft@google.com", // CrOS Firmware Developers
			"aluo@chromium.org",        // Test Author
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr:         []string{"group:gsc", "gsc_dt_ab", "gsc_image_ti50"},
		Fixture:      fixture.Ti50CcdOpen,
		Params: []testing.Param{{
			Name: "spi",
			Val:  "SPI",
		}, {
			Name:      "i2c",
			Val:       "I2C",
			ExtraAttr: []string{"gsc_ot_fpga_cw310"},
		}},
	})
}

func Ti50Tpm(ctx context.Context, s *testing.State) {
	mode := s.Param().(string)

	f := s.FixtValue().(*fixture.Value)
	b := utils.NewDevboardHelper(f.DevBoard(), s)
	i := ti50.NewCrOSImage(b)
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}

	var strap ti50.GpioStrap
	var bus ti50.TpmBus
	switch mode {
	case "SPI":
		strap = ti50.TpmSpi
		bus = ti50.TpmBusSpi
	case "I2C":
		strap = ti50.TpmI2c
		bus = ti50.TpmBusI2c
	}

	s.Logf("Restarting Ti50 with %s straps", mode)
	b.GpioApplyStrap(ctx, strap, ti50.CcdSuzyQ, ti50.FfClamshell)
	th.MustSucceed(b.Reset(ctx), "Reset board")
	m, err := b.ReadSerialSubmatch(ctx, regexp.MustCompile(`Strap config: .* TPM Bus: ([^;]+);`))
	if err != nil || string(m[1]) != mode {
		s.Fatal("Wrong TPM strap")
	}
	th.MustSucceed(i.WaitUntilBooted(ctx), "Ti50 revives after reboot")

	// Tell Ti50 that the AP came out of reset.  This will cause Ti50 to start responding to
	// TPM commands.
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, true)

	tpmHandle := b.Tpm(ctx, bus)
	didVid := tpmHandle.ReadRegister(ti50.TpmRegDidVid)
	if !bytes.Equal(didVid, ti50.TpmDidVidValue) {
		s.Error("Unexpected TPM DID_VID: ", didVid)
	}

	if err := tpm2.Startup(tpmHandle, tpm2.StartupClear); err != nil {
		s.Error("TPM startup error: ", err)
	}

	// Read boot mode as a simple check of vendor command.
	bm, err := tpmHandle.TpmvGetBootMode()
	if err != nil {
		s.Error("boot mode error: ", err)
	}
	s.Logf("Read boot mode %d", bm)
}
