// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	common "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/common/flashrom"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type recCacheBootKeysParam struct {
	bootMode common.BootMode
	numIters int
}

func init() {
	testing.AddTest(&testing.Test{
		Func: RecoveryCacheBootKeys,
		Desc: "Verify MRC cache used when available, and memory training occurs when cache is invalidated",
		Contacts: []string{
			"chromeos-faft@google.com",
			"tij@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware", "firmware_usb"},
		Vars:         []string{"firmware.skipFlashUSB"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		SoftwareDeps: []string{"has_recovery_mrc_cache"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		Params: []testing.Param{
			{
				Name:      "normal",
				Fixture:   fixture.NormalMode,
				ExtraAttr: []string{"firmware_unstable"},
				Timeout:   15 * time.Minute,
				Val: recCacheBootKeysParam{
					bootMode: common.BootModeNormal,
					numIters: 1,
				},
			}, {
				Name:      "dev",
				Fixture:   fixture.DevModeGBB,
				ExtraAttr: []string{"firmware_unstable"},
				Timeout:   15 * time.Minute,
				Val: recCacheBootKeysParam{
					bootMode: common.BootModeDev,
					numIters: 1,
				},
			},
			{
				Name:      "normal_stress",
				Fixture:   fixture.NormalMode,
				ExtraAttr: []string{"firmware_stress"},
				Timeout:   120 * time.Minute,
				Val: recCacheBootKeysParam{
					bootMode: common.BootModeNormal,
					numIters: 15,
				},
			}, {
				Name:      "dev_stress",
				Fixture:   fixture.DevModeGBB,
				ExtraAttr: []string{"firmware_stress"},
				Timeout:   120 * time.Minute,
				Val: recCacheBootKeysParam{
					bootMode: common.BootModeDev,
					numIters: 15,
				},
			}},
	})
}

func RecoveryCacheBootKeys(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	params := s.Param().(recCacheBootKeysParam)
	bootMode := params.bootMode
	numIters := params.numIters

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to get config: ", err)
	}

	// This is to run the shutdown defer right after, as we no longer need flashrom after this command.
	func() {
		var flashromConfig flashrom.Config
		flashromInstance, ctx, shutdown, _, err := flashromConfig.
			FlashromInit("").
			SetDut(h.DUT).
			ProgrammerInit(flashrom.ProgrammerHost, "").
			Probe(ctx)
		defer func() {
			if err := shutdown(); err != nil {
				s.Error("Failed to shutdown flashromInstance: ", err)
			}
		}()
		if err != nil {
			s.Fatal("Flashrom probe failed, unable to build flashrom instance: ", err)
		}

		// The contents of the section are irrelevant, we just want to make sure it exists.
		// flashromInstance.Read reports no "SUCCESS" in the output as an error.
		if _, err := flashromInstance.Read(ctx, "/dev/null", []string{"RECOVERY_MRC_CACHE"}); err != nil {
			s.Fatal("Failed to check for MRC section in flash")
		}
	}()

	skipFlashUSB := false
	if skipFlashUSBStr, ok := s.Var("firmware.skipFlashUSB"); ok {
		var err error
		skipFlashUSB, err = strconv.ParseBool(skipFlashUSBStr)
		if err != nil {
			s.Fatalf("Invalid value for var firmware.skipFlashUSB: got %q, want true/false", skipFlashUSBStr)
		}
	}
	cs := s.CloudStorage()
	if skipFlashUSB {
		cs = nil
	}
	if err := h.SetupUSBKey(ctx, cs); err != nil {
		s.Fatal("USBKey not working: ", err)
	}

	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		s.Fatal("Creating mode switcher: ", err)
	}

	// Boot to recovery mode once and back to make sure the memory training cache was created
	// so the first part of the test can verify the cache gets used.
	s.Log("Rebooting to recovery mode")
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateRec); err != nil {
		s.Fatal("Failed to set power_state to rec: ", err)
	}
	s.Log("Wait for powerstate S0")
	if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout, "S0"); err != nil {
		s.Fatal("Failed to get S0 powerstate")
	}

	s.Log("Rebooting to test boot mode: ", bootMode)
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateReset); err != nil {
		s.Fatal("Failed to set power_state to rec: ", err)
	}
	waitConnectCtx, cancel := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
	defer cancel()
	if err := h.WaitConnect(waitConnectCtx); err != nil {
		s.Fatal("Failed to connect to DUT: ", err)
	}

	for i := 0; i < numIters; i++ {
		s.Logf("-------Starting iteration %d of %d-------", i+1, numIters)

		// From original autotest:
		// With EFS, when the EC is in RO and we do a recovery boot, it
		// causes the EC to do a soft reboot, which will in turn cause
		// a PMIC reset (double reboot) on SKL/KBL power architectures.
		// This causes us to lose the request to do MRC training for
		// this test.  The solution is to make sure that the EC is in
		// RW before doing a recovery boot to ensure that the double
		// reboot does not occur and information/requests are not lost.
		if err := moveECToRW(ctx, h, ms, bootMode); err != nil {
			s.Fatal("EC not in RW: ", err)
		}

		// Boot back to recovery, expect the cache to be used.
		s.Log("Rebooting to recovery mode")
		if err := ms.RebootToMode(ctx, common.BootModeRecovery); err != nil {
			s.Fatal("Failed to reboot to USB in recovery mode: ", err)
		}

		cbmemLog, err := h.DUT.Conn().CommandContext(ctx, "sh", "-c", "cbmem -1 | grep MRC").Output(ssh.DumpLogOnError)
		if err != nil {
			s.Fatal("Failed to check logs for previous boot on cbmem: ", err)
		}

		msg1 := `MRC: Hash comparison successful\. Using data from RECOVERY_MRC_CACHE`
		msg2 := `MRC: Hash idx 0x100b comparison successful`
		match := regexp.MustCompile(fmt.Sprintf("(%s)|(%s)", msg1, msg2)).FindStringSubmatch(string(cbmemLog))
		if match == nil {
			s.Fatal("Recovery cache was not used, cbmem had following logs about MRC: ", string(cbmemLog))
		}
		s.Log("Found expected messages in cbmem log: ", match[0])

		// Reset test to start from boot mode that is being tested.
		s.Logf("Rebooting to %s mode", bootMode)
		if err := ms.RebootToMode(ctx, bootMode, firmware.AllowGBBForce); err != nil {
			s.Fatalf("Failed to reboot back to %v mode: %v", bootMode, err)
		}

		// Force memory training on recovery mode boot.
		if err := ms.RebootToMode(ctx, common.BootModeRecovery, firmware.RecoveryForceMRCBoot); err != nil {
			s.Fatal("Failed to reboot to USB with rec_force_mrc boot: ", err)
		}

		cbmemLog, err = h.DUT.Conn().CommandContext(ctx, "sh", "-c", "cbmem -1 | grep MRC").Output(ssh.DumpLogOnError)
		if err != nil {
			s.Fatal("Failed to check logs for previous boot on cbmem: ", err)
		}

		match = regexp.MustCompile(`MRC: cache data \'RECOVERY_MRC_CACHE\' needs update`).FindStringSubmatch(string(cbmemLog))
		if match == nil {
			s.Fatal("Recovery cache was not updated, cbmem had following logs about MRC: ", string(cbmemLog))
		}
		s.Log("Found expected messages in cbmem log: ", match[0])

		s.Logf("Rebooting back to %s mode", bootMode)
		if err := ms.RebootToMode(ctx, bootMode, firmware.AllowGBBForce); err != nil {
			s.Fatalf("Failed to reboot back to %v mode: %v", bootMode, err)
		}
	}

}

