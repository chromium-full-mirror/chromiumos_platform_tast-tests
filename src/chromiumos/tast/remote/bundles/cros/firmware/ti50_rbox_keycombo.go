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

	board, err := f.DevBoard(ctx, 10000, time.Second)
	if err != nil {
		s.Fatal("Could not get board: ", err)
	}

	i := ti50.NewCrOSImage(board)

	if _, err = board.OpenTitanToolCommand(ctx, "transport", "init"); err != nil {
		s.Fatal("Failed to reset gpio to good state: ", err)
	}

	testing.ContextLog(ctx, "Restarting ti50 with clamshell straps")
	if err := board.GpioApplyStrap(ctx, ti50.FfClamshell); err != nil {
		s.Fatalf("Failed to set %s form factor: %s", ti50.FfClamshell, err)
	}
	if err := board.Reset(ctx); err != nil {
		s.Fatal("Failed to reset: ", err)
	}
	if err := i.WaitUntilBooted(ctx); err != nil {
		s.Fatal("Ti50 did revive after reboot: ", err)
	}

	testing.ContextLog(ctx, "Verifying KSO is passed through when power button not pressed")
	utils.MustSetGpio(ctx, board, s, ti50.GpioTi50PowerBtnL, true)
	utils.MustSetGpio(ctx, board, s, ti50.GpioTi50EcKso2Inv, true)
	if utils.MustGetGpio(ctx, board, s, ti50.GpioTi50Kso2) != false {
		s.Error("GSC did not forward asserted GpioTi50EcKso2Inv")
	}

	utils.MustSetGpio(ctx, board, s, ti50.GpioTi50EcKso2Inv, false)
	if utils.MustGetGpio(ctx, board, s, ti50.GpioTi50Kso2) != true {
		s.Error("GSC did not forward de-asserted GpioTi50EcKso2Inv")
	}

	testing.ContextLog(ctx, "Pushing Power button")
	utils.MustSetGpio(ctx, board, s, ti50.GpioTi50PowerBtnL, false)

	// Ensure that that KSO is is low/asserted when power button is pressed
	if utils.MustGetGpio(ctx, board, s, ti50.GpioTi50Kso2) != false {
		s.Error("GSC is not asserting KSO low when power button is pressed")
	}

	testing.ContextLog(ctx, "Pushing Refresh key")
	utils.MustSetGpio(ctx, board, s, ti50.GpioTi50KsiRefresh, false)

	// TODO(b/262618201) ensure that EC_RST_L pulsed
	// TODO(b/262618201) finish the rest of test for other form factors
	// TODO(b/262618201) ensure that GSC reset key combo works
	// TODO(b/262618201) ensure that AP RO bypass key combo works
	// TODO(b/262618201) ensure that battery disconnect key combo works
	// TODO(b/262618201) ensure that RMA triggered keycombo works
}
