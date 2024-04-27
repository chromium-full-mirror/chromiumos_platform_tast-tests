// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
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
		Func: FpSystemIsLocked,
		Desc: "Verify that system_is_locked() is true in the firmware (i.e., CONFIG_CMD_FPSENSOR_DEBUG) is disabled",
		Contacts: []string{
			"chromeos-fingerprint@google.com",
			"tomhughes@chromium.org", // Test author
		},
		// ChromeOS > Platform > Services > Fingerprint
		BugComponent: "b:782045",
		Attr:         []string{"group:fingerprint-cq", "group:fingerprint-release"},
		Timeout:      7 * time.Minute,
		SoftwareDeps: []string{"biometrics_daemon"},
		HardwareDeps: hwdep.D(hwdep.Fingerprint()),
		ServiceDeps:  []string{"tast.cros.platform.UpstartService", dutfs.ServiceName},
		TestBedDeps:  []string{tbdep.ServoStateWorking},
		Vars:         []string{"servo"},
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

func FpSystemIsLocked(ctx context.Context, s *testing.State) {
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

	testing.ContextLog(ctx, "Checking that firmware is functional")
	if _, err := fingerprint.CheckFirmwareIsFunctional(ctx, d.DUT()); err != nil {
		s.Fatal("Firmware is not functional: ", err)
	}

	testing.ContextLog(ctx, "Checking that system is locked")
	if err := fingerprint.CheckSystemIsLocked(ctx, d.DUT()); err != nil {
		s.Fatal("System is not locked: ", err)
	}

	testing.ContextLog(ctx, "Checking that we cannot access raw frame")
	if err := fingerprint.CheckRawFPFrameFails(ctx, d); err != nil {
		s.Fatal("Reading raw frame should fail: ", err)
	}
}
