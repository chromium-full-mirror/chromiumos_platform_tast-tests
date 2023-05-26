// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"time"

	"github.com/google/go-tpm/tpm2"

	"chromiumos/tast/remote/bundles/cros/gscdevboard/utils"
	"chromiumos/tast/remote/firmware/ti50/fixture"
	"go.chromium.org/tast-tests/cros/common/firmware/ti50"

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
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:gsc", "gsc_dt_ab", "gsc_image_ti50"},
		Fixture:      fixture.Ti50CcdOpen,
	})
}

func Ti50Tpm(ctx context.Context, s *testing.State) {
	f := s.FixtValue().(*fixture.Value)

	b := utils.NewDevboardHelper(f.DevBoard(), s)
	i := ti50.NewCrOSImage(b)

	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}

	s.Log("Restarting ti50 with SPI straps")
	b.GpioApplyStrap(ctx, ti50.TpmSpi)
	th.MustSucceed(b.Reset(ctx), "Reset board")
	th.MustSucceed(i.WaitUntilBooted(ctx), "Ti50 revives after reboot")

	// Tell Ti50 that the AP came out of reset.  This will cause Ti50 to start responding to
	// TPM commands.
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, true)

	tpmHandle := b.Tpm(ctx, ti50.TpmBusSpi)
	didVid := tpmHandle.ReadRegister(ti50.TpmRegDidVid)
	if didVid != ti50.TpmDidVidHexValue {
		s.Error("Unexpected TPM DID_VID: ", didVid)
	}

	if err := tpm2.Startup(tpmHandle, tpm2.StartupClear); err != nil {
		s.Error("TPM error: ", err)
	}
}
