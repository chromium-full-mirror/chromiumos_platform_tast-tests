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

	board := f.DevBoard()
	i := ti50.NewCrOSImage(board)

	testing.ContextLog(ctx, "Restarting ti50 with SPI straps")
	if err := board.GpioApplyStrap(ctx, ti50.TpmSpi); err != nil {
		s.Fatalf("Failed to set TPM to SPI: %s", err)
	}
	if err := board.Reset(ctx); err != nil {
		s.Fatal("Failed to reset: ", err)
	}
	if err := i.WaitUntilBooted(ctx); err != nil {
		s.Fatal("Ti50 did revive after reboot: ", err)
	}

	// Tell Ti50 that the AP came out of reset.  This will cause Ti50 to start responding to
	// TPM commands.
	utils.MustSetGpio(ctx, board, s, ti50.GpioTi50PltRstL, true)

	data, err := board.OpenTitanToolCommand(ctx, "spi", "tpm", "read-register", ti50.TpmRegDidVid)
	if err != nil {
		s.Fatal("OpenTitanToolCommand: ", err)
	}

	if data["hexdata"].(string) != ti50TpmDidVid {
		s.Error("Unexpected TPM DID_VID: ", data["hexdata"])
	}
}
