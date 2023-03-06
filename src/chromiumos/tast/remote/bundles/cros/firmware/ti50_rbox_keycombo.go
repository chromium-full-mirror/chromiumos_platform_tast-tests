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
		Func:    Ti50RboxKeycombo,
		Desc:    "Ti50 firmware update over CCD using gsctool",
		Timeout: 5 * time.Minute,
		Contacts: []string{
			"chromeos-faft@google.com",
			"ti50-core@google.com",
			"jettrink@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware"},
		Fixture:      fixture.Ti50,
	})
}

func Ti50RboxKeycombo(ctx context.Context, s *testing.State) {
	f := s.FixtValue().(*fixture.Value)
	b := utils.NewDevboardHelper(f.DevBoard(), s)
	i := ti50.NewCrOSImage(b)
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}

	s.Log("Restarting ti50 with clamshell straps")
	b.GpioApplyStrap(ctx, ti50.FfClamshell)
	th.MustSucceed(b.Reset(ctx), "Reset board")
	th.MustSucceed(i.WaitUntilBooted(ctx), "Ti50 revives after reboot")

	s.Log("Verifying KSO is passed through when power button not pressed")
	b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, true)
	b.GpioSet(ctx, ti50.GpioTi50EcKso2Inv, true)
	if b.GpioGet(ctx, ti50.GpioTi50Kso2) != false {
		s.Error("GSC should forward asserted GpioTi50EcKso2Inv")
	}

	b.GpioSet(ctx, ti50.GpioTi50EcKso2Inv, false)
	if b.GpioGet(ctx, ti50.GpioTi50Kso2) != true {
		s.Error("GSC should forward de-asserted GpioTi50EcKso2Inv")
	}

	s.Log("Pushing Power button")
	b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, false)
	if b.GpioGet(ctx, ti50.GpioTi50Kso2) != false {
		s.Error("GSC should be asserting KSO low when power button is pressed")
	}

	s.Log("Pushing Refresh key")
	b.GpioSet(ctx, ti50.GpioTi50KsiRefresh, false)

	// TODO(b/262618201) ensure that EC_RST_L pulsed
	// TODO(b/262618201) finish the rest of test for other form factors
	// TODO(b/262618201) ensure that GSC reset key combo works
	// TODO(b/262618201) ensure that AP RO bypass key combo works
	// TODO(b/262618201) ensure that battery disconnect key combo works
	// TODO(b/262618201) ensure that RMA triggered keycombo works
}
