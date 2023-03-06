// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"time"

	"chromiumos/tast/common/firmware/ti50"
	"chromiumos/tast/remote/bundles/cros/firmware/utils"
	"chromiumos/tast/remote/firmware/ti50/fixture"
	"chromiumos/tast/testing"
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
		Attr:         []string{"group:firmware"},
		Fixture:      fixture.Ti50,
	})
}

const ti50TpmDidVid = "66664a50"

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

	data := th.MustSucceedVal(b.OpenTitanToolCommand(ctx, "spi", "tpm", "read-register", ti50.TpmRegDidVid))

	if data.(map[string]interface{})["hexdata"].(string) != ti50TpmDidVid {
		s.Error("Unexpected TPM DID_VID: ", data.(map[string]interface{})["hexdata"].(string))
	}
}
