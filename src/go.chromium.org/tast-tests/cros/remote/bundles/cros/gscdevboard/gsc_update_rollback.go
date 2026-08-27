// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/utils"
	"go.chromium.org/tast/core/testing"
)

type testUpdateRollbackCmd string

const crashCmd testUpdateRollbackCmd = "crash"
const rebootCmd testUpdateRollbackCmd = "reboot"
const rollbackCmd testUpdateRollbackCmd = "rollback"

type testUpdateRollbackConfig struct {
	cmd          testUpdateRollbackCmd
	useDBG       bool
	invalidateRW bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:    GSCUpdateRollback,
		Desc:    "Verify that GSC will rollback image new image crashes multiple times before AP starts up",
		Timeout: 4 * time.Minute,
		Contacts: []string{
			"cros-hwsec@google.com",
			"jettrink@chromium.org",
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr: []string{"group:gsc",
			"gsc_dt_shield", "gsc_ot_fpga_cw310",
			"gsc_image_ti50",
			"gsc_nightly"},
		Fixture: fixture.GSCOpenCCD,
		Params: []testing.Param{{
			Name: "dbg_crash",
			Val: testUpdateRollbackConfig{
				cmd:          crashCmd,
				useDBG:       true,
				invalidateRW: false,
			},
			ExtraAttr: []string{"gsc_h1_shield", "gsc_ot_shield"},
		}, {
			Name: "dbg_crash_invalid_rw",
			Val: testUpdateRollbackConfig{
				cmd:          crashCmd,
				useDBG:       true,
				invalidateRW: true,
			},
			ExtraAttr: []string{"gsc_h1_shield", "gsc_ot_shield"},
		}, {
			Name: "console_reboot_invalid_rw",
			Val: testUpdateRollbackConfig{
				cmd:          rebootCmd,
				useDBG:       false,
				invalidateRW: true,
			},
		}, {
			Name: "console_reboot",
			Val: testUpdateRollbackConfig{
				cmd:          rebootCmd,
				useDBG:       false,
				invalidateRW: false,
			},
		}, {
			Name: "dbg_rollback",
			Val: testUpdateRollbackConfig{
				cmd:          rollbackCmd,
				useDBG:       true,
				invalidateRW: false,
			},
			ExtraAttr: []string{"gsc_h1_shield", "gsc_ot_shield"},
		}},
	})
}

