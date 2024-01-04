// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"time"

	fwCommon "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type fwScreenCloseLidParam struct {
	bootMode     fwCommon.BootMode
	bootToScreen fwCommon.FwScreenType
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         FWScreenCloseLid,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that lid close triggers shutdown during firmware screens",
		Contacts: []string{
			"chromeos-faft@google.com",
			"cienet-firmware@cienet.corp-partner.google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		// TODO: When stable, change firmware_unstable to a different attr.
		Attr:         []string{"group:firmware", "firmware_unstable"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.Lid()),
		Timeout:      15 * time.Minute,
		Params: []testing.Param{{
			Name:    "dev",
			Fixture: fixture.DevMode,
			Val: &fwScreenCloseLidParam{
				bootMode:     fwCommon.BootModeDev,
				bootToScreen: fwCommon.FwDeveloperScreen,
			},
		}, {
			Name:    "to_norm",
			Fixture: fixture.DevMode,
			Val: &fwScreenCloseLidParam{
				bootMode:     fwCommon.BootModeDev,
				bootToScreen: fwCommon.FwToNormScreen,
			},
		}, {
			Name:    "broken",
			Fixture: fixture.NormalMode,
			Val: &fwScreenCloseLidParam{
				bootMode:     fwCommon.BootModeNormal,
				bootToScreen: fwCommon.FwBrokenScreen,
			},
		}},
		// FWScreenCloseLid is not functional in recovery mode. Due to the changes
		// introduced by crrev/c/4371358, this limitation prevents us from verifying that a
		// DUT remains powered off when the lid is closed in recovery mode. Refer to
		// crrev.com/c/4903046 for further discussions.
	})
}

func FWScreenCloseLid(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}
	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}

	param := s.Param().(*fwScreenCloseLidParam)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 4*time.Minute)
	defer cancel()

	defer func(ctx context.Context) {
		if err := h.EnsureDUTBooted(ctx); err != nil {
			s.Fatal("Failed to ensure dut has booted: ", err)
		}
		if err := h.Servo.OpenLid(ctx); err != nil {
			s.Fatal("Failed to set lid state: ", err)
		}
	}(cleanupCtx)

	s.Log("Rebooting to firmware screen")
	if err := rebootToScreen(ctx, h, param.bootToScreen); err != nil {
		s.Fatal("Failed to reboot to firmware screen: ", err)
	}

	if err := rebootDUTWithLidCloseOpen(ctx, h); err != nil {
		s.Fatal("Failed to run shutdown process: ", err)
	}

	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		s.Fatal("Failed to create mode switcher: ", err)
	}

	if param.bootMode == fwCommon.BootModeNormal {
		waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
		defer cancelWaitConnect()
		if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
			s.Fatal("Failed to reconnect to the DUT: ", err)
		}
	} else {
		devModeBypasserParams := firmware.RunBypasser{
			BypasserMethod:        ms.BypassDevMode,
			RepeatBypasser:        true,
			WaitUntilDUTConnected: h.Config.DelayRebootToPing,
		}
		if err := ms.RunBypasserUntilDUTConnected(ctx, devModeBypasserParams); err != nil {
			s.Fatal("Failed to boot through dev mode: ", err)
		}
	}
	if isExpMode, err := h.Reporter.CheckBootMode(ctx, param.bootMode); err != nil {
		s.Fatal("Failed to check boot mode: ", err)
	} else if !isExpMode {
		s.Fatal("Found unexpected boot mode")
	}
}

func rebootToScreen(ctx context.Context, h *firmware.Helper, bootToScreen fwCommon.FwScreenType) error {
	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		return errors.Wrap(err, "failed to create mode switcher")
	}
	switch bootToScreen {
	case fwCommon.FwToNormScreen:
		if err := ms.RebootToFirmwareScreen(ctx, fwCommon.FwDeveloperScreen); err != nil {
			return errors.Wrap(err, "failed to reboot to firmware screen")
		}
		if err := ms.TriggerToNormScreen(ctx); err != nil {
			return errors.Wrap(err, "failed to trigger the to-norm screen")
		}
	default:
		if err := ms.RebootToFirmwareScreen(ctx, bootToScreen); err != nil {
			return errors.Wrap(err, "failed to reboot to firmware screen")
		}
	}
	return nil
}

func rebootDUTWithLidCloseOpen(ctx context.Context, h *firmware.Helper) error {
	if err := h.Servo.CloseLid(ctx); err != nil {
		return errors.Wrap(err, "failed to close DUT lid")
	}
	testing.ContextLog(ctx, "Waiting for powerstate G3")
	if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout, "G3"); err != nil {
		return errors.Wrap(err, "failed to get powerstate G3")
	}
	if err := h.Servo.OpenLid(ctx); err != nil {
		return errors.Wrap(err, "failed to open DUT lid")
	}
	return nil
}
