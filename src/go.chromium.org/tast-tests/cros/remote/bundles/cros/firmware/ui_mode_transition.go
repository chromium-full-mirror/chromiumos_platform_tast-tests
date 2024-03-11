// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"time"

	fwCommon "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         UIModeTransition,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test mode transition via menu navigation instead of keyboard shortcuts",
		Contacts: []string{
			"chromeos-faft@google.com",
			"cienet-firmware@cienet.corp-partner.google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware", "firmware_bios", "firmware_level2"},
		Requirements: []string{"sys-fw-0021-v01", "sys-fw-0024-v01", "sys-fw-0025-v01"},
		Fixture:      fixture.NormalMode,
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Timeout:      15 * time.Minute,
	})
}

func UIModeTransition(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}
	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}
	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		s.Fatal("Creating mode switcher: ", err)
	}
	var bypassDevMode, triggerDevToNormal, triggerRecToDev func(ctx context.Context) error
	switch h.Config.ModeSwitcherType {
	case firmware.KeyboardDevSwitcher:
		newbp, err := firmware.NewBypasser(ctx, h)
		if err != nil {
			s.Fatal("Failed to create a new bypasser: ", err)
		}
		bypassDevMode = newbp.BypassDevMode
		triggerDevToNormal = newbp.TriggerDevToNormal
		triggerRecToDev = newbp.TriggerRecToDev
	default:
		newmbp, err := firmware.NewMenuBypasser(ctx, h)
		if err != nil {
			s.Fatal("Failed to create a new menu bypasser: ", err)
		}
		bypassDevMode = newmbp.BypassDevMode
		triggerDevToNormal = newmbp.TriggerDevToNormal
		triggerRecToDev = newmbp.TriggerRecToDev
	}
	type uiModeTransition int
	const (
		normalToDev uiModeTransition = iota
		devToNormal
	)
	for _, boot := range []struct {
		transition   uiModeTransition
		bypassToBoot func(ctx context.Context) error
		expBootMode  fwCommon.BootMode
	}{
		{
			transition:   normalToDev,
			bypassToBoot: bypassDevMode,
			expBootMode:  fwCommon.BootModeDev,
		},
		{
			transition:   devToNormal,
			bypassToBoot: triggerDevToNormal,
			expBootMode:  fwCommon.BootModeNormal,
		},
	} {
		switch boot.transition {
		case normalToDev:
			if err := ms.EnableRecMode(ctx, servo.PowerStateRec, servo.USBMuxHost); err != nil {
				s.Fatal("Failed to enable recovery mode: ", err)
			}
			s.Logf("Waiting for %s (firmware screen)", h.Config.FirmwareScreen)
			// GoBigSleepLint: Allow time for DUT to reach firmware screen.
			if err := testing.Sleep(ctx, h.Config.FirmwareScreen); err != nil {
				s.Fatal("Failed to wait for firmware screen: ", err)
			}
			if err := triggerRecToDev(ctx); err != nil {
				s.Fatal("Failed to trigger recovery to dev: ", err)
			}
		case devToNormal:
			if err := h.Servo.SetPowerState(ctx, servo.PowerStateReset); err != nil {
				s.Fatal("Faild to reset DUT: ", err)
			}
		}
		s.Logf("Waiting for %s (firmware screen)", h.Config.FirmwareScreen)
		// GoBigSleepLint: Sleep for model specific time.
		if err := testing.Sleep(ctx, h.Config.FirmwareScreen); err != nil {
			s.Fatal("Failed to wait for firmware screen: ", err)
		}
		if err := boot.bypassToBoot(ctx); err != nil {
			s.Fatal("Failed to bypass firmware screen: ", err)
		}
		if boot.transition == devToNormal && (h.Config.ModeSwitcherType == firmware.KeyboardDevSwitcher || h.Config.ModeSwitcherType == firmware.TabletDetachableSwitcher) {
			s.Log("Pressing Ctrl+U on the to_norm_confirmed screen")
			if err := h.Servo.KeypressWithDuration(ctx, servo.CtrlU, servo.DurTab); err != nil {
				s.Fatal("Failed to press Ctrl+U on the to_norm_confirmed screen: ", err)
			}
		}
		waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
		defer cancelWaitConnect()
		if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
			s.Fatal("Failed to reconnect to the DUT: ", err)
		}
		if isExpMode, err := h.Reporter.CheckBootMode(ctx, boot.expBootMode); err != nil {
			s.Fatal("Failed to check boot mode: ", err)
		} else if !isExpMode {
			s.Fatal("Found unexpected boot mode")
		}
	}
}
