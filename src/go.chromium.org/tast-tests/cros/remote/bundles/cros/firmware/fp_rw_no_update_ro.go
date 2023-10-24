// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/remote/dutfs"
	"go.chromium.org/tast-tests/cros/remote/firmware/fingerprint"
	"go.chromium.org/tast-tests/cros/remote/firmware/fingerprint/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/fingerprint/rpcdut"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: FpRWNoUpdateRO,
		Desc: "Enables hardware write protect, attempts to flash the RO fingerprint firmware, and verifies that the flashing fails",
		Contacts: []string{
			"chromeos-fingerprint@google.com",
			"tomhughes@chromium.org", // Test author
		},
		// ChromeOS > Platform > Services > Fingerprint
		BugComponent: "b:782045",
		Attr:         []string{"group:mainline", "group:fingerprint-cq", "group:cq-medium"},
		Timeout:      7 * time.Minute,
		SoftwareDeps: []string{"biometrics_daemon"},
		HardwareDeps: hwdep.D(hwdep.Fingerprint()),
		ServiceDeps:  []string{"tast.cros.platform.UpstartService", dutfs.ServiceName},
		Vars:         []string{"servo"},
		Fixture:      fixture.FingerprintImages,
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

func FpRWNoUpdateRO(ctx context.Context, s *testing.State) {
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

	testImages := s.FixtValue().(*fixture.ImagesTestData).TestImages

	if err := fingerprint.CheckRunningFirmwareCopy(ctx, d.DUT(), fingerprint.ImageTypeRW); err != nil {
		s.Fatal("Test expects RW firmware copy to be running")
	}

	// Hardware write protect must be enabled for the test to work correctly.
	if err := fingerprint.CheckWriteProtectStateCorrect(ctx, d.DUT(), true, true); err != nil {
		s.Fatal("Failed to validate write protect settings: ", err)
	}

	s.Log("Flashing RO firmware (expected to fail)")

	if err := fingerprint.FlashFirmwareUpdate(ctx, d, fingerprint.ImageTypeRO, testImages[fingerprint.TestImageTypeDev].Path); err == nil {
		s.Fatal("Flashing RO firmware should not succeed")
	}

	s.Log("Flashing failed as expected")
}
