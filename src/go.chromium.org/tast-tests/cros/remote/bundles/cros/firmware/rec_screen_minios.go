// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type recScreenMiniOSTestParams struct {
	miniOSMenuOld  bool
	miniOSPriority string
	kbShortcutBoot bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func: RecScreenMiniOS,
		Desc: "Verify DUT can boot to MiniOS",
		Contacts: []string{
			"chromeos-faft@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		TestBedDeps:  append([]string{tbdep.ServoUSBState("NORMAL")}, tbdep.ServoPresentAndWorking...),
		Attr:         []string{"group:firmware", "firmware_bios", "firmware_meets_kpi", "firmware_stressed", "firmware_bios_ro", "firmware_bios_rw"},
		HardwareDeps: hwdep.D(hwdep.MiniOS()),
		Fixture:      fixture.NormalMode,
		Params: []testing.Param{{
			Name: "menu",
			Val: recScreenMiniOSTestParams{
				miniOSMenuOld:  false,
				kbShortcutBoot: false,
			},
		}, {
			Name: "menu_old",
			Val: recScreenMiniOSTestParams{
				miniOSMenuOld:  true,
				kbShortcutBoot: false,
			},
		}, {
			Name: "priority_minios_a",
			Val: recScreenMiniOSTestParams{
				miniOSPriority: "A",
				kbShortcutBoot: true,
			},
		}, {
			Name: "priority_minios_b",
			Val: recScreenMiniOSTestParams{
				miniOSPriority: "B",
				kbShortcutBoot: true,
			},
		}},
		Timeout: 120 * time.Minute,
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

	var state firmware.CheckAndSetServoCharger = h.CheckServoChargerBeforeBootingFromUSB(ctx)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 4*time.Minute)
	defer cancel()

	defer func(ctx context.Context) {
		if miniOSConnectTimeoutErr.err != nil {
			cs := s.CloudStorage()
			if err := restoreDUTConnectionWithUSB(ctx, h, cs, &state); err != nil {
				s.Error("Failed to restore DUT connection with USB: ", err)
			} else {
				saveLogPath := filepath.Join(s.OutDir(), "check_minios_corrupted.log")
				if miniOSCorrupted, err := checkMiniOSCorruptedAndSaveLog(ctx, h, saveLogPath); err != nil {
					s.Error("Failed to check if MiniOS is corrupted: ", err)
				} else if miniOSCorrupted {
					s.Error("MiniOS is corrupted")
				} else {
					s.Error("DUT remained in recovery select screen, keypress might not work properly")
				}
			}
		}
		s.Log("Warm resetting DUT")
		if err := h.Servo.SetPowerState(ctx, servo.PowerStateWarmReset); err != nil {
			s.Error("Failed to warm reset DUT: ", err)
		}

		waitUnreachableCtx, cancelUnreachable := context.WithTimeout(ctx, 2*time.Minute)
		defer cancelUnreachable()
		if err := h.DUT.WaitUnreachable(waitUnreachableCtx); err != nil {
			s.Error("Failed to wait DUT unreachable: ", err)
		}

		waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
		defer cancelWaitConnect()
		if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
			s.Error("Failed to reconnect to dut: ", err)
		}
		if state.RemoveServoChargerRequired && !state.IsServoChargerConnected {
			if err := h.SetDUTPower(ctx, true); err != nil {
				s.Fatal("Failed to connect charger: ", err)
			}
			state.IsServoChargerConnected = true
			waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, 2*time.Minute)
			defer cancelWaitConnect()
			if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
				s.Fatal("Failed to reconnect to the DUT: ", err)
			}
		}
		if tc.miniOSPriority != restoreMiniOSPriority {
			s.Logf("Restoring current MiniOS priority to %s", restoreMiniOSPriority)
			if err := h.SetMiniOSPriority(ctx, restoreMiniOSPriority); err != nil {
				s.Fatal("Failed to restore MiniOS priority: ", err)
			}
		}
	}(cleanupCtx)

	if tc.miniOSPriority != "" {
		s.Logf("Setting MiniOS priority to %s", tc.miniOSPriority)
		if err := h.SetMiniOSPriority(ctx, tc.miniOSPriority); err != nil {
			s.Fatal("Failed to set MiniOS priority: ", err)
		}
	} else {
		s.Log("Using the original MiniOS priority setting")
	}

	if err := h.LaunchMiniOS(ctx, tc.kbShortcutBoot, tc.miniOSMenuOld); err != nil {
		s.Fatal("Failed to launch MiniOS menu: ", err)
	}
	s.Log("Waiting for DUT to reconnect")
	waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, 360*time.Second)
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

func restoreDUTConnectionWithUSB(ctx context.Context, h *firmware.Helper, cs *testing.CloudStorage, state *firmware.CheckAndSetServoCharger) error {
	if err := h.SetupUSBKey(ctx, cs); err != nil {
		return errors.Wrap(err, "usbkey not working")
	}
	if state.RemoveServoChargerRequired {
		if err := h.SetDUTPower(ctx, false); err != nil {
			return errors.Wrap(err, "failed to remove charger")
		}
		state.IsServoChargerConnected = false
		// GoBigSleepLint: Wait for a while between removing the charger and
		// booting the DUT from USB to prevent USB disconnected issues.
		if err := testing.Sleep(ctx, 5*time.Second); err != nil {
			return errors.Wrap(err, "failed to sleep")
		}
	}
	testing.ContextLog(ctx, "Setting DFP mode")
	if err := h.Servo.SetDUTPDDataRole(ctx, servo.DFP); err != nil {
		testing.ContextLogf(ctx, "Failed to set pd data role to DFP: %.400s", err)
	}
	testing.ContextLog(ctx, "Inserting a valid USB to DUT")
	if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxDUT); err != nil {
		return errors.Wrap(err, "failed to insert a valid USB to the DUT")
	}
	testing.ContextLog(ctx, "Checking if DUT boots from USB")
	if err := h.WaitDUTConnectDuringBootFromUSB(ctx, true); err != nil {
		return errors.Wrap(err, "failed to boot from USB")
	}
	return nil
}

func checkMiniOSCorruptedAndSaveLog(ctx context.Context, h *firmware.Helper, logPath string) (bool, error) {
	out, err := h.Reporter.GetCBMEMLogs(ctx)
	if err != nil {
		return false, errors.Wrap(err, "failed to run cbmem command")
	}
	if err := h.ScanWithoutExpectedSequenceInSource(ctx, out, []string{"Failed to boot from MiniOS"}); err != nil {
		if saveLogErr := os.WriteFile(logPath, []byte(out), 0666); saveLogErr != nil {
			return false, errors.Wrap(err, "failed to write firmware log")
		}
		return false, nil
	}
	return true, nil
}
