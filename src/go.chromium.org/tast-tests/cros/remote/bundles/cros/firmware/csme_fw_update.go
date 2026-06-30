// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"strconv"
	"time"

	"go.chromium.org/chromiumos/config/go/api"
	fwCommon "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	fwpb "go.chromium.org/tast-tests/cros/services/cros/firmware"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CsmeFwUpdate,
		Desc:         "Verifies that CSME RW firmware sync process using `forced cse sync` firmware feature",
		Contacts:     []string{"chromeos-firmware@google.com", "digehlot@google.com"},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		HardwareDeps: hwdep.D(hwdep.CPUSocFamily("intel")),
		SoftwareDeps: []string{"csme_update"},
		ServiceDeps:  []string{"tast.cros.firmware.BiosService"},

		TestBedDeps: tbdep.ServoPresentAndWorking,
		Attr:        []string{"group:firmware", "firmware_bios", "firmware_stressed", "firmware_bios_ro", "firmware_bios_rw"},
		Vars:        []string{"firmware_branch", "ro_versions"},
		Data:        []string{"shipped-firmwares.json"},
		Timeout:     10 * time.Minute,
		Params: []testing.Param{
			{
				Name:      "normal",
				ExtraAttr: []string{"firmware_meets_kpi"},
				Fixture:   fixture.NormalMode,
			},
			{
				Name:    "dev",
				Fixture: fixture.DevModeGBB,
			},
		},
	})
}

// CsmeFwUpdate tests csme rw firmware update feature by changing the me_rw
// image in firmware main regions with a different version
func CsmeFwUpdate(ctx context.Context, s *testing.State) {
	/*
	 * Leverage the firmware feature "CSE force sync" to test the CSE sync operation,
	 * if vboot CBFS integration is enabled.
	 */
	vbootCbfs := s.Features("").Hardware.HardwareFeatures.FwConfig.VbootCbfsIntegration == api.HardwareFeatures_PRESENT
	if vbootCbfs {
		s.Log("Voot CBFS sync is enabled, performing forced cse sync")
		if err := performForcedCseSync(ctx, s); err != nil {
			s.Fatal("Forced CSE sync failed: ", err)
		}
		return
	}
	if err := performForcedCseSync(ctx, s); err != nil {
		s.Fatal("Forced CSE sync failed: ", err)
	}

}

func performForcedCseSync(ctx context.Context, s *testing.State) error {
	h := s.FixtValue().(*fixture.Value).Helper
	/*
	 * Force test CSE firmware update scenario if below conditions are being met:
	 *  - CSE Update not required
	 *  - VB2_GBB_FLAG_FORCE_CSE_SYNC gbb flag is set,
	 *  - CSE FW is in RO
	 */

	// clear event log
	if err := h.Reporter.ClearEventlog(ctx); err != nil {
		return errors.Wrap(err, "failed to clear event log")
	}

	/*
	 * The EC may reboot and reset the clock during fixture mode switch. Adding a reboot
	 * ping delay ensures that the EC has been up for more than a complete boot cycle time.
	 */
	s.Logf("Sleeping for %s (DelayRebootToPing) ", h.Config.DelayRebootToPing)
	if err := testing.Sleep(ctx, h.Config.DelayRebootToPing); err != nil {
		s.Fatalf("Failed to sleep for %s: %v", h.Config.DelayRebootToPing, err)
	}

	s.Log("Enabling GBB flag for forced CSE sync")
	req := fwpb.GBBFlagsState{Set: []fwpb.GBBFlag{fwpb.GBBFlag_FORCE_CSE_SYNC}}
	if _, err := fwCommon.ClearAndSetGBBFlags(ctx, h.DUT, &req); err != nil {
		return errors.Wrap(err, "failed to enable gbb for forced cse sync")
	}

	// Get initial boot ID and EC uptime
	initialBootID, initialEcUptime := getBootIdEcUptime(ctx, s)

	s.Log("Performing EC reboot")
	if err := h.Servo.RunECCommand(ctx, "reboot"); err != nil {
		return errors.Wrap(err, "failed to reboot EC")
	}
	if err := h.WaitConnect(ctx); err != nil {
		return errors.Wrap(err, "device is not responding post EC reboot")
	}

	// Get initial boot ID and EC uptime post EC reset
	currBootID, currEcUptime := getBootIdEcUptime(ctx, s)

	// The boot ID should have changed after the reboot.
	if initialBootID == currBootID {
		s.Fatal("EC reboot failed, the boot id remains same")
	}

	/*
	 * Given that the EC clock resets upon rebooting, it's expected that the current EC
	 * time won't exceed the initial captured boot time.
	 */
	if initialEcUptime < currEcUptime {
		s.Fatal("EC reboot failed, the EC clock is not reset")
	}

	s.Log("Forced CSE sync successful")
	return nil
}

func getBootIdEcUptime(ctx context.Context, s *testing.State) (string, int64) {
	h := s.FixtValue().(*fixture.Value).Helper

	// Get initial boot ID.
	currBootID, err := h.Reporter.BootID(ctx)
	if err != nil {
		s.Fatal("Failed to get boot id: ", err)
	}
	currEcUptime, err := getEcTime(ctx, h)
	if err != nil {
		s.Fatal("Failed to read EC clock: ", err)
	}
	return currBootID, currEcUptime
}

func getEcTime(ctx context.Context, h *firmware.Helper) (int64, error) {
	var ecTime int64
	err := testing.Poll(ctx, func(ctx context.Context) error {
		result, err := h.Servo.RunECCommandGetOutputNoConsoleLogs(ctx, "gettime", []string{`Time:\s+0x(\S+)\s`})
		if err != nil {
			return errors.Wrap(err, "failed to get ec time")
		}
		ecTime, err = strconv.ParseInt(result[0][1], 16, 64)
		if err != nil {
			return errors.Wrap(err, "could not parse")
		}
		return nil
	}, &testing.PollOptions{Timeout: 30 * time.Second})
	return ecTime, err
}