// GSCUpdateRollback verifies that GSC will rollback to previous image.
func GSCUpdateRollback(ctx context.Context, s *testing.State) {
	config := s.Param().(testUpdateRollbackConfig)
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	b := utils.NewDevboardHelper(s, s.FixtValue().(*fixture.Value))
	i := ti50.MustOpenCrOSImage(ctx, b, s, b.TestbedType)
	defer i.Close(ctx)

	f := s.FixtValue().(*fixture.Value)

	// Inform fixture that this test may replace the firmware image in flash.
	th.MustSucceed(f.ImageMayBeUpdatedByTest(), "image may be updated")

	currentImage := f.ImagePath

	_, currentVer, _, _, err := b.GSCToolBinVersion(ctx, currentImage)
	th.MustSucceed(err, "Unable to get version from current image "+currentImage)

	startImage := currentImage
	startVer := currentVer
	targetVer := currentVer
	if config.useDBG {
		// Validate debug image is available
		startImage, err = f.DebugImagePath(ctx)
		if err != nil {
			s.Fatal("DUT must have DBG image")
		}

		_, debugVer, _, _, err := b.GSCToolBinVersion(ctx, startImage)
		th.MustSucceed(err, "Unable to get version from "+startImage)

		if debugVer.Less(currentVer) || debugVer == currentVer {
			s.Fatal("DBG version must be greater than current version")
		}
		startVer = debugVer
		// If the inactive image is invalidated the DBG image should
		// still be running after rollback.
		if config.invalidateRW {
			targetVer = debugVer
		}
	}
	versionChange := targetVer.String() != startVer.String()

	s.Logf("Image under test %s: %s", currentVer, currentImage)
	s.Logf("Initial image %s: %s", startVer, startImage)
	s.Logf("Target rollback version: %s", targetVer)

	s.Log("Enabling CCD mode and resetting")
	tpmBus := b.GscProperties().PreferredTPMBus()
	tpm := b.ResetAndTpmStartupForBus(ctx, i, tpmBus, ti50.CCDModeOn, ti50.FfClamshell)
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")

	// AP turns on so TPM bus will be active
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, true)
	b.GSCToolCommandViaTPM(ctx, tpmBus, startImage)

	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")
	version, err := i.VersionInfo(ctx)
	th.MustSucceed(err, "failed to get version")
	s.Log("Setup GSC")
	s.Logf("RW_A: %+v", version.RwA)
	s.Logf("RW_B: %+v", version.RwB)

	if config.invalidateRW {
		s.Log("Invalidate RW")
		b.WaitForTpmStartup(ctx, tpm)
		err = tpm.TpmvInvalidateInactiveRW()
		th.MustSucceed(err, "failed to send invalidate RW")
		version, err := i.VersionInfo(ctx)
		th.MustSucceed(err, "failed to get version")
		s.Logf("RW_A: %+v", version.RwA)
		s.Logf("RW_B: %+v", version.RwB)
	}

	if version.ActiveRw().Version != startVer.String() {
		s.Fatalf("Unable to flash %s", startImage)
	}

	if config.cmd == "crash" {
		// Turn AP off to simulate crashing Ti50 FW image
		b.GpioSet(ctx, ti50.GpioTi50PltRstL, false)
	}

	earlyPrint := false
	rollbackDetected := false
	lastResetCount := uint32(0)
	attempt := 0
	for ; attempt < 10; attempt++ {
		// Give the device a little more time than normal to reboot since we are
		// using the watchdog reset.
		err := i.CommandImage.WaitUntilBooted(ctx, 8*time.Second)
		th.MustSucceed(err, "GSC revives after attempt %d", attempt)

		version, err = i.VersionInfo(ctx)
		th.MustSucceed(err, "get version info")
		s.Logf("Version info after %s", config.cmd)
		s.Logf("RW_A: %+v", version.RwA)
		s.Logf("RW_B: %+v", version.RwB)

		sysinfo, err := i.Sysinfo(ctx)
		th.MustSucceed(err, "get sysinfo")
		s.Logf("sysinfo on attempt %d: %+v", attempt, sysinfo)

		// Rollback should work after the 1st attempt
		if attempt == 1 && config.cmd == rollbackCmd {
			if version.ActiveRw().Version == targetVer.String() {
				rollbackDetected = true
			}
			break
		}
		if attempt != 0 && lastResetCount == sysinfo.ResetCount {
			// OT doesn't increment the reset count if there is no pending update
			if b.GscProperties().ChipType() != ti50.GscOT && config.invalidateRW {
				s.Errorf("attempt %d: %s did not increment reset count (%d)", attempt, config.cmd, lastResetCount)
			}
		}
		lastResetCount = sysinfo.ResetCount

		if sysinfo.RollbackDetected {
			rollbackDetected = true
			if version.ActiveRw().Version == targetVer.String() {
				break
			}
			earlyPrint = true
			s.Errorf("attempt %d: reset count %d: GSC printed 'Rollback detected' before rollback",
				attempt, lastResetCount)
		}

		if versionChange && version.ActiveRw().Version == targetVer.String() {
			s.Errorf("attempt %d: reset count %d: GSC did not print 'Rollback detected' after rollback",
				attempt, lastResetCount)
			break
		}

		switch config.cmd {
		case crashCmd:
			s.Log("Running crash")
			th.MustSucceed(i.SendDBGConsoleCrashCmd(ctx), "calling crash cmd")
			immediateCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
			defer cancel()
			// We need to find and remove the fatal message from the UART output
			// otherwise other console matches will return an error when they detect
			// the crash output.
			_, _, err = i.ReadSerialSubmatch(immediateCtx, ti50.FatalMsg)
			th.MustSucceed(err, "failed to find fatal reset found in UART")
		case rollbackCmd:
			s.Log("Running GSC rollback")
			th.MustSucceed(i.Rollback(ctx), "failed to send rollback command")
		case rebootCmd:
			s.Log("Rebooting GSC")
			th.MustSucceed(i.Reboot(ctx), "failed to send reboot command")
		default:
			s.Fatalf("Invalid command: %s", config.cmd)

		}
	}

	s.Logf("Reset count after %s commands: %d", config.cmd, attempt)
	if !rollbackDetected {
		if b.TestbedType != ti50.GscH1Shield && config.invalidateRW {
			s.Logf("%s command: invalidRW: Ti50 still responsive after %d resets", config.cmd, attempt)
		} else {
			s.Errorf("%s command: did not detect rollback after %d resets ", config.cmd, attempt)
		}
	} else if config.cmd == rollbackCmd {
		if attempt != 1 {
			s.Errorf("%s command: Reset count %d out of range", config.cmd, attempt)
		}
	} else if attempt < 5 || attempt > 8 {
		s.Errorf("%s command: Reset count %d out of range", config.cmd, attempt)
	}
	if earlyPrint {
		s.Errorf("%s command: actual rollback reset count %d", config.cmd, attempt)
	}

	// Give the device a little more time than normal to reboot since we are
	// using the watchdog reset.
	err = i.CommandImage.WaitUntilBooted(ctx, 8*time.Second)
	th.MustSucceed(err, "GSC revives after %s", config.cmd)

	version, err = i.VersionInfo(ctx)
	th.MustSucceed(err, "get version info")
	s.Logf("Version info final %s", config.cmd)
	s.Logf("RW_A: %+v", version.RwA)
	s.Logf("RW_B: %+v", version.RwB)

	if version.ActiveRw().Version != targetVer.String() {
		s.Errorf("Running %+v not target %s", version.ActiveRw(), targetVer.String())
	}
}
