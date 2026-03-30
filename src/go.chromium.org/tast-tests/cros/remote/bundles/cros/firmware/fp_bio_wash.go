// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/dutfs"
	"go.chromium.org/tast-tests/cros/remote/firmware/fingerprint"
	"go.chromium.org/tast-tests/cros/remote/firmware/fingerprint/rpcdut"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: FpBioWash,
		Desc: "Validate bio_wash behavior",
		Contacts: []string{
			"chromeos-fingerprint@google.com",
			"josienordrum@google.com", // Test author
			"tomhughes@chromium.org",
		},
		// ChromeOS > Platform > Services > Fingerprint
		BugComponent: "b:782045",
		Attr:         []string{"group:fingerprint-cq", "group:fingerprint-release"},
		Timeout:      10 * time.Minute,
		SoftwareDeps: []string{"biometrics_daemon"},
		HardwareDeps: hwdep.D(hwdep.Fingerprint()),
		ServiceDeps:  []string{"tast.cros.platform.UpstartService", dutfs.ServiceName},
		Vars:         []string{"servo"},
	})
}

func FpBioWash(ctx context.Context, s *testing.State) {
	d, err := rpcdut.NewRPCDUT(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect RPCDUT: ", err)
	}
	defer d.Close(ctx)

	servoSpec, ok := s.Var("servo")
	if !ok {
		servoSpec = ""
	}
	// HW wp must be disabled to flash_fp_mcu.
	firmwareFile, err := fingerprint.NewMPFirmwareFile(ctx, d)
	if err != nil {
		s.Fatal("Failed to create MP firmwareFile: ", err)
	}
	t, err := fingerprint.NewFirmwareTest(ctx, d, servoSpec, s.OutDir(), firmwareFile, false, false)
	if err != nil {
		if strings.Contains(err.Error(), "failed to connect to servo") {
			s.Error("Test did not run")
		}
		s.Fatal("Failed to create new firmware test: ", err)
	}
	ctxForCleanup := ctx
	defer func() {
		if err := t.Close(ctxForCleanup); err != nil {
			s.Fatal("Failed to clean up: ", err)
		}
	}()
	ctx, cancel := ctxutil.Shorten(ctx, t.CleanupTime())
	defer cancel()

	// This test requires a forced flash without entropy
	// initialization to clear entropy.
	testing.ContextLog(ctx, "Force flashing original FP firmware")
	if err := fingerprint.FlashFirmware(ctx, d, t.FirmwareFile().FilePath, t.NeedsRebootAfterFlashing()); err != nil {
		s.Fatal("Failed to flash original FP firmware: ", err)
	}

	// Wait for FPMCU to boot to RW. Fail if it does not.
	testing.ContextLog(ctx, "Waiting for FPMCU to reboot to RW")
	if err := fingerprint.WaitForRunningFirmwareImage(ctx, d.DUT(), fingerprint.ImageTypeRW); err != nil {
		s.Fatal("Failed to boot to RW image: ", err)
	}

	testing.ContextLog(ctx, "Saving initial rollback state")
	initialRollback, err := fingerprint.RollbackInfo(ctx, d.DUT())
	if err != nil {
		s.Fatal("Failed to get initial rollback state: ", err)
	}
	testing.ContextLogf(ctx, "Initial rollback block ID: %d", initialRollback.BlockID)

	// Enable hardware write protect first.
	testing.ContextLog(ctx, "Enabling hardware write protect")
	if err := t.Servo().Servo().SetFWWPState(ctx, servo.FWWPStateForceOn); err != nil {
		s.Fatal("Failed to ensable hardware write protection: ", err)
	}

	// Enable software write protect.
	testing.ContextLog(ctx, "Enabling software write protect")
	if err := fingerprint.SetSoftwareWriteProtect(ctx, d.DUT(), true); err != nil {
		s.Fatal("Failed to enable software write protect")
	}

	testing.ContextLog(ctx, "Checking that firmware is functional")
	if _, err := fingerprint.CheckFirmwareIsFunctional(ctx, d.DUT()); err != nil {
		s.Fatal("Firmware is not functional after initialization: ", err)
	}

	testing.ContextLog(ctx, "Calling bio_wash with factory_init")
	if err := fingerprint.BioWash(ctx, d, false); err != nil {
		s.Fatal("Failed to call bio_wash with factory_init: ", err)
	}

	testing.ContextLog(ctx, "Validating rollback block ID increases by 1")
	expectedRollback := initialRollback
	expectedRollback.BlockID++
	if err := fingerprint.CheckRollbackState(ctx, d, expectedRollback); err != nil {
		s.Fatal("Unexpected rollback state: ", err)
	}

	testing.ContextLog(ctx, "Calling bio_wash")
	if err := fingerprint.BioWash(ctx, d, true); err != nil {
		s.Fatal("Failed to call bio_wash: ", err)
	}

	testing.ContextLog(ctx, "Validating Block ID increases by 2, but nothing else")
	expectedRollback.BlockID += 2
	if err := fingerprint.CheckRollbackState(ctx, d, expectedRollback); err != nil {
		s.Fatal("Unexpected rollback state: ", err)
	}
}
