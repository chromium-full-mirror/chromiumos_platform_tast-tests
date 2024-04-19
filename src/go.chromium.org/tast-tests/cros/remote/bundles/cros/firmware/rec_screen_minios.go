// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type recScreenMiniOSTestParams struct {
	miniOSMenuOld  bool
	miniOSPriority string
}

func init() {
	testing.AddTest(&testing.Test{
		Func: RecScreenMiniOS,
		Desc: "Verify DUT can boot to MiniOS",
		Contacts: []string{
			"chromeos-faft@google.com",
			"cienet-firmware@cienet.corp-partner.google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware"},
		HardwareDeps: hwdep.D(hwdep.MiniOS()),
		Fixture:      fixture.NormalMode,
		Params: []testing.Param{{
			Name:              "menu",
			ExtraAttr:         []string{"group:firmware", "firmware_bios", "firmware_level2", "firmware_ro"},
			ExtraRequirements: []string{"sys-fw-0021-v01", "sys-fw-0024-v01"},
			Val: recScreenMiniOSTestParams{
				miniOSMenuOld: false,
			},
		}, {
			Name:              "menu_old",
			ExtraAttr:         []string{"group:firmware", "firmware_bios", "firmware_level2", "firmware_ro"},
			ExtraRequirements: []string{"sys-fw-0021-v01", "sys-fw-0024-v01"},
			Val: recScreenMiniOSTestParams{
				miniOSMenuOld: true,
			},
		}, {
			Name: "priority_minios_a",
			// TODO: When stable, change firmware_unstable to a different attr.
			ExtraAttr: []string{"firmware_unstable"},
			Val: recScreenMiniOSTestParams{
				miniOSPriority: "A",
			},
		}, {
			Name: "priority_minios_b",
			// TODO: When stable, change firmware_unstable to a different attr.
			ExtraAttr: []string{"firmware_unstable"},
			Val: recScreenMiniOSTestParams{
				miniOSPriority: "B",
			},
		}},
		Timeout: 15 * time.Minute,
	})
}

func RecScreenMiniOS(ctx context.Context, s *testing.State) {
	tc := s.Param().(recScreenMiniOSTestParams)
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}

	type miniOSConnectTimeout struct {
		err error
	}
	miniOSConnectTimeoutErr := miniOSConnectTimeout{}

	var restoreMiniOSPriority string
	var err error
	if tc.miniOSPriority != "" {
		s.Log("Getting current MiniOS priority")
		restoreMiniOSPriority, err = h.Reporter.GetMiniOSPriority(ctx)
		if err != nil {
			s.Fatal("Failed to get MiniOS priority: ", err)
		}
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 4*time.Minute)
	defer cancel()

	defer func(ctx context.Context) {
		s.Log("Leaving MiniOS by a warm reset")
		if err := h.Servo.SetPowerState(ctx, servo.PowerStateWarmReset); err != nil {
			s.Fatal("Failed to warm reset DUT: ", err)
		}
		waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
		defer cancelWaitConnect()
		if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
			s.Fatal("Failed to reconnect to dut: ", err)
		}
		if miniOSConnectTimeoutErr.err != nil && h.Config.HasMiniDiagCapability(firmware.CbmemPreservedByAPReset) {
			out, err := h.Reporter.GetCBMEMLogs(ctx, reporters.ConsoleLog)
			if err != nil {
				s.Fatal("Failed to get CBMEM logs: ", err)
			}
			miniosVersionRe := regexp.MustCompile(`cros_minios_version=\d+.\d+.\d+`)
			match := miniosVersionRe.FindStringSubmatch(out)
			if match == nil {
				s.Fatal("DUT might contains an invalid MiniOS or keys did not be pressed on firmware screen")
			} else {
				s.Fatalf("Reconnect timeout is not enough to connect to MiniOS, got %s", match[0])
			}
		}
		if tc.miniOSPriority != restoreMiniOSPriority {
			s.Logf("Restoring current MiniOS priority to %s", restoreMiniOSPriority)
			if err := h.SetMiniOSPriority(ctx, restoreMiniOSPriority); err != nil {
				s.Fatal("Failed to restore MiniOS priority: ", err)
			}
		}
	}(cleanupCtx)

	if err := launchMiniOS(ctx, h, tc.miniOSPriority, tc.miniOSMenuOld); err != nil {
		s.Fatal("Failed to launch MiniOS menu: ", err)
	}
	s.Log("Waiting for DUT to reconnect")
	waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, 90*time.Second)
	defer cancelWaitConnect()
	if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
		miniOSConnectTimeoutErr.err = err
		s.Fatal("Failed to reconnect to DUT: ", err)
	}

	s.Log("Checking if DUT boots to MiniOS")
	miniOSBoot, err := h.Reporter.CheckMiniOSBoot(ctx)
	if err != nil {
		s.Fatal("Failed to check MiniOS boot: ", err)
	}
	if !miniOSBoot {
		s.Fatal("MiniOS boot was unsuccessful")
	}
}

func launchMiniOS(ctx context.Context, h *firmware.Helper, miniosPriority string, miniOSOld bool) error {
	if miniosPriority != "" {
		testing.ContextLogf(ctx, "Setting MiniOS priority to %s", miniosPriority)
		if err := h.SetMiniOSPriority(ctx, miniosPriority); err != nil {
			return errors.Wrap(err, "failed to set MiniOS priority")
		}
	} else {
		testing.ContextLog(ctx, "Using the original MiniOS priority setting")
	}
	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		return errors.Wrap(err, "failed to create mode switcher")
	}
	if err := ms.EnableRecMode(ctx, servo.PowerStateRec, servo.USBMuxOff); err != nil {
		return errors.Wrap(err, "failed to enable recovery mode")
	}
	if err := h.WaitFirmwareScreen(ctx, h.Config.FirmwareScreenRecMode); err != nil {
		return errors.Wrap(err, "failed to get to firmware screen")
	}
	if miniosPriority != "" {
		newbp, err := firmware.NewBypasser(ctx, h)
		if err != nil {
			return errors.Wrap(err, "failed to create a new bypasser")
		}
		if err := newbp.TriggerRecToMiniOS(ctx); err != nil {
			return errors.Wrap(err, "failed to boot to MiniOS")
		}
	} else {
		menuOperator, err := firmware.NewMenuOperator(ctx, h)
		if err != nil {
			return errors.Wrap(err, "failed to create a new menu operator")
		}
		if err := menuOperator.TriggerRecToMiniOS(ctx, miniOSOld); err != nil {
			return errors.Wrap(err, "failed to boot to MiniOS")
		}
	}
	return nil
}