func moveECToRW(ctx context.Context, h *firmware.Helper, ms *firmware.ModeSwitcher, bootMode common.BootMode) error {
	activeCopy, err := h.Servo.GetString(ctx, "ec_active_copy")
	if err != nil {
		return errors.Wrap(err, "failed to get EC active copy")
	}
	if strings.HasPrefix(activeCopy, "RW") {
		// EC already in RW.
		return nil
	}

	testing.ContextLogf(ctx, "EC active copy %q, want RW, perform sysjump", activeCopy)
	if err := h.Servo.RunECCommand(ctx, "sysjump RW"); err != nil {
		return errors.Wrap(err, "failed to send 'sysjump RW' to EC")
	}

	activeCopy, err = h.Servo.GetString(ctx, "ec_active_copy")
	if err != nil {
		return errors.Wrap(err, "failed to get EC active copy")
	}
	if !strings.HasPrefix(activeCopy, "RW") {
		return errors.Wrapf(err, "EC booted to %s, expected to be in RW", activeCopy)
	}

	if err := ms.RebootToMode(ctx, bootMode, firmware.AllowGBBForce); err != nil {
		return errors.Wrapf(err, "failed to reboot back to %v mode", bootMode)
	}
	waitConnectCtx, cancel := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
	defer cancel()
	if err := h.WaitConnect(waitConnectCtx); err != nil {
		return errors.Wrap(err, "failed to connect to DUT")
	}

	return nil
}
