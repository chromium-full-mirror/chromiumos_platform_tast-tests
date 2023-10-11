// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/dutfs"
	"go.chromium.org/tast-tests/cros/remote/firmware/fingerprint"
	"go.chromium.org/tast-tests/cros/remote/firmware/fingerprint/rpcdut"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: FpReadFlash,
		Desc: "Verify that fingerprint flash cannot be read",
		Contacts: []string{
			"chromeos-fingerprint@google.com",
			"tomhughes@chromium.org", // Test author
		},
		// ChromeOS > Platform > Services > Fingerprint
		BugComponent: "b:782045",
		Attr:         []string{"group:mainline", "group:fingerprint-cq", "group:cq-medium", "group:fingerprint-release"},
		Timeout:      7 * time.Minute,
		SoftwareDeps: []string{"biometrics_daemon"},
		HardwareDeps: hwdep.D(hwdep.Fingerprint()),
		ServiceDeps:  []string{"tast.cros.platform.UpstartService", dutfs.ServiceName},
		TestBedDeps:  []string{tbdep.Fingerprint},
		Vars:         []string{"servo"},
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

func FpReadFlash(ctx context.Context, s *testing.State) {
	d, err := rpcdut.NewRPCDUT(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect RPCDUT: ", err)
	}
	defer d.Close(ctx)

	servoSpec, ok := s.Var("servo")
	if !ok {
		servoSpec = ""
	}
	firmwareFile, err := fingerprint.NewMPFirmwareFile(ctx, d)
	if err != nil {
		s.Fatal("Failed to create MP firmwareFile: ", err)
	}
	t, err := fingerprint.NewFirmwareTest(ctx, d, servoSpec, s.OutDir(), firmwareFile, true, true)
	if err != nil {
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

	firmwareCopy, err := fingerprint.RunningFirmwareCopy(ctx, d.DUT())
	if err != nil {
		s.Fatal("Failed to query running firmware copy: ", err)
	}
	if firmwareCopy != fingerprint.ImageTypeRW {
		s.Fatal("Not running RW firmware")
	}

	// Ensure that entropy is set and that the state is normal.
	rollback, err := fingerprint.RollbackInfo(ctx, d.DUT())
	if err != nil {
		s.Fatal("Failed to get rollbackinfo: ", err)
	}
	if rollback.IsAntiRollbackSet() {
		s.Fatalf("Anti-rollback is set: %+v", rollback)
	}
	if !rollback.IsEntropySet() {
		s.Fatalf("Entropy is unset: %+v", rollback)
	}

	testing.ContextLog(ctx, "Reading from flash while running RW firmware should fail")
	if err := fingerprint.ReadFromRollbackFlashFails(ctx, d.DUT(), t.FPBoard(), filepath.Join(t.DUTTempDir(), "test1.bin")); err != nil {
		s.Fatal("Failed to confirm that rollback is not readable: ", err)
	}

	testing.ContextLog(ctx, "Reboot to RO")
	if err := fingerprint.RebootFpmcu(ctx, d.DUT(), fingerprint.ImageTypeRO); err != nil {
		s.Fatal("Failed to reboot to RO: ", err)
	}

	testing.ContextLog(ctx, "Reading from flash while running RO firmware should fail")
	if err := fingerprint.ReadFromRollbackFlashFails(ctx, d.DUT(), t.FPBoard(), filepath.Join(t.DUTTempDir(), "test2.bin")); err != nil {
		s.Fatal("Failed to confirm that rollback is not readable: ", err)
	}

	testing.ContextLog(ctx, "Reboot to RW")
	if err := fingerprint.RebootFpmcu(ctx, d.DUT(), fingerprint.ImageTypeRW); err != nil {
		s.Fatal("Failed to reboot to RW: ", err)
	}
}
