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

	board := f.DevBoard()
	i := ti50.NewCrOSImage(board)

	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}

	testing.ContextLog(ctx, "Restarting ti50 with clamshell straps")
	th.MustSucceed(board.GpioApplyStrap(ctx, ti50.FfClamshell), "Set FfClamshell form factor")
	th.MustSucceed(board.Reset(ctx), "Reset board")
	th.MustSucceed(i.WaitUntilBooted(ctx), "Ti50 revives after reboot")

	testing.ContextLog(ctx, "Verifying KSO is passed through when power button not pressed")
	th.MustSucceed(board.GpioWrite(ctx, ti50.GpioTi50PowerBtnL, true), "Assert GpioTi50PowerBtnL")
	th.MustSucceed(board.GpioWrite(ctx, ti50.GpioTi50EcKso2Inv, true), "Assert GpioTi50EcKso2Inv")
	if th.MustSucceedBool(board.GpioRead(ctx, ti50.GpioTi50Kso2)) != false {
		s.Error("GSC should forward asserted GpioTi50EcKso2Inv")
	}

	th.MustSucceed(board.GpioWrite(ctx, ti50.GpioTi50EcKso2Inv, false), "De-assert GpioTi50EcKso2Inv")
	if th.MustSucceedBool(board.GpioRead(ctx, ti50.GpioTi50Kso2)) != true {
		s.Error("GSC should forward de-asserted GpioTi50EcKso2Inv")
	}

	testing.ContextLog(ctx, "Pushing Power button")
	th.MustSucceed(board.GpioWrite(ctx, ti50.GpioTi50PowerBtnL, false), "De-assert GpioTi50PowerBtnL")
	if th.MustSucceedBool(board.GpioRead(ctx, ti50.GpioTi50Kso2)) != false {
		s.Error("GSC should be asserting KSO low when power button is pressed")
	}

	testing.ContextLog(ctx, "Pushing Refresh key")
	th.MustSucceed(board.GpioWrite(ctx, ti50.GpioTi50KsiRefresh, false), "De-assert GpioTi50KsiRefresh")

	// TODO(b/262618201) ensure that EC_RST_L pulsed
	// TODO(b/262618201) finish the rest of test for other form factors
	// TODO(b/262618201) ensure that GSC reset key combo works
	// TODO(b/262618201) ensure that AP RO bypass key combo works
	// TODO(b/262618201) ensure that battery disconnect key combo works
	// TODO(b/262618201) ensure that RMA triggered keycombo works
}
