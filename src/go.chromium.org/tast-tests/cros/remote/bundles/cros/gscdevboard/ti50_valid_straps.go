// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type ti50ValidStrapsParam struct {
	tpmCommunication ti50.GpioStrap
	formFactor       ti50.GpioStrap
	brdpropOutput    string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:    Ti50ValidStraps,
		Desc:    "Test Ti50 strap configuration using a dev board",
		Timeout: 30 * time.Second,
		Contacts: []string{
			"chromeos-faft@google.com",
			"ti50-core@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware"},
		Fixture:      fixture.Ti50CcdOpen,
		Params: []testing.Param{{
			Name: "spi_tablet",
			Val: ti50ValidStrapsParam{
				tpmCommunication: ti50.TpmSpi,
				formFactor:       ti50.FfTablet,
				brdpropOutput:    "TPM Bus: SPI; FormFactor: Tablet",
			},
		}, {
			Name: "spi_clamshell",
			Val: ti50ValidStrapsParam{
				tpmCommunication: ti50.TpmSpi,
				formFactor:       ti50.FfClamshell,
				brdpropOutput:    "TPM Bus: SPI; FormFactor: Clamshell",
			},
		}, {
			Name: "spi_box",
			Val: ti50ValidStrapsParam{
				tpmCommunication: ti50.TpmSpi,
				formFactor:       ti50.FfBox,
				brdpropOutput:    "TPM Bus: SPI; FormFactor: Box",
			},
		}, {
			Name: "i2c_tablet",
			Val: ti50ValidStrapsParam{
				tpmCommunication: ti50.TpmI2c,
				formFactor:       ti50.FfTablet,
				brdpropOutput:    "TPM Bus: I2C; FormFactor: Tablet",
			},
		}, {
			Name: "i2c_clamshell",
			Val: ti50ValidStrapsParam{
				tpmCommunication: ti50.TpmI2c,
				formFactor:       ti50.FfClamshell,
				brdpropOutput:    "TPM Bus: I2C; FormFactor: Clamshell",
			},
		}, {
			Name: "i2c_box",
			Val: ti50ValidStrapsParam{
				tpmCommunication: ti50.TpmI2c,
				formFactor:       ti50.FfBox,
				brdpropOutput:    "TPM Bus: I2C; FormFactor: Box",
			},
		}},
	})
}

func Ti50ValidStraps(ctx context.Context, s *testing.State) {
	userParams := s.Param().(ti50ValidStrapsParam)

	f := s.FixtValue().(*fixture.Value)
	b := utils.NewDevboardHelper(f.DevBoard(), s)
	i := ti50.NewCrOSImage(b)
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}

	s.Log("Restarting ti50 with " + string(userParams.tpmCommunication) + " transport straps and " + string(userParams.formFactor) + " rbox straps")
	b.GpioApplyStrap(ctx, userParams.tpmCommunication)
	b.GpioApplyStrap(ctx, userParams.formFactor)

	s.Log("Hard resetting board")
	th.MustSucceed(b.Reset(ctx), "Reset board")
	th.MustSucceed(i.WaitUntilBooted(ctx), "Ti50 revives after reboot")
	if err := checkGSCBrdpropCommandOutput(ctx, i, userParams.brdpropOutput); err != nil {
		s.Error("Failed verify straps after hard reset: ", err)
	}

	s.Log("Power-on resetting board by sending `reboot` GSC command")
	// Check that an error occurred since we expect `Command(ctx, "reboot")` to
	// fail.
	if _, err := i.Command(ctx, "reboot"); err == nil {
		s.Error("Failed Ti50 `reboot` command failed to restart device")
	}
	th.MustSucceed(i.WaitUntilBooted(ctx), "Ti50 revives after reboot")
	if err := checkGSCBrdpropCommandOutput(ctx, i, userParams.brdpropOutput); err != nil {
		s.Error("Failed verify straps after power-on reset: ", err)
	}

	s.Log("Putting Ti50 into deep sleep")
	th.MustSucceed(i.WaitUntilDeepSleep(ctx, time.Minute), "Ti50 did not sleep")
	th.MustSucceed(i.WaitUntilBooted(ctx), "Ti50 did not wake up")
	if err := checkGSCBrdpropCommandOutput(ctx, i, userParams.brdpropOutput); err != nil {
		s.Error("Failed verify straps after wakeup from deep sleep: ", err)
	}
}

func checkGSCBrdpropCommandOutput(ctx context.Context, i *ti50.CrOSImage, re string) error {
	output, err := i.Command(ctx, "brdprop")
	if err != nil {
		return errors.New("failed to run `brdprop` GSC command")
	}
	matched, err := regexp.MatchString(re, output)
	if err != nil {
		return errors.New("failed to run regexp match to check `brdprop` output")
	}
	if !matched {
		return errors.New("failed to match `brdprop` output, expected pattern = '" + re + "', got = " + output)
	}

	return nil
}
