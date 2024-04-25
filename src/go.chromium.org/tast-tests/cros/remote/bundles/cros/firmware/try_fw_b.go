// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"time"

	fwCommon "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/common/servo"
	fwUtils "go.chromium.org/tast-tests/cros/remote/bundles/cros/firmware/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: TryFWB,
		Desc: "Servo based boot firmware B test",
		Contacts: []string{
			"chromeos-faft@google.com",
			"pf@semihalf.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		// TODO: When stable, change firmware_unstable to a different attr.
		Attr:         []string{"group:firmware", "firmware_unstable", "firmware_usb"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Timeout:      15 * time.Minute,
		SoftwareDeps: []string{"crossystem"},
		ServiceDeps:  []string{"tast.cros.firmware.UtilsService"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		Params: []testing.Param{
			{
				Name:    "normal_mode",
				Fixture: fixture.NormalMode,
				Val:     "normal",
			},
			{
				Name:    "dev_mode",
				Fixture: fixture.DevModeGBB,
				Val:     "developer",
			},
		},
	})
}

func TryFWB(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}

	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		s.Fatal("Creating mode switcher: ", err)
	}

	s.Log("Set the USB Mux direction to Host")
	if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxHost); err != nil {
		s.Fatal(err, "failed to set the USB Mux direction to the Host")
	}

	s.Log("Start test with FW A")
	if err := pollToChangeFWVariant(ctx, h, ms, fwCommon.RWSectionA); err != nil {
		s.Fatal("Failed to change FW variant: ", err)
	}

	s.Log("Switch firmware to B variant, reboot")
	if err := pollToChangeFWVariant(ctx, h, ms, fwCommon.RWSectionB); err != nil {
		s.Fatal("Failed to change FW variant: ", err)
	}

	if err := ms.ModeAwareReboot(ctx, firmware.WarmReset); err != nil {
		s.Fatal("Failed to perform mode aware reboot: ", err)
	}

	finalFWVer := "B"

	if isFWVerCorrect, err := h.Reporter.CheckFWVersion(ctx, finalFWVer); err != nil {
		s.Fatal(err, "failed to check a firmware version")
	} else if !isFWVerCorrect {
		s.Fatalf("Failed to boot into the %s firmware", finalFWVer)
	}
}

func pollToChangeFWVariant(ctx context.Context, h *firmware.Helper, ms *firmware.ModeSwitcher, fwVar fwCommon.RWSection) error {
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := fwUtils.ChangeFWVariant(ctx, h, ms, fwVar); err != nil {
			if _, ok := err.(*fwUtils.FWVariantErr); ok {
				return err
			}
			return testing.PollBreak(err)

		}
		return nil
	}, &testing.PollOptions{Timeout: 300 * time.Second}); err != nil {
		return errors.Wrap(err, "failed to change FW variant")
	}
	return nil
}
